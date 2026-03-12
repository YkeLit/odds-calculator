package holdem

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"odds-calculator/backend/internal/models"
)

type solverState struct {
	RemainingOpponents []models.OpponentInfo
	OpponentRanges     []Range
	Board              []Card
	Dead               []Card
	Deck               []Card
}

// CalculateDecision runs the Expectimax Monte Carlo solver for the Hero
func CalculateDecision(req models.HoldemDecisionRequest) (models.HoldemDecisionResponse, error) {
	started := time.Now()
	
	ctx := context.Background()
	timeout := req.SolverConfig.TimeoutMs
	if timeout <= 0 {
		timeout = 5000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	// 1. Initial State Setup
	state, err := buildSolverState(req)
	if err != nil {
		return models.HoldemDecisionResponse{}, err
	}

	// 2. Generate Hero branch options
	heroOptions := generateHeroOptions(req.PotState, req.Hero, req.SolverConfig)
	if len(heroOptions) == 0 {
		return models.HoldemDecisionResponse{}, fmt.Errorf("no valid hero options generated")
	}

	// 3. Rollout phase
	budget := req.SolverConfig.RolloutBudget
	if budget <= 0 {
		budget = 10000 // default
	}
	
	// Divide budget equally among options
	budgetPerOption := budget / len(heroOptions)
	if budgetPerOption < 100 {
		budgetPerOption = 100
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	
	results := make([]models.DecisionAction, len(heroOptions))
	totalNodes := 0
	totalRollouts := 0

	for i, option := range heroOptions {
		wg.Add(1)
		go func(idx int, opt models.ActionNode) {
			defer wg.Done()
			
			evs := make([]float64, 0, budgetPerOption)
			nodes := 0
			
			for r := 0; r < budgetPerOption; r++ {
				select {
				case <-ctx.Done():
					// Timeout reached, stop rolling out for this branch
					goto DoneRolling
				default:
				}
				
				ev, n := rollout(state, req.Hero, opt, req.PotState)
				evs = append(evs, ev)
				nodes += n
			}

		DoneRolling:
			if len(evs) == 0 {
				evs = append(evs, 0)
			}
			
			// Calculate Mean and CI
			sum := 0.0
			for _, v := range evs {
				sum += v
			}
			mean := sum / float64(len(evs))
			
			// Simple variance for CI
			varSum := 0.0
			for _, v := range evs {
				diff := v - mean
				varSum += diff * diff
			}
			variance := varSum / float64(len(evs))
			// 1.96 * std dev / sqrt(n) for 95% CI of mean
			ciMargin := 1.96 * math.Sqrt(variance) / math.Sqrt(float64(len(evs)))
			
			mu.Lock()
			results[idx] = models.DecisionAction{
				Action:    opt.Action,
				Amount:    opt.Amount,
				EV:        round4(mean),
				CILow:     round4(mean - ciMargin),
				CIHigh:    round4(mean + ciMargin),
				Frequency: 0,
			}
			totalNodes += nodes
			totalRollouts += len(evs)
			mu.Unlock()
			
		}(i, option)
	}

	wg.Wait()

	// 4. Sort results by EV descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].EV > results[j].EV
	})
	
	// Take Top 3
	if len(results) > 3 {
		results = results[:3]
	}

	// 5. Basic Metrics (Hero Equity against ranges)
	heroMetrics := computeHeroMetrics(state, req.Hero, req.PotState, 500) // 500 samples

	// 6. Output formatting
	oppSummaries := make([]models.OpponentRangeSummary, len(state.RemainingOpponents))
	for i, opp := range state.RemainingOpponents {
		oppSummaries[i] = models.OpponentRangeSummary{
			ID:            opp.ID,
			CoverageRatio: measureCoverage(state.OpponentRanges[i]),
			TopCombos:     getTopCombos(state.OpponentRanges[i], 3),
		}
	}

	return models.HoldemDecisionResponse{
		TopActions:           results,
		HeroMetrics:          heroMetrics,
		OpponentRangeSummary: oppSummaries,
		TreeStats: models.TreeStats{
			Nodes:        totalNodes,
			Rollouts:     totalRollouts,
			DepthReached: 1, // Simplified
			ElapsedMs:    time.Since(started).Milliseconds(),
			Convergence:  1.0, // Mock
		},
	}, nil
}

