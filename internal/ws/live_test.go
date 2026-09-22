package ws

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/knightsofeternity/kfire-server/internal/livestate"
)

func TestLiveExpire(t *testing.T) {
	now := time.Now()
	e := liveEntry{updatedAt: now.Add(-liveTTL - time.Second)}
	if !e.expired(now) {
		t.Errorf("an entry older than the TTL must be expired")
	}
	fresh := liveEntry{updatedAt: now.Add(-time.Second)}
	if fresh.expired(now) {
		t.Errorf("an entry from a second ago must not be expired")
	}
}

func TestLiveSharedState(t *testing.T) {
	// A hub with no dependencies is enough: setLive and LiveMatch only touch
	// the in-memory map. That is what makes this state testable while the
	// rest of the hub is not.
	h := NewHub(nil, nil, "", nil, nil)

	s := livestate.State{
		Slug: "rocket-league",
		Match: map[string]any{
			"team_blue_score": 2, "team_orange_score": 1,
			"seconds_remaining": 143, "goals": 1, "saves": 2, "shots": 3, "score": 310,
		},
	}
	h.setLive("u1", s, sourceClient)

	got := h.LiveMatch("u1")
	if got == nil {
		t.Fatal("LiveMatch returns nil right after setLive")
	}
	if got["team_blue_score"] != 2 || got["seconds_remaining"] != 143 {
		t.Errorf("state read back incorrectly: %v", got)
	}
	if _, named := got["user_id"]; named {
		t.Error("the broadcast state must not carry identity, the hub adds it around")
	}

	// A member with no match in progress has no state.
	if h.LiveMatch("unknown") != nil {
		t.Error("LiveMatch returns a state for a member who is not playing")
	}

	// La fin de partie efface l'état : c'est ce qui fait disparaître la carte
	// du portail, donc c'est du comportement, pas un détail interne.
	if !h.clearLive("u1", "rocket-league", sourceClient) {
		t.Error("clearLive devait signaler qu'il y avait un match en cours")
	}
	if h.LiveMatch("u1") != nil {
		t.Error("l'état survit à la fin du match")
	}
	if h.clearLive("u1", "rocket-league", sourceClient) {
		t.Error("clearLive sur un membre sans match doit rendre false")
	}
}

// This test only has value run with -race: it makes reads and writes on the
// shared map cross paths, which no other test in the package does. Without
// it, `go test -race ./internal/ws/` proves nothing about h.live.
func TestLiveConcurrentAccess(t *testing.T) {
	h := NewHub(nil, nil, "", nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		member := fmt.Sprintf("u%d", i%3)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				h.setLive(member, livestate.State{Slug: "rocket-league", Match: map[string]any{"team_blue_score": j % 5}}, sourceClient)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = h.LiveMatch(member)
			}
		}()
	}
	wg.Wait()
}

func TestSweepForgetsAMatchThatWentQuiet(t *testing.T) {
	// Le jeu d'un membre plante mais KFIRE reste connecté : la socket ne se
	// ferme pas, donc unregister ne passe jamais. Sans le balayage, sa carte
	// resterait figée à l'écran de toute la guilde.
	h := NewHub(nil, nil, "", nil, nil)
	h.setLive("frais", livestate.State{Slug: "rocket-league", Match: map[string]any{"x": 1}}, sourceClient)
	h.setLive("perdu", livestate.State{Slug: "rocket-league", Match: map[string]any{"x": 2}}, sourceClient)

	h.mu.Lock()
	e := h.live["perdu"]
	e.updatedAt = time.Now().Add(-liveTTL - time.Second)
	h.live["perdu"] = e
	h.mu.Unlock()

	ended := h.sweepExpired(time.Now())
	if len(ended) != 1 || ended[0] != "perdu" {
		t.Fatalf("balayés = %v, want [perdu]", ended)
	}
	if h.LiveMatch("perdu") != nil {
		t.Error("un état expiré doit disparaître")
	}
	if h.LiveMatch("frais") == nil {
		t.Error("un état frais ne doit PAS être balayé")
	}
}

