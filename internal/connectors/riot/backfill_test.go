package riot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMatchIDsBeforeBorneParLeTemps(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`["EUW1_1","EUW1_2"]`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIHostTmpl = srv.URL + "/%s"
	c.SetRate(1000)

	before := time.Unix(1750000000, 0).UTC()
	ids, err := c.MatchIDsBefore(context.Background(), "europe", "un-puuid", before, 100)
	if err != nil {
		t.Fatalf("MatchIDsBefore: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("%d identifiants, attendu 2", len(ids))
	}
	if !strings.Contains(gotQuery, "endTime=1750000000") {
		t.Fatalf("requête %q, il manque la borne de temps", gotQuery)
	}
	if !strings.Contains(gotQuery, "count=100") {
		t.Fatalf("requête %q, il manque le nombre", gotQuery)
	}
}

// Une borne nulle veut dire « depuis le plus récent » : pas de endTime du tout,
// sinon on demanderait les matchs antérieurs à l'époque Unix, donc aucun.
func TestMatchIDsBeforeSansBorne(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New("clé-de-test")
	c.APIHostTmpl = srv.URL + "/%s"
	c.SetRate(1000)

	if _, err := c.MatchIDsBefore(context.Background(), "europe", "un-puuid", time.Time{}, 100); err != nil {
		t.Fatalf("MatchIDsBefore: %v", err)
	}
	if strings.Contains(gotQuery, "endTime") {
		t.Fatalf("requête %q, une borne nulle ne doit poser aucun endTime", gotQuery)
	}
}
