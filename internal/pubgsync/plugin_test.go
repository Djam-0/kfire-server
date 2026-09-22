package pubgsync

import (
	"context"
	"testing"

	"github.com/knightsofeternity/kfire-server/internal/connectors/pubg"
	"github.com/knightsofeternity/kfire-server/internal/gameplugin"
)

var _ gameplugin.Plugin = (*Plugin)(nil)

func TestIdentiteDuPlugin(t *testing.T) {
	p := NewPlugin(nil, nil)
	if p.ID() != "pubg" {
		t.Errorf("ID() = %q, want \"pubg\"", p.ID())
	}
	if p.Connector() != "pubg" {
		t.Errorf("Connector() = %q, want \"pubg\"", p.Connector())
	}
}

// The one failure this guards against is silent: a sync filing matches under
// one slug while the plugin reads another would leave every page empty with
// nothing logged. The single constant is what rules it out.
func TestLeSlugEstCeluiDeLaSynchronisation(t *testing.T) {
	got := NewPlugin(nil, nil).Slugs()
	if len(got) != 1 || got[0] != SLUG {
		t.Fatalf("Slugs() = %v, want [%s]", got, SLUG)
	}
	if SLUG != "pubg-battlegrounds" {
		t.Errorf("SLUG = %q, want \"pubg-battlegrounds\"", SLUG)
	}
}

func TestSansCleLePluginEstIndisponible(t *testing.T) {
	if NewPlugin(nil, pubg.New("")).Available() {
		t.Error("Available() = true without a key, want false")
	}
	if !NewPlugin(nil, pubg.New("une-cle")).Available() {
		t.Error("Available() = false with a key, want true")
	}
}

// Refresh must not touch the store: it is called for every active plugin while
// a member's page is built, and this one holds a nil store in the test.
func TestRefreshNeFaitRien(t *testing.T) {
	NewPlugin(nil, nil).Refresh(context.Background(), "u1", SLUG)
}
