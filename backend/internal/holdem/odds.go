package holdem

import (
	"fmt"
	"math"
	"sort"
	"time"

	"odds-calculator/backend/internal/models"
)

type parsedPlayer struct {
	ID           string
	HoleCards    [2]Card
	Contribution float64
	AllIn        bool
}

type sidePotInternal struct {
	Name          string
	Amount        float64
	PreRakeAmount float64
	Eligible      []int
}

func CalculateOdds(req models.HoldemOddsRequest) (models.HoldemOddsResponse, error) {
	started := time.Now()
	players, board, dead, deck, err := parseCommonState(req.Players, req.BoardCards, req.DeadCards)
	if err != nil {
		return models.HoldemOddsResponse{}, err
	}
	if len(players) < 2 || len(players) > 6 {
		return models.HoldemOddsResponse{}, fmt.Errorf("players must be in range [2,6]")
	}
	_ = dead

	need := 5 - len(board)
	if need < 0 {
		return models.HoldemOddsResponse{}, fmt.Errorf("board cards cannot exceed 5")
	}
	if need > len(deck) {
		return models.HoldemOddsResponse{}, fmt.Errorf("not enough cards to complete board")
	}

	wins := make([]float64, len(players))
	ties := make([]float64, len(players))
	equities := make([]float64, len(players))
	combos := 0

	enumerateCompletions(deck, need, func(completion []Card) {
		finalBoard := make([]Card, 0, 5)
		finalBoard = append(finalBoard, board...)
		finalBoard = append(finalBoard, completion...)

		ranks := make([]HandRank, len(players))
		best := HandRank{Category: -1}
		winners := make([]int, 0, len(players))
		for i, p := range players {
			seven := []Card{p.HoleCards[0], p.HoleCards[1]}
			seven = append(seven, finalBoard...)
			ranks[i] = EvaluateSeven(seven)
			if best.Category == -1 {
				best = ranks[i]
				winners = []int{i}
				continue
			}
			cmp := CompareHandRank(ranks[i], best)
			if cmp > 0 {
				best = ranks[i]
				winners = []int{i}
			} else if cmp == 0 {
				winners = append(winners, i)
			}
		}

		if len(winners) == 1 {
			wins[winners[0]]++
		} else {
			for _, idx := range winners {
				ties[idx]++
			}
		}
		share := 1.0 / float64(len(winners))
		for _, idx := range winners {
			equities[idx] += share
		}
		combos++
	})

	if combos == 0 {
		return models.HoldemOddsResponse{}, fmt.Errorf("no board combinations generated")
	}

	results := make([]models.HoldemOddsPlayerResult, 0, len(players))
	for i, p := range players {
		results = append(results, models.HoldemOddsPlayerResult{
			ID:      p.ID,
			WinRate: wins[i] / float64(combos),
			TieRate: ties[i] / float64(combos),
			Equity:  equities[i] / float64(combos),
		})
	}

	return models.HoldemOddsResponse{
		Results:         results,
		CombosEvaluated: combos,
		ElapsedMs:       time.Since(started).Milliseconds(),
	}, nil
}