func TestLiveVisibilityCutsTheStream(t *testing.T) {
	h := NewHub(nil, nil, "", nil, nil)
	h.setLiveVisible("u1", true, "online")
	if !h.liveAllowed("u1") {
		t.Fatal("a visible member must be able to broadcast")
	}
	h.setLive("u1", livestate.State{Slug: "rocket-league", Match: map[string]any{"team_blue_score": 1}}, sourceClient)

	// They go hidden mid-match.
	h.SetVisibility("u1", true, "invisible")
	if h.liveAllowed("u1") {
		t.Error("an invisible member must no longer broadcast")
	}
	if h.LiveMatch("u1") != nil {
		t.Error("their live match should have disappeared immediately")
	}

	// A member who hides their activity without declaring themselves
	// invisible, too.
	h.setLiveVisible("u2", false, "online")
	if h.liveAllowed("u2") {
		t.Error("hidden activity must be enough to cut the stream")
	}
}

// The visibility toggle arrives over an HTTP goroutine while the
// connection's read loop consults the permission. This is exactly the race
// that made the first version of this fix unacceptable.
func TestLiveVisibilityConcurrentAccess(t *testing.T) {
	h := NewHub(nil, nil, "", nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				h.SetVisibility("u1", j%2 == 0, "online")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				if h.liveAllowed("u1") {
					h.setLive("u1", livestate.State{Slug: "rocket-league"}, sourceClient)
				}
			}
		}()
	}
	wg.Wait()
}

func TestPublishLiveRespecteLInvisibilite(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", false, "online")
	h.PublishLive(context.Background(), "membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion": "Ahri"},
	})
	if h.LiveMatch("membre") != nil {
		t.Fatal("un membre invisible ne doit pas apparaître, même publié par le serveur")
	}
}

func TestPublishLiveStockeEtEfface(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", true, "online")
	h.PublishLive(context.Background(), "membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion": "Ahri"},
	})
	if h.LiveMatch("membre") == nil {
		t.Fatal("l'état publié doit être visible")
	}
	h.PublishLive(context.Background(), "membre", livestate.State{Slug: "league-of-legends", Ended: true})
	if h.LiveMatch("membre") != nil {
		t.Fatal("la fin doit effacer l'état")
	}
}

// Une source lente doit pouvoir dire elle-même combien de temps son état reste
// valable. Sans ça, le poller LoL (une minute) se ferait balayer par un défaut
// calibré pour un client qui émet deux fois par seconde, et sa carte
// clignoterait au lieu de rester affichée.
func TestExpiredUtiliseLaDureeDeLaSource(t *testing.T) {
	now := time.Now()
	slow := liveEntry{updatedAt: now.Add(-30 * time.Second), ttl: 3 * time.Minute}
	if slow.expired(now) {
		t.Fatal("une source qui annonce trois minutes ne doit pas expirer en trente secondes")
	}
	if !(liveEntry{updatedAt: now.Add(-4 * time.Minute), ttl: 3 * time.Minute}).expired(now) {
		t.Fatal("elle doit tout de même expirer passé sa propre durée")
	}
	// Zéro veut dire « le défaut », pas « expire immédiatement » : c'est le cas
	// de Rocket League, qui ne déclare rien.
	if (liveEntry{updatedAt: now.Add(-5 * time.Second)}).expired(now) {
		t.Fatal("une durée nulle doit retomber sur le défaut, pas expirer aussitôt")
	}
	if !(liveEntry{updatedAt: now.Add(-30 * time.Second)}).expired(now) {
		t.Fatal("le défaut doit toujours expirer au-delà de liveTTL")
	}
}

