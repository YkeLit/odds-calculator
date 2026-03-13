package holdem

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
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

// CalculateDecision runs the MCCFR-based solver for the Hero.
//
// The algorithm:
//  1. Build game state (parse cards, shrink opponent ranges).
//  2. Run multiple MCCFR iterations in parallel batches.
//     Each iteration samples opponent hands + remaining board once,
//     then traverses the game tree with external-sampling CFR.
//  3. Extract the converged average strategy at the hero's root
//     information set as the recommended mixed strategy.
//  4. Map each action to an EV estimate via additional rollouts.
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

	heroHole, err := parseCards(req.Hero.HoleCards)
	if err != nil {
		return models.HoldemDecisionResponse{}, err
	}
	if len(heroHole) < 2 {
		return models.HoldemDecisionResponse{}, fmt.Errorf("hero must have 2 hole cards")
	}

	// Build position list: hero first, then opponents
	positions := []string{req.Hero.Position}
	stacks := []float64{req.Hero.Stack}
	for _, opp := range req.Opponents {
		positions = append(positions, opp.Position)
		// Use hero stack as default if opponent stack not tracked
		stacks = append(stacks, req.Hero.Stack)
	}

	// 2. MCCFR iterations
	iterations := req.SolverConfig.RolloutBudget
	if iterations <= 0 {
		iterations = 10000
	}

	// Run parallel MCCFR workers
	// MCCFR_WORKERS 环境变量可在运行时调整并行度，默认 4
	numWorkers := 4
	if wStr := os.Getenv("MCCFR_WORKERS"); wStr != "" {
		if w, err := strconv.Atoi(wStr); err == nil && w > 0 {
			numWorkers = w
		}
	}
	iterPerWorker := iterations / numWorkers
	if iterPerWorker < 100 {
		iterPerWorker = 100
	}

	solvers := make([]*MCCFRSolver, numWorkers)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		solvers[w] = NewMCCFRSolver(
			[2]Card{heroHole[0], heroHole[1]},
			positions,
			state.OpponentRanges,
			state.Board,
			state.Dead,
			req.PotState,
			stacks,
			iterPerWorker,
		)
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			solver := solvers[idx]
			for i := 0; i < solver.Iterations; i++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				// ES-CFR+ alternating updates:
				// each player takes a turn as traverser per iteration
				oppHoles, deck := solver.sampleChance()
				for traverser := 0; traverser < solver.PlayerCount; traverser++ {
					root := solver.makeRoot(oppHoles, deck)
					solver.cfrTraverse(root, traverser, 1.0, i)
				}
			}
		}(w)
	}
	wg.Wait()

	// 3. Merge info sets from all workers
	merged := NewInfoSetMap()
	for _, solver := range solvers {
		merged.Merge(solver.InfoSets)
	}

	// 3b. Accumulate into the global persistent cache and read back,
	// so repeated calls for the same situation converge more tightly.
	globalCacheMu.Lock()
	globalCache.Merge(merged) // push new iterations in
	if globalCache.Size() > maxGlobalCacheSize {
		// Simple eviction: reset when limit exceeded to avoid unbounded growth
		globalCache = NewInfoSetMap()
		globalCache.Merge(merged)
	}
	// Build a request-local view by starting from the accumulated global state
	accumulated := NewInfoSetMap()
	accumulated.Merge(globalCache)
	globalCacheMu.Unlock()

	// 4. Extract hero root strategy
	rootSolver := solvers[0]
	rootActions := rootSolver.nodeActions(req.PotState.ToCall, req.Hero.Stack)
	rootKey := rootSolver.makeInfoSetKey(
		[2]Card{heroHole[0], heroHole[1]},
		state.Board,
		"",
	)
	rootData := accumulated.GetOrCreate(rootKey, len(rootActions))
	avgStrategy := rootData.GetAverageStrategy()

	// 5. Estimate EV per action via rollouts
	evSamples := 2000
	results := make([]models.DecisionAction, len(rootActions))

	for i, act := range rootActions {
		opt := actionToNode(act, req.PotState, req.Hero)
		evs := make([]float64, 0, evSamples)
		for s := 0; s < evSamples; s++ {
			ev, _ := rollout(state, req.Hero, opt, req.PotState)
			evs = append(evs, ev)
		}
		mean, ciLow, ciHigh := computeStats(evs)
		results[i] = models.DecisionAction{
			Action:    act,
			Amount:    opt.Amount,
			EV:        round4(mean),
			CILow:     round4(ciLow),
			CIHigh:    round4(ciHigh),
			Frequency: round4(avgStrategy[i]),
			Regret:    round4(rootData.CumulativeRegret[i]),
		}
	}

	// 6. Sort by MCCFR frequency (primary), EV (secondary)
	sort.Slice(results, func(i, j int) bool {
		if math.Abs(results[i].Frequency-results[j].Frequency) > 0.01 {
			return results[i].Frequency > results[j].Frequency
		}
		return results[i].EV > results[j].EV
	})

	// Take Top 3
	if len(results) > 3 {
		results = results[:3]
	}

	// 7. Hero Metrics
	heroMetrics := computeHeroMetrics(state, req.Hero, req.PotState, 500)

	// 8. Opponent range summaries
	oppSummaries := make([]models.OpponentRangeSummary, len(state.RemainingOpponents))
	for i, opp := range state.RemainingOpponents {
		oppSummaries[i] = models.OpponentRangeSummary{
			ID:            opp.ID,
			CoverageRatio: measureCoverage(state.OpponentRanges[i]),
			TopCombos:     getTopCombos(state.OpponentRanges[i], 3),
		}
	}

	totalIterations := numWorkers * iterPerWorker

	// Compute convergence metric from regret magnitudes
	convergence := computeConvergence(rootData)

	return models.HoldemDecisionResponse{
		TopActions:           results,
		HeroMetrics:          heroMetrics,
		OpponentRangeSummary: oppSummaries,
		TreeStats: models.TreeStats{
			Nodes:        accumulated.Size(),
			Rollouts:     totalIterations,
			DepthReached: depthFromStreet(req.Street),
			ElapsedMs:    time.Since(started).Milliseconds(),
			Convergence:  convergence,
		},
	}, nil
}

