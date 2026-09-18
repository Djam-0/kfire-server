package hearthstone

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
		{"bataille de gueux", `{"game_slug":"hearthstone","mode":"battlegrounds","turn":11,"placement":4}`, true},
		{"bataille de gueux sans position", `{"game_slug":"hearthstone","mode":"battlegrounds","turn":1}`, true},
		{"construit", `{"game_slug":"hearthstone","mode":"constructed","turn":7}`, true},
		{"construit avec position nulle", `{"game_slug":"hearthstone","mode":"constructed","turn":7,"placement":null}`, true},
		{"fin de partie", `{"game_slug":"hearthstone","ended":true}`, true},
		{"mode inconnu", `{"game_slug":"hearthstone","mode":"arena","turn":7}`, false},
		{"mode vide", `{"game_slug":"hearthstone","turn":7}`, false},
		{"mode avec balise", `{"game_slug":"hearthstone","mode":"<b>bg</b>","turn":7}`, false},
		{"tour zero", `{"game_slug":"hearthstone","mode":"constructed","turn":0}`, false},
		{"tour negatif", `{"game_slug":"hearthstone","mode":"constructed","turn":-3}`, false},
		{"tour aberrant", `{"game_slug":"hearthstone","mode":"constructed","turn":2147483647}`, false},
		{"position zero", `{"game_slug":"hearthstone","mode":"battlegrounds","turn":5,"placement":0}`, false},
		{"position hors lobby", `{"game_slug":"hearthstone","mode":"battlegrounds","turn":5,"placement":9}`, false},
		{"position negative", `{"game_slug":"hearthstone","mode":"battlegrounds","turn":5,"placement":-1}`, false},
		// Une position en construit n'est pas une valeur à ignorer : elle
		// signale un client qui ne sait pas ce qu'il envoie.
		{"position en construit", `{"game_slug":"hearthstone","mode":"constructed","turn":5,"placement":4}`, false},
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
	// épinglée pour qu'elle ne s'allonge pas par accident : le journal de
	// Hearthstone nomme l'adversaire et contient toutes les cartes jouées.
	r := NewLiveReporter()
	got, err := r.Shape(json.RawMessage(`{"game_slug":"hearthstone","mode":"battlegrounds","turn":11,"placement":4}`))
	if err != nil {
		t.Fatalf("Shape() = %v", err)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"mode", "placement", "turn"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("champs diffusés = %v, want %v", keys, want)
	}
}

func TestLiveShapeOmetLaPositionEnConstruit(t *testing.T) {
	// Pas de clé du tout plutôt qu'un zéro : la carte n'a rien à afficher,
	// elle ne doit pas avoir à cacher une valeur.
	r := NewLiveReporter()
	got, err := r.Shape(json.RawMessage(`{"game_slug":"hearthstone","mode":"constructed","turn":7}`))
	if err != nil {
		t.Fatalf("Shape() = %v", err)
	}
	if _, has := got["placement"]; has {
		t.Errorf("une partie construite ne doit porter aucune position : %v", got)
	}
}

func TestLiveShapeNeverCarriesANameOrACard(t *testing.T) {
	// Le journal du jeu nomme l'adversaire et liste chaque carte jouée. Rien
	// de cela ne doit atteindre l'écran d'un autre membre.
	r := NewLiveReporter()
	got, _ := r.Shape(json.RawMessage(`{"game_slug":"hearthstone","mode":"battlegrounds","turn":11,
		"placement":4,"opponent":"Bushido","cards":["Sinistral","Pyroblast"],"hero_card_id":"TB_BaconShop"}`))
	raw, _ := json.Marshal(got)
	for _, forbidden := range []string{"Bushido", "opponent", "cards", "Pyroblast", "hero_card_id", "TB_BaconShop"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("%s a fuité dans %s", forbidden, raw)
		}
	}
}

func TestLiveReporterClaimsItsSlug(t *testing.T) {
	if got := NewLiveReporter().Slug(); got != SLUG {
		t.Errorf("Slug() = %q, want %q", got, SLUG)
	}
}

// TestLiveShapeRejectsWithErrInvalidLive pins the sentinel a caller matches on,
// not just its presence.
func TestLiveShapeRejectsWithErrInvalidLive(t *testing.T) {
	_, err := NewLiveReporter().Shape(json.RawMessage(`{"game_slug":"hearthstone","mode":"arena","turn":3}`))
	if err != livestate.ErrInvalidLive {
		t.Errorf("err = %v, want %v", err, livestate.ErrInvalidLive)
	}
}
