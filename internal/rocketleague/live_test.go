package rocketleague

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/knightsofeternity/kfire-server/internal/livestate"
)

// TestLivePayloadValidation carries the same business rules as the retired
// ws.livePayload.valid(), minus the slug: the registry owns that check now,
// exactly as it does for a finished match.
func TestLivePayloadValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"en cours", `{"game_slug":"rocket-league","team_blue_score":2,"team_orange_score":1,"seconds_remaining":143,"overtime":false,"goals":1,"assists":0,"saves":2,"shots":3,"score":310,"demos":0}`, true},
		{"prolongation", `{"game_slug":"rocket-league","team_blue_score":3,"team_orange_score":3,"seconds_remaining":47,"overtime":true,"goals":1,"assists":1,"saves":0,"shots":2,"score":280,"demos":1}`, true},
		{"fin de match", `{"game_slug":"rocket-league","ended":true}`, true},
		{"score negatif", `{"game_slug":"rocket-league","team_blue_score":-1,"team_orange_score":1,"seconds_remaining":143}`, false},
		{"chrono negatif", `{"game_slug":"rocket-league","team_blue_score":2,"team_orange_score":1,"seconds_remaining":-5}`, false},
		{"chrono aberrant", `{"game_slug":"rocket-league","team_blue_score":2,"team_orange_score":1,"seconds_remaining":99999}`, false},
		{"stat negative", `{"game_slug":"rocket-league","team_blue_score":2,"team_orange_score":1,"seconds_remaining":143,"saves":-2}`, false},
		{"score de match aberrant", `{"game_slug":"rocket-league","team_blue_score":999,"team_orange_score":1,"seconds_remaining":143}`, false},
		{"stat aberrante", `{"game_slug":"rocket-league","team_blue_score":2,"team_orange_score":1,"seconds_remaining":143,"score":2147483647}`, false},
	}
	r := NewLiveReporter()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.Shape(json.RawMessage(tc.body))
			got := err == nil
			if got != tc.ok {
				t.Errorf("Shape() err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestLiveShapeReturnsExactlyWhatIsBroadcast(t *testing.T) {
	// Ce que Shape rend EST ce que les navigateurs voient. La liste est
	// épinglée pour qu'elle ne s'allonge pas par accident.
	r := NewLiveReporter()
	got, err := r.Shape(json.RawMessage(`{"game_slug":"rocket-league",
		"team_blue_score":2,"team_orange_score":1,"seconds_remaining":143,
		"overtime":false,"goals":1,"assists":0,"saves":2,"shots":3,
		"score":310,"demos":0}`))
	if err != nil {
		t.Fatalf("Shape() = %v", err)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{
		"assists", "demos", "goals", "overtime", "saves", "score",
		"seconds_remaining", "shots", "team_blue_score", "team_orange_score",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("champs diffusés = %v, want %v", keys, want)
	}
}

func TestLiveShapeNeverCarriesAName(t *testing.T) {
	// Le flux du jeu nomme tous les joueurs du match. Rien de cela ne doit
	// atteindre l'écran d'un autre membre, même si ce n'est jamais stocké.
	r := NewLiveReporter()
	got, _ := r.Shape(json.RawMessage(`{"game_slug":"rocket-league",
		"team_blue_score":2,"team_orange_score":1,"seconds_remaining":143,
		"Players":[{"Name":"Bushido"}],"opponent":"Kran"}`))
	raw, _ := json.Marshal(got)
	for _, forbidden := range []string{"Bushido", "Kran", "Players", "opponent", "Name"} {
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

// TestLiveShapeRejectsWithErrInvalidLive pins the sentinel a caller matches
// on, not just its presence.
func TestLiveShapeRejectsWithErrInvalidLive(t *testing.T) {
	r := NewLiveReporter()
	_, err := r.Shape(json.RawMessage(`{"game_slug":"rocket-league","team_blue_score":-1}`))
	if err != livestate.ErrInvalidLive {
		t.Errorf("err = %v, want %v", err, livestate.ErrInvalidLive)
	}
}