func actionToNode(act models.ActionType, pot models.PotState, hero models.HeroState) models.ActionNode {
	switch act {
	case models.ActionFold:
		return models.ActionNode{Action: models.ActionFold, Amount: 0}
	case models.ActionCheck:
		return models.ActionNode{Action: models.ActionCheck, Amount: 0}
	case models.ActionCall:
		return models.ActionNode{Action: models.ActionCall, Amount: math.Min(pot.ToCall, hero.Stack)}
	case models.ActionBet:
		betAmt := math.Min(pot.PotSize*0.66, hero.Stack)
		if betAmt < pot.Blinds[1] {
			betAmt = math.Min(pot.Blinds[1], hero.Stack)
		}
		return models.ActionNode{Action: models.ActionBet, Amount: round2(betAmt)}
	case models.ActionRaise:
		raiseAmt := math.Min(pot.MinRaiseTo, hero.Stack)
		if raiseAmt <= pot.ToCall {
			raiseAmt = math.Min(pot.ToCall+pot.Blinds[1], hero.Stack)
		}
		return models.ActionNode{Action: models.ActionRaise, Amount: round2(raiseAmt)}
	case models.ActionAllIn:
		return models.ActionNode{Action: models.ActionAllIn, Amount: hero.Stack}
	}
	return models.ActionNode{Action: act, Amount: 0}
}

func computeConvergence(data *InfoSetData) float64 {
	if data == nil || data.ActionCount == 0 {
		return 0
	}
	// Convergence proxy: 1 - (max regret / total regret magnitude)
	maxReg := 0.0
	totalReg := 0.0
	for _, r := range data.CumulativeRegret {
		absR := math.Abs(r)
		totalReg += absR
		if absR > maxReg {
			maxReg = absR
		}
	}
	if totalReg == 0 {
		return 1.0
	}
	return round4(1.0 - maxReg/totalReg)
}

func depthFromStreet(s models.Street) int {
	switch s {
	case models.StreetPreflop:
		return 4
	case models.StreetFlop:
		return 3
	case models.StreetTurn:
		return 2
	case models.StreetRiver:
		return 1
	}
	return 1
}

func computeStats(evs []float64) (mean, ciLow, ciHigh float64) {
	if len(evs) == 0 {
		return 0, 0, 0
	}
	sum := 0.0
	for _, v := range evs {
		sum += v
	}
	mean = sum / float64(len(evs))

	varSum := 0.0
	for _, v := range evs {
		d := v - mean
		varSum += d * d
	}
	variance := varSum / float64(len(evs))
	ciMargin := 1.96 * math.Sqrt(variance) / math.Sqrt(float64(len(evs)))
	ciLow = mean - ciMargin
	ciHigh = mean + ciMargin
	return
}

