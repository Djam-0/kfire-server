package pubg

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUn429EstReessaye(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.Header().Set("X-RateLimit-Reset", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"ok"}}`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	var out struct {
		Data struct{ ID string } `json:"data"`
	}
	if err := c.get("/shards/steam/players", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Data.ID != "ok" {
		t.Fatal("la seconde tentative n'a pas été décodée")
	}
	if n != 2 {
		t.Fatalf("%d appels, attendu 2", n)
	}
}

func TestUneErreurPorteSonStatut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	var out struct{}
	err := c.get("/shards/steam/players", &out)
	if err == nil {
		t.Fatal("un 404 doit remonter")
	}
	if !NotFound(err) {
		t.Fatalf("NotFound devrait reconnaître ce cas : %v", err)
	}
}

func TestLesEntetesSontCeuxQueLApiExige(t *testing.T) {
	var auth, accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		accept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIBase = srv.URL
	c.SetRate(1000)

	var out struct{}
	if err := c.get("/shards/steam/players", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if auth != "Bearer clé-de-test" {
		t.Fatalf("Authorization = %q", auth)
	}
	if accept != "application/vnd.api+json" {
		t.Fatalf("Accept = %q", accept)
	}
}

func TestSeulsLesCheminsFacturesSontFreines(t *testing.T) {
	cas := []struct {
		chemin string
		compte bool
	}{
		{"/shards/steam/players", true},
		{"/shards/steam/players/account.abc", true},
		{"/shards/steam/samples", true},
		{"/shards/steam/seasons", true},
		// Le quota de l'éditeur ne compte ni les matchs ni la télémétrie :
		// les freiner rendrait une synchronisation lente pour rien.
		{"/shards/steam/matches/0e4025d6-701b-422a-a87d-204380983645", false},
		{"/shards/steam/telemetry/whatever", false},
	}
	for _, c := range cas {
		if got := countsAgainstQuota(c.chemin); got != c.compte {
			t.Errorf("countsAgainstQuota(%q) = %v, attendu %v", c.chemin, got, c.compte)
		}
	}
}

func TestUnConnecteurSansCleEstDesactive(t *testing.T) {
	if New("").Enabled() {
		t.Fatal("sans clé, le connecteur doit être désactivé")
	}
	if !New("clé-de-test").Enabled() {
		t.Fatal("avec une clé, le connecteur doit être actif")
	}
}
