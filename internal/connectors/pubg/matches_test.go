package pubg

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// matchBody is the real shape of a match answer, kept faithful down to the
// roster and the asset entry, with every player name and identifier replaced
// by an invented one. A real match names a hundred real people, and none of
// them belong in this repository.
//
// damage is written raw on purpose, so a test can send the very same match
// with an integer and with a float, which is what the API actually does.
func matchBody(matchType, mapName, damage string, custom bool) string {
	return fmt.Sprintf(`{
  "data": {
    "type": "match",
    "id": "match-1",
    "attributes": {
      "createdAt": "2026-09-20T23:36:59Z",
      "duration": 1935,
      "gameMode": "squad-fpp",
      "isCustomMatch": %t,
      "mapName": %q,
      "matchType": %q,
      "seasonState": "progress",
      "shardId": "steam",
      "stats": null,
      "tags": null
    },
    "relationships": {"rosters": {"data": [{"type": "roster", "id": "roster-1"}]}}
  },
  "included": [
    {"type": "asset", "id": "asset-1", "attributes": {"name": "telemetry"}},
    {"type": "participant", "id": "part-1", "attributes": {"actor": "", "shardId": "steam", "stats": {
      "DBNOs": 2, "assists": 1, "boosts": 4, "damageDealt": %s, "deathType": "byplayer",
      "headshotKills": 1, "heals": 3, "killPlace": 7, "killStreaks": 1, "kills": 3,
      "longestKill": 121, "name": "Coequipier", "playerId": "account.autre",
      "revives": 0, "rideDistance": 0, "roadKills": 0, "swimDistance": 0, "teamKills": 0,
      "timeSurvived": 1402, "vehicleDestroys": 0, "walkDistance": 1234.56,
      "weaponsAcquired": 5, "winPlace": 12
    }}},
    {"type": "participant", "id": "part-2", "attributes": {"actor": "", "shardId": "steam", "stats": {
      "DBNOs": 3, "assists": 2, "boosts": 6, "damageDealt": %s, "deathType": "byplayer",
      "headshotKills": 4, "heals": 5, "killPlace": 2, "killStreaks": 2, "kills": 7,
      "longestKill": 88.5, "name": "LeMembre", "playerId": "account.membre",
      "revives": 1, "rideDistance": 512.25, "roadKills": 0, "swimDistance": 0, "teamKills": 0,
      "timeSurvived": 1810, "vehicleDestroys": 1, "walkDistance": 2345.5,
      "weaponsAcquired": 8, "winPlace": 3
    }}},
    {"type": "roster", "id": "roster-1", "attributes": {"stats": {"rank": 3, "teamId": 25}, "won": "false", "shardId": "steam"},
     "relationships": {"team": {"data": null},
       "participants": {"data": [{"type": "participant", "id": "part-1"}, {"type": "participant", "id": "part-2"}]}}}
  ],
  "links": {}, "meta": {}
}`, custom, mapName, matchType, damage, damage)
}

func serveMatch(t *testing.T, body string) *Connector {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)
	return c
}

func TestUnMatchNormalRendLaLigneDuBonParticipant(t *testing.T) {
	c := serveMatch(t, matchBody("official", "Baltic_Main", "612.34", false))

	m, err := c.MatchForPlayer("steam", "match-1", "account.membre")
	if err != nil {
		t.Fatalf("MatchForPlayer: %v", err)
	}
	if m.MatchID != "match-1" || m.GameMode != "squad-fpp" || m.MapName != "Baltic_Main" || m.MatchType != "official" {
		t.Fatalf("identité du match = %+v", m)
	}
	if m.WinPlace != 3 || m.Kills != 7 || m.Assists != 2 || m.HeadshotKills != 4 || m.Revives != 1 {
		t.Fatalf("c'est l'autre participant qui a été retenu : %+v", m)
	}
	if m.DamageDealt != 612.34 {
		t.Fatalf("DamageDealt = %v", m.DamageDealt)
	}
	if m.TimeSurvived != 1810 || m.DurationSecs != 1935 {
		t.Fatalf("durées = %+v", m)
	}
	if got := m.PlayedAt.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-09-20T23:36:59Z" {
		t.Fatalf("PlayedAt = %q", got)
	}
}

func TestUnDamageDealtEntierEtUnFlottantSeDecodentTousLesDeux(t *testing.T) {
	// Relevé le 2026-09-22 : la même clé revient entière sur certains matchs
	// et flottante sur d'autres, sans régularité.
	for _, cas := range []struct {
		brut   string
		attend float64
	}{
		{"612", 612},
		{"612.34", 612.34},
	} {
		c := serveMatch(t, matchBody("official", "Baltic_Main", cas.brut, false))
		m, err := c.MatchForPlayer("steam", "match-1", "account.membre")
		if err != nil {
			t.Fatalf("damageDealt=%s : %v", cas.brut, err)
		}
		if m.DamageDealt != cas.attend {
			t.Fatalf("damageDealt=%s a donné %v", cas.brut, m.DamageDealt)
		}
	}
}

func TestUnEntrainementNEstPasUnePartie(t *testing.T) {
	cas := []struct {
		nom       string
		matchType string
		mapName   string
		custom    bool
	}{
		{"tutoriel", "tutorialatoz", "Baltic_Main", false},
		{"champ de tir", "official", "Range_Main", false},
		{"partie personnalisée", "official", "Baltic_Main", true},
		{"mode inconnu écarté par défaut", "airoyale", "Baltic_Main", false},
	}
	for _, k := range cas {
		t.Run(k.nom, func(t *testing.T) {
			c := serveMatch(t, matchBody(k.matchType, k.mapName, "612.34", k.custom))
			_, err := c.MatchForPlayer("steam", "match-1", "account.membre")
			if !errors.Is(err, ErrNotPlayable) {
				t.Fatalf("attendu ErrNotPlayable, obtenu %v", err)
			}
		})
	}
}

func TestUnMatchSansLeMembreEstUneErreurDistincte(t *testing.T) {
	c := serveMatch(t, matchBody("official", "Baltic_Main", "612.34", false))

	_, err := c.MatchForPlayer("steam", "match-1", "account.inconnu")
	if !errors.Is(err, ErrNoParticipant) {
		t.Fatalf("attendu ErrNoParticipant, obtenu %v", err)
	}
	if errors.Is(err, ErrNotPlayable) {
		// Les deux se sautent, mais l'un est normal et l'autre signale que la
		// liaison du compte est fausse : les confondre cacherait le second.
		t.Fatal("un match sans le membre ne doit pas passer pour un entraînement")
	}
}

// TestLesChampsConservesSontExactementCeuxLa épingle la liste, parce qu'un
// champ ajouté par inadvertance pourrait porter le pseudo d'un autre joueur.
func TestLesChampsConservesSontExactementCeuxLa(t *testing.T) {
	attendus := map[string]bool{
		"MatchID": true, "GameMode": true, "MapName": true, "MatchType": true,
		"WinPlace": true, "Kills": true, "Assists": true, "DamageDealt": true,
		"TimeSurvived": true, "HeadshotKills": true, "Revives": true,
		"PlayedAt": true, "DurationSecs": true,
	}
	typ := reflect.TypeOf(MatchResult{})
	for i := range typ.NumField() {
		nom := typ.Field(i).Name
		if !attendus[nom] {
			t.Errorf("champ inattendu dans MatchResult : %s", nom)
		}
		delete(attendus, nom)
	}
	for nom := range attendus {
		t.Errorf("champ manquant dans MatchResult : %s", nom)
	}
}
