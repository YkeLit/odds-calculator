package holdem

import (
	"math"
	"math/rand"
	"sync"

	"odds-calculator/backend/internal/models"
)

// ═══════════════════════════════════════════════════════════════
// ES-CFR+ (External-Sampled CFR with CFR+ regret truncation)
//
// Key design points:
//
// 1. Alternating Updates: One full iteration = two DFS passes.
//    Pass 1: player 0 (hero) is the traverser → update hero regrets.
//    Pass 2: player 1 (opponent) is the traverser → update opp regrets.
//    The non-traverser's actions are sampled from their current strategy.
//
// 2. Compact InfoSet keys: [sorted-hole | board | action-history]
//    encoded as a short string to keep the hash map lightweight.
//
// 3. CFR+ truncation: after updating cumulative regrets, negative
//    values are clamped to 0 so the algorithm "forgets" bad actions
//    quickly and converges much faster.
// ═══════════════════════════════════════════════════════════════

// ──── InfoSet data structures ────

// InfoSetKey uniquely identifies an information set.
type InfoSetKey string

// InfoSetData stores cumulative regrets and strategy sums per action.
type InfoSetData struct {
	CumulativeRegret   []float64
	CumulativeStrategy []float64
	ActionCount        int
}

// GetStrategy returns the current iteration strategy via regret-matching.
func (d *InfoSetData) GetStrategy() []float64 {
	strat := make([]float64, d.ActionCount)
	posSum := 0.0
	for i := 0; i < d.ActionCount; i++ {
		if d.CumulativeRegret[i] > 0 {
			posSum += d.CumulativeRegret[i]
		}
	}
	if posSum > 0 {
		for i := 0; i < d.ActionCount; i++ {
			if d.CumulativeRegret[i] > 0 {
				strat[i] = d.CumulativeRegret[i] / posSum
			}
		}
	} else {
		uni := 1.0 / float64(d.ActionCount)
		for i := range strat {
			strat[i] = uni
		}
	}
	return strat
}

// GetAverageStrategy returns the converged (average) strategy.
func (d *InfoSetData) GetAverageStrategy() []float64 {
	strat := make([]float64, d.ActionCount)
	sum := 0.0
	for _, v := range d.CumulativeStrategy {
		sum += v
	}
	if sum > 0 {
		for i, v := range d.CumulativeStrategy {
			strat[i] = v / sum
		}
	} else {
		uni := 1.0 / float64(d.ActionCount)
		for i := range strat {
			strat[i] = uni
		}
	}
	return strat
}

// InfoSetMap is a concurrency-safe store for InfoSetData.
type InfoSetMap struct {
	mu   sync.RWMutex
	data map[InfoSetKey]*InfoSetData
}

// NewInfoSetMap creates an empty map.
func NewInfoSetMap() *InfoSetMap {
	return &InfoSetMap{data: make(map[InfoSetKey]*InfoSetData)}
}

