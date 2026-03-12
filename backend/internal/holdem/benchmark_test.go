package holdem

import (
	"testing"

	"odds-calculator/backend/internal/models"
)

func BenchmarkCalculateOddsSixPlayers(b *testing.B) {
	req := models.HoldemOddsRequest{
		Players: []models.PlayerInput{
			{ID: "p1", HoleCards: []string{"As", "Ah"}},
			{ID: "p2", HoleCards: []string{"Ks", "Kh"}},
			{ID: "p3", HoleCards: []string{"Qs", "Qh"}},
			{ID: "p4", HoleCards: []string{"Js", "Jh"}},
			{ID: "p5", HoleCards: []string{"Ts", "Th"}},
			{ID: "p6", HoleCards: []string{"9s", "9h"}},
		},
		BoardCards: []string{"2c", "7d", "9c"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CalculateOdds(req); err != nil {
			b.Fatalf("CalculateOdds: %v", err)
		}
	}
}

func BenchmarkCalculateAllInEVSixPlayers(b *testing.B) {
	req := models.HoldemAllInEVRequest{
		Players: []models.PlayerInput{
			{ID: "p1", HoleCards: []string{"As", "Ah"}},
			{ID: "p2", HoleCards: []string{"Ks", "Kh"}},
			{ID: "p3", HoleCards: []string{"Qs", "Qh"}},
			{ID: "p4", HoleCards: []string{"Js", "Jh"}},
			{ID: "p5", HoleCards: []string{"Ts", "Th"}},
			{ID: "p6", HoleCards: []string{"9s", "9h"}},
		},
		BoardCards: []string{"2c", "7d", "9c"},
		Contributions: map[string]float64{
			"p1": 100,
			"p2": 90,
			"p3": 80,
			"p4": 70,
			"p5": 60,
			"p6": 50,
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CalculateAllInEV(req); err != nil {
			b.Fatalf("CalculateAllInEV: %v", err)
		}
	}
}
