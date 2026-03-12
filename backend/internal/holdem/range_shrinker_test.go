package holdem

import (
	"testing"
	"odds-calculator/backend/internal/models"
)

func TestRangeShrinker_ApplyActionHistory(t *testing.T) {
	shrinker := NewRangeShrinker()
	
	// Start with full range
	rng := NewFullRange()
	_ = rng.Normalize()

	// Pretend UTG raised preflop
	history := []models.ActionNode{
		{
			Street: models.StreetPreflop,
			Actor:  "UTG",
			Action: models.ActionRaise,
			Amount: 10,
		},
	}

	newRng := shrinker.ApplyActionHistory(rng, history, nil)

	// Check that AA is now much more likely than 72o
	idxAA := ComboToIndex["AdAh"]
	idx72o := ComboToIndex["7s2c"]

	if newRng[idxAA] <= newRng[idx72o] {
		t.Errorf("Expected AA to be more likely than 72o after a raise. AA: %f, 72o: %f", newRng[idxAA], newRng[idx72o])
	}
}

func TestRangeShrinker_Postflop(t *testing.T) {
	shrinker := NewRangeShrinker()
	
	rng := NewFullRange()
	_ = rng.Normalize()

	board := []Card{
		{Rank: 14, Suit: 's', Code: "As"},
		{Rank: 13, Suit: 's', Code: "Ks"},
		{Rank: 12, Suit: 's', Code: "Qs"},
	}

	history := []models.ActionNode{
		{
			Street: models.StreetFlop,
			Actor:  "BTN",
			Action: models.ActionRaise,
			Amount: 50,
		},
	}

	newRng := shrinker.ApplyActionHistory(rng, history, board)

	// JTs gives straight flush = very likely to raise
	idxJTs := ComboToIndex["JsTs"]
	// 2d3h gives nothing = very unlikely to raise
	idx23o := ComboToIndex["3h2d"]

	if newRng[idxJTs] <= newRng[idx23o] {
		t.Errorf("Expected JTs to be more likely than 3h2d after a flop raise on AsKsQs. JTs: %f, 3h2d: %f", newRng[idxJTs], newRng[idx23o])
	}
}
