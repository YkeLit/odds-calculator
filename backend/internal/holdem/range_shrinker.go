package holdem

import (
	"math"

	"odds-calculator/backend/internal/models"
)

// RangeShrinker provides a Bayesian model to shrink opponents' ranges
// based on their action history. When MCCFR InfoSet data is available,
// it uses the converged strategy frequencies as the likelihood function
// (theoretically correct). Falls back to heuristic likelihood otherwise.
type RangeShrinker struct {
	infoSets *InfoSetMap // MCCFR global cache (may be nil)
	board    []Card
}

func NewRangeShrinker() *RangeShrinker {
	return &RangeShrinker{}
}

// NewRangeShrinkerWithInfoSets creates a shrinker backed by MCCFR data.
// This enables theoretically correct Bayesian updates:
//
//	P(combo | action_seq) ∝ P(combo) × Π P_σ(a_t | combo, h_t)
//
// where P_σ is the MCCFR average strategy at that information set.
func NewRangeShrinkerWithInfoSets(infoSets *InfoSetMap, board []Card) *RangeShrinker {
	return &RangeShrinker{infoSets: infoSets, board: board}
}

// ApplyActionHistory takes an initial range and sequentially applies
// Bayesian likelihood updates for each action the opponent took.
//
// For each combo i and each action a_t in the history:
//
//	P_new(combo_i) = P_old(combo_i) × L(a_t | combo_i, history_before_t)
//
// Then normalize so the posterior sums to 1.
func (rs *RangeShrinker) ApplyActionHistory(initialRange Range, history []models.ActionNode, board []Card) Range {
	currentRange := initialRange

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

// actionHistoryPrefix converts a slice of ActionNodes into the MCCFR history
// string format used for InfoSet keys (e.g. "fkcrba").
func actionHistoryPrefix(history []models.ActionNode) string {
	s := ""
	for _, h := range history {
		s += actionToChar(h.Action)
	}
	return s
}

func (rs *RangeShrinker) applyAction(rng *Range, action models.ActionNode, board []Card) {
	for i := 0; i < 1326; i++ {
		if rng[i] <= 0 {
			continue
		}

		combo := GlobalCombos[i]

		// Try MCCFR-informed likelihood first
		likelihood := rs.getMCCFRLikelihood(combo, board, action)
		if likelihood < 0 {
			// No MCCFR data available; fall back to heuristic
			if action.Street == models.StreetPreflop {
				likelihood = rs.getPreflopLikelihood(combo, action.Action)
			} else {
				likelihood = rs.getPostflopLikelihood(combo, board, action.Action)
			}
		}

		rng[i] *= likelihood
	}
}

// getMCCFRLikelihood queries the MCCFR InfoSet cache for the opponent's
// strategy at the information set defined by (combo, board, historyPrefix).
// Returns the probability that the opponent would take `action` given `combo`,
// or -1 if no MCCFR data is available (signals caller to use heuristic).
func (rs *RangeShrinker) getMCCFRLikelihood(combo HandCombo, board []Card, action models.ActionNode) float64 {
	if rs.infoSets == nil {
		return -1
	}

	// Build the InfoSet key for this opponent holding this combo
	hole := [2]Card{combo.Card1, combo.Card2}
	// Canonicalize: sorted by code so lookup is consistent with makeInfoSetKey
	h1, h2 := hole[0].Code, hole[1].Code
	if h1 > h2 {
		h1, h2 = h2, h1
	}
	key := InfoSetKey(h1 + h2 + "|")
	for _, b := range board {
		key += InfoSetKey(b.Code)
	}
	key += "|" // history separator; we check the root node first

	// Look up in the cache
	rs.infoSets.mu.RLock()
	data, found := rs.infoSets.data[key]
	rs.infoSets.mu.RUnlock()

	if !found || data.ActionCount == 0 {
		return -1
	}

	// Map the action to the MCCFR action index
	// The action list at this node depends on toCall > 0 or == 0
	// We reconstruct the same list to find the index
	avgStrategy := data.GetAverageStrategy()

	// Determine action order (must match nodeActions logic)
	// We don't know the exact toCall, but we can infer from the action set:
	// If fold is in the strategy, then toCall > 0 → [fold, call, raise, allin]
	// If check is in the strategy, then toCall == 0 → [check, bet, allin]
	actionIdx := rs.findActionIndex(data.ActionCount, action.Action)
	if actionIdx < 0 || actionIdx >= len(avgStrategy) {
		return -1
	}

	prob := avgStrategy[actionIdx]

	// Clamp: never fully zero out a combo (allow minimal exploration)
	if prob < 0.001 {
		prob = 0.001
	}
	return prob
}

// findActionIndex maps an ActionType to the index in the MCCFR action array.
// nodeActions produces either:
//   - toCall <= 0: [check, bet, allin]              (3 actions)
//   - toCall > 0:  [fold, call, raise, allin]        (4 actions)
//
// We infer the action set from the count.
func (rs *RangeShrinker) findActionIndex(actionCount int, action models.ActionType) int {
	if actionCount <= 3 {
		// toCall == 0: [check, bet, allin]
		switch action {
		case models.ActionCheck:
			return 0
		case models.ActionBet:
			return 1
		case models.ActionAllIn:
			return 2
		default:
			return -1
		}
	}
	// toCall > 0: [fold, call, raise, allin]
	switch action {
	case models.ActionFold:
		return 0
	case models.ActionCall:
		return 1
	case models.ActionRaise:
		return 2
	case models.ActionAllIn:
		return 3
	default:
		return -1
	}
}

// ──── Heuristic fallbacks (used when no MCCFR data available) ────

func (rs *RangeShrinker) getPreflopLikelihood(combo HandCombo, action models.ActionType) float64 {
	r1 := combo.Card1.Rank
	r2 := combo.Card2.Rank
	isPair := r1 == r2
	isSuited := combo.Card1.Suit == combo.Card2.Suit

	score := float64(r1+r2) * 2.5
	if isPair {
		score += 30
	}
	if isSuited {
		score += 10
	}

	switch action {
	case models.ActionFold:
		if score > 80 {
			return 0.01
		}
		if score < 40 {
			return 1.0
		}
		return math.Max(0.1, 1.0-(score/100.0))
	case models.ActionCall:
		if score < 40 {
			return 0.1
		}
		if score > 90 {
			return 0.5
		}
		return 1.0
	case models.ActionRaise, models.ActionBet, models.ActionAllIn:
		if score > 75 {
			return 1.0
		}
		if score < 30 {
			return 0.05
		}
		return (score / 100.0)
	case models.ActionCheck:
		if score > 85 {
			return 0.2
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

	seven := make([]Card, 0, len(board)+2)
	seven = append(seven, combo.Card1, combo.Card2)
	seven = append(seven, board...)
	rank := EvaluateSeven(seven)
	category := rank.Category

	switch action {
	case models.ActionFold:
		if category >= 2 {
			return 0.01
		}
		if category == 1 {
			return 0.5
		}
		return 1.0
	case models.ActionCall:
		if category >= 1 {
			return 1.0
		}
		return 0.2
	case models.ActionRaise, models.ActionBet, models.ActionAllIn:
		if category >= 3 {
			return 1.0
		}
		if category >= 1 {
			return 0.4
		}
		return 0.1
	case models.ActionCheck:
		if category >= 6 {
			return 0.1
		}
		if category >= 3 {
			return 0.5
		}
		return 1.0
	default:
		return 1.0
	}
}
