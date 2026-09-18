package riot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Un 429 avec Retry-After doit être réessayé, pas remonté à l'appelant : le
// rattrapage ferait sinon un trou dans l'historique à chaque pic de quota.
func TestGetReessayeApresUn429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIHostTmpl = srv.URL + "/%s"
	// Only the retry count is under test here, not the pace: the fake host has
	// no quota to protect.
	c.SetRate(1000)

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.get(context.Background(), "europe", "/peu-importe", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !out.OK {
		t.Fatal("la réponse de la seconde tentative n'a pas été décodée")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("%d appels, attendu 2 (un refus puis une réussite)", got)
	}
}

// Un 429 permanent doit finir par rendre la main, sinon une requête HTTP
// resterait bloquée indéfiniment.
func TestGetAbandonneApresPlusieurs429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIHostTmpl = srv.URL + "/%s"
	// Only the retry count is under test here, not the pace: the fake host has
	// no quota to protect.
	c.SetRate(1000)

	var out struct{}
	err := c.get(context.Background(), "europe", "/peu-importe", &out)
	if err == nil {
		t.Fatal("un 429 permanent doit finir par échouer")
	}
	var apiErr *APIError
	if !asAPIError(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests {
		t.Fatalf("erreur inattendue : %v", err)
	}
}