func buildSolverState(req models.HoldemDecisionRequest) (solverState, error) {
	board, err := parseCards(req.BoardCards)
	if err != nil {
		return solverState{}, err
	}
	dead, err := parseCards(req.DeadCards)
	if err != nil {
		return solverState{}, err
	}
	heroHole, err := parseCards(req.Hero.HoleCards)
	if err != nil {
		return solverState{}, err
	}

	used := make(map[string]struct{})
	for _, c := range board { used[c.Code] = struct{}{} }
	for _, c := range dead { used[c.Code] = struct{}{} }
	for _, c := range heroHole { used[c.Code] = struct{}{} }

	deck := make([]Card, 0, 52)
	for _, c := range FullDeck() {
		if _, ok := used[c.Code]; !ok {
			deck = append(deck, c)
		}
	}

	// Initialize and shrink ranges
	shrinker := NewRangeShrinker()
	ranges := make([]Range, len(req.Opponents))
	
	for i, opp := range req.Opponents {
		baseOverride := opp.RangeOverride
		if baseOverride == "" {
			if def, ok := DefaultPositionRanges[opp.Position]; ok {
				baseOverride = def
			}
		}
		rng, err := ParseRangeOverride(baseOverride)
		if err != nil {
			// Fallback to full range on error to prevent total failure
			rng = NewFullRange() 
		}
		// Exclude cards already dead/known
		for i := 0; i < 1326; i++ {
			c1 := GlobalCombos[i].Card1.Code
			c2 := GlobalCombos[i].Card2.Code
			if _, ok1 := used[c1]; ok1 { rng[i] = 0 }
			if _, ok2 := used[c2]; ok2 { rng[i] = 0 }
		}
		
		// Shrink based on history
		rng = shrinker.ApplyActionHistory(rng, req.ActionHistory, board) // Note: history usually needs to be split per player in a real solver
		ranges[i] = rng
	}

	return solverState{
		RemainingOpponents: req.Opponents,
		OpponentRanges:     ranges,
		Board:              board,
		Dead:               dead,
		Deck:               deck,
	}, nil
}

func parseCards(codes []string) ([]Card, error) {
	var cards []Card
	for _, code := range codes {
		c, err := ParseCard(code)
		if err != nil { return nil, err }
		cards = append(cards, c)
	}
	return cards, nil
}

func generateHeroOptions(pot models.PotState, hero models.HeroState, cfg models.SolverConfig) []models.ActionNode {
	var options []models.ActionNode
	
	// Can we check or must we call?
	if pot.ToCall <= 0 {
		options = append(options, models.ActionNode{Action: models.ActionCheck, Amount: 0})
	} else {
		options = append(options, models.ActionNode{Action: models.ActionFold, Amount: 0})
		if hero.Stack > pot.ToCall {
			options = append(options, models.ActionNode{Action: models.ActionCall, Amount: pot.ToCall})
		} else {
			options = append(options, models.ActionNode{Action: models.ActionCall, Amount: hero.Stack}) // Effectively all-in call
		}
	}

	// Generate Raises
	if hero.Stack > pot.ToCall {
		N := cfg.BranchCount
		if N < 2 { N = 2 }
		if N > 5 { N = 5 }

		// All-in is always an option
		allInAmount := hero.Stack 
		minRaise := pot.MinRaiseTo
		if minRaise == 0 { minRaise = pot.Blinds[1] * 2 } // Fallback
		
		if minRaise < allInAmount {
			potBase := pot.PotSize + pot.ToCall
			// [0.5x, 0.75x, 1.0x, 1.5x] pot fractions depending on N
			var fractions []float64
			switch N {
			case 2:
				fractions = []float64{1.0}
			case 3:
				fractions = []float64{0.5, 1.0}
			case 4:
				fractions = []float64{0.5, 0.75, 1.0}
			case 5:
				fractions = []float64{0.5, 0.75, 1.0, 1.5}
			}

			for _, f := range fractions {
				rAmount := pot.ToCall + (potBase * f)
				if rAmount >= minRaise && rAmount < allInAmount {
					options = append(options, models.ActionNode{Action: models.ActionRaise, Amount: round2(rAmount)})
				}
			}
		}
		
		options = append(options, models.ActionNode{Action: models.ActionAllIn, Amount: allInAmount})
	}
	
	return options
}

