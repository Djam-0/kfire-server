package riot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// Transient decides whether a failed match is retried or skipped for good, and
// getting it backwards is silently destructive in one direction: a rate limit
// mistaken for a missing match makes the backfill's cursor step over history
// that nothing will ever walk again.
func TestTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"pas d'erreur", nil, false},
		{"quota dépassé", &APIError{Status: http.StatusTooManyRequests}, true},
		{"panne serveur", &APIError{Status: http.StatusInternalServerError}, true},
		{"passerelle indisponible", &APIError{Status: http.StatusServiceUnavailable}, true},
		{"match introuvable", &APIError{Status: http.StatusNotFound}, false},
		{"clé refusée", &APIError{Status: http.StatusForbidden}, false},
		{"requête invalide", &APIError{Status: http.StatusBadRequest}, false},
		{"le membre n'est pas dans ce match", ErrNoParticipant, false},
		{"erreur emballée autour du participant absent",
			fmt.Errorf("%w: match EUW1_1", ErrNoParticipant), false},
		{"contexte annulé", context.Canceled, true},
		{"panne réseau", errors.New("dial tcp: connection refused"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Transient(c.err); got != c.want {
				t.Fatalf("Transient(%v) = %v, attendu %v", c.err, got, c.want)
			}
		})
	}
}
