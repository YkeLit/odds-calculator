import React from 'react'
import type {
  DecisionViewModel,
  HoldemValidation,
  HoldemDecisionDataSet,
  OpponentDraft,
  ActionDraft
} from '../types/holdem-ui'
import type { HoldemDecisionRequest, Street } from '../types/api'

export const CARD_SUITS: Array<{ code: 's' | 'h' | 'd' | 'c'; label: string; symbol: string; color: string }> = [
  { code: 's', label: '黑桃', symbol: '♠', color: '#1a1a1b' }, // 黑
  { code: 'h', label: '红桃', symbol: '♥', color: '#e1102b' }, // 红
  { code: 'd', label: '方片', symbol: '♦', color: '#e1102b' }, // 红
  { code: 'c', label: '梅花', symbol: '♣', color: '#15803d' }  // 绿 (4色扑克常用)
]

export const CARD_RANKS = ['A', 'K', 'Q', 'J', 'T', '9', '8', '7', '6', '5', '4', '3', '2'] as const

export const POSITION_OPTIONS = [
  { value: 'UTG', label: '枪口 (UTG)' },
  { value: 'MP', label: '中位 (MP)' },
  { value: 'CO', label: '关煞 (CO)' },
  { value: 'BTN', label: '庄位 (BTN)' },
  { value: 'SB', label: '小盲 (SB)' },
  { value: 'BB', label: '大盲 (BB)' },
] as const

export const SEAT_ORDER = ['SB', 'BB', 'UTG', 'MP', 'CO', 'BTN'] as const

export function getNextActor(
  history: ActionDraft[],
  street: Street,
  activePositions: Array<{ value: string; label: string }>
): string {
  // 翻牌前：UTG 先行动，BB 最后；翻牌后：SB 先行动，BTN 最后
  const preflopOrder = ['UTG', 'MP', 'CO', 'BTN', 'SB', 'BB']
  const postflopOrder = ['SB', 'BB', 'UTG', 'MP', 'CO', 'BTN']
  const actionOrder = street === 'preflop' ? preflopOrder : postflopOrder

  // 按本街行动顺序过滤出实际参与的座位
  const positionValues = activePositions.map(p => p.value)
  const orderedPositions = actionOrder.filter(p => positionValues.includes(p))

  const folded = new Set<string>()
  for (const act of history) {
    if (act.action === 'fold') folded.add(act.actor)
  }

  // 还在场上的玩家（按正确顺序）
  const available = orderedPositions.filter(p => !folded.has(p))
  if (available.length === 0) return activePositions[0]?.value || 'UTG'

  const lastAction = history[history.length - 1]

  // 无历史或刚切换街道 → 返回本街第一个行动者
  if (!lastAction || lastAction.street !== street) {
    return available[0]
  }

  // 在完整顺序表（含已弃牌）中找到最后动作者的位置，再向后找第一个存活者
  const lastActorIdx = orderedPositions.indexOf(lastAction.actor)
  if (lastActorIdx === -1) return available[0]

  const n = orderedPositions.length
  for (let i = 1; i <= n; i++) {
    const candidate = orderedPositions[(lastActorIdx + i) % n]
    if (available.includes(candidate)) return candidate
  }

  return available[0]
}

