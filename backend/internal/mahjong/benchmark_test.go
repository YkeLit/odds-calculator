package mahjong

import (
	"testing"

	"odds-calculator/backend/internal/models"
)

func BenchmarkAnalyzeTwoDraw(b *testing.B) {
	req := models.MahjongAnalyzeRequest{
		HandTiles: []string{
			"m1", "m1", "m2", "m2", "m3", "m3",
			"p4", "p4", "p5", "p5", "s6", "s6", "s7", "s8",
		},
		MissingSuit: "",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Analyze(req); err != nil {
			b.Fatalf("Analyze: %v", err)
		}
	}
}