func CalculateAllInEV(req models.HoldemAllInEVRequest) (models.HoldemAllInEVResponse, error) {
	started := time.Now()
	players, board, _, deck, err := parseCommonState(req.Players, req.BoardCards, req.DeadCards)
	if err != nil {
		return models.HoldemAllInEVResponse{}, err
	}
	if len(players) < 2 || len(players) > 6 {
		return models.HoldemAllInEVResponse{}, fmt.Errorf("players must be in range [2,6]")
	}

	contribByID := make(map[string]float64, len(players))
	for _, p := range players {
		isAllIn := p.AllIn
		if req.AllInFlags != nil {
			if flag, ok := req.AllInFlags[p.ID]; ok {
				isAllIn = flag
			}
		}
		if !isAllIn {
			return models.HoldemAllInEVResponse{}, fmt.Errorf("player %s is not all-in; MVP endpoint only supports all-in decisions", p.ID)
		}

		contrib := p.Contribution
		if req.Contributions != nil {
			if v, ok := req.Contributions[p.ID]; ok {
				contrib = v
			}
		}
		if contrib < 0 {
			return models.HoldemAllInEVResponse{}, fmt.Errorf("negative contribution for player %s", p.ID)
		}
		contribByID[p.ID] = contrib
	}

	pots, totalPot := buildSidePots(players, contribByID)
	if totalPot <= 0 {
		return models.HoldemAllInEVResponse{}, fmt.Errorf("total pot must be positive")
	}
	appliedRake := applyRake(pots, req.RakeConfig)

	need := 5 - len(board)
	if need < 0 {
		return models.HoldemAllInEVResponse{}, fmt.Errorf("board cards cannot exceed 5")
	}
	if need > len(deck) {
		return models.HoldemAllInEVResponse{}, fmt.Errorf("not enough cards to complete board")
	}

	payoutTotals := make(map[string]float64, len(players))
	combos := 0
	enumerateCompletions(deck, need, func(completion []Card) {
		finalBoard := make([]Card, 0, 5)
		finalBoard = append(finalBoard, board...)
		finalBoard = append(finalBoard, completion...)

		ranks := make([]HandRank, len(players))
		for i, p := range players {
			seven := []Card{p.HoleCards[0], p.HoleCards[1]}
			seven = append(seven, finalBoard...)
			ranks[i] = EvaluateSeven(seven)
		}

		for _, pot := range pots {
			if pot.Amount == 0 || len(pot.Eligible) == 0 {
				continue
			}
			best := HandRank{Category: -1}
			winners := make([]int, 0, len(pot.Eligible))
			for _, idx := range pot.Eligible {
				if best.Category == -1 {
					best = ranks[idx]
					winners = []int{idx}
					continue
				}
				cmp := CompareHandRank(ranks[idx], best)
				if cmp > 0 {
					best = ranks[idx]
					winners = []int{idx}
				} else if cmp == 0 {
					winners = append(winners, idx)
				}
			}
			if len(winners) == 0 {
				continue
			}
			share := pot.Amount / float64(len(winners))
			for _, idx := range winners {
				payoutTotals[players[idx].ID] += share
			}
		}
		combos++
	})

	if combos == 0 {
		return models.HoldemAllInEVResponse{}, fmt.Errorf("no board combinations generated")
	}

	potBreakdown := make([]models.SidePot, 0, len(pots))
	for _, pot := range pots {
		ids := make([]string, 0, len(pot.Eligible))
		for _, idx := range pot.Eligible {
			ids = append(ids, players[idx].ID)
		}
		potBreakdown = append(potBreakdown, models.SidePot{
			Name:              pot.Name,
			Amount:            round2(pot.Amount),
			PreRakeAmount:     round2(pot.PreRakeAmount),
			EligiblePlayerIDs: ids,
		})
	}

	playerResults := make([]models.HoldemAllInPlayerEV, 0, len(players))
	afterRake := make(map[string]float64, len(players))
	for _, p := range players {
		expectedPayout := payoutTotals[p.ID] / float64(combos)
		afterRake[p.ID] = round4(expectedPayout)
		contrib := contribByID[p.ID]
		playerResults = append(playerResults, models.HoldemAllInPlayerEV{
			ID:             p.ID,
			ExpectedPayout: round4(expectedPayout),
			PlayerEV:       round4(expectedPayout - contrib),
			RequiredEquity: round4(contrib / totalPot),
		})
	}

	return models.HoldemAllInEVResponse{
		Players:          playerResults,
		PotBreakdown:     potBreakdown,
		AfterRakePayout:  afterRake,
		CombosEvaluated:  combos,
		ElapsedMs:        time.Since(started).Milliseconds(),
		AppliedRakeTotal: round2(appliedRake),
	}, nil
}