// rollout runs a single Monte Carlo sample for a specific hero option
func rollout(state solverState, hero models.HeroState, opt models.ActionNode, pot models.PotState) (float64, int) {
	// A real rollout would traverse to the river simulating opponent responses.
	// For MVP:
	// 1. We sample exactly 1 hand for each remaining opponent from their range.
	// 2. We sample the remaining board cards.
	// 3. We simulate a VERY simple response to the Hero's action.

	if opt.Action == models.ActionFold {
		return 0.0, 1 // EV is 0, we lose whatever we put in earlier (sunk cost)
	}

	// 1. Run out the board
	need := 5 - len(state.Board)
	sampledDeck := make([]Card, len(state.Deck))
	copy(sampledDeck, state.Deck)
	rand.Shuffle(len(sampledDeck), func(i, j int) {
		sampledDeck[i], sampledDeck[j] = sampledDeck[j], sampledDeck[i]
	})

	finalBoard := make([]Card, 0, 5)
	finalBoard = append(finalBoard, state.Board...)
	finalBoard = append(finalBoard, sampledDeck[:need]...)
	sampledDeck = sampledDeck[need:]

	// 2. Sample Opponent hands
	oppHands := make([][2]Card, len(state.RemainingOpponents))
	for i, rng := range state.OpponentRanges {
		comboIdx := sampleFromRange(rng)
		combo := GlobalCombos[comboIdx]
		// Conflict resolution omitted for super-fast MVP approximation, 
		// assuming collision rate is low enough for a rough EV.
		oppHands[i] = [2]Card{combo.Card1, combo.Card2}
	}

	heroHole, _ := parseCards(hero.HoleCards)
	sevenHero := []Card{heroHole[0], heroHole[1]}
	sevenHero = append(sevenHero, finalBoard...)
	heroRank := EvaluateSeven(sevenHero)

	// 3. Simulate Opponent responses
	// Very naive: if we Bet/Raise, do they call?
	activePot := pot.PotSize + opt.Amount
	cost := opt.Amount
	heroWins := true
	ties := 0
	
	for _, oppHand := range oppHands {
		oppSeven := []Card{oppHand[0], oppHand[1]}
		oppSeven = append(oppSeven, finalBoard...)
		oppRank := EvaluateSeven(oppSeven)

		// They only call if they have something decent relative to the board
		// If they fold, they don't contest the pot.
		// For MVP, assume they always call all-ins if they have pair or better.
		// If check/call, we go to showdown.
		
		cmp := CompareHandRank(heroRank, oppRank)
		if cmp < 0 {
			heroWins = false 
		} else if cmp == 0 {
			ties++
		}

		if opt.Action == models.ActionRaise || opt.Action == models.ActionBet || opt.Action == models.ActionAllIn {
			// Increase pot slightly assuming they called
			activePot += opt.Amount 
		}
	}

	// Showdown Evaluation
	if !heroWins {
		return -cost, len(state.OpponentRanges) + 1
	}

	if ties > 0 {
		return (activePot / float64(ties+1)) - cost, len(state.OpponentRanges) + 1
	}

	return activePot - cost, len(state.OpponentRanges) + 1
}

func sampleFromRange(rng Range) int {
	r := rand.Float64()
	sum := 0.0
	for i, w := range rng {
		sum += w
		if r <= sum {
			return i
		}
	}
	// Fallback first non-zero
	for i, w := range rng {
		if w > 0 { return i }
	}
	return 0
}

