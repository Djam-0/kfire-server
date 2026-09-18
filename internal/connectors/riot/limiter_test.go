package riot

import (
	"context"
	"testing"
	"time"
)

func TestLimiterEspaceLesAppels(t *testing.T) {
	l := newLimiter(10) // 10 par seconde
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.wait(ctx); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	// Le premier part tout de suite, les deux suivants sont espacés de 100 ms.
	if d := time.Since(start); d < 180*time.Millisecond {
		t.Fatalf("trois appels ont pris %v, trop rapide pour 10/s", d)
	}
}

func TestLimiterRespecteLAnnulation(t *testing.T) {
	l := newLimiter(1)
	ctx, cancel := context.WithCancel(context.Background())
	if err := l.wait(ctx); err != nil { // consomme le jeton initial
		t.Fatalf("premier wait: %v", err)
	}
	cancel()
	if err := l.wait(ctx); err == nil {
		t.Fatal("un contexte annulé doit faire échouer l'attente, pas la bloquer")
	}
}

func TestLimiterPauseApresUn429(t *testing.T) {
	l := newLimiter(1000) // assez rapide pour que seule la pause compte
	l.penalise(150 * time.Millisecond)
	start := time.Now()
	if err := l.wait(context.Background()); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if d := time.Since(start); d < 120*time.Millisecond {
		t.Fatalf("l'attente a duré %v, la pénalité n'a pas été respectée", d)
	}
}
