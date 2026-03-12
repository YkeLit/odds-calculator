export interface UserInfo {
  id: number
  username: string
}

export interface AuthResponse {
  accessToken: string
  user: UserInfo
}

export interface HoldemPlayerInput {
  id: string
  holeCards: string[]
  contribution?: number
  allIn?: boolean
}

export interface HoldemOddsRequest {
  players: HoldemPlayerInput[]
  boardCards: string[]
  deadCards?: string[]
}

export interface HoldemOddsPlayerResult {
  id: string
  winRate: number
  tieRate: number
  equity: number
}

export interface HoldemOddsResponse {
  results: HoldemOddsPlayerResult[]
  combosEvaluated: number
  elapsedMs: number
}

export interface RakeConfig {
  enabled: boolean
  rakePercent: number
  rakeCap: number
}

export interface HoldemAllInEVRequest {
  players: HoldemPlayerInput[]
  boardCards: string[]
  deadCards?: string[]
  contributions: Record<string, number>
  allinFlags?: Record<string, boolean>
  rakeConfig: RakeConfig
}

export interface SidePot {
  name: string
  amount: number
  preRakeAmount: number
  eligiblePlayerIds: string[]
}

export interface HoldemAllInPlayerEV {
  id: string
  expectedPayout: number
  playerEV: number
  requiredEquity: number
}

export interface HoldemAllInEVResponse {
  players: HoldemAllInPlayerEV[]
  potBreakdown: SidePot[]
  afterRakePayout: Record<string, number>
  combosEvaluated: number
  elapsedMs: number
  appliedRakeTotal: number
}

export type Street = 'preflop' | 'flop' | 'turn' | 'river'
export type ActionType = 'fold' | 'call' | 'check' | 'bet' | 'raise' | 'allin'

export interface HeroState {
  holeCards: string[]
  position: string
  stack: number
}

export interface TableState {
  playerCount: number
  positions: string[]
  effectiveStacks: Record<string, number>
  rakeConfig: RakeConfig
}

export interface PotState {
  potSize: number
  toCall: number
  minRaiseTo: number
  blinds: [number, number]
}

export interface ActionNode {
  street: Street
  actor: string
  action: ActionType
  amount: number
}

export interface OpponentInfo {
  id: string
  position: string
  stylePreset: string
  rangeOverride?: string
}

export interface SolverConfig {
  branchCount: number
  timeoutMs: number
  rolloutBudget: number
}

export interface HoldemDecisionRequest {
  hero: HeroState
  table: TableState
  street: Street
  boardCards: string[]
  deadCards: string[]
  potState: PotState
  actionHistory: ActionNode[]
  opponents: OpponentInfo[]
  solverConfig: SolverConfig
}

export interface DecisionAction {
  action: ActionType
  amount?: number
  ev: number
  ciLow: number
  ciHigh: number
  frequency: number
}

export interface HeroMetrics {
  equity: number
  tieRate: number
  potOdds: number
  requiredEquity: number
}

export interface OpponentRangeSummary {
  id: string
  coverageRatio: number
  topCombos: string
}

export interface TreeStats {
  nodes: number
  rollouts: number
  depthReached: number
  elapsedMs: number
  convergence: number
}

export interface HoldemDecisionResponse {
  topActions: DecisionAction[]
  heroMetrics: HeroMetrics
  opponentRangeSummary: OpponentRangeSummary[]
  treeStats: TreeStats
}

export interface MahjongAnalyzeRequest {
  handTiles: string[]
  melds: string[][]
  visibleTiles: string[]
  missingSuit: string
}

export interface FanContribution {
  fan: string
  contribution: number
}

export interface DiscardRecommendation {
  tile: string
  expectedFan: number
  huProb: number
  topFanBreakdown: FanContribution[]
}

export interface MahjongAnalyzeResponse {
  isTing: boolean
  winningTiles: string[]
  nextTwoDrawHuProb: number
  discardRecommendations: DiscardRecommendation[]
  elapsedMs: number
}

export interface CalcHistoryRecord {
  id: number
  gameType: string
  requestJson: string
  responseJson: string
  createdAt: string
}

export interface HistoryResponse {
  items: CalcHistoryRecord[]
  page: number
  pageSize: number
  total: number
}
