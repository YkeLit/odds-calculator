import type { HoldemDecisionResponse, ActionType, Street } from './api'

export interface CardSlotState {
  card: string
  disabled: boolean
  label: string
}

export interface DecisionCalcState {
  status: 'idle' | 'loading' | 'success' | 'error'
  error?: string
  updatedAt?: number
}

// UI Representation of an opponent
export interface OpponentDraft {
  id: string
  position: string
  stylePreset: 'tight' | 'loose' | 'balanced' | 'maniac'
  rangeOverride: string 
  stack?: number
}

// UI Representation of an action flow
export interface ActionDraft {
  id: string
  street: Street
  actor: string
  action: ActionType
  amount: string 
}

export interface HoldemValidation {
  errors: string[]
  canRequest: boolean
}

export type CardTarget =
  | { kind: 'hero'; slot: 0 | 1 }
  | { kind: 'board'; slot: number }
  | { kind: 'dead'; index: number }
  | { kind: 'dead-new' }

export interface PickerState {
  open: boolean
  target: CardTarget | null
  suit: 's' | 'h' | 'd' | 'c' | ''
  rect?: DOMRect
}

export interface DecisionViewModel {
  // To display results
  topActions: Array<{
    action: string
    amountInfo: string
    evText: string
    ciText: string
    freqText: string
    regretText: string
    isPrimary: boolean
    rawAction: string
    rawAmount: number
  }>
  heroStats: {
    equity: string
    tieRate: string
    potOdds: string
    reqEquity: string
  }
  opponents: Array<{
    id: string
    coverage: string
    topCombos: string
  }>
  treeStats: string
}

export interface HoldemDecisionDataSet {
  response: HoldemDecisionResponse | null
}