export function checkIsStreetClosed(
  blinds: [number, number],
  history: ActionDraft[],
  activePositions: Array<{ value: string; label: string }>,
  currentStreet?: Street
): boolean {
  if (history.length === 0) return false

  const targetStreet = currentStreet ?? history[history.length - 1].street
  const streetActions = history.filter(h => h.street === targetStreet)
  // 刚切换街道还没有任何动作，不能认为已关闭
  if (streetActions.length === 0) return false

  // 收集全局弃牌和 all-in 信息（跨街）
  const folded = new Set<string>()
  const allIn = new Set<string>()
  for (const act of history) {
    if (act.action === 'fold') folded.add(act.actor)
    if (act.action === 'allin') allIn.add(act.actor)
  }

  // 还在场上（未弃牌）的玩家
  const activePlayers = activePositions.filter(p => !folded.has(p.value))

  // 只剩 0 或 1 人 → 牌局直接结束
  if (activePlayers.length <= 1) return true

  // ── 计算本街投入 ──
  const invested: Record<string, number> = {}
  let currentBet = 0

  if (targetStreet === 'preflop') {
    activePositions.forEach(p => {
      if (p.value === 'SB') invested[p.value] = blinds[0]
      if (p.value === 'BB') invested[p.value] = blinds[1]
    })
    currentBet = blinds[1]
  } else {
    for (const p of activePositions) {
      invested[p.value] = 0
    }
  }

  // ── 用 "待表态集合" 追踪动作闭环 ──
  // 初始：所有存活且未 all-in 的玩家都需要表态
  const needsToAct = new Set<string>()
  for (const p of activePlayers) {
    if (!allIn.has(p.value)) {
      needsToAct.add(p.value)
    }
  }

  for (const act of streetActions) {
    const actor = act.actor
    const amt = Number(act.amount) || 0

    // 该玩家已表态，从待表态集合中移除
    needsToAct.delete(actor)

    switch (act.action) {
      case 'call':
        invested[actor] = currentBet
        break
      case 'check':
        // 无金额变动
        break
      case 'fold':
        // 已在全局 folded 中处理
        break
      case 'bet':
      case 'raise': {
        const totalBet = amt > 0 ? amt : currentBet
        invested[actor] = totalBet
        if (totalBet > currentBet) {
          currentBet = totalBet
          // 加注重新打开动作：所有存活、非 all-in、非加注者需重新表态
          for (const p of activePlayers) {
            if (p.value !== actor && !folded.has(p.value) && !allIn.has(p.value)) {
              needsToAct.add(p.value)
            }
          }
        }
        break
      }
      case 'allin': {
        const totalBet = amt > 0 ? amt : currentBet
        invested[actor] = totalBet
        allIn.add(actor)
        if (totalBet > currentBet) {
          currentBet = totalBet
          for (const p of activePlayers) {
            if (p.value !== actor && !folded.has(p.value) && !allIn.has(p.value)) {
              needsToAct.add(p.value)
            }
          }
        }
        break
      }
    }
  }

  // ── 判断是否关闭 ──

  // 1. 还有人需要表态 → 未关闭
  if (needsToAct.size > 0) return false

  // 2. 注码平齐：所有非弃牌、非 all-in 玩家投入 ≥ currentBet
  const nonAllInActive = activePlayers.filter(p => !allIn.has(p.value))
  const uncalled = nonAllInActive.filter(p => (invested[p.value] ?? 0) < currentBet)
  if (uncalled.length > 0) return false

  return true
}


/**
 * 根据人数动态获取标准位置
 * 2人 (Heads-up): SB/BTN (庄兼小盲), BB
 * 3人: BTN, SB, BB
 * 4人: CO, BTN, SB, BB
 * 5人: MP, CO, BTN, SB, BB
 * 6人: UTG, MP, CO, BTN, SB, BB
 */
export function getDynamicPositions(playerCount: number): Array<{ value: string; label: string }> {
  const all = [
    { value: 'UTG', label: '枪口 (UTG)' },
    { value: 'MP', label: '中位 (MP)' },
    { value: 'CO', label: '关煞 (CO)' },
    { value: 'BTN', label: '庄位 (BTN)' },
    { value: 'SB', label: '小盲 (SB)' },
    { value: 'BB', label: '大盲 (BB)' },
  ]

  const count = Math.max(2, Math.min(6, playerCount))

  if (count === 2) {
    return [
      { value: 'SB', label: '庄位/小盲 (BTN/SB)' },
      { value: 'BB', label: '大盲 (BB)' },
    ]
  }

  // 根据人数截取末尾的几个位置（德州扑克位置是从 BB 往前推的）
  return all.slice(6 - count)
}

/**
 * 根据盲注和动作历史自动计算底池大小、需跟注额和最小加注额。
 *
 * 逻辑：
 * 1. 初始底池 = SB + BB
 * 2. 遍历每条动作：
 *    - call: 该玩家补足差额到当前最大下注，差额进入底池
 *    - bet/raise/allin: 更新当前最大下注和上一次加注的增量
 *    - fold/check: 无底池变化
 * 3. toCall = 当前最大下注 - 用户已投入的金额
 * 4. minRaiseTo = 当前最大下注 + 上一次加注增量（最少为一个大盲）
 */
export function computePotState(
  blinds: [number, number],
  actions: ActionDraft[],
  userPosition: string,
  opponents: OpponentDraft[],
  userInitialStack: number
): { 
  potSize: number; 
  toCall: number; 
  minRaiseTo: number; 
  userRemainingStack: number;
  invested: Record<string, number>;
  activePositions: string[];
} {
  const [sb, bb] = blinds

  const sbPos = 'SB'
  const bbPos = 'BB'
  const allPositions = [userPosition, ...opponents.map(o => o.position)]

  const invested: Record<string, number> = {}
  const folded = new Set<string>()
  invested[sbPos] = sb
  invested[bbPos] = bb

  let pot = sb + bb        // 底池总金额
  let currentBet = bb      // 当前圈最大下注
  let lastRaiseIncrement = bb  // 上一次加注增量

  for (const act of actions) {
    const actor = act.actor
    const prevInvested = invested[actor] ?? 0
    const amountValue = Number(act.amount) || 0

    if (act.action === 'fold') {
      folded.add(actor)
      continue
    }

    switch (act.action) {
      case 'call': {
        const toPayForCall = currentBet - prevInvested
        if (toPayForCall > 0) {
          pot += toPayForCall
          invested[actor] = currentBet
        }
        break
      }
      case 'bet':
      case 'raise':
      case 'allin': {
        const totalBet = amountValue > 0 ? amountValue : currentBet
        const diff = totalBet - prevInvested
        if (diff > 0) {
          pot += diff
          invested[actor] = totalBet
        }
        if (totalBet > currentBet) {
          lastRaiseIncrement = totalBet - currentBet
          currentBet = totalBet
        }
        break
      }
      case 'check':
      default:
        break
    }
  }

  const userInvested = invested[userPosition] ?? 0
  const userRemainingStack = Math.max(0, userInitialStack - userInvested)
  const toCall = Math.max(0, currentBet - userInvested)
  const minRaiseTo = Math.max(currentBet + lastRaiseIncrement, currentBet + bb)

  const activePositions = allPositions.filter(p => !folded.has(p))

  return { 
    potSize: pot, 
    toCall, 
    minRaiseTo, 
    userRemainingStack,
    invested,
    activePositions
  }
}

