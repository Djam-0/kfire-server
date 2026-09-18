package riotsync

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/knightsofeternity/kfire-server/internal/livestate"
)

func TestLivePayloadValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"partie en cours", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,
			"kills":7,"deaths":2,"assists":9,"creep_score":142,"gold":8350,"game_time_seconds":843}`, true},
		{"debut de partie a zero partout", `{"game_slug":"league-of-legends","champion":"Ahri","level":1,
			"kills":0,"deaths":0,"assists":0,"creep_score":0,"gold":0,"game_time_seconds":0}`, true},
		{"champion avec apostrophe", `{"game_slug":"league-of-legends","champion":"Kai'Sa","level":6}`, true},
		{"champion avec point", `{"game_slug":"league-of-legends","champion":"Dr. Mundo","level":6}`, true},
		{"champion avec esperluette", `{"game_slug":"league-of-legends","champion":"Nunu & Willump","level":6}`, true},
		{"fin de partie", `{"game_slug":"league-of-legends","ended":true}`, true},

		{"champion absent", `{"game_slug":"league-of-legends","level":6}`, false},
		{"champion avec balise", `{"game_slug":"league-of-legends","champion":"<b>Ahri</b>","level":6}`, false},
		{"champion bavard", `{"game_slug":"league-of-legends","champion":"` + strings.Repeat("A", 33) + `","level":6}`, false},
		{"niveau zero", `{"game_slug":"league-of-legends","champion":"Ahri","level":0}`, false},
		{"niveau au dela du plafond", `{"game_slug":"league-of-legends","champion":"Ahri","level":19}`, false},
		{"niveau negatif", `{"game_slug":"league-of-legends","champion":"Ahri","level":-3}`, false},
		{"eliminations aberrantes", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,"kills":2147483647}`, false},
		{"morts negatives", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,"deaths":-1}`, false},
		{"assistances aberrantes", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,"assists":1000}`, false},
		{"sbires aberrants", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,"creep_score":10000}`, false},
		{"or aberrant", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,"gold":1000000}`, false},
		{"chrono aberrant", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,"game_time_seconds":36001}`, false},
		{"chrono negatif", `{"game_slug":"league-of-legends","champion":"Ahri","level":11,"game_time_seconds":-1}`, false},
	}
	r := NewLiveReporter()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.Shape(json.RawMessage(tc.body))
			if got := err == nil; got != tc.ok {
				t.Errorf("Shape() err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestLiveShapeReturnsExactlyWhatIsBroadcast(t *testing.T) {
	// Ce que Shape rend EST ce que les navigateurs voient. La liste est
	// épinglée pour qu'elle ne s'allonge pas par accident : l'API locale de
	// Riot nomme les dix participants de la partie, leurs objets et leurs
	// runes, et aucun de ces noms ne doit jamais pouvoir s'ajouter ici.
	r := NewLiveReporter()
	got, err := r.Shape(json.RawMessage(`{"game_slug":"league-of-legends","champion":"Ahri","level":11,
		"kills":7,"deaths":2,"assists":9,"creep_score":142,"gold":8350,"game_time_seconds":843}`))
	if err != nil {
		t.Fatalf("Shape() = %v", err)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"assists", "champion", "creep_score", "deaths", "game_time_seconds", "gold", "kills", "level"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("champs diffusés = %v, want %v", keys, want)
	}
}

func TestLiveShapeNeverCarriesAParticipant(t *testing.T) {
	// L'API locale liste les dix joueurs de la partie, avec leur pseudo Riot,
	// leurs objets et leur équipe. Rien de cela ne doit atteindre l'écran d'un
	// autre membre : les neuf autres n'ont rien accepté de tel.
	r := NewLiveReporter()
	got, err := r.Shape(json.RawMessage(`{"game_slug":"league-of-legends","champion":"Ahri","level":11,
		"kills":7,"deaths":2,"assists":9,"creep_score":142,"gold":8350,"game_time_seconds":843,
		"summoner_name":"Bushido#EUW","allPlayers":[{"summonerName":"Ouranos#EUW","championName":"Yasuo"}],
		"riot_id":"Bushido#EUW","items":["Rabadon"],"team":"ORDER","events":[{"KillerName":"Ouranos#EUW"}]}`))
	if err != nil {
		t.Fatalf("Shape() = %v", err)
	}
	raw, _ := json.Marshal(got)
	forbidden := []string{
		"Bushido", "Ouranos", "summoner_name", "summonerName", "allPlayers",
		"riot_id", "items", "Rabadon", "team", "ORDER", "events", "KillerName", "Yasuo",
	}
	for _, f := range forbidden {
		if strings.Contains(string(raw), f) {
			t.Errorf("%s a fuité dans %s", f, raw)
		}
	}
}

func TestLiveReporterClaimsItsSlug(t *testing.T) {
	// Le même slug que le poller Spectator : un jeu, un slug, deux sources.
	if got := NewLiveReporter().Slug(); got != liveSlug {
		t.Errorf("Slug() = %q, want %q", got, liveSlug)
	}
}

// TestLiveShapeRejectsWithErrInvalidLive pins the sentinel a caller matches on,
// not just its presence.
func TestLiveShapeRejectsWithErrInvalidLive(t *testing.T) {
	_, err := NewLiveReporter().Shape(json.RawMessage(`{"game_slug":"league-of-legends","champion":"","level":3}`))
	if err != livestate.ErrInvalidLive {
		t.Errorf("err = %v, want %v", err, livestate.ErrInvalidLive)
	}
}