func buildSolverState(req models.HoldemDecisionRequest) (solverState, error) {
	board, err := parseCards(req.BoardCards)
	if err != nil {
		return solverState{}, err
	}
	requiredBoardCount := 0
	switch req.Street {
	case models.StreetFlop:
		requiredBoardCount = 3
	case models.StreetTurn:
		requiredBoardCount = 4
	case models.StreetRiver:
		requiredBoardCount = 5
	}
	if len(board) < requiredBoardCount {
		return solverState{}, fmt.Errorf("street %s requires at least %d board cards, got %d", req.Street, requiredBoardCount, len(board))
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
	for _, c := range board {
		used[c.Code] = struct{}{}
	}
	for _, c := range dead {
		used[c.Code] = struct{}{}
	}
	for _, c := range heroHole {
		used[c.Code] = struct{}{}
	}

	deck := make([]Card, 0, 52)
	for _, c := range FullDeck() {
		if _, ok := used[c.Code]; !ok {
			deck = append(deck, c)
		}
	}

	// Initialize and shrink ranges (using MCCFR-informed Bayesian updates if cache available)
	globalCacheMu.Lock()
	cachedInfoSets := NewInfoSetMap()
	cachedInfoSets.Merge(globalCache)
	globalCacheMu.Unlock()

	shrinker := NewRangeShrinkerWithInfoSets(cachedInfoSets, board)
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
			if _, ok1 := used[c1]; ok1 {
				rng[i] = 0
			}
			if _, ok2 := used[c2]; ok2 {
				rng[i] = 0
			}
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
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, nil
}

// rollout runs a single Monte Carlo sample for EV estimation of a specific hero action.
// Used after MCCFR to attach EV values to the converged strategy.
func rollout(state solverState, hero models.HeroState, opt models.ActionNode, pot models.PotState) (float64, int) {
	if opt.Action == models.ActionFold {
		return 0.0, 1
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

	// 2. Sample Opponent hands with conflict detection
	used := make(map[string]struct{})
	heroHole, _ := parseCards(hero.HoleCards)
	used[heroHole[0].Code] = struct{}{}
	used[heroHole[1].Code] = struct{}{}
	for _, b := range finalBoard {
		used[b.Code] = struct{}{}
	}
	for _, d := range state.Dead {
		used[d.Code] = struct{}{}
	}

	oppHands := make([][2]Card, len(state.RemainingOpponents))
	for i, rng := range state.OpponentRanges {
		for attempts := 0; attempts < 200; attempts++ {
			comboIdx := sampleFromRange(rng)
			combo := GlobalCombos[comboIdx]
			c1, c2 := combo.Card1.Code, combo.Card2.Code
			if _, ok := used[c1]; ok {
				continue
			}
			if _, ok := used[c2]; ok {
				continue
			}
			oppHands[i] = [2]Card{combo.Card1, combo.Card2}
			used[c1] = struct{}{}
			used[c2] = struct{}{}
			break
		}
	}

	sevenHero := []Card{heroHole[0], heroHole[1]}
	sevenHero = append(sevenHero, finalBoard...)
	heroRank := EvaluateSeven(sevenHero)

	// 3. EV calculation
	cost := opt.Amount
	activePot := pot.PotSize + cost
	heroWins := true
	ties := 0

	for idx, oppHand := range oppHands {
		if oppHand[0].Code == "" {
			continue // failed to sample
		}
		oppSeven := []Card{oppHand[0], oppHand[1]}
		oppSeven = append(oppSeven, finalBoard...)
		oppRank := EvaluateSeven(oppSeven)

		cmp := CompareHandRank(heroRank, oppRank)
		if cmp < 0 {
			heroWins = false
		} else if cmp == 0 {
			ties++
		}

		// Estimate opponent contribution: call if they have >= median hand
		if opt.Action == models.ActionRaise || opt.Action == models.ActionBet || opt.Action == models.ActionAllIn {
			if oppRank.Category >= 1 { // pair or better → calls
				activePot += math.Min(cost, 100) // simplified
			}
		}
		_ = idx
	}

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
		if w > 0 {
			return i
		}
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
	for i := 0; i < samples; i++ {
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
			if combo.Card1.Suit == combo.Card2.Suit {
				suffix = "s"
			}
			if r1Code == r2Code {
				suffix = ""
			}

			pairs = append(pairs, Pair{r1Code + r2Code + suffix, w})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].w > pairs[j].w })

	limit := n
	if len(pairs) < limit {
		limit = len(pairs)
	}

	out := ""
	for i := 0; i < limit; i++ {
		out += pairs[i].code
		if i < limit-1 {
			out += ","
		}
	}
	return out
}
