import type {
  DecisionViewModel,
  HoldemValidation,
  HoldemDecisionDataSet,
  OpponentDraft,
  ActionDraft
} from '../types/holdem-ui'
import type { HoldemDecisionRequest, Street } from '../types/api'

export const CARD_SUITS: Array<{ code: 's' | 'h' | 'd' | 'c'; label: string; symbol: string }> = [
  { code: 's', label: '黑桃', symbol: '♠' },
  { code: 'h', label: '红桃', symbol: '♥' },
  { code: 'd', label: '方片', symbol: '♦' },
  { code: 'c', label: '梅花', symbol: '♣' }
]

export const CARD_RANKS = ['A', 'K', 'Q', 'J', 'T', '9', '8', '7', '6', '5', '4', '3', '2'] as const

export function normalizeCard(value: string): string {
  const raw = value.trim()
  if (!raw) return ''
  const match = raw.match(/^([2-9]|10|[tjqkaTJQKA])([shdcSHDC])$/)
  if (!match) return ''
  const rank = match[1].toUpperCase() === '10' ? 'T' : match[1].toUpperCase()
  const suit = match[2].toLowerCase()
  return `${rank}${suit}`
}

export function formatCardDisplay(value: string): string {
  const card = normalizeCard(value)
  if (!card) return ''
  const rank = card[0]
  const suit = card[1] as 's' | 'h' | 'd' | 'c'
  const symbol = CARD_SUITS.find((item) => item.code === suit)?.symbol ?? suit
  return `${rank}${symbol}`
}

export function validateHoldemDecision(
  heroCards: string[],
  boardCards: string[],
  deadCards: string[],
  opponents: OpponentDraft[],
  potToCall: number
): HoldemValidation {
  const errors: string[] = []
  const cardOwner = new Map<string, string>()

  if (heroCards.length !== 2) {
    errors.push('Hero 需要两张手牌')
  } else {
    heroCards.forEach((c, i) => {
      const norm = normalizeCard(c)
      if (!norm) {
        errors.push(`Hero 第 ${i + 1} 张手牌无效`)
      } else if (cardOwner.has(norm)) {
        errors.push(`手牌重复冲突: ${norm}`)
      } else {
        cardOwner.set(norm, 'Hero')
      }
    })
  }

  boardCards.forEach((c, i) => {
    if (!c.trim()) return
    const norm = normalizeCard(c)
    if (!norm) {
      errors.push(`公共牌第 ${i + 1} 张无效`)
    } else if (cardOwner.has(norm)) {
      errors.push(`公共牌重复冲突: ${norm}`)
    } else {
      cardOwner.set(norm, 'Board')
    }
  })

  deadCards.forEach((c, i) => {
    if (!c.trim()) return
    const norm = normalizeCard(c)
    if (!norm) {
      errors.push(`死牌第 ${i + 1} 张无效`)
    } else if (cardOwner.has(norm)) {
      errors.push(`死牌重复冲突: ${norm}`)
    } else {
      cardOwner.set(norm, 'Dead')
    }
  })

  if (opponents.length === 0) {
    errors.push('需要至少一个对手')
  }

  if (Number.isNaN(potToCall) || potToCall < 0) {
    errors.push('需跟注额度必须是非负数')
  }

  return {
    errors,
    canRequest: errors.length === 0
  }
}

export function toHoldemDecisionPayload(
  heroCards: string[],
  heroPosition: string,
  heroStack: number,
  boardCards: string[],
  deadCards: string[],
  potSize: number,
  toCall: number,
  minRaise: number,
  blinds: [number, number],
  street: Street,
  opponents: OpponentDraft[],
  history: ActionDraft[],
  branchCount: number
): HoldemDecisionRequest {
  const normsHero = heroCards.map(normalizeCard).filter(Boolean)
  const normsBoard = boardCards.map(normalizeCard).filter(Boolean)
  const normsDead = deadCards.map(normalizeCard).filter(Boolean)

  return {
    hero: {
      holeCards: normsHero,
      position: heroPosition,
      stack: heroStack
    },
    table: {
      playerCount: opponents.length + 1,
      positions: [heroPosition, ...opponents.map(o => o.position)],
      effectiveStacks: {}, 
      rakeConfig: { enabled: false, rakePercent: 0, rakeCap: 0 }
    },
    street,
    boardCards: normsBoard,
    deadCards: normsDead,
    potState: {
      potSize,
      toCall,
      minRaiseTo: minRaise,
      blinds
    },
    actionHistory: history.map(h => ({
      street: h.street,
      actor: h.actor,
      action: h.action,
      amount: Number(h.amount) || 0
    })),
    opponents: opponents.map(o => ({
      id: o.id,
      position: o.position,
      stylePreset: o.stylePreset,
      rangeOverride: o.rangeOverride || undefined
    })),
    solverConfig: {
      branchCount,
      timeoutMs: 5000,
      rolloutBudget: 5000
    }
  }
}

export function buildDecisionViewModel(data: HoldemDecisionDataSet): DecisionViewModel {
  if (!data.response) {
    return {
      topActions: [],
      heroStats: { equity: '-', tieRate: '-', potOdds: '-', reqEquity: '-' },
      opponents: [],
      treeStats: ''
    }
  }

  const { topActions, heroMetrics, opponentRangeSummary, treeStats } = data.response

  const formatPct = (val: number) => (val * 100).toFixed(1) + '%'

  return {
    topActions: topActions.map((act, idx) => {
      const actType = act.action.toUpperCase()
      const amtStr = act.amount && act.amount > 0 ? ` ${act.amount}` : ''
      return {
        action: actType,
        amountInfo: actType + amtStr,
        evText: act.ev.toFixed(2),
        ciText: `[${act.ciLow.toFixed(2)}, ${act.ciHigh.toFixed(2)}]`,
        isPrimary: idx === 0
      }
    }),
    heroStats: {
      equity: formatPct(heroMetrics.equity),
      tieRate: formatPct(heroMetrics.tieRate),
      potOdds: formatPct(heroMetrics.potOdds),
      reqEquity: formatPct(heroMetrics.requiredEquity)
    },
    opponents: opponentRangeSummary.map(o => ({
      id: o.id,
      coverage: formatPct(o.coverageRatio),
      topCombos: o.topCombos || '均等'
    })),
    treeStats: `${treeStats.nodes} 个节点 / ${treeStats.rollouts} 次采样 / ${treeStats.elapsedMs}ms`
  }
}
