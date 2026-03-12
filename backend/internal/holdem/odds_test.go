package holdem

import (
	"math"
	"testing"

	"odds-calculator/backend/internal/models"
)

func TestCalculateOddsDeterministicWin(t *testing.T) {
	resp, err := CalculateOdds(models.HoldemOddsRequest{
		Players: []models.PlayerInput{
			{ID: "p1", HoleCards: []string{"As", "Ah"}},
			{ID: "p2", HoleCards: []string{"Kc", "Kd"}},
		},
		BoardCards: []string{"2c", "7d", "9h", "Js", "3d"},
	})
	if err != nil {
		t.Fatalf("CalculateOdds error: %v", err)
	}
	if resp.CombosEvaluated != 1 {
		t.Fatalf("expected 1 combo, got %d", resp.CombosEvaluated)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].WinRate != 1 || resp.Results[0].Equity != 1 {
		t.Fatalf("p1 expected full win, got %+v", resp.Results[0])
	}
	if resp.Results[1].WinRate != 0 || resp.Results[1].Equity != 0 {
		t.Fatalf("p2 expected zero equity, got %+v", resp.Results[1])
	}
}

func TestCalculateOddsBoardTie(t *testing.T) {
	resp, err := CalculateOdds(models.HoldemOddsRequest{
		Players: []models.PlayerInput{
			{ID: "p1", HoleCards: []string{"Ah", "Ad"}},
			{ID: "p2", HoleCards: []string{"Ks", "Kd"}},
		},
		BoardCards: []string{"2c", "3c", "4c", "5c", "6c"},
	})
	if err != nil {
		t.Fatalf("CalculateOdds error: %v", err)
	}
	if resp.CombosEvaluated != 1 {
		t.Fatalf("expected 1 combo, got %d", resp.CombosEvaluated)
	}
	for _, r := range resp.Results {
		if r.TieRate != 1 {
			t.Fatalf("expected tieRate=1, got %+v", r)
		}
		if r.Equity != 0.5 {
			t.Fatalf("expected equity=0.5, got %+v", r)
		}
	}
}

func TestCalculateAllInEVSidePots(t *testing.T) {
	resp, err := CalculateAllInEV(models.HoldemAllInEVRequest{
		Players: []models.PlayerInput{
			{ID: "p1", HoleCards: []string{"As", "Ah"}, AllIn: true},
			{ID: "p2", HoleCards: []string{"Kc", "Kd"}, AllIn: true},
			{ID: "p3", HoleCards: []string{"Qc", "Qd"}, AllIn: true},
		},
		BoardCards: []string{"2c", "7d", "9h", "Js", "3d"},
		Contributions: map[string]float64{
			"p1": 100,
			"p2": 50,
			"p3": 25,
		},
	})
	if err != nil {
		t.Fatalf("CalculateAllInEV error: %v", err)
	}
	if resp.CombosEvaluated != 1 {
		t.Fatalf("expected 1 combo, got %d", resp.CombosEvaluated)
	}
	if len(resp.PotBreakdown) != 3 {
		t.Fatalf("expected 3 side pots, got %d", len(resp.PotBreakdown))
	}
	if resp.PotBreakdown[0].Amount != 75 || resp.PotBreakdown[1].Amount != 50 || resp.PotBreakdown[2].Amount != 50 {
		t.Fatalf("unexpected pot amounts: %+v", resp.PotBreakdown)
	}

	players := map[string]models.HoldemAllInPlayerEV{}
	for _, p := range resp.Players {
		players[p.ID] = p
	}
	assertNear(t, players["p1"].ExpectedPayout, 175, 0.0001)
	assertNear(t, players["p1"].PlayerEV, 75, 0.0001)
	assertNear(t, players["p2"].ExpectedPayout, 0, 0.0001)
	assertNear(t, players["p3"].ExpectedPayout, 0, 0.0001)
}

func TestCalculateAllInEVWithRakeCap(t *testing.T) {
	resp, err := CalculateAllInEV(models.HoldemAllInEVRequest{
		Players: []models.PlayerInput{
			{ID: "p1", HoleCards: []string{"As", "Ah"}, AllIn: true},
			{ID: "p2", HoleCards: []string{"Kc", "Kd"}, AllIn: true},
			{ID: "p3", HoleCards: []string{"Qc", "Qd"}, AllIn: true},
		},
		BoardCards: []string{"2c", "7d", "9h", "Js", "3d"},
		Contributions: map[string]float64{
			"p1": 100,
			"p2": 50,
			"p3": 25,
		},
		RakeConfig: models.RakeConfig{Enabled: true, RakePercent: 10, RakeCap: 10},
	})
	if err != nil {
		t.Fatalf("CalculateAllInEV error: %v", err)
	}
	assertNear(t, resp.AppliedRakeTotal, 10, 0.0001)

	totalPayout := 0.0
	for _, p := range resp.Players {
		totalPayout += p.ExpectedPayout
	}
	assertNear(t, totalPayout, 165, 0.001)
}

func assertNear(t *testing.T, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Fatalf("got %.6f, want %.6f (eps %.6f)", got, want, eps)
	}
}
