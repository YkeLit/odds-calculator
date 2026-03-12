package models

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	CreatedAt    string `json:"createdAt"`
}

type UserInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type AuthResponse struct {
	AccessToken string   `json:"accessToken"`
	User        UserInfo `json:"user"`
}

type PlayerInput struct {
	ID           string   `json:"id"`
	HoleCards    []string `json:"holeCards"`
	Contribution float64  `json:"contribution,omitempty"`
	AllIn        bool     `json:"allIn,omitempty"`
}

type HoldemOddsRequest struct {
	Players    []PlayerInput `json:"players"`
	BoardCards []string      `json:"boardCards"`
	DeadCards  []string      `json:"deadCards"`
}

type HoldemOddsPlayerResult struct {
	ID      string  `json:"id"`
	WinRate float64 `json:"winRate"`
	TieRate float64 `json:"tieRate"`
	Equity  float64 `json:"equity"`
}

type HoldemOddsResponse struct {
	Results         []HoldemOddsPlayerResult `json:"results"`
	CombosEvaluated int                      `json:"combosEvaluated"`
	ElapsedMs       int64                    `json:"elapsedMs"`
}

type RakeConfig struct {
	Enabled     bool    `json:"enabled"`
	RakePercent float64 `json:"rakePercent"`
	RakeCap     float64 `json:"rakeCap"`
}

type HoldemAllInEVRequest struct {
	Players       []PlayerInput      `json:"players"`
	BoardCards    []string           `json:"boardCards"`
	DeadCards     []string           `json:"deadCards"`
	Contributions map[string]float64 `json:"contributions"`
	AllInFlags    map[string]bool    `json:"allinFlags"`
	RakeConfig    RakeConfig         `json:"rakeConfig"`
}

type SidePot struct {
	Name              string   `json:"name"`
	Amount            float64  `json:"amount"`
	PreRakeAmount     float64  `json:"preRakeAmount"`
	EligiblePlayerIDs []string `json:"eligiblePlayerIds"`
}

type HoldemAllInPlayerEV struct {
	ID             string  `json:"id"`
	ExpectedPayout float64 `json:"expectedPayout"`
	PlayerEV       float64 `json:"playerEV"`
	RequiredEquity float64 `json:"requiredEquity"`
}

type HoldemAllInEVResponse struct {
	Players          []HoldemAllInPlayerEV `json:"players"`
	PotBreakdown     []SidePot             `json:"potBreakdown"`
	AfterRakePayout  map[string]float64    `json:"afterRakePayout"`
	CombosEvaluated  int                   `json:"combosEvaluated"`
	ElapsedMs        int64                 `json:"elapsedMs"`
	AppliedRakeTotal float64               `json:"appliedRakeTotal"`
}

type Street string

const (
	StreetPreflop Street = "preflop"
	StreetFlop    Street = "flop"
	StreetTurn    Street = "turn"
	StreetRiver   Street = "river"
)

type ActionType string

const (
	ActionFold  ActionType = "fold"
	ActionCall  ActionType = "call"
	ActionCheck ActionType = "check"
	ActionBet   ActionType = "bet"
	ActionRaise ActionType = "raise"
	ActionAllIn ActionType = "allin"
)

type HeroState struct {
	HoleCards []string `json:"holeCards"`
	Position  string   `json:"position"`
	Stack     float64  `json:"stack"`
}

type TableState struct {
	PlayerCount     int                `json:"playerCount"`
	Positions       []string           `json:"positions"`
	EffectiveStacks map[string]float64 `json:"effectiveStacks"`
	RakeConfig      RakeConfig         `json:"rakeConfig"`
}

type PotState struct {
	PotSize    float64   `json:"potSize"`
	ToCall     float64   `json:"toCall"`
	MinRaiseTo float64   `json:"minRaiseTo"`
	Blinds     [2]float64 `json:"blinds"` // [SB, BB]
}

type ActionNode struct {
	Street Street     `json:"street"`
	Actor  string     `json:"actor"` // Position string e.g. "UTG"
	Action ActionType `json:"action"`
	Amount float64    `json:"amount"` // Could be 0 for fold/check
}

