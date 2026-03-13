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
			ToCall:     10,
			MinRaiseTo: 20,
			Blinds:     [2]float64{1, 2},
		},
		ActionHistory: []models.ActionNode{
			{Street: models.StreetFlop, Actor: "BB", Action: models.ActionBet, Amount: 10},
		},
		Opponents: []models.OpponentInfo{
			{ID: "BB", Position: "BB", StylePreset: "balanced", RangeOverride: ""},
		},
		SolverConfig: models.SolverConfig{
			BranchCount:   3,
			TimeoutMs:     10000,
			RolloutBudget: 2000,
		},
	}

	res, err := CalculateDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.TopActions) == 0 {
		t.Fatalf("expected some actions to be recommended")
	}

	// MCCFR should produce actions with frequency > 0
	hasNonZeroFreq := false
	for _, act := range res.TopActions {
		t.Logf("Action: %s, Amount: %.1f, EV: %.4f, Freq: %.4f", act.Action, act.Amount, act.EV, act.Frequency)
		if act.Frequency > 0 {
			hasNonZeroFreq = true
		}
		if act.Action == models.ActionFold {
			if math.Abs(act.EV) > 0.01 {
				t.Errorf("Fold EV should be 0, got %f", act.EV)
			}
		}
	}

	if !hasNonZeroFreq {
		t.Errorf("expected at least one action with non-zero frequency from MCCFR")
	}

	if res.TreeStats.Rollouts < 1000 {
		t.Errorf("expected at least 1000 MCCFR iterations, got %d", res.TreeStats.Rollouts)
	}

	if len(res.OpponentRangeSummary) != 1 {
		t.Fatalf("expected 1 opponent summary")
	}

	if res.HeroMetrics.Equity < 0.2 || res.HeroMetrics.Equity > 0.8 {
		t.Errorf("expected reasonable equity estimation (0.2-0.8 for strong draw), got %f", res.HeroMetrics.Equity)
	}

	if res.TreeStats.Convergence < 0 || res.TreeStats.Convergence > 1 {
		t.Errorf("convergence should be in [0,1], got %f", res.TreeStats.Convergence)
	}
}

func TestCalculateDecision_DeadHand(t *testing.T) {
	req := models.HoldemDecisionRequest{
		Hero: models.HeroState{
			HoleCards: []string{"2c", "7s"},
			Position:  "BTN",
			Stack:     100,
		},
		Street:     models.StreetRiver,
		BoardCards: []string{"As", "Ah", "Ac", "Ad", "Ks"},
		DeadCards:  []string{},
		Opponents: []models.OpponentInfo{
			{ID: "BB", Position: "BB", RangeOverride: "KK"},
		},
		PotState: models.PotState{
			ToCall:     50,
			PotSize:    10,
			MinRaiseTo: 100,
			Blinds:     [2]float64{1, 2},
		},
		SolverConfig: models.SolverConfig{
			BranchCount:   3,
			TimeoutMs:     10000,
			RolloutBudget: 1000,
		},
	}

	res, err := CalculateDecision(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.TopActions) == 0 {
		t.Fatalf("no actions")
	}

	for _, act := range res.TopActions {
		t.Logf("Action: %s, Amount: %.1f, EV: %.4f, Freq: %.4f", act.Action, act.Amount, act.EV, act.Frequency)
	}
}

func TestMCCFR_BasicConvergence(t *testing.T) {
	// Test that MCCFR converges to a reasonable strategy
	// Hero has AA on a dry flop - should strongly favor value betting
	heroHole := [2]Card{
		{Rank: 14, Suit: 's', Code: "As"},
		{Rank: 14, Suit: 'h', Code: "Ah"},
	}
	board := []Card{
		{Rank: 7, Suit: 'd', Code: "7d"},
		{Rank: 2, Suit: 'c', Code: "2c"},
		{Rank: 9, Suit: 's', Code: "9s"},
	}

	oppRange, _ := ParseRangeOverride("22+,A2s+,KTs+,QJs")
	positions := []string{"BTN", "BB"}
	stacks := []float64{100, 100}
	pot := models.PotState{
		PotSize:    10,
		ToCall:     0,
		MinRaiseTo: 4,
		Blinds:     [2]float64{1, 2},
	}

	solver := NewMCCFRSolver(heroHole, positions, []Range{oppRange}, board, nil, pot, stacks, 3000)
	solver.Run()

	actions, strategy := solver.GetRootStrategy()
	t.Log("MCCFR converged strategy:")
	for i, act := range actions {
		t.Logf("  %s: %.4f", act, strategy[i])
	}

	// With AA on a dry board and no bet to call, check or bet should dominate
	// Fold should have ~0 frequency (can't fold when toCall=0 anyway)
	if len(actions) == 0 {
		t.Fatal("no actions generated")
	}

	totalFreq := 0.0
	for _, f := range strategy {
		totalFreq += f
	}
	if math.Abs(totalFreq-1.0) > 0.01 {
		t.Errorf("strategy frequencies should sum to 1.0, got %f", totalFreq)
	}
}
