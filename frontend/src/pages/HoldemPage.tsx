import { useMemo, useState } from 'react'
import { api } from '../api/client'
import type { HoldemDecisionResponse, Street } from '../types/api'
import type {
  CardTarget,
  DecisionCalcState,
  PickerState,
  OpponentDraft,
  ActionDraft
} from '../types/holdem-ui'
import {
  CARD_RANKS,
  CARD_SUITS,
  buildDecisionViewModel,
  formatCardDisplay,
  normalizeCard,
  validateHoldemDecision,
  toHoldemDecisionPayload
} from './holdem-decision-utils'
import { DecisionResultPanel } from '../components/DecisionResultPanel'

export function HoldemPage({ token }: { token?: string }) {
  // Hero State
  const [heroCards, setHeroCards] = useState<[string, string]>(['', ''])
  const [heroPosition, setHeroPosition] = useState('BTN')
  const [heroStack, setHeroStack] = useState(100)
  
  // Board & Pot State
  const [boardCards, setBoardCards] = useState<string[]>(['', '', '', '', ''])
  const [deadCards, setDeadCards] = useState<string[]>([])
  const [potSize, setPotSize] = useState(3)
  const [toCall, setToCall] = useState(0)
  
  // Base configuration
  const [street, setStreet] = useState<Street>('preflop')
  const [blinds, setBlinds] = useState<[number, number]>([1, 2])
  
  // Opponents
  const [opponents, setOpponents] = useState<OpponentDraft[]>([
    { id: 'BB', position: 'BB', stylePreset: 'balanced', rangeOverride: '' }
  ])
  
  // Timeline (Actions)
  const [history, setHistory] = useState<ActionDraft[]>([])

  // Engine Status
  const [result, setResult] = useState<HoldemDecisionResponse | null>(null)
  const [calcState, setCalcState] = useState<DecisionCalcState>({
    status: 'idle'
  })
  const [picker, setPicker] = useState<PickerState>({ open: false, target: null, suit: '' })

  const usedCards = useMemo(() => {
    const set = new Set<string>()
    heroCards.forEach(c => c && set.add(normalizeCard(c)))
    boardCards.forEach(c => c && set.add(normalizeCard(c)))
    deadCards.forEach(c => c && set.add(normalizeCard(c)))
    return set
  }, [heroCards, boardCards, deadCards])

  const usedCardsWithoutCurrentTarget = useMemo(() => {
    const set = new Set(usedCards)
    if (!picker.target) return set
    let current = ''
    if (picker.target.kind === 'hero') current = heroCards[picker.target.slot]
    if (picker.target.kind === 'board') current = boardCards[picker.target.slot]
    if (picker.target.kind === 'dead') current = deadCards[picker.target.index]
    
    if (current) set.delete(normalizeCard(current))
    return set
  }, [usedCards, picker.target, heroCards, boardCards, deadCards])

  const validation = useMemo(() => 
    validateHoldemDecision(heroCards, boardCards, deadCards, opponents, toCall),
  [heroCards, boardCards, deadCards, opponents, toCall])

  const viewModel = useMemo(() => buildDecisionViewModel({response: result}), [result])

  const runDecision = async () => {
    if (!validation.canRequest) {
      setCalcState({
        status: 'error',
        error: validation.errors[0]
      })
      return
    }

    setCalcState({ status: 'loading' })
    try {
      const payload = toHoldemDecisionPayload(
        heroCards, heroPosition, heroStack,
        boardCards, deadCards,
        potSize, toCall, blinds[1]*2, blinds,
        street, opponents, history, 3
      )
      
      const res = await api.holdemDecision(payload, token)
      setResult(res)
      setCalcState({ status: 'success', updatedAt: Date.now() })
    } catch (err: any) {
      setCalcState({ status: 'error', error: err.message || '求解失败' })
    }
  }

  // Pickers and helpers logic
  const openPicker = (e: React.MouseEvent<HTMLButtonElement>, target: CardTarget) => setPicker({ open: true, target, suit: '', rect: e.currentTarget.getBoundingClientRect() })
  const closePicker = () => setPicker({ open: false, target: null, suit: '' })

  const applyCardToTarget = (target: CardTarget, card: string) => {
    const normalized = normalizeCard(card)
    if (!normalized) return

    if (target.kind === 'hero') {
      setHeroCards(prev => {
        const next = [...prev] as [string, string]
        next[target.slot] = normalized
        return next
      })
    } else if (target.kind === 'board') {
      setBoardCards(prev => prev.map((c, i) => i === target.slot ? normalized : c))
    } else if (target.kind === 'dead') {
      setDeadCards(prev => prev.map((c, i) => i === target.index ? normalized : c))
    } else if (target.kind === 'dead-new') {
      setDeadCards(prev => [...prev, normalized])
    }
  }

  const onPickRank = (rank: string) => {
    if (!picker.open || !picker.target || !picker.suit) return
    const card = `${rank}${picker.suit}`
    if (usedCardsWithoutCurrentTarget.has(card)) return
    applyCardToTarget(picker.target, card)
    closePicker()
  }

  // Quick Action Helpers
  const addOpponent = () => {
    setOpponents(prev => [...prev, { id: 'UTG', position: 'UTG', stylePreset: 'balanced', rangeOverride: '' }])
  }
  
  const removeOpponent = (idx: number) => {
    setOpponents(prev => prev.filter((_, i) => i !== idx))
  }

  const addAction = () => {
    setHistory(prev => [...prev, {
      id: Date.now().toString(),
      street: street,
      actor: opponents[0]?.id || 'UTG',
      action: 'raise',
      amount: '0'
    }])
  }

  const removeAction = (idx: number) => {
    setHistory(prev => prev.filter((_, i) => i !== idx))
  }

  return (
    <section className="holdem-page">
      <header className="holdem-header">
        <h2>智能决策模式 (Solver)</h2>
      </header>

      <div className="holdem-layout">
        <section className="holdem-input-panel">
          
          <div className="input-block">
            <h3>Hero 视角</h3>
            <div className="hero-setup-grid">
              <label>
                位置
                <select value={heroPosition} onChange={e => setHeroPosition(e.target.value)}>
                  <option value="UTG">枪口 (UTG)</option>
                  <option value="MP">中位 (MP)</option>
                  <option value="CO">关煞 (CO)</option>
                  <option value="BTN">庄位 (BTN)</option>
                  <option value="SB">小盲 (SB)</option>
                  <option value="BB">大盲 (BB)</option>
                </select>
              </label>
              <label>
                有效筹码 (Stack)
                <input type="number" value={heroStack} onChange={e => setHeroStack(Number(e.target.value))} />
              </label>
            </div>
            <div className="seat-cards hero-cards-area">
              {[0, 1].map((slot) => {
                const card = heroCards[slot as 0 | 1]
                return (
                  <button
                    key={slot}
                    type="button"
                    className={`card-slot ${card ? 'filled' : ''}`}
                    onClick={(e) => openPicker(e, { kind: 'hero', slot: slot as 0 | 1 })}
                  >
                    {card ? formatCardDisplay(card) : `手牌${slot + 1}`}
                  </button>
                )
              })}
            </div>
            {heroCards[0] === '' || heroCards[1] === '' ? <p className="error">必须输入 Hero 的两张手牌</p> : null}
          </div>

          <div className="input-block">
            <h3>底池与进度</h3>
            <div className="board-setup-grid">
                <label>
                  阶段
                  <select value={street} onChange={e => setStreet(e.target.value as Street)}>
                    <option value="preflop">翻牌前 (Preflop)</option>
                    <option value="flop">翻牌 (Flop)</option>
                    <option value="turn">转牌 (Turn)</option>
                    <option value="river">河牌 (River)</option>
                  </select>
                </label>
                <label title="Pot">
                  当前底池
                  <input type="number" value={potSize} onChange={e => setPotSize(Number(e.target.value))} />
                </label>
                <label title="To Call">
                  需跟注额
                  <input type="number" value={toCall} onChange={e => setToCall(Number(e.target.value))} />
                </label>
                <label title="Small Blind">
                  小盲 (SB)
                  <input type="number" value={blinds[0]} onChange={e => setBlinds([Number(e.target.value), blinds[1]])} />
                </label>
                <label title="Big Blind">
                  大盲 (BB)
                  <input type="number" value={blinds[1]} onChange={e => setBlinds([blinds[0], Number(e.target.value)])} />
                </label>
            </div>
          </div>

          <div className="input-block">
             <h3>公共牌</h3>
             <div className="community-slots">
                {boardCards.map((card, index) => {
                  if (street === 'preflop') return null;
                  if (street === 'flop' && index > 2) return null;
                  if (street === 'turn' && index > 3) return null;

                  return (
                    <div key={index} className="community-slot">
                      <button
                        type="button"
                        className={`card-slot ${card ? 'filled' : ''}`}
                        onClick={(e) => openPicker(e, { kind: 'board', slot: index })}
                      >
                        {card ? formatCardDisplay(card) : `牌 ${index + 1}`}
                      </button>
                      {card && (
                        <button type="button" className="ghost-btn" onClick={() => setBoardCards(prev => prev.map((c, i) => i === index ? '' : c))}>清空</button>
                      )}
                    </div>
                  )
                })}
              </div>
          </div>

          <div className="input-block">
            <div className="holdem-input-top">
              <h3>对手设置</h3>
              <button className="ghost-btn" onClick={addOpponent}>+ 添加对手</button>
            </div>
            {opponents.map((opp, idx) => (
              <div key={idx} className="opponent-row">
                <select value={opp.position} onChange={e => {
                    const val = e.target.value
                    setOpponents(prev => prev.map((o, i) => i === idx ? { ...o, position: val, id: val } : o))
                }}>
                  <option value="UTG">枪口 (UTG)</option>
                  <option value="MP">中位 (MP)</option>
                  <option value="CO">关煞 (CO)</option>
                  <option value="BTN">庄位 (BTN)</option>
                  <option value="SB">小盲 (SB)</option>
                  <option value="BB">大盲 (BB)</option>
                </select>

                <select value={opp.stylePreset} onChange={e => {
                    const val = e.target.value as any
                    setOpponents(prev => prev.map((o, i) => i === idx ? { ...o, stylePreset: val } : o))
                }}>
                  <option value="tight">紧手 (Tight)</option>
                  <option value="balanced">平衡 (Balanced)</option>
                  <option value="loose">松手 (Loose)</option>
                  <option value="maniac">疯鱼 (Maniac)</option>
                </select>
                
                <input value={opp.rangeOverride} onChange={e => {
                  const val = e.target.value
                  setOpponents(prev => prev.map((o, i) => i === idx ? { ...o, rangeOverride: val } : o))
                }} placeholder="自定义范围 (例: AA,AKs)" style={{ flex: 1 }} />
                
                <button className="ghost-btn" onClick={() => removeOpponent(idx)}>×</button>
              </div>
            ))}
          </div>

          <div className="input-block">
            <div className="holdem-input-top">
              <h3>动作历史</h3>
              <button className="ghost-btn" onClick={addAction}>+ 添加动作</button>
            </div>
            {history.length === 0 && <p className="hint">暂无前期动作</p>}
            {history.map((act, idx) => (
              <div key={act.id} className="action-row">
                <select value={act.street} onChange={e => {
                    const val = e.target.value as Street
                    setHistory(prev => prev.map((h, i) => i === idx ? { ...h, street: val } : h))
                }}>
                  <option value="preflop">翻前 (PF)</option>
                  <option value="flop">翻牌 (FL)</option>
                  <option value="turn">转牌 (TN)</option>
                  <option value="river">河牌 (RV)</option>
                </select>
                
                <input value={act.actor} onChange={e => {
                  const val = e.target.value
                  setHistory(prev => prev.map((h, i) => i === idx ? { ...h, actor: val } : h))
                }} placeholder="行动座次 (如: UTG)" style={{ width: '130px' }} />

                <select value={act.action} onChange={e => {
                    const val = e.target.value as any
                    setHistory(prev => prev.map((h, i) => i === idx ? { ...h, action: val } : h))
                }}>
                  <option value="check">过牌 (Check)</option>
                  <option value="call">跟注 (Call)</option>
                  <option value="bet">下注 (Bet)</option>
                  <option value="raise">加注 (Raise)</option>
                  <option value="allin">全下 (All-In)</option>
                </select>

                <input type="number" value={act.amount} onChange={e => {
                  const val = e.target.value
                  setHistory(prev => prev.map((h, i) => i === idx ? { ...h, amount: val } : h))
                }} placeholder="金额" style={{ width: '80px' }} />
                
                <button className="ghost-btn" onClick={() => removeAction(idx)}>×</button>
              </div>
            ))}
          </div>

          {picker.open && picker.target && (
            <>
              <div className="card-picker-backdrop" onClick={closePicker}></div>
              <section 
                className="card-picker-popover" 
                onClick={e => e.stopPropagation()}
                style={picker.rect ? {
                  top: `${picker.rect.bottom + window.scrollY + 8}px`,
                  left: `${Math.min(picker.rect.left + window.scrollX, window.innerWidth - 320)}px`
                } : {}}
              >
                <div className="card-picker-head">
                  <strong>选牌器</strong>
                  <button type="button" className="ghost-btn" onClick={closePicker}>关闭</button>
                </div>
                <div className="suit-row">
                  {CARD_SUITS.map((suit) => (
                    <button
                      key={suit.code}
                      type="button"
                      className={`suit-btn ${picker.suit === suit.code ? 'active' : ''}`}
                      onClick={() => setPicker((prev) => ({ ...prev, suit: suit.code }))}
                    >
                      {suit.symbol} {suit.label}
                    </button>
                  ))}
                </div>
                <div className="rank-grid">
                  {CARD_RANKS.map((rank) => {
                    const disabled = !picker.suit || usedCardsWithoutCurrentTarget.has(`${rank}${picker.suit}`)
                    return (
                      <button key={rank} type="button" disabled={disabled} onClick={() => onPickRank(rank)}>
                        {rank}
                      </button>
                    )
                  })}
                </div>
              </section>
            </>
          )}

          <div className="holdem-action-bar">
            {validation.errors.length > 0 && <p className="error" style={{flex: 1}}>{validation.errors[0]}</p>}
            <button type="button" className="calculate-btn" onClick={runDecision} disabled={calcState.status === 'loading' || !validation.canRequest}>
              {calcState.status === 'loading' ? '计算引擎运行中...' : '提交给求解器 =>'}
            </button>
          </div>

        </section>

        <section className="holdem-result-panel">
           {calcState.status !== 'idle' ? (
              <DecisionResultPanel viewModel={calcState.status === 'loading' ? null : viewModel} status={calcState.status} error={calcState.error} />
           ) : (
              <div className="hint-container">
                 <h3>等待输入并提交求解</h3>
                 <p className="hint">1. 配置您的底池和手牌和行动历史。<br/>2. 点击底部“提交给求解器”。<br/>3. 查看并获得最佳推荐动作及EV。</p>
              </div>
           )}
        </section>
      </div>
    </section>
  )
}
