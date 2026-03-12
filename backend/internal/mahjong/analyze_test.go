package mahjong

import (
	"testing"

	"odds-calculator/backend/internal/models"
)

func TestCanHuStandard(t *testing.T) {
	counts := mustCounts(t,
		"m1", "m2", "m3",
		"m4", "m5", "m6",
		"p2", "p3", "p4",
		"s7", "s8", "s9",
		"p5", "p5",
	)
	if !canHu(counts, 0, -1) {
		t.Fatalf("expected hand to be winning")
	}
	if canHu(counts, 0, 1) {
		t.Fatalf("expected missing suit restriction to fail")
	}
}

func TestCalculateFanQiDui(t *testing.T) {
	counts := mustCounts(t,
		"m1", "m1", "m2", "m2", "m3", "m3",
		"p4", "p4", "p5", "p5", "s6", "s6", "s9", "s9",
	)
	fan := calculateFan(counts, nil, -1)
	if fan.TotalFan != 4 {
		t.Fatalf("expected 4 fan, got %+v", fan)
	}
}

func TestAnalyzeTing13(t *testing.T) {
	resp, err := Analyze(models.MahjongAnalyzeRequest{
		HandTiles: []string{
			"m1", "m2", "m3",
			"m4", "m5", "m6",
			"p2", "p3", "p4",
			"s7", "s8", "s9",
			"p5",
		},
	})
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if !resp.IsTing {
		t.Fatalf("expected isTing=true")
	}
	if !contains(resp.WinningTiles, "p5") {
		t.Fatalf("expected p5 in winning tiles: %+v", resp.WinningTiles)
	}
}

func TestAnalyzeRecommendationsSorted(t *testing.T) {
	resp, err := Analyze(models.MahjongAnalyzeRequest{
		HandTiles: []string{
			"m1", "m1", "m2", "m2", "m3", "m3",
			"p4", "p4", "p5", "p5", "s6", "s6", "s7", "s8",
		},
	})
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if len(resp.DiscardRecommendations) == 0 {
		t.Fatalf("expected recommendations")
	}
	for i := 1; i < len(resp.DiscardRecommendations); i++ {
		prev := resp.DiscardRecommendations[i-1]
		curr := resp.DiscardRecommendations[i]
		if prev.ExpectedFan < curr.ExpectedFan {
			t.Fatalf("recommendations not sorted by expectedFan: %+v", resp.DiscardRecommendations)
		}
	}
}

func mustCounts(t *testing.T, tiles ...string) [tileKinds]int {
	t.Helper()
	var counts [tileKinds]int
	if err := addTiles(&counts, tiles); err != nil {
		t.Fatalf("addTiles: %v", err)
	}
	return counts
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