type OpponentInfo struct {
	ID            string `json:"id"`            // Usually matches position, e.g. "UTG"
	Position      string `json:"position"`
	StylePreset   string `json:"stylePreset"`   // "tight", "loose", "balanced", etc.
	RangeOverride string `json:"rangeOverride,omitempty"` // e.g. "AA,AKs:0.5"
}

type SolverConfig struct {
	BranchCount   int   `json:"branchCount"`   // N=2..5
	TimeoutMs     int64 `json:"timeoutMs"`     // Defaults to 5000
	RolloutBudget int   `json:"rolloutBudget"` // Iteration limits per branch
}

type HoldemDecisionRequest struct {
	Hero          HeroState      `json:"hero"`
	Table         TableState     `json:"table"`
	Street        Street         `json:"street"`
	BoardCards    []string       `json:"boardCards"`
	DeadCards     []string       `json:"deadCards"`
	PotState      PotState       `json:"potState"`
	ActionHistory []ActionNode   `json:"actionHistory"`
	Opponents     []OpponentInfo `json:"opponents"`
	SolverConfig  SolverConfig   `json:"solverConfig"`
}

type DecisionAction struct {
	Action      ActionType `json:"action"`
	Amount      float64    `json:"amount,omitempty"`
	EV          float64    `json:"ev"`
	CILow       float64    `json:"ciLow"`
	CIHigh      float64    `json:"ciHigh"`
	Frequency   float64    `json:"frequency"` // Option for mixed strats, later use
}

type HeroMetrics struct {
	Equity         float64 `json:"equity"`
	TieRate        float64 `json:"tieRate"`
	PotOdds        float64 `json:"potOdds"`
	RequiredEquity float64 `json:"requiredEquity"`
}

type OpponentRangeSummary struct {
	ID            string  `json:"id"`
	CoverageRatio float64 `json:"coverageRatio"`
	TopCombos     string  `json:"topCombos"` // Top N combos summary text
}

type TreeStats struct {
	Nodes        int     `json:"nodes"`
	Rollouts     int     `json:"rollouts"`
	DepthReached int     `json:"depthReached"`
	ElapsedMs    int64   `json:"elapsedMs"`
	Convergence  float64 `json:"convergence"` // 0.0 - 1.0 proxy for stability
}

type HoldemDecisionResponse struct {
	TopActions           []DecisionAction       `json:"topActions"`
	HeroMetrics          HeroMetrics            `json:"heroMetrics"`
	OpponentRangeSummary []OpponentRangeSummary `json:"opponentRangeSummary"`
	TreeStats            TreeStats              `json:"treeStats"`
}

type MahjongAnalyzeRequest struct {
	HandTiles    []string       `json:"handTiles"`
	Melds        [][]string     `json:"melds"`
	VisibleTiles []string       `json:"visibleTiles"`
	MissingSuit  string         `json:"missingSuit"`
	RoundContext map[string]any `json:"roundContext"`
}

type FanItem struct {
	Name string `json:"name"`
	Fan  int    `json:"fan"`
}

type FanResult struct {
	TotalFan int       `json:"totalFan"`
	Fans     []FanItem `json:"fans"`
}

type FanContribution struct {
	Fan          string  `json:"fan"`
	Contribution float64 `json:"contribution"`
}

type DiscardRecommendation struct {
	Tile            string            `json:"tile"`
	ExpectedFan     float64           `json:"expectedFan"`
	HuProb          float64           `json:"huProb"`
	TopFanBreakdown []FanContribution `json:"topFanBreakdown"`
}

type MahjongAnalyzeResponse struct {
	IsTing                 bool                    `json:"isTing"`
	WinningTiles           []string                `json:"winningTiles"`
	NextTwoDrawHuProb      float64                 `json:"nextTwoDrawHuProb"`
	DiscardRecommendations []DiscardRecommendation `json:"discardRecommendations"`
	ElapsedMs              int64                   `json:"elapsedMs"`
}

type CalcHistoryRecord struct {
	ID           int64  `json:"id"`
	GameType     string `json:"gameType"`
	RequestJSON  string `json:"requestJson"`
	ResponseJSON string `json:"responseJson"`
	CreatedAt    string `json:"createdAt"`
}

type HistoryResponse struct {
	Items    []CalcHistoryRecord `json:"items"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
	Total    int                 `json:"total"`
}