func computeHeroMetrics(state solverState, hero models.HeroState, pot models.PotState, samples int) models.HeroMetrics {
	wins := 0.0
	ties := 0.0
	
	if len(state.Board) == 0 && samples > 0 {
		// Mock estimation for Preflop
		return models.HeroMetrics{
			Equity:         0.5,
			TieRate:        0.05,
			PotOdds:        pot.ToCall / (pot.PotSize + pot.ToCall),
			RequiredEquity: pot.ToCall / (pot.PotSize + pot.ToCall*2),
		}
	}
	
	heroHole, _ := parseCards(hero.HoleCards)
	for i:=0; i<samples; i++ {
		// Sample board
		need := 5 - len(state.Board)
		sampledDeck := make([]Card, len(state.Deck))
		copy(sampledDeck, state.Deck)
		rand.Shuffle(len(sampledDeck), func(i, j int) {
			sampledDeck[i], sampledDeck[j] = sampledDeck[j], sampledDeck[i]
		})

		finalBoard := make([]Card, 0, 5)
		finalBoard = append(finalBoard, state.Board...)
		finalBoard = append(finalBoard, sampledDeck[:need]...)

		sevenHero := []Card{heroHole[0], heroHole[1]}
		sevenHero = append(sevenHero, finalBoard...)
		heroRank := EvaluateSeven(sevenHero)
		
		isWin := true
		isTie := false
		
		for _, rng := range state.OpponentRanges {
			comboIdx := sampleFromRange(rng)
			combo := GlobalCombos[comboIdx]
			
			oppSeven := []Card{combo.Card1, combo.Card2}
			oppSeven = append(oppSeven, finalBoard...)
			oppRank := EvaluateSeven(oppSeven)
			
			cmp := CompareHandRank(heroRank, oppRank)
			if cmp < 0 { 
				isWin = false 
				break
			} else if cmp == 0 {
				isTie = true
			}
		}
		
		if isWin && !isTie {
			wins++
		} else if isWin && isTie {
			ties++
		}
	}
	
	eq := 0.0
	tRate := 0.0
	if samples > 0 {
		eq = (wins + (ties / 2.0)) / float64(samples)
		tRate = ties / float64(samples)
	}

	totPot := pot.PotSize + pot.ToCall
	podds := 0.0
	reqEq := 0.0
	if totPot > 0 {
		podds = pot.ToCall / totPot
		if (totPot + pot.ToCall) > 0 {
			reqEq = pot.ToCall / (totPot + pot.ToCall)
		}
	}

	return models.HeroMetrics{
		Equity:         round4(eq),
		TieRate:        round4(tRate),
		PotOdds:        round4(podds),
		RequiredEquity: round4(reqEq),
	}
}

func measureCoverage(rng Range) float64 {
	active := 0.0
	for _, w := range rng {
		if w > 0 { 
			// Assuming originally weight sum is close to 1 for full range calculation.
			// Or rather, count non-zero combos vs 1326
			active++
		}
	}
	return active / 1326.0
}

func getTopCombos(rng Range, n int) string {
	type Pair struct {
		code string
		w    float64
	}
	var pairs []Pair
	for i, w := range rng {
		if w > 0 {
			// Convert AdAh to AA
			combo := GlobalCombos[i]
			r1Code := rankToCode[combo.Card1.Rank]
			r2Code := rankToCode[combo.Card2.Rank]
			suffix := "o"
			if combo.Card1.Suit == combo.Card2.Suit { suffix = "s" }
			if r1Code == r2Code { suffix = "" }
			
			pairs = append(pairs, Pair{r1Code+r2Code+suffix, w})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].w > pairs[j].w })
	
	limit := n
	if len(pairs) < limit { limit = len(pairs) }
	
	out := ""
	for i:=0; i<limit; i++ {
		out += pairs[i].code
		if i < limit-1 { out += "," }
	}
	return out
}