// La fin d'une partie ne doit effacer que l'état du jeu qui l'annonce.
// L'état en cours est indexé par membre, pas par jeu : tant qu'un seul jeu
// savait annoncer une fin, personne ne pouvait effacer la partie d'un autre.
// Dès qu'un deuxième le peut, une fin de Hearthstone effacerait un match de
// Rocket League encore en cours pour le même membre.
func TestUneFinNEffaceQueSonPropreJeu(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", true, "online")
	// Source serveur, parce que ce test parle du slug et de rien d'autre :
	// une fin publiée par le serveur ne peut pas effacer un état poussé par un
	// client encore frais, et ce n'est pas la règle qu'on vérifie ici.
	h.setLive("membre", livestate.State{
		Slug:  "rocket-league",
		Match: map[string]any{"team_blue_score": 2},
	}, sourceServer)

	h.PublishLive(context.Background(), "membre", livestate.State{Slug: "hearthstone", Ended: true})
	if h.LiveMatch("membre") == nil {
		t.Fatal("une fin de Hearthstone a effacé le match de Rocket League")
	}

	// Et la fin du bon jeu efface bien, sinon la carte ne disparaîtrait jamais.
	h.PublishLive(context.Background(), "membre", livestate.State{Slug: "rocket-league", Ended: true})
	if h.LiveMatch("membre") != nil {
		t.Fatal("la fin du jeu en cours doit effacer son état")
	}
}

// clearLive est l'endroit où la règle vit : le test la vérifie aussi
// directement, parce que handleLiveMatch passe par le même point.
func TestClearLiveNEffaceQueLeJeuNomme(t *testing.T) {
	h := NewHub(nil, nil, "", nil, nil)
	h.setLive("membre", livestate.State{Slug: "rocket-league", Match: map[string]any{"x": 1}}, sourceClient)

	if h.clearLive("membre", "hearthstone", sourceClient) {
		t.Error("clearLive a prétendu effacer un état qui n'est pas celui du jeu nommé")
	}
	if h.LiveMatch("membre") == nil {
		t.Error("l'état d'un autre jeu doit survivre")
	}
	if !h.clearLive("membre", "rocket-league", sourceClient) {
		t.Error("clearLive devait signaler qu'il y avait un match en cours")
	}
	if h.LiveMatch("membre") != nil {
		t.Error("l'état survit à la fin du match")
	}
	if h.clearLive("membre", "rocket-league", sourceClient) {
		t.Error("clearLive sur un membre sans match doit rendre false")
	}
}

// League of Legends est désormais rapporté par DEUX sources pour le même
// membre et le même jeu : le client pousse l'état riche de l'API locale
// plusieurs fois par seconde, le poller Spectator tire un état pauvre une fois
// par minute. Sans règle, le poller écraserait périodiquement l'état riche et
// la carte perdrait son KDA une seconde sur soixante.
func TestUnEtatServeurNEcrasePasUnEtatClientFrais(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", true, "online")

	h.setLive("membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion": "Ahri", "kills": 7, "gold": 8350},
	}, sourceClient)

	h.PublishLive(context.Background(), "membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion_name": "Ahri", "mode": "CLASSIC"},
		TTL:   3 * time.Minute,
	})

	got := h.LiveMatch("membre")
	if got == nil {
		t.Fatal("l'état du membre a disparu")
	}
	if got["kills"] != 7 {
		t.Fatalf("l'état riche du client a été écrasé par celui du serveur : %v", got)
	}
}

// La fin annoncée par le serveur est le même écrasement par un autre chemin :
// Spectator peut répondre 404 en pleine partie, et le client qui pousse
// toujours prouve que la partie continue.
func TestUneFinServeurNEffacePasUnEtatClientFrais(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", true, "online")
	h.setLive("membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion": "Ahri", "kills": 7},
	}, sourceClient)

	h.PublishLive(context.Background(), "membre", livestate.State{Slug: "league-of-legends", Ended: true})
	if h.LiveMatch("membre") == nil {
		t.Fatal("une fin tirée par le serveur a effacé l'état poussé par le client")
	}
}

