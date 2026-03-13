import { useMemo, useState, useEffect } from 'react'
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
  toHoldemDecisionPayload,
  computePotState,
  getNextActor,
  checkIsStreetClosed,
  getDynamicPositions,
  getValidActions
} from './holdem-decision-utils'
import { DecisionResultPanel } from '../components/DecisionResultPanel'

export function HoldemPage({ token }: { token?: string }) {
  // 用户视角 (User State)
  const [userCards, setUserCards] = useState<[string, string]>(['', ''])
  const [userPosition, setUserPosition] = useState('BTN')
  const [userStack, setUserStack] = useState(100)
  
  // Board & Pot State
  const [boardCards, setBoardCards] = useState<string[]>(['', '', '', '', ''])
  const [deadCards, setDeadCards] = useState<string[]>([])
  
  // Base configuration
  const [street, setStreet] = useState<Street>('preflop')
  const [blinds, setBlinds] = useState<[number, number]>([1, 2])
  
  // Opponents
  const [opponents, setOpponents] = useState<OpponentDraft[]>([
    { id: 'BB', position: 'BB', stylePreset: 'balanced', rangeOverride: '', stack: 100 }
  ])
  
  // Solver Config
  const [rolloutBudget, setRolloutBudget] = useState(10000)

  // Timeline (Actions)
  const [history, setHistory] = useState<ActionDraft[]>([])
  const [streetMessage, setStreetMessage] = useState('')
  const [gameEnded, setGameEnded] = useState(false)
  const [result, setResult] = useState<HoldemDecisionResponse | null>(null)
  const [calcState, setCalcState] = useState<DecisionCalcState>({
    status: 'idle'
  })
  const [picker, setPicker] = useState<PickerState>({ open: false, target: null, suit: '' })

  const usedCards = useMemo(() => {
    const set = new Set<string>()
    userCards.forEach(c => c && set.add(normalizeCard(c)))
    boardCards.forEach(c => c && set.add(normalizeCard(c)))
    deadCards.forEach(c => c && set.add(normalizeCard(c)))
    return set
  }, [userCards, boardCards, deadCards])

  const dynamicPositions = useMemo(() => getDynamicPositions(opponents.length + 1), [opponents.length])

  const { potSize, toCall, minRaiseTo, userRemainingStack, invested, activePositions: livePositions } = useMemo(
    () => computePotState(blinds, history, userPosition, opponents, userStack),
    [blinds, history, userPosition, opponents, userStack]
  )

  // 有效筹码计算已移除，按用户要求仅保留剩余筹码逻辑

  // 当人数变化导致位置列表变化时，自动修正无效的位置
  useEffect(() => {
    const validValues = new Set(dynamicPositions.map(p => p.value))
    
    if (!validValues.has(userPosition)) {
      setUserPosition(dynamicPositions[0].value)
    }

    setOpponents(prev => prev.map(opp => {
      if (!validValues.has(opp.position)) {
        return { ...opp, position: dynamicPositions[dynamicPositions.length - 1].value, id: dynamicPositions[dynamicPositions.length - 1].value }
      }
      return opp
    }))
  }, [dynamicPositions, userPosition])

  const usedCardsWithoutCurrentTarget = useMemo(() => {
    const set = new Set(usedCards)
    if (!picker.target) return set
    let current = ''
    if (picker.target.kind === 'hero') current = userCards[picker.target.slot]
    if (picker.target.kind === 'board') current = boardCards[picker.target.slot]
    if (picker.target.kind === 'dead') current = deadCards[picker.target.index]
    
    if (current) set.delete(normalizeCard(current))
    return set
  }, [usedCards, picker.target, userCards, boardCards, deadCards])

  const validation = useMemo(() => 
    validateHoldemDecision(userCards, boardCards, deadCards, opponents, toCall),
  [userCards, boardCards, deadCards, opponents, toCall])

  const viewModel = useMemo(() => buildDecisionViewModel({response: result}), [result])

  const activePositions = useMemo(() => {
    const list = [{ value: userPosition, label: `用户 (${userPosition})` }]
    opponents.forEach(o => {
      list.push({ value: o.position, label: `对手 (${o.position})` })
    })
    return list
  }, [userPosition, opponents])

  // 1. 当回合 (street) 变换时，清空之前的计算结果和状态，并给出提示
  useEffect(() => {
    setResult(null)
    setCalcState({ status: 'idle' })
    
    const streetLabels: Record<string, string> = {
      preflop: '翻牌前 (Preflop)',
      flop: '翻牌 (Flop)',
      turn: '转牌 (Turn)',
      river: '河牌 (River)'
    }
    setStreetMessage(`已进入 ${streetLabels[street]} 阶段`)
    const timer = setTimeout(() => setStreetMessage(''), 3000)
    return () => clearTimeout(timer)
  }, [street])

  // 2. 自动检测回合是否结束并跳转或提前结算
  useEffect(() => {
    if (history.length === 0 || gameEnded) return
    
    // 检查是否只剩下一个活着的玩家（其他人全 fold 了）
    const folded = new Set<string>()
    history.forEach(act => {
      if (act.action === 'fold') folded.add(act.actor)
    })
    
    const survivors = activePositions.filter(p => !folded.has(p.value))
    
    if (survivors.length === 1) {
      // 只有一个生存者，提前自动结算
      const winner = survivors[0].value
      setStreetMessage(`玩家 ${winner} 通过对手弃牌将在 2 秒后获胜...`)
      setTimeout(() => {
        // 再次检查，确保状态没有变化
        if (!gameEnded && history.length > 0) {
          handleSettleHand(winner)
        }
      }, 1500)
      return
    }

    const isClosed = checkIsStreetClosed(blinds, history, activePositions, street)
    if (isClosed) {
      setStreetMessage('本轮下注已平齐，即将进入下一回合...')
      // 增加一点延迟，让用户看清最后一个动作
      const closedStreet = street
      setTimeout(() => {
        setHistory(prev => {
          // 再次检查确认在延迟期间没有发生变化
          if (!checkIsStreetClosed(blinds, prev, activePositions, closedStreet)) return prev
          
          if (closedStreet === 'preflop') setStreet('flop')
          else if (closedStreet === 'flop') setStreet('turn')
          else if (closedStreet === 'turn') setStreet('river')
          return prev
        })
      }, 1000)
    }
  }, [history, blinds, activePositions, street, gameEnded])

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
        userCards, userPosition, userStack,
        boardCards, deadCards,
        potSize, toCall, minRaiseTo, blinds,
        street, opponents, history, 3, rolloutBudget
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
      setUserCards(prev => {
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
    setOpponents(prev => {
      if (prev.length >= 5) return prev // 最多 6 人桌面（1 用户 + 5 对手）
      
      // 预判添加后的位置列表
      const nextPositions = getDynamicPositions(prev.length + 2)
      const occupied = new Set([userPosition, ...prev.map(o => o.position)])
      const nextPos = nextPositions.find(p => !occupied.has(p.value))
      
      if (!nextPos) return prev
      
      return [...prev, { 
        id: nextPos.value, 
        position: nextPos.value, 
        stylePreset: 'balanced', 
        rangeOverride: '', 
        stack: 100 
      }]
    })
  }
  
  const removeOpponent = (idx: number) => {
    setOpponents(prev => prev.filter((_, i) => i !== idx))
  }

  const addAction = () => {
    const nextActor = getNextActor(history, street, activePositions)
    
    // 根据当前底池状态选择合理的默认动作：
    // 有人下注则默认 fold（保守安全），否则默认 check（继续游戏）
    // 用户可通过下拉菜单改为其他合法动作
    const defaultAction = toCall > 0 ? 'fold' : 'check'
    
    setHistory(prev => [...prev, {
      id: Date.now().toString(),
      street: street,
      actor: nextActor,
      action: defaultAction,
      amount: '0'
    }])
  }

  const removeAction = (idx: number) => {
    setHistory(prev => prev.filter((_, i) => i !== idx))
  }

  const handleApplyRecommendedAction = (recAction: string, recAmount: number) => {
    const nextActionType = recAction.toLowerCase() as any
    const newHistory = [...history, {
      id: Date.now().toString(),
      street: street,
      actor: userPosition,
      action: nextActionType,
      amount: recAmount.toString()
    }]
    setHistory(newHistory)
  }

  const handleSettleHand = (winnerPos: string) => {
    // 标记游戏已结束
    setGameEnded(true)
    
    // 赢家拿走全部 potSize
    const newUserStack = userRemainingStack + (winnerPos === userPosition ? potSize : 0)
    setUserStack(newUserStack)

    setOpponents(prev => prev.map(opp => {
      const inv = invested[opp.position] ?? 0
      const remaining = Math.max(0, (opp.stack || 0) - inv)
      return { ...opp, stack: remaining + (opp.position === winnerPos ? potSize : 0) }
    }))

    // 设置更清晰的结算提示信息
    const winnerLabel = winnerPos === userPosition ? '您' : `玩家 ${winnerPos}`
    setStreetMessage(`🎯 ${winnerLabel} 获胜！赢得 ${potSize} 筹码`)
    
    // 延迟重置，让用户有时间看清结算结果
    setTimeout(() => {
      // 重置牌局
      setUserCards(['', ''])
      setBoardCards(['', '', '', '', ''])
      setHistory([])
      setStreet('preflop')
      setResult(null)
      setCalcState({ status: 'idle' })
      setGameEnded(false)
      setStreetMessage('新手开始...')
      setTimeout(() => setStreetMessage(''), 2000)
    }, 3000)
  }


  // 按回合过滤动作历史
  const filteredHistory = useMemo(() => {
    return history.map((act, originalIdx) => ({ ...act, originalIdx })).filter(act => act.street === street)
  }, [history, street])

  const stepInfo = useMemo(() => {
    if (!userCards[0] || !userCards[1]) return { step: 1, text: '请设置您的两张手牌' }
    if (opponents.length === 0) return { step: 2, text: '请添加至少一位对手' }
    
    const boardCount = boardCards.filter(Boolean).length
    if (street === 'flop' && boardCount < 3) return { step: 3, text: '请补充翻牌 (3张公共牌)' }
    if (street === 'turn' && boardCount < 4) return { step: 3, text: '请补充转牌 (4张公共牌)' }
    if (street === 'river' && boardCount < 5) return { step: 3, text: '请补充河牌 (5张公共牌)' }

    // 动作历史逻辑：引导用户记录本轮动作
    if (filteredHistory.length === 0) {
      return { step: 4, text: '请记录本轮动作历史' }
    }

    return { step: 5, text: '准备就绪，点击上方按钮提交给求解器' }
  }, [userCards, opponents, street, boardCards, filteredHistory.length])

  return (
    <section className="holdem-page">
      <header className="holdem-header">
        <div className="header-with-steps">
          <h2>智能决策模式 (Solver)</h2>
          <div className="step-indicator">
            <span className="step-badge">步骤 {stepInfo.step}</span>
            <span className="step-text">{stepInfo.text}</span>
          </div>
        </div>
      </header>

      <div className="holdem-layout">
        <section className="holdem-input-panel">
          <div className="sticky-action-bar">
             <button type="button" className="calculate-btn ripple" onClick={runDecision} disabled={calcState.status === 'loading' || !validation.canRequest}>
                {calcState.status === 'loading' ? '计算引擎运行中...' : '立即提交计算 =>'}
              </button>
              {validation.errors.length > 0 && <p className="error-mini">{validation.errors[0]}</p>}
          </div>
          
          <div className="input-block">
            <h3>用户视角</h3>
            <div className="hero-setup-grid">
              <label>
                位置
                <select value={userPosition} onChange={e => setUserPosition(e.target.value)}>
                  {dynamicPositions.map(opt => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>
              </label>
              <label>
                初始筹码
                <input type="number" value={userStack} onChange={e => setUserStack(Number(e.target.value))} />
              </label>
              <label title="扣除强制盲注及本手牌已下注后的剩余金额">
                剩余筹码
                <input type="number" value={userRemainingStack} readOnly className="readonly-input" style={{ color: 'var(--brand)', fontWeight: 'bold' }} />
              </label>
            </div>
            <div className="seat-cards hero-cards-area">
              {[0, 1].map((slot) => {
                const card = userCards[slot as 0 | 1]
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
            {userCards[0] === '' || userCards[1] === '' ? <p className="error">必须输入用户的两张手牌</p> : null}
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
                  <input type="text" value={potSize} readOnly className="readonly-input" />
                </label>
                <label title="To Call">
                  需跟注额
                  <input type="text" value={toCall} readOnly className="readonly-input" />
                </label>
                <label title="Small Blind">
                  小盲 (SB)
                  <input type="number" value={blinds[0]} onChange={e => setBlinds([Number(e.target.value), blinds[1]])} />
                </label>
                <label title="Big Blind">
                  大盲 (BB)
                  <input type="number" value={blinds[1]} onChange={e => setBlinds([blinds[0], Number(e.target.value)])} />
                </label>
                <label title="MCCFR 迭代次数，越大越精确但越慢">
                  求解迭代数
                  <input type="number" value={rolloutBudget} min={500} max={100000} step={500}
                    onChange={e => setRolloutBudget(Math.max(500, Number(e.target.value)))} />
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
                  {dynamicPositions.map(opt => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>

                <select value={opp.stylePreset} onChange={e => {
                    const val = e.target.value as any
                    setOpponents(prev => prev.map((o, i) => i === idx ? { ...o, stylePreset: val } : o))
                }} style={{ width: '100px' }}>
                  <option value="tight">紧</option>
                  <option value="balanced">平</option>
                  <option value="loose">松</option>
                  <option value="maniac">疯</option>
                </select>
                
                <input type="number" 
                  value={opp.stack || 0} 
                  onChange={e => {
                    const val = Number(e.target.value)
                    setOpponents(prev => prev.map((o, i) => i === idx ? { ...o, stack: val } : o))
                  }} 
                  placeholder="筹码"
                  style={{ width: '80px' }}
                  title="初始筹码"
                />
                <input
                  type="number"
                  value={Math.max(0, (opp.stack || 0) - (invested[opp.position] ?? 0))}
                  readOnly
                  className="readonly-input"
                  style={{ width: '72px', color: 'var(--brand)', fontWeight: 'bold' }}
                  title="剩余筹码（自动计算）"
                />

                <input value={opp.rangeOverride} onChange={e => {
                  const val = e.target.value
                  setOpponents(prev => prev.map((o, i) => i === idx ? { ...o, rangeOverride: val } : o))
                }} placeholder="范围" style={{ flex: 1 }} />
                
                <button className="ghost-btn" onClick={() => removeOpponent(idx)}>×</button>
              </div>
            ))}
          </div>

          <div className="input-block">
            <div className="holdem-input-top">
              <h3>动作历史 ({street === 'preflop' ? '翻前' : street === 'flop' ? '翻牌' : street === 'turn' ? '转牌' : '河牌'})</h3>
              <button className="ghost-btn" onClick={addAction}>+ 添加本轮动作</button>
            </div>
            {filteredHistory.length === 0 && <p className="hint">本轮暂无动作记录</p>}
            {filteredHistory.map((act) => {
              // 计算该动作发生前的历史，以确定此刻的合法动作集合
              const priorHistory = history.slice(0, act.originalIdx)
              const validActions = getValidActions(blinds, priorHistory, act.actor, opponents)

              // 动作标签映射
              const ACTION_LABELS: Record<string, string> = {
                fold: '弃牌 (Fold)',
                check: '过牌 (Check)',
                call: '跟注 (Call)',
                bet: '下注 (Bet)',
                raise: '加注 (Raise)',
                allin: '全下 (All-In)',
              }

              return (
              <div key={act.id} className="action-row">
                <select value={act.street} onChange={e => {
                    const val = e.target.value as Street
                    setHistory(prev => prev.map((h, i) => i === act.originalIdx ? { ...h, street: val } : h))
                }}>
                  <option value="preflop">翻前 (PF)</option>
                  <option value="flop">翻牌 (FL)</option>
                  <option value="turn">转牌 (TN)</option>
                  <option value="river">河牌 (RV)</option>
                </select>
                
                <select value={act.actor} onChange={e => {
                  const val = e.target.value
                  setHistory(prev => prev.map((h, i) => {
                    if (i === act.originalIdx) {
                      let newAmount = h.amount
                      if (h.action === 'allin') {
                        const actorStack = val === userPosition 
                          ? userStack 
                          : (opponents.find(o => o.position === val)?.stack || 0)
                        newAmount = actorStack.toString()
                      }
                      return { ...h, actor: val, amount: newAmount }
                    }
                    return h
                  }))
                }} style={{ width: '130px' }}>
                  {activePositions.map(pos => (
                     <option key={pos.value} value={pos.value}>{pos.label}</option>
                  ))}
                </select>

                <select
                  value={validActions.includes(act.action) ? act.action : validActions[0]}
                  onChange={e => {
                    const val = e.target.value as any
                    setHistory(prev => prev.map((h, i) => {
                      if (i === act.originalIdx) {
                        let newAmount = h.amount
                        if (val === 'allin') {
                          const actorStack = h.actor === userPosition 
                            ? userStack 
                            : (opponents.find(o => o.position === h.actor)?.stack || 0)
                          newAmount = actorStack.toString()
                        } else if ((val === 'raise' || val === 'bet') && (h.amount === '0' || h.amount === '')) {
                          newAmount = minRaiseTo.toString()
                        }
                        return { ...h, action: val, amount: newAmount }
                      }
                      return h
                    }))
                  }}
                >
                  {validActions.map(a => (
                    <option key={a} value={a}>{ACTION_LABELS[a]}</option>
                  ))}
                </select>

                {/* 仅在需要设置金额的动作下显示输入框 */}
                {!['fold', 'check', 'call'].includes(act.action) && (
                  <input type="number" value={act.amount} onChange={e => {
                    const val = e.target.value
                    setHistory(prev => prev.map((h, i) => i === act.originalIdx ? { ...h, amount: val } : h))
                  }} placeholder="金额" style={{ width: '80px' }} />
                )}
                
                <button className="ghost-btn" onClick={() => removeAction(act.originalIdx)}>×</button>
              </div>
              )
            })}
            {history.length > filteredHistory.length && (
              <p className="hint-secondary">其他轮次动作已隐藏，切换阶段即可查看</p>
            )}
          </div>

          <div className="input-block settlement-block">
            <div className="holdem-input-top">
              <h3>局末结算</h3>
              <p className="hint">选择本局赢家，将自动更新各玩家筹码余额并开启下一手</p>
            </div>
            <div className="settlement-actions">
               {livePositions.map(pos => (
                 <button 
                   key={pos} 
                   className="ghost-btn primary" 
                   onClick={() => handleSettleHand(pos)}
                 >
                   {pos === userPosition ? '用户' : pos} 获胜 (+{potSize})
                 </button>
               ))}
               {livePositions.length === 0 && <p className="hint-secondary">暂无活跃玩家，请先添加动作</p>}
            </div>
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
                      style={{ color: suit.color }}
                    >
                      <span style={{ fontSize: '1.2em', marginRight: '4px' }}>{suit.symbol}</span> {suit.label}
                    </button>
                  ))}
                </div>
                <div className="rank-grid">
                  {CARD_RANKS.map((rank) => {
                    const disabled = !picker.suit || usedCardsWithoutCurrentTarget.has(`${rank}${picker.suit}`)
                    return (
                      <button 
                        key={rank} 
                        type="button" 
                        disabled={disabled} 
                        onClick={() => onPickRank(rank)}
                        style={!disabled ? { color: CARD_SUITS.find(s => s.code === picker.suit)?.color } : {}}
                      >
                        {rank}
                      </button>
                    )
                  })}
                </div>
              </section>
            </>
          )}



        </section>

        <section className="holdem-result-panel">
           {calcState.status !== 'idle' ? (
              <DecisionResultPanel 
                viewModel={calcState.status === 'loading' ? null : viewModel} 
                status={calcState.status} 
                error={calcState.error} 
                onApplyAction={handleApplyRecommendedAction}
              />
           ) : (
              <div className="hint-container">
                 <h3>等待输入并提交求解</h3>
                 <p className="hint">1. 配置您的底池和手牌和行动历史。<br/>2. 点击底部“提交给求解器”。<br/>3. 查看并获得最佳推荐动作及EV。</p>
              </div>
           )}
        </section>
      </div>
      {streetMessage && (
        <div className="street-switching-toast">
          {streetMessage}
        </div>
      )}
    </section>
  )
}
