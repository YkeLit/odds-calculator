import type { DecisionViewModel } from '../types/holdem-ui'

interface DecisionResultPanelProps {
  viewModel: DecisionViewModel | null
  status: 'idle' | 'loading' | 'success' | 'error'
  error?: string
}

export function DecisionResultPanel({ viewModel, status, error }: DecisionResultPanelProps) {
  if (status === 'error') {
    return (
      <div className="holdem-result-panel">
        <div className="error-card">
          <h3>计算失败</h3>
          <p>{error}</p>
        </div>
      </div>
    )
  }

  if (status === 'loading') {
    return (
      <div className="holdem-result-panel">
        <p className="loading-state">求解器正在采样计算中，请稍候...</p>
      </div>
    )
  }

  if (!viewModel || status === 'idle') {
    return (
      <div className="holdem-result-panel">
        <p className="hint">等待输入信号以启动决策引擎</p>
      </div>
    )
  }

  return (
    <div className="holdem-result-panel">
      <div className="summary-grid">
        <article className="summary-card">
          <p>胜率 Equity</p>
          <strong>{viewModel.heroStats.equity}</strong>
        </article>
        <article className="summary-card">
          <p>底池赔率 Pot Odds</p>
          <strong>{viewModel.heroStats.potOdds}</strong>
        </article>
        <article className="summary-card">
          <p>需要胜率 Req. Equity</p>
          <strong>{viewModel.heroStats.reqEquity}</strong>
        </article>
        <article className="summary-card">
          <p>平局率 Tie Rate</p>
          <strong>{viewModel.heroStats.tieRate}</strong>
        </article>
      </div>

      <div className="result-grid actions-grid">
        <article className="result-card full-width">
          <div className="result-head">
            <h3>推荐动作 (Top Actions)</h3>
          </div>
          {viewModel.topActions.length === 0 && <p className="hint">暂无可用动作</p>}
          <div className="action-cards">
            {viewModel.topActions.map((act, i) => (
              <div key={i} className={`action-card ${act.isPrimary ? 'primary-action' : ''}`}>
                <h4>{act.action === 'FOLD' ? '弃牌 (Fold)' : act.action === 'CALL' ? '跟注 (Call)' : act.action === 'CHECK' ? '过牌 (Check)' : act.action === 'RAISE' ? '加注 (Raise)' : act.action === 'ALLIN' ? '全下 (All-In)' : act.action}</h4>
                <div className="action-amt">{act.amountInfo}</div>
                <div className="action-ev">
                  <span>EV:</span> <strong>{act.evText}</strong>
                </div>
                <div className="action-ci">
                  <span>95% CI:</span> {act.ciText}
                </div>
              </div>
            ))}
          </div>
        </article>
      </div>

      <div className="result-grid">
        <article className="result-card full-width">
          <div className="result-head">
            <h3>对手范围分析</h3>
          </div>
          <table className="data-table holdem-table">
            <thead>
              <tr>
                <th>对手</th>
                <th>剩余范围覆盖率</th>
                <th>最可能持有的组合 (Top 3)</th>
              </tr>
            </thead>
            <tbody>
              {viewModel.opponents.map(opp => (
                <tr key={opp.id}>
                  <td>{opp.id}</td>
                  <td>{opp.coverage}</td>
                  <td>{opp.topCombos}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </article>
      </div>

      <div className="tree-stats hint" style={{ marginTop: '1rem', textAlign: 'right' }}>
        求解树状态: {viewModel.treeStats}
      </div>
    </div>
  )
}