// GetOrCreate returns existing data or creates a zero-initialised entry.
func (m *InfoSetMap) GetOrCreate(key InfoSetKey, actionCount int) *InfoSetData {
	m.mu.RLock()
	d, ok := m.data[key]
	m.mu.RUnlock()
	if ok {
		return d
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok = m.data[key]; ok {
		return d
	}
	d = &InfoSetData{
		CumulativeRegret:   make([]float64, actionCount),
		CumulativeStrategy: make([]float64, actionCount),
		ActionCount:        actionCount,
	}
	m.data[key] = d
	return d
}

// Size returns the number of infosets stored.
func (m *InfoSetMap) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Merge accumulates all entries from src into m.
func (m *InfoSetMap) Merge(src *InfoSetMap) {
	src.mu.RLock()
	defer src.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, sd := range src.data {
		if md, ok := m.data[key]; ok {
			for i := 0; i < sd.ActionCount; i++ {
				md.CumulativeRegret[i] += sd.CumulativeRegret[i]
				md.CumulativeStrategy[i] += sd.CumulativeStrategy[i]
			}
		} else {
			clone := &InfoSetData{
				CumulativeRegret:   append([]float64(nil), sd.CumulativeRegret...),
				CumulativeStrategy: append([]float64(nil), sd.CumulativeStrategy...),
				ActionCount:        sd.ActionCount,
			}
			m.data[key] = clone
		}
	}
}

// globalCache persists InfoSet data across requests so regrets accumulate
// over successive calls for the same game situation, reducing frequency churn.
// Capped at maxGlobalCacheSize entries (LRU-style: clear when exceeded).
const maxGlobalCacheSize = 200_000

var globalCache = NewInfoSetMap()
var globalCacheMu sync.Mutex

// ──── Game-tree node ────

type nodeType int

const (
	nodeChance nodeType = iota
	nodePlayer
	nodeTerminal
)

type gameNode struct {
	Type   nodeType
	Street models.Street

	ActorIdx int
	ActorPos string

	Pot        float64
	Invested   map[string]float64
	CurrentBet float64
	Board      []Card
	HeroHole   [2]Card
	OppHoles   [][2]Card
	FoldedMask []bool
	AllInMask  []bool
	Deck       []Card

	HistoryStr string
}

// ──── MCCFRSolver ────

// MCCFRSolver implements ES-CFR+ with alternating updates.
type MCCFRSolver struct {
	InfoSets *InfoSetMap

	HeroIdx      int // always 0
	PlayerCount  int
	Positions    []string
	InitialBoard []Card
	DeadCards    []Card
	HeroHole     [2]Card
	OppRanges    []Range
	PotState     models.PotState
	Stacks       []float64

	Iterations int
}

// NewMCCFRSolver creates a new solver.
func NewMCCFRSolver(
	heroHole [2]Card,
	positions []string,
	oppRanges []Range,
	initialBoard []Card,
	dead []Card,
	potState models.PotState,
	stacks []float64,
	iterations int,
) *MCCFRSolver {
	return &MCCFRSolver{
		InfoSets:     NewInfoSetMap(),
		HeroIdx:      0,
		PlayerCount:  len(positions),
		Positions:    positions,
		InitialBoard: initialBoard,
		DeadCards:    dead,
		HeroHole:     heroHole,
		OppRanges:    oppRanges,
		PotState:     potState,
		Stacks:       stacks,
		Iterations:   iterations,
	}
}

// Run executes ES-CFR+ with alternating updates.
// One iteration = traverse for player 0 + traverse for player 1..N.
func (s *MCCFRSolver) Run() {
	for iter := 0; iter < s.Iterations; iter++ {
		// Sample chance once per iteration (external sampling)
		oppHoles, deck := s.sampleChance()

		// Alternating updates: each player takes a turn as traverser
		for traverser := 0; traverser < s.PlayerCount; traverser++ {
			root := s.makeRoot(oppHoles, deck)
			s.cfrTraverse(root, traverser, 1.0, iter)
		}
	}
}

// GetRootStrategy returns the converged average strategy at the
// hero's first decision point (the root information set).
func (s *MCCFRSolver) GetRootStrategy() ([]models.ActionType, []float64) {
	actions := s.nodeActions(s.PotState.ToCall, s.Stacks[0])
	key := s.makeInfoSetKey(s.HeroHole, s.InitialBoard, "")
	data := s.InfoSets.GetOrCreate(key, len(actions))
	strat := data.GetAverageStrategy()
	return actions, strat
}

// ──── Chance sampling ────

func (s *MCCFRSolver) sampleChance() ([][2]Card, []Card) {
	used := make(map[string]struct{})
	for _, c := range s.InitialBoard {
		used[c.Code] = struct{}{}
	}
	for _, c := range s.DeadCards {
		used[c.Code] = struct{}{}
	}
	used[s.HeroHole[0].Code] = struct{}{}
	used[s.HeroHole[1].Code] = struct{}{}

	oppHoles := make([][2]Card, len(s.OppRanges))
	for i, rng := range s.OppRanges {
		for attempts := 0; attempts < 200; attempts++ {
			idx := sampleFromRange(rng)
			combo := GlobalCombos[idx]
			c1, c2 := combo.Card1.Code, combo.Card2.Code
			if _, ok := used[c1]; ok {
				continue
			}
			if _, ok := used[c2]; ok {
				continue
			}
			oppHoles[i] = [2]Card{combo.Card1, combo.Card2}
			used[c1] = struct{}{}
			used[c2] = struct{}{}
			break
		}
	}

	deck := make([]Card, 0, 52)
	for _, c := range FullDeck() {
		if _, ok := used[c.Code]; !ok {
			deck = append(deck, c)
		}
	}
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return oppHoles, deck
}

func (s *MCCFRSolver) makeRoot(oppHoles [][2]Card, deck []Card) *gameNode {
	invested := make(map[string]float64)
	for _, pos := range s.Positions {
		invested[pos] = 0
	}
	return &gameNode{
		Type:       nodePlayer,
		Street:     streetFromBoardLen(len(s.InitialBoard)),
		ActorIdx:   0,
		ActorPos:   s.Positions[0],
		Pot:        s.PotState.PotSize,
		Invested:   invested,
		CurrentBet: s.PotState.ToCall,
		Board:      append([]Card{}, s.InitialBoard...),
		HeroHole:   s.HeroHole,
		OppHoles:   oppHoles,
		FoldedMask: make([]bool, s.PlayerCount),
		AllInMask:  make([]bool, s.PlayerCount),
		Deck:       deck,
		HistoryStr: "",
	}
}

// ──── ES-CFR+ traversal ────

// cfrTraverse performs one DFS pass for the given traverser.
//   - At traverser's nodes: explore ALL actions, compute counterfactual values,
//     update regrets (with CFR+ truncation) and accumulate strategy.
//   - At non-traverser's nodes: SAMPLE one action from current strategy (external sampling).
//   - At chance nodes: use pre-sampled board cards.
func (s *MCCFRSolver) cfrTraverse(node *gameNode, traverser int, reachProb float64, iter int) float64 {
	if node.Type == nodeTerminal {
		return s.terminalValue(node, traverser)
	}
	if node.Type == nodeChance {
		return s.handleChanceNode(node, traverser, reachProb, iter)
	}

	// Player node
	isTraverser := node.ActorIdx == traverser
	toCall := math.Max(0, node.CurrentBet-node.Invested[node.ActorPos])
	remaining := s.Stacks[node.ActorIdx] - node.Invested[node.ActorPos]

	actions := s.nodeActions(toCall, remaining)
	if len(actions) == 0 {
		return s.terminalValue(node, traverser)
	}

	// Look up hole cards for this actor
	var holeCards [2]Card
	if node.ActorIdx == 0 {
		holeCards = node.HeroHole
	} else {
		holeCards = node.OppHoles[node.ActorIdx-1]
	}

	key := s.makeInfoSetKey(holeCards, node.Board, node.HistoryStr)
	data := s.InfoSets.GetOrCreate(key, len(actions))
	strategy := data.GetStrategy()

	if isTraverser {
		// ── Traverser: explore ALL actions ──
		actionValues := make([]float64, len(actions))
		nodeValue := 0.0

		for i, act := range actions {
			child := s.applyAction(node, act)
			actionValues[i] = s.cfrTraverse(child, traverser, reachProb*strategy[i], iter)
			nodeValue += strategy[i] * actionValues[i]
		}

		// Update regrets with CFR+ truncation (clamp negatives to 0)
		for i := range actions {
			regret := actionValues[i] - nodeValue
			data.CumulativeRegret[i] += regret
			// CFR+ truncation: forget bad actions instantly
			if data.CumulativeRegret[i] < 0 {
				data.CumulativeRegret[i] = 0
			}
		}

		// Accumulate strategy sum (weighted by reach probability)
		for i := range actions {
			data.CumulativeStrategy[i] += reachProb * strategy[i]
		}

		return nodeValue
	}

	// ── Non-traverser: SAMPLE one action (external sampling) ──
	actionIdx := sampleAction(strategy)
	child := s.applyAction(node, actions[actionIdx])
	return s.cfrTraverse(child, traverser, reachProb, iter)
}

// terminalValue returns the payoff from the perspective of the traverser.
func (s *MCCFRSolver) terminalValue(node *gameNode, traverser int) float64 {
	traverserPos := s.Positions[traverser]
	traverserInvested := 0.0
	if node.Invested != nil {
		traverserInvested = node.Invested[traverserPos]
	}

	// Traverser folded → loses what they invested
	if node.FoldedMask[traverser] {
		return -traverserInvested
	}

	// Everyone else folded → traverser wins the pot
	allOthersFolded := true
	for i := 0; i < s.PlayerCount; i++ {
		if i != traverser && !node.FoldedMask[i] {
			allOthersFolded = false
			break
		}
	}
	if allOthersFolded {
		return node.Pot - traverserInvested
	}

	return s.showdownValue(node, traverser)
}

// showdownValue evaluates the traverser's payoff at showdown.
func (s *MCCFRSolver) showdownValue(node *gameNode, traverser int) float64 {
	board := node.Board
	if len(board) < 5 {
		need := 5 - len(board)
		if need <= len(node.Deck) {
			board = append(append([]Card{}, board...), node.Deck[:need]...)
		}
	}
	if len(board) < 5 {
		return 0
	}

	// Get traverser's hole cards
	var traverserHole [2]Card
	if traverser == 0 {
		traverserHole = node.HeroHole
	} else {
		traverserHole = node.OppHoles[traverser-1]
	}

	traverserSeven := append([]Card{traverserHole[0], traverserHole[1]}, board...)
	traverserRank := EvaluateSeven(traverserSeven)

	traverserPos := s.Positions[traverser]
	traverserInvested := 0.0
	if node.Invested != nil {
		traverserInvested = node.Invested[traverserPos]
	}

	// Compare against all non-folded opponents
	bestOppRank := HandRank{Category: -1}
	hasActiveOpp := false
	for i := 0; i < s.PlayerCount; i++ {
		if i == traverser || node.FoldedMask[i] {
			continue
		}
		hasActiveOpp = true
		var oppHole [2]Card
		if i == 0 {
			oppHole = node.HeroHole
		} else {
			oppHole = node.OppHoles[i-1]
		}
		oppSeven := append([]Card{oppHole[0], oppHole[1]}, board...)
		oppRank := EvaluateSeven(oppSeven)
		if bestOppRank.Category == -1 || CompareHandRank(oppRank, bestOppRank) > 0 {
			bestOppRank = oppRank
		}
	}

	if !hasActiveOpp {
		return node.Pot - traverserInvested
	}

	cmp := CompareHandRank(traverserRank, bestOppRank)
	if cmp > 0 {
		return node.Pot - traverserInvested
	} else if cmp == 0 {
		alive := 0
		for i := 0; i < s.PlayerCount; i++ {
			if !node.FoldedMask[i] {
				alive++
			}
		}
		if alive == 0 {
			alive = 1
		}
		return (node.Pot / float64(alive)) - traverserInvested
	}
	return -traverserInvested
}

func (s *MCCFRSolver) handleChanceNode(node *gameNode, traverser int, reachProb float64, iter int) float64 {
	var newBoard []Card
	var newDeck []Card
	var nextStreet models.Street

	switch node.Street {
	case models.StreetPreflop:
		if len(node.Deck) < 3 {
			return s.showdownValue(node, traverser)
		}
		newBoard = append(append([]Card{}, node.Board...), node.Deck[:3]...)
		newDeck = node.Deck[3:]
		nextStreet = models.StreetFlop
	case models.StreetFlop:
		if len(node.Deck) < 1 {
			return s.showdownValue(node, traverser)
		}
		newBoard = append(append([]Card{}, node.Board...), node.Deck[0])
		newDeck = node.Deck[1:]
		nextStreet = models.StreetTurn
	case models.StreetTurn:
		if len(node.Deck) < 1 {
			return s.showdownValue(node, traverser)
		}
		newBoard = append(append([]Card{}, node.Board...), node.Deck[0])
		newDeck = node.Deck[1:]
		nextStreet = models.StreetRiver
	case models.StreetRiver:
		return s.showdownValue(node, traverser)
	}

	firstActor := s.nextActivePlayer(node.FoldedMask, node.AllInMask, -1)
	if firstActor == -1 {
		return s.showdownValue(&gameNode{
			Board:      newBoard,
			HeroHole:   node.HeroHole,
			OppHoles:   node.OppHoles,
			FoldedMask: node.FoldedMask,
			AllInMask:  node.AllInMask,
			Pot:        node.Pot,
			Invested:   node.Invested,
			Deck:       newDeck,
		}, traverser)
	}

	invested := make(map[string]float64)
	for _, pos := range s.Positions {
		invested[pos] = 0
	}

	child := &gameNode{
		Type:       nodePlayer,
		Street:     nextStreet,
		ActorIdx:   firstActor,
		ActorPos:   s.Positions[firstActor],
		Pot:        node.Pot,
		Invested:   invested,
		CurrentBet: 0,
		Board:      newBoard,
		HeroHole:   node.HeroHole,
		OppHoles:   node.OppHoles,
		FoldedMask: copyBoolSlice(node.FoldedMask),
		AllInMask:  copyBoolSlice(node.AllInMask),
		Deck:       newDeck,
		HistoryStr: node.HistoryStr,
	}
	return s.cfrTraverse(child, traverser, reachProb, iter)
}

// ──── Action generation & application ────

func (s *MCCFRSolver) nodeActions(toCall, remainingStack float64) []models.ActionType {
	if remainingStack <= 0 {
		return nil
	}
	var actions []models.ActionType
	if toCall <= 0 {
		actions = append(actions, models.ActionCheck)
		if remainingStack > 0 {
			actions = append(actions, models.ActionBet)
		}
	} else {
		actions = append(actions, models.ActionFold)
		if remainingStack >= toCall {
			actions = append(actions, models.ActionCall)
		}
		if remainingStack > toCall {
			actions = append(actions, models.ActionRaise)
		}
	}
	if remainingStack > 0 {
		actions = append(actions, models.ActionAllIn)
	}
	return actions
}

func (s *MCCFRSolver) applyAction(node *gameNode, action models.ActionType) *gameNode {
	newFolded := copyBoolSlice(node.FoldedMask)
	newAllIn := copyBoolSlice(node.AllInMask)
	invested := copyInvested(node.Invested)
	pot := node.Pot
	currentBet := node.CurrentBet
	actorPos := node.ActorPos
	remaining := s.Stacks[node.ActorIdx] - invested[actorPos]

	historyStr := node.HistoryStr + actionToChar(action)

	switch action {
	case models.ActionFold:
		newFolded[node.ActorIdx] = true
	case models.ActionCheck:
		// no change
	case models.ActionCall:
		callAmt := math.Min(currentBet-invested[actorPos], remaining)
		if callAmt > 0 {
			pot += callAmt
			invested[actorPos] += callAmt
		}
	case models.ActionBet:
		betAmt := math.Min(pot*0.5, remaining)
		if betAmt <= 0 {
			betAmt = math.Min(s.PotState.Blinds[1], remaining)
		}
		pot += betAmt
		invested[actorPos] += betAmt
		currentBet = invested[actorPos]
	case models.ActionRaise:
		raiseTotal := math.Min(currentBet*2.5, remaining+invested[actorPos])
		if raiseTotal <= currentBet {
			raiseTotal = math.Min(currentBet+s.PotState.Blinds[1], remaining+invested[actorPos])
		}
		addAmt := raiseTotal - invested[actorPos]
		if addAmt > 0 {
			pot += addAmt
			invested[actorPos] = raiseTotal
			currentBet = raiseTotal
		}
	case models.ActionAllIn:
		addAmt := remaining
		if addAmt > 0 {
			pot += addAmt
			invested[actorPos] += addAmt
			if invested[actorPos] > currentBet {
				currentBet = invested[actorPos]
			}
			newAllIn[node.ActorIdx] = true
		}
	}

	// Check if only one player remains
	aliveCnt := 0
	lastAlive := -1
	for i := 0; i < s.PlayerCount; i++ {
		if !newFolded[i] {
			aliveCnt++
			lastAlive = i
		}
	}
	if aliveCnt <= 1 {
		return &gameNode{
			Type:       nodeTerminal,
			Pot:        pot,
			HeroHole:   node.HeroHole,
			OppHoles:   node.OppHoles,
			FoldedMask: newFolded,
			AllInMask:  newAllIn,
			Invested:   invested,
			Board:      node.Board,
			Deck:       node.Deck,
			ActorIdx:   lastAlive,
			HistoryStr: historyStr,
		}
	}

	nextIdx := s.nextActivePlayer(newFolded, newAllIn, node.ActorIdx)

	if nextIdx == -1 || s.isStreetClosed(invested, currentBet, newFolded, newAllIn, historyStr) {
		allActorsAllInOrFolded := true
		for i := 0; i < s.PlayerCount; i++ {
			if !newFolded[i] && !newAllIn[i] {
				allActorsAllInOrFolded = false
				break
			}
		}
		if allActorsAllInOrFolded || node.Street == models.StreetRiver {
			return s.makeShowdownOrChance(node, pot, invested, newFolded, newAllIn, historyStr)
		}
		return &gameNode{
			Type:       nodeChance,
			Street:     node.Street,
			Pot:        pot,
			Invested:   invested,
			CurrentBet: currentBet,
			Board:      node.Board,
			HeroHole:   node.HeroHole,
			OppHoles:   node.OppHoles,
			FoldedMask: newFolded,
			AllInMask:  newAllIn,
			Deck:       node.Deck,
			HistoryStr: historyStr,
		}
	}

	return &gameNode{
		Type:       nodePlayer,
		Street:     node.Street,
		ActorIdx:   nextIdx,
		ActorPos:   s.Positions[nextIdx],
		Pot:        pot,
		Invested:   invested,
		CurrentBet: currentBet,
		Board:      node.Board,
		HeroHole:   node.HeroHole,
		OppHoles:   node.OppHoles,
		FoldedMask: newFolded,
		AllInMask:  newAllIn,
		Deck:       node.Deck,
		HistoryStr: historyStr,
	}
}

func (s *MCCFRSolver) makeShowdownOrChance(node *gameNode, pot float64, invested map[string]float64, foldedMask, allInMask []bool, historyStr string) *gameNode {
	board := node.Board
	deck := node.Deck
	need := 5 - len(board)
	if need > 0 && need <= len(deck) {
		board = append(append([]Card{}, board...), deck[:need]...)
		deck = deck[need:]
	}
	return &gameNode{
		Type:       nodeTerminal,
		Pot:        pot,
		Board:      board,
		HeroHole:   node.HeroHole,
		OppHoles:   node.OppHoles,
		FoldedMask: foldedMask,
		AllInMask:  allInMask,
		Invested:   invested,
		Deck:       deck,
		HistoryStr: historyStr,
	}
}

// ──── Helpers ────

func (s *MCCFRSolver) nextActivePlayer(folded, allIn []bool, currentIdx int) int {
	for i := 1; i < s.PlayerCount; i++ {
		idx := (currentIdx + i) % s.PlayerCount
		if !folded[idx] && !allIn[idx] {
			return idx
		}
	}
	return -1
}

func (s *MCCFRSolver) isStreetClosed(invested map[string]float64, currentBet float64, folded, allIn []bool, history string) bool {
	for i := 0; i < s.PlayerCount; i++ {
		if folded[i] || allIn[i] {
			continue
		}
		if invested[s.Positions[i]] < currentBet {
			return false
		}
	}
	return len(history) > 0
}

func (s *MCCFRSolver) makeInfoSetKey(hole [2]Card, board []Card, history string) InfoSetKey {
	h1, h2 := hole[0].Code, hole[1].Code
	if h1 > h2 {
		h1, h2 = h2, h1
	}
	key := h1 + h2 + "|"
	for _, b := range board {
		key += b.Code
	}
	key += "|" + history
	return InfoSetKey(key)
}

func sampleAction(strategy []float64) int {
	r := rand.Float64()
	cumulative := 0.0
	for i, p := range strategy {
		cumulative += p
		if r <= cumulative {
			return i
		}
	}
	return len(strategy) - 1
}

func actionToChar(a models.ActionType) string {
	switch a {
	case models.ActionFold:
		return "f"
	case models.ActionCheck:
		return "k"
	case models.ActionCall:
		return "c"
	case models.ActionBet:
		return "b"
	case models.ActionRaise:
		return "r"
	case models.ActionAllIn:
		return "a"
	}
	return "?"
}

func streetFromBoardLen(n int) models.Street {
	switch {
	case n >= 5:
		return models.StreetRiver
	case n >= 4:
		return models.StreetTurn
	case n >= 3:
		return models.StreetFlop
	default:
		return models.StreetPreflop
	}
}

func copyBoolSlice(s []bool) []bool {
	c := make([]bool, len(s))
	copy(c, s)
	return c
}

func copyInvested(m map[string]float64) map[string]float64 {
	c := make(map[string]float64, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
