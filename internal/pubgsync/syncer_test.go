package pubgsync

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"testing"

	"github.com/knightsofeternity/kfire-server/internal/connectors/pubg"
	"github.com/knightsofeternity/kfire-server/internal/store"
)

// fakePuller answers whatever the test tells it to, and records what it was
// asked for, which is how "a stored match is not read again" gets proven.
type fakePuller struct {
	results map[string]pubg.MatchResult
	errs    map[string]error
	read    []string
}

func (f *fakePuller) MatchForPlayer(_, matchID, _ string) (pubg.MatchResult, error) {
	f.read = append(f.read, matchID)
	if err := f.errs[matchID]; err != nil {
		return pubg.MatchResult{}, err
	}
	return f.results[matchID], nil
}

func member() store.PubgPlayer {
	return store.PubgPlayer{UserID: "u1", AccountID: "account.abc", Platform: "steam"}
}

func TestUnMatchDejaStockeNEstPasRelu(t *testing.T) {
	p := &fakePuller{results: map[string]pubg.MatchResult{
		"m2": {MatchID: "m2"},
	}}
	var saved []string
	res := walk(context.Background(), p, member(), []string{"m1", "m2"},
		map[string]bool{"m1": true}, func(r pubg.MatchResult) error {
			saved = append(saved, r.MatchID)
			return nil
		})

	if len(p.read) != 1 || p.read[0] != "m2" {
		t.Fatalf("matchs lus = %v, seul m2 était attendu", p.read)
	}
	if len(saved) != 1 || saved[0] != "m2" {
		t.Fatalf("matchs enregistrés = %v", saved)
	}
	if res.known != 1 || res.stored != 1 {
		t.Fatalf("bilan = %+v", res)
	}
}

func TestUnEntrainementEstSauteSansEtreUneErreur(t *testing.T) {
	p := &fakePuller{
		results: map[string]pubg.MatchResult{"m2": {MatchID: "m2"}},
		errs:    map[string]error{"m1": pubg.ErrNotPlayable},
	}
	var saved []string
	res := walk(context.Background(), p, member(), []string{"m1", "m2"}, nil,
		func(r pubg.MatchResult) error {
			saved = append(saved, r.MatchID)
			return nil
		})

	if res.notPlayable != 1 {
		t.Fatalf("un entraînement devait être compté comme tel : %+v", res)
	}
	if res.retryable != 0 || res.permanent != 0 {
		t.Fatalf("un entraînement n'est pas une erreur : %+v", res)
	}
	// Et surtout, le match suivant est quand même enregistré.
	if len(saved) != 1 || saved[0] != "m2" {
		t.Fatalf("matchs enregistrés = %v", saved)
	}
}

func TestUneErreurPassagereNInterromptPasLeLotEtLaisseLeMatchARattraper(t *testing.T) {
	// A 429 in the middle of a batch. The match must NOT be counted as done:
	// the publisher deletes it in 14 days, so "later" has to stay possible.
	p := &fakePuller{
		results: map[string]pubg.MatchResult{"m1": {MatchID: "m1"}, "m3": {MatchID: "m3"}},
		errs: map[string]error{"m2": &pubg.APIError{
			Status: http.StatusTooManyRequests, Path: "/shards/steam/matches/m2",
		}},
	}
	var saved []string
	res := walk(context.Background(), p, member(), []string{"m1", "m2", "m3"}, nil,
		func(r pubg.MatchResult) error {
			saved = append(saved, r.MatchID)
			return nil
		})

	if res.retryable != 1 || res.permanent != 0 {
		t.Fatalf("un 429 est passager : %+v", res)
	}
	if len(saved) != 2 || saved[0] != "m1" || saved[1] != "m3" {
		t.Fatalf("les autres matchs du lot devaient passer : %v", saved)
	}
	// Rien n'a été écrit pour m2, donc rien ne le marque comme traité : le
	// passage du lendemain le reverra dans la liste et le relira.
	for _, id := range saved {
		if id == "m2" {
			t.Fatal("m2 n'aurait pas dû être enregistré")
		}
	}
}

func TestUneErreurDefinitiveNEstPasCompteeCommePassagere(t *testing.T) {
	p := &fakePuller{errs: map[string]error{
		"m1": pubg.ErrNoParticipant,
		"m2": &pubg.APIError{Status: http.StatusNotFound, Path: "/shards/steam/matches/m2"},
	}}
	res := walk(context.Background(), p, member(), []string{"m1", "m2"}, nil,
		func(pubg.MatchResult) error { return nil })

	if res.permanent != 2 || res.retryable != 0 {
		t.Fatalf("bilan = %+v", res)
	}
}

func TestUneEcritureRefuseeSeraRejouee(t *testing.T) {
	// The insert is idempotent, so a database hiccup costs nothing but a day:
	// the match stays a candidate because nothing recorded it as handled.
	p := &fakePuller{results: map[string]pubg.MatchResult{"m1": {MatchID: "m1"}}}
	res := walk(context.Background(), p, member(), []string{"m1"}, nil,
		func(pubg.MatchResult) error { return errors.New("connexion perdue") })

	if res.stored != 0 || res.retryable != 1 {
		t.Fatalf("bilan = %+v", res)
	}
}

func TestUnContexteAnnuleArreteLeLot(t *testing.T) {
	p := &fakePuller{results: map[string]pubg.MatchResult{"m1": {MatchID: "m1"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := walk(ctx, p, member(), []string{"m1", "m2"}, nil,
		func(pubg.MatchResult) error { return nil })

	if len(p.read) != 0 || res.stored != 0 {
		t.Fatalf("aucun appel n'était attendu après l'annulation : %v %+v", p.read, res)
	}
}

// Une violation de contrainte ne guérit jamais. La confondre avec une panne
// passagère ferait retenter le match chaque jour pendant les quatorze jours où
// l'éditeur le garde, avec une erreur par jour, pour le perdre quand même.
func TestUneViolationDeContrainteEstDefinitive(t *testing.T) {
	violation := &pgconn.PgError{Code: "23514", Message: "check constraint"}
	if !isCheckViolation(violation) {
		t.Fatal("une 23514 doit être reconnue comme définitive")
	}
	// Enveloppée, comme elle arrive vraiment depuis le store.
	if !isCheckViolation(fmt.Errorf("insert: %w", violation)) {
		t.Fatal("elle doit être reconnue à travers un enrobage")
	}
	// Une panne de connexion, elle, se retente.
	if isCheckViolation(&pgconn.PgError{Code: "08006"}) {
		t.Fatal("une panne de connexion n'est pas définitive")
	}
	if isCheckViolation(errors.New("contexte annulé")) {
		t.Fatal("une erreur quelconque n'est pas une violation de contrainte")
	}
}
