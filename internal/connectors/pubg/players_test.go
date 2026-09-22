package pubg

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// playerBody is the real shape of a players answer, with the account id and
// the in-game name replaced by invented values: a real response names a real
// person, and none of them belong in this repository.
const playerBody = `{"data":[{"type":"player","id":"account.abc",
  "attributes":{"name":"Pseudo","shardId":"steam","banType":"Innocent","clanId":"","titleId":"bluehole-pubg","patchVersion":"","stats":null},
  "relationships":{"assets":{"data":[]},
    "matches":{"data":[{"type":"match","id":"m1"},{"type":"match","id":"m2"},{"type":"match","id":"m3"}]}}}]}`

func TestUnJoueurTrouveRendSonIdentifiantEtSesMatchs(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("filter[playerNames]")
		_, _ = w.Write([]byte(playerBody))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	p, err := c.PlayerByName("steam", "Pseudo")
	if err != nil {
		t.Fatalf("PlayerByName: %v", err)
	}
	if p.AccountID != "account.abc" {
		t.Fatalf("AccountID = %q", p.AccountID)
	}
	if p.Name != "Pseudo" {
		t.Fatalf("Name = %q", p.Name)
	}
	// L'ordre est celui de l'API, du plus récent au plus ancien, et la
	// synchronisation s'appuie dessus pour s'arrêter tôt.
	if len(p.MatchIDs) != 3 || p.MatchIDs[0] != "m1" || p.MatchIDs[2] != "m3" {
		t.Fatalf("MatchIDs = %v", p.MatchIDs)
	}
	if gotPath != "/shards/steam/players" {
		t.Fatalf("chemin = %q", gotPath)
	}
	if gotQuery != "Pseudo" {
		t.Fatalf("filtre = %q", gotQuery)
	}
}

func TestUnPseudoAEchapperResteIntact(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("filter[playerNames]")
		_, _ = w.Write([]byte(playerBody))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	if _, err := c.PlayerByName("steam", "a b&c=d"); err != nil {
		t.Fatalf("PlayerByName: %v", err)
	}
	if gotQuery != "a b&c=d" {
		t.Fatalf("filtre = %q", gotQuery)
	}
}

func TestUnPseudoInconnuEstReconnaissable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"title":"Not Found","detail":"No Players Found Matching Criteria"}]}`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	_, err := c.PlayerByName("steam", "personne")
	if err == nil {
		t.Fatal("un pseudo inconnu doit rendre une erreur")
	}
	if !NotFound(err) {
		t.Fatalf("le membre doit comprendre qu'il s'est trompé de pseudo : %v", err)
	}
}

func TestUnTableauVideNEstPasUnePanique(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	_, err := c.PlayerByName("steam", "personne")
	if err == nil {
		t.Fatal("un data vide doit rendre une erreur")
	}
	if !NotFound(err) {
		t.Fatalf("un data vide se traite comme un pseudo inconnu : %v", err)
	}
}

func TestUnJoueurSansMatchNEstPasUneErreur(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"type":"player","id":"account.abc",
		  "attributes":{"name":"Pseudo"},"relationships":{"matches":{"data":[]}}}]}`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	p, err := c.PlayerByName("steam", "Pseudo")
	if err != nil {
		t.Fatalf("PlayerByName: %v", err)
	}
	if p.AccountID != "account.abc" || len(p.MatchIDs) != 0 {
		t.Fatalf("joueur = %+v", p)
	}
}
