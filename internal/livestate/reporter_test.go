package livestate

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// fake est un rapporteur de test qui note ce qu'on lui a passé.
type fake struct {
	slug   string
	err    error
	out    map[string]any
	gotRaw string
	ttl    time.Duration
}

func (f *fake) Slug() string { return f.slug }

func (f *fake) TTL() time.Duration { return f.ttl }

func (f *fake) Shape(raw json.RawMessage) (map[string]any, error) {
	f.gotRaw = string(raw)
	if f.err != nil {
		return nil, f.err
	}
	if f.out != nil {
		return f.out, nil
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
		// Un slug à rallonge : le motif borne la longueur autant que
		// l'alphabet. Ce cas vivait dans les tests de Rocket League avant
		// que le slug ne devienne l'affaire du registre.
		`{"game_slug":"` + strings.Repeat("a", 200) + `"}`,
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

func TestRegistryRefusesAnOversizedReturn(t *testing.T) {
	// Le plafond vaut aussi pour ce que rend un rapporteur : ce qu'il rend est
	// diffusé tel quel, et un rapporteur défaillant ne doit pas pouvoir en
	// décider seul.
	big := &fake{slug: "rocket-league", out: map[string]any{"x": strings.Repeat("a", MaxPayload)}}
	reg := NewRegistry(big)
	if _, err := reg.Shape(body("rocket-league")); !errors.Is(err, ErrInvalidLive) {
		t.Error("un rapporteur qui rend trop gros doit être refusé")
	}
}

func TestRegistryRefusesAnEndSignalForAnUnknownGame(t *testing.T) {
	// Un jeu que personne ne revendique n'a jamais pu créer d'état en cours,
	// puisque ses états normaux sont refusés. Son signal de fin n'a donc rien
	// à effacer, et l'accepter laisserait un client effacer le match d'un
	// autre jeu : l'état est indexé par membre, pas par jeu.
	reg := NewRegistry(&fake{slug: "rocket-league"})
	if _, err := reg.Shape(json.RawMessage(`{"game_slug":"minecraft","ended":true}`)); !errors.Is(err, ErrUnknownGame) {
		t.Errorf("Shape() = %v, want ErrUnknownGame", err)
	}
}

// La durée déclarée par un jeu doit arriver jusqu'à l'état, sinon elle ne sert
// à rien : c'est elle qui empêche une source lente d'être balayée en pleine
// partie. Bug remonté le 2026-09-21 sur Hearthstone, dont la carte disparaissait
// de la page de la guilde pendant la phase d'achat.
func TestLaDureeDuRapporteurArriveDansLEtat(t *testing.T) {
	r := &fake{slug: "un-jeu", out: map[string]any{"tour": 3}, ttl: 2 * time.Minute}
	reg := NewRegistry(r)

	st, err := reg.Shape([]byte(`{"game_slug":"un-jeu","tour":3}`))
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if st.TTL != 2*time.Minute {
		t.Fatalf("TTL %v, attendu 2m : une source lente sera balayée en pleine partie", st.TTL)
	}

	// Et zéro veut dire « le défaut », pas « expire tout de suite ».
	r2 := &fake{slug: "rapide", out: map[string]any{"score": 1}}
	st2, err := NewRegistry(r2).Shape([]byte(`{"game_slug":"rapide","score":1}`))
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if st2.TTL != 0 {
		t.Fatalf("TTL %v, attendu 0 pour retomber sur le défaut", st2.TTL)
	}
}
