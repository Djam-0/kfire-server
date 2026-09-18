package livestate

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fake est un rapporteur de test qui note ce qu'on lui a passé.
type fake struct {
	slug   string
	err    error
	gotRaw string
}

func (f *fake) Slug() string { return f.slug }

func (f *fake) Shape(raw json.RawMessage) (map[string]any, error) {
	f.gotRaw = string(raw)
	if f.err != nil {
		return nil, f.err
	}
	return map[string]any{"ok": true}, nil
}

func body(slug string) json.RawMessage {
	return json.RawMessage(`{"game_slug":"` + slug + `","x":1}`)
}

func TestRegistryRoutesOnTheSlug(t *testing.T) {
	rl := &fake{slug: "rocket-league"}
	hs := &fake{slug: "hearthstone"}
	reg := NewRegistry(rl, hs)

	got, err := reg.Shape(body("rocket-league"))
	if err != nil {
		t.Fatalf("Shape() = %v, want nil", err)
	}
	if got.Slug != "rocket-league" || got.Ended {
		t.Errorf("mauvais aiguillage : %+v", got)
	}
	if hs.gotRaw != "" {
		t.Error("hearthstone a reçu un état qui ne le concerne pas")
	}
}

func TestRegistryUnknownSlug(t *testing.T) {
	reg := NewRegistry(&fake{slug: "rocket-league"})
	if _, err := reg.Shape(body("minecraft")); !errors.Is(err, ErrUnknownGame) {
		t.Errorf("Shape() = %v, want ErrUnknownGame", err)
	}
}

func TestRegistryEmptyAndMalformedSlug(t *testing.T) {
	reg := NewRegistry(&fake{slug: "rocket-league"})
	for _, raw := range []string{
		`{"x":1}`,
		`{"game_slug":""}`,
		`{"game_slug":"Rocket-League"}`,
		`{"game_slug":"<script>alert(1)</script>"}`,
	} {
		if _, err := reg.Shape(json.RawMessage(raw)); !errors.Is(err, ErrInvalidLive) {
			t.Errorf("%s : Shape() = %v, want ErrInvalidLive", raw, err)
		}
	}
}

func TestRegistryRefusesAnOversizedPayload(t *testing.T) {
	// C'est le seul message de la fonctionnalité dont le contenu repart vers
	// d'autres membres : sa taille est bornée avant tout décodage.
	reg := NewRegistry(&fake{slug: "rocket-league"})
	huge := `{"game_slug":"rocket-league","x":"` + strings.Repeat("a", MaxPayload) + `"}`
	if _, err := reg.Shape(json.RawMessage(huge)); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("une charge de %d octets aurait dû être refusée", len(huge))
	}
}

func TestRegistryPassesTheEndSignalWithoutAskingTheGame(t *testing.T) {
	// La fin de partie est générique : aucun rapporteur n'a à la connaître.
	rl := &fake{slug: "rocket-league"}
	reg := NewRegistry(rl)

	got, err := reg.Shape(json.RawMessage(`{"game_slug":"rocket-league","ended":true}`))
	if err != nil {
		t.Fatalf("Shape() = %v, want nil", err)
	}
	if !got.Ended {
		t.Error("le signal de fin doit être reconnu")
	}
	if rl.gotRaw != "" {
		t.Error("le rapporteur n'a pas à voir une fin de partie")
	}
}

func TestRegistryPropagatesTheReporterError(t *testing.T) {
	boom := errors.New("boom")
	reg := NewRegistry(&fake{slug: "rocket-league", err: boom})
	if _, err := reg.Shape(body("rocket-league")); !errors.Is(err, boom) {
		t.Errorf("Shape() = %v, want boom", err)
	}
}