/**
 * 根据当前动作之前的历史，判断该演员在此时的合法动作列表。
 *
 * - toCall > 0（有人已下注/加注）→ 只能 fold / call / raise / allin
 * - toCall === 0（无人下注）→ 只能 check / bet / allin
 */
export function getValidActions(
  blinds: [number, number],
  priorHistory: ActionDraft[],
  actor: string,
  opponents: OpponentDraft[]
): string[] {
  // 利用 computePotState 站在 actor 角度计算 toCall
  const { toCall } = computePotState(blinds, priorHistory, actor, opponents, 0)

  if (toCall > 0) {
    return ['fold', 'call', 'raise', 'allin']
  } else {
    return ['check', 'bet', 'allin']
  }
}



export function normalizeCard(value: string): string {
  const raw = value.trim()
  if (!raw) return ''
  const match = raw.match(/^([2-9]|10|[tjqkaTJQKA])([shdcSHDC])$/)
  if (!match) return ''
  const rank = match[1].toUpperCase() === '10' ? 'T' : match[1].toUpperCase()
  const suit = match[2].toLowerCase()
  return `${rank}${suit}`
}

export function formatCardDisplay(value: string): React.ReactNode {
  const card = normalizeCard(value)
  if (!card) return ''
  const rank = card.length === 3 ? card.substring(0, 2) : card[0]
  const suitCode = (card.length === 3 ? card[2] : card[1]) as 's' | 'h' | 'd' | 'c'
  
  const suitInfo = CARD_SUITS.find((item) => item.code === suitCode)
  const symbol = suitInfo?.symbol ?? suitCode
  const color = suitInfo?.color ?? 'inherit'

  return React.createElement('span', { className: 'formatted-card' },
    rank,
    React.createElement('span', { style: { color, marginLeft: '2px' } }, symbol)
  )
}

export function validateHoldemDecision(
  userCards: string[],
  boardCards: string[],
  deadCards: string[],
  opponents: OpponentDraft[],
  potToCall: number
): HoldemValidation {
  const errors: string[] = []
  const cardOwner = new Map<string, string>()

  if (userCards.length !== 2) {
    errors.push('用户需要两张手牌')
  } else {
    userCards.forEach((c, i) => {
      const norm = normalizeCard(c)
      if (!norm) {
        errors.push(`用户第 ${i + 1} 张手牌无效`)
      } else if (cardOwner.has(norm)) {
        errors.push(`手牌重复冲突: ${norm}`)
      } else {
        cardOwner.set(norm, '用户')
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
  userCards: string[],
  userPosition: string,
  userStack: number,
  boardCards: string[],
  deadCards: string[],
  potSize: number,
  toCall: number,
  minRaise: number,
  blinds: [number, number],
  street: Street,
  opponents: OpponentDraft[],
  history: ActionDraft[],
  branchCount: number,
  rolloutBudget: number = 10000
): HoldemDecisionRequest {
  const normsUser = userCards.map(normalizeCard).filter(Boolean)
  const normsBoard = boardCards.map(normalizeCard).filter(Boolean)
  const normsDead = deadCards.map(normalizeCard).filter(Boolean)

  return {
    hero: {
      holeCards: normsUser,
      position: userPosition,
      stack: userStack
    },
    table: {
      playerCount: opponents.length + 1,
      positions: [userPosition, ...opponents.map(o => o.position)],
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
      rolloutBudget
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
      const amt = act.amount ? Math.round(act.amount) : 0
      const amtStr = amt > 0 ? ` ${amt}` : ''
      return {
        action: actType,
        amountInfo: actType + amtStr,
        evText: act.ev.toFixed(1),
        ciText: `[${Math.round(act.ciLow)}, ${Math.round(act.ciHigh)}]`,
        freqText: (act.frequency * 100).toFixed(1) + '%',
        regretText: Math.round(act.regret).toString(),
        isPrimary: idx === 0,
        rawAction: act.action,
        rawAmount: amt
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