func parseCommonState(inputs []models.PlayerInput, boardCodes, deadCodes []string) ([]parsedPlayer, []Card, []Card, []Card, error) {
	players := make([]parsedPlayer, 0, len(inputs))
	used := map[string]struct{}{}
	for idx, p := range inputs {
		if p.ID == "" {
			return nil, nil, nil, nil, fmt.Errorf("player %d id cannot be empty", idx)
		}
		if len(p.HoleCards) != 2 {
			return nil, nil, nil, nil, fmt.Errorf("player %s must have exactly 2 hole cards", p.ID)
		}
		c1, err := ParseCard(p.HoleCards[0])
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("player %s hole card 1: %w", p.ID, err)
		}
		c2, err := ParseCard(p.HoleCards[1])
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("player %s hole card 2: %w", p.ID, err)
		}
		if _, ok := used[c1.Code]; ok {
			return nil, nil, nil, nil, fmt.Errorf("duplicate card %s", c1.Code)
		}
		if _, ok := used[c2.Code]; ok {
			return nil, nil, nil, nil, fmt.Errorf("duplicate card %s", c2.Code)
		}
		used[c1.Code] = struct{}{}
		used[c2.Code] = struct{}{}
		players = append(players, parsedPlayer{ID: p.ID, HoleCards: [2]Card{c1, c2}, Contribution: p.Contribution, AllIn: p.AllIn})
	}

	board := make([]Card, 0, len(boardCodes))
	for _, code := range boardCodes {
		c, err := ParseCard(code)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("board card: %w", err)
		}
		if _, ok := used[c.Code]; ok {
			return nil, nil, nil, nil, fmt.Errorf("duplicate card %s", c.Code)
		}
		used[c.Code] = struct{}{}
		board = append(board, c)
	}

	dead := make([]Card, 0, len(deadCodes))
	for _, code := range deadCodes {
		c, err := ParseCard(code)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("dead card: %w", err)
		}
		if _, ok := used[c.Code]; ok {
			return nil, nil, nil, nil, fmt.Errorf("duplicate card %s", c.Code)
		}
		used[c.Code] = struct{}{}
		dead = append(dead, c)
	}

	deck := make([]Card, 0, 52-len(used))
	for _, c := range FullDeck() {
		if _, ok := used[c.Code]; ok {
			continue
		}
		deck = append(deck, c)
	}
	return players, board, dead, deck, nil
}

func enumerateCompletions(deck []Card, need int, cb func([]Card)) {
	if need == 0 {
		cb(nil)
		return
	}
	picked := make([]Card, need)
	var dfs func(start, depth int)
	dfs = func(start, depth int) {
		if depth == need {
			combination := make([]Card, need)
			copy(combination, picked)
			cb(combination)
			return
		}
		remaining := need - depth
		for i := start; i <= len(deck)-remaining; i++ {
			picked[depth] = deck[i]
			dfs(i+1, depth+1)
		}
	}
	dfs(0, 0)
}

func buildSidePots(players []parsedPlayer, contrib map[string]float64) ([]sidePotInternal, float64) {
	levels := make([]float64, 0, len(players))
	seen := map[float64]struct{}{}
	total := 0.0
	for _, p := range players {
		c := contrib[p.ID]
		total += c
		if c <= 0 {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		levels = append(levels, c)
	}
	sort.Float64s(levels)

	pots := make([]sidePotInternal, 0, len(levels))
	prev := 0.0
	sideIdx := 1
	for _, level := range levels {
		layer := level - prev
		if layer <= 0 {
			continue
		}
		eligible := make([]int, 0, len(players))
		for i, p := range players {
			if contrib[p.ID] >= level {
				eligible = append(eligible, i)
			}
		}
		if len(eligible) == 0 {
			prev = level
			continue
		}
		name := "main"
		if len(pots) > 0 {
			name = fmt.Sprintf("side-%d", sideIdx)
			sideIdx++
		}
		amount := layer * float64(len(eligible))
		pots = append(pots, sidePotInternal{
			Name:          name,
			Amount:        amount,
			PreRakeAmount: amount,
			Eligible:      eligible,
		})
		prev = level
	}
	return pots, total
}

func applyRake(pots []sidePotInternal, cfg models.RakeConfig) float64 {
	if !cfg.Enabled || cfg.RakePercent <= 0 {
		return 0
	}
	total := 0.0
	for i := range pots {
		pots[i].PreRakeAmount = pots[i].Amount
		total += pots[i].Amount
	}
	if total <= 0 {
		return 0
	}
	rake := total * cfg.RakePercent / 100.0
	if cfg.RakeCap > 0 && rake > cfg.RakeCap {
		rake = cfg.RakeCap
	}
	rake = math.Min(rake, total)
	ratio := (total - rake) / total
	for i := range pots {
		pots[i].Amount *= ratio
	}
	return rake
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