// L'inverse doit rester vrai, sinon la couverture qui justifie Spectator est
// perdue : dès que le client se tait assez longtemps, le serveur reprend la
// main.
func TestLeServeurReprendLaMainApresExpirationDuClient(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", true, "online")
	h.setLive("membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion": "Ahri", "kills": 7},
	}, sourceClient)
	ageLiveEntry(h, "membre", liveTTL+time.Second)

	h.PublishLive(context.Background(), "membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion_name": "Ahri", "mode": "CLASSIC"},
		TTL:   3 * time.Minute,
	})

	got := h.LiveMatch("membre")
	if got == nil {
		t.Fatal("le serveur n'a pas repris la main sur un état client périmé")
	}
	if _, rich := got["kills"]; rich {
		t.Fatalf("l'état client périmé est resté en place : %v", got)
	}
}

// Un état client remplace toujours un état serveur, quel que soit son âge :
// c'est la source la plus riche et la plus fraîche des deux.
func TestUnEtatClientRemplaceToujoursUnEtatServeur(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", true, "online")
	h.PublishLive(context.Background(), "membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion_name": "Ahri", "mode": "CLASSIC"},
		TTL:   3 * time.Minute,
	})

	h.setLive("membre", livestate.State{
		Slug:  "league-of-legends",
		Match: map[string]any{"champion": "Ahri", "kills": 7},
	}, sourceClient)

	got := h.LiveMatch("membre")
	if got == nil || got["kills"] != 7 {
		t.Fatalf("l'état client doit remplacer celui du serveur : %v", got)
	}
}

// Spectator seul doit continuer de fonctionner, pour le membre qui n'a pas de
// client KFIRE à jour : c'est cette couverture qui justifie la source.
func TestSpectatorSeulSeSuccedeAluiMeme(t *testing.T) {
	h := NewHub([]byte("secret"), nil, "", nil, nil)
	h.SetVisibility("membre", true, "online")
	for _, mode := range []string{"CLASSIC", "ARAM"} {
		h.PublishLive(context.Background(), "membre", livestate.State{
			Slug:  "league-of-legends",
			Match: map[string]any{"champion_name": "Ahri", "mode": mode},
			TTL:   3 * time.Minute,
		})
	}
	got := h.LiveMatch("membre")
	if got == nil || got["mode"] != "ARAM" {
		t.Fatalf("un poll serveur doit succéder au précédent : %v", got)
	}
}

// ageLiveEntry vieillit artificiellement l'entrée d'un membre, pour ne pas
// faire attendre le test pendant toute la durée du TTL.
func ageLiveEntry(h *Hub, userID string, by time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.live[userID]
	e.updatedAt = e.updatedAt.Add(-by)
	h.live[userID] = e
}

// TestLiveSnapshot covers what a freshly loaded page is handed.
//
// The regression it guards is Hearthstone's: a reload mid-turn showed nothing
// at all, because the browser store starts empty and Hearthstone only emits on
// turn changes. The hub knew; it just never said.
func TestLiveSnapshot(t *testing.T) {
	h := NewHub(nil, nil, "", nil, nil)

	if h.LiveSnapshot("inconnu") != nil {
		t.Error("a member with no match must produce no snapshot")
	}

	h.setLive("u1", livestate.State{
		Slug:  "hearthstone",
		Match: map[string]any{"mode": "battlegrounds", "turn": 7},
		TTL:   2 * time.Minute,
	}, sourceClient)

	got := h.LiveSnapshot("u1")
	if got == nil {
		t.Fatal("a match in progress must produce a snapshot")
	}
	if got["game_slug"] != "hearthstone" {
		t.Errorf("the snapshot must name its game, got %v", got["game_slug"])
	}
	m, ok := got["match"].(map[string]any)
	if !ok || m["turn"] != 7 {
		t.Errorf("the snapshot must carry the state itself, got %v", got["match"])
	}

	// An entry nobody has refreshed is as good as gone: handing it to a page
	// would paint a score that stopped being true minutes ago.
	h.mu.Lock()
	e := h.live["u1"]
	e.updatedAt = time.Now().Add(-3 * time.Minute)
	h.live["u1"] = e
	h.mu.Unlock()

	if h.LiveSnapshot("u1") != nil {
		t.Error("an expired entry must produce no snapshot")
	}
}
