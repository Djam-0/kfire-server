package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/knightsofeternity/kfire-server/internal/config"
	"github.com/knightsofeternity/kfire-server/internal/connectors/pubg"
)

func TestUnePlateformeInconnueEstRefusee(t *testing.T) {
	for _, p := range pubg.KnownPlatforms() {
		if !validPubgPlatform(p) {
			t.Errorf("validPubgPlatform(%q) = false, attendu true", p)
		}
	}
	// The platform is pasted into a shard path, so anything that is not one of
	// the known values has to be refused rather than escaped.
	for _, bad := range []string{
		"", "Steam", "pc", "steam; DROP TABLE users",
		"../../etc/passwd", "steam/../xbox", "steam%2F..",
	} {
		if validPubgPlatform(bad) {
			t.Errorf("validPubgPlatform(%q) = true, attendu false", bad)
		}
	}
}

// TestLesRoutesPubgSontMonteesEtProtegees proves the four PUBG routes are
// wired and all require a bearer token.
//
// The package has no fixture that mounts the full api.Register (it needs a
// live database), so this mounts the same handler methods directly, with a
// disabled connector. That is enough to exercise requireAuth's gate, which
// runs before any store or PUBG access.
func TestLesRoutesPubgSontMonteesEtProtegees(t *testing.T) {
	h := &handlers{
		cfg:  &config.Config{JWTSecret: "test-secret"},
		pubg: pubg.New(""), // disabled: no API key configured
	}

	app := fiber.New()
	v1 := app.Group("/api/v1")
	v1.Post("/connect/pubg", h.requireAuth, h.connectPubg)
	v1.Get("/connect/pubg/platform", h.requireAuth, h.pubgPlatform)
	v1.Patch("/connect/pubg/platform", h.requireAuth, h.updatePubgPlatform)
	v1.Delete("/connect/pubg", h.requireAuth, h.disconnectPubg)

	cases := []struct {
		method, path string
	}{
		{"POST", "/api/v1/connect/pubg"},
		{"GET", "/api/v1/connect/pubg/platform"},
		{"PATCH", "/api/v1/connect/pubg/platform"},
		{"DELETE", "/api/v1/connect/pubg"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s %s : requête échouée : %v", tc.method, tc.path, err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("%s %s sans jeton : %d obtenu, 401 attendu", tc.method, tc.path, resp.StatusCode)
		}
	}
}
