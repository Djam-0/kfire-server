package riotsync

import (
	"testing"
	"time"
)

// Le curseur doit reculer d'une page à l'autre, et se poser sur le match le
// PLUS ANCIEN de la page : c'est ce qui rend le rattrapage reprenable.
func TestCurseurRecule(t *testing.T) {
	oldest := time.Unix(1700000000, 0).UTC()
	newest := time.Unix(1700009999, 0).UTC()
	got := oldestPlayedAt([]time.Time{newest, oldest})
	if !got.Equal(oldest) {
		t.Fatalf("curseur %v, attendu le plus ancien %v", got, oldest)
	}
}

// Une page vide veut dire qu'on a atteint le bout de ce que Riot garde.
func TestPageVideTermineLeRattrapage(t *testing.T) {
	if !backfillDone(0) {
		t.Fatal("une page sans identifiant doit terminer le rattrapage")
	}
	if backfillDone(3) {
		t.Fatal("une page pleine ne doit pas terminer le rattrapage")
	}
}
