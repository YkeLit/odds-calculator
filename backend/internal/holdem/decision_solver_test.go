package holdem

import (
	"math"
	"testing"

	"odds-calculator/backend/internal/models"
)

func TestCalculateDecision_BasicFlop(t *testing.T) {
	req := models.HoldemDecisionRequest{
		Hero: models.HeroState{
			HoleCards: []string{"As", "Ks"},
			Position:  "BTN",
			Stack:     100,
		},
		Table: models.TableState{
			PlayerCount:     2,
			Positions:       []string{"BB", "BTN"},
			EffectiveStacks: map[string]float64{"BB": 100, "BTN": 100},
			RakeConfig:      models.RakeConfig{Enabled: false},
		},
		Street:     models.StreetFlop,
		BoardCards: []string{"Qs", "Js", "2d"},
		DeadCards:  []string{},
		PotState: models.PotState{
			PotSize:    20,
			ToCall:     10, // BB bet 10 into 10
			MinRaiseTo: 20,
			Blinds:     [2]float64{1, 2},
		},
		ActionHistory: []models.ActionNode{
			{Street: models.StreetFlop, Actor: "BB", Action: models.ActionBet, Amount: 10},
		},
		Opponents: []models.OpponentInfo{
			{ID: "BB", Position: "BB", StylePreset: "balanced", RangeOverride: ""}, // Will use default BB range
		},
		SolverConfig: models.SolverConfig{
			BranchCount:   3,
			TimeoutMs:     5000,
			RolloutBudget: 1500,
		},
	}

	res, err := CalculateDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.TopActions) == 0 {
		t.Fatalf("expected some actions to be recommended")
	}

	// We have a massive draw (Royal Flush draw)
	// Even against a tight range, going all in or calling should be positive EV
	hasPositiveEV := false
	for _, act := range res.TopActions {
		if act.Action == models.ActionFold {
			if math.Abs(act.EV) > 0.01 { // Fold EV is always 0
				t.Errorf("Fold EV should be 0, got %f", act.EV)
			}
		} else {
			if act.EV > 0 {
				hasPositiveEV = true
			}
		}
	}

	if !hasPositiveEV {
		t.Errorf("expected at least one positive EV action with AsKs on QsJs2d against BB")
	}

	if res.TreeStats.Rollouts < 1000 {
		t.Errorf("expected at least 1000 rollouts, got %d", res.TreeStats.Rollouts)
	}

	if len(res.OpponentRangeSummary) != 1 {
		t.Fatalf("expected 1 opponent summary")
	}

	if res.HeroMetrics.Equity < 0.2 || res.HeroMetrics.Equity > 0.8 {
		t.Errorf("expected reasonable equity estimation (0.2-0.8 for strong draw), got %f", res.HeroMetrics.Equity)
	}
}

func TestCalculateDecision_DeadHand(t *testing.T) {
	req := models.HoldemDecisionRequest{
		Hero: models.HeroState{
			HoleCards: []string{"2c", "7o"}, // Invalid card "7o", will fail at ParseCards or similar
			Position:  "BTN",
			Stack:     100,
		},
	}
	// "7o" is an invalid card string for the parser, it expects rank+suit like "7s" or "7c"
	// However, let's use a real card 7s but give a board that makes it impossible to win
	req.Hero.HoleCards = []string{"2c", "7s"}
	req.BoardCards = []string{"As", "Ah", "Ac", "Ad", "Ks"} // Hero plays board, best is AAAAK
	req.Opponents = []models.OpponentInfo{
		{ID: "BB", Position: "BB", RangeOverride: "KK"}, // Opponent has KK -> plays AAAAKK (Fullhouse? No, AAAAK is better. Wait, opponent plays AAAAK too)
	}
	req.PotState.ToCall = 50
	req.PotState.PotSize = 10
	req.SolverConfig.RolloutBudget = 1000

	res, err := CalculateDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Since both play board AAAAK, it's a tie. 
	// Pot size is 10. We have to call 50.
	// If it's a tie, we split 10+50+50 = 110 / 2 = 55.
	// Our cost is 50. EV = 5 (positive).
	if len(res.TopActions) == 0 {
		t.Fatalf("no actions")
	}
}
