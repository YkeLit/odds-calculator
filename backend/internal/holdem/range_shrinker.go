package holdem

import (
	"math"

	"odds-calculator/backend/internal/models"
)

// RangeShrinker provides a simplistic Bayesian model to shrink opponents' ranges
// based on their action history.
type RangeShrinker struct {
	// Pre-calculated tables or configs could go here later.
}

func NewRangeShrinker() *RangeShrinker {
	return &RangeShrinker{}
}

// ApplyActionHistory takes an initial range and sequentially applies likelihood penalties
// based on every action the player took, shrinking the range accordingly.
func (rs *RangeShrinker) ApplyActionHistory(initialRange Range, history []models.ActionNode, board []Card) Range {
	currentRange := initialRange

	// If no history, just normalize and return
	if len(history) == 0 {
		_ = currentRange.Normalize()
		return currentRange
	}

	for _, action := range history {
		rs.applyAction(&currentRange, action, board)
	}

	_ = currentRange.Normalize()
	return currentRange
}

func (rs *RangeShrinker) applyAction(rng *Range, action models.ActionNode, board []Card) {
	// Simplified likelihood multipliers based on action type
	// 0.0 means impossible, 1.0 means perfectly likely.

	// The likelihood function here is extremely naive for the MVP.
	// In a real solver, this would evaluate the hand equity against the board
	// and use an S-curve to determine likelihood of action.

	for i := 0; i < 1326; i++ {
		if rng[i] <= 0 {
			continue // Already ruled out
		}

		combo := GlobalCombos[i]

		// For preflop, we only look at hole cards strength
		// For postflop, we would ideally look at hand rank against board
		
		multiplier := 1.0

		// Extremely basic logic just to fulfill the architectural requirement of "shrinking"
		if action.Street == models.StreetPreflop {
			multiplier = rs.getPreflopLikelihood(combo, action.Action)
		} else {
			multiplier = rs.getPostflopLikelihood(combo, board, action.Action)
		}

		rng[i] *= multiplier
	}
}

func (rs *RangeShrinker) getPreflopLikelihood(combo HandCombo, action models.ActionType) float64 {
	r1 := combo.Card1.Rank
	r2 := combo.Card2.Rank
	isPair := r1 == r2
	isSuited := combo.Card1.Suit == combo.Card2.Suit

	// Basic heuristic score 1-100
	score := float64(r1+r2) * 2.5
	if isPair {
		score += 30
	}
	if isSuited {
		score += 10
	}

	switch action {
	case models.ActionFold:
		// People fold weak hands
		if score > 80 {
			return 0.01 // very unlikely to fold AA/KK
		}
		if score < 40 {
			return 1.0
		}
		return math.Max(0.1, 1.0-(score/100.0))
	case models.ActionCall:
		// People call with medium to strong hands
		if score < 40 {
			return 0.1 // unlikely to call 72o
		}
		if score > 90 {
			// Super premiums usually raise, but might trap
			return 0.5
		}
		return 1.0
	case models.ActionRaise, models.ActionBet, models.ActionAllIn:
		// People raise with strong hands or bluffs (bluffs not modeled well here)
		if score > 75 {
			return 1.0
		}
		if score < 30 {
			return 0.05
		}
		return (score / 100.0)
	case models.ActionCheck:
		// Usually weak/medium
		if score > 85 {
			return 0.2 // Trapping?
		}
		return 1.0
	default:
		return 1.0
	}
}

func (rs *RangeShrinker) getPostflopLikelihood(combo HandCombo, board []Card, action models.ActionType) float64 {
	if len(board) == 0 {
		return 1.0
	}

	// For MVP, we use the Evaluator to get a hand rank category 
	// which is 0 (High Card) to 8 (Straight Flush)
	seven := make([]Card, 0, len(board)+2)
	seven = append(seven, combo.Card1, combo.Card2)
	seven = append(seven, board...)

	// This is a bit slow to do 1326 times * steps, but fast enough for C++ -> Go port MVP.
	rank := EvaluateSeven(seven)
	category := rank.Category

	switch action {
	case models.ActionFold:
		if category >= 2 { // Two pair or better
			return 0.01 
		}
		if category == 1 { // Pair
			return 0.5
		}
		return 1.0 // High card
	case models.ActionCall:
		if category >= 1 {
			return 1.0
		}
		return 0.2 // Float/draw?
	case models.ActionRaise, models.ActionBet, models.ActionAllIn:
		if category >= 3 { // Trips or better
			return 1.0
		}
		if category >= 1 {
			return 0.4
		}
		return 0.1 // Bluff
	case models.ActionCheck:
		if category >= 6 { // Full house or better
			return 0.1 // Slow play
		}
		if category >= 3 {
			return 0.5
		}
		return 1.0
	default:
		return 1.0
	}
}
