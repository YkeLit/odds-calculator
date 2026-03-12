import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { HistoryResponse } from '../types/api'

interface HistoryPageProps {
  token?: string
}

export function HistoryPage({ token }: HistoryPageProps) {
  const [gameType, setGameType] = useState('')
  const [data, setData] = useState<HistoryResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    if (!token) return
    setLoading(true)
    setError(null)
    try {
      const resp = await api.getHistory({ page: 1, pageSize: 20, gameType }, token)
      setData(resp)
    } catch (err) {
      setError(err instanceof Error ? err.message : '查询失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, gameType])

  if (!token) {
    return <p>请先登录后查看历史记录。</p>
  }

  return (
    <section>
      <h2>历史记录</h2>
      <div className="row-actions">
        <label>
          游戏类型
          <select value={gameType} onChange={(e) => setGameType(e.target.value)}>
            <option value="">全部</option>
            <option value="holdem_odds">德州胜率</option>
            <option value="holdem_allin_ev">德州全下EV</option>
            <option value="mahjong">四川麻将</option>
          </select>
        </label>
        <button onClick={load} disabled={loading}>
          刷新
        </button>
      </div>
      {error && <p className="error">{error}</p>}
      {data && (
        <table className="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>类型</th>
              <th>时间</th>
              <th>请求</th>
              <th>响应</th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((item) => (
              <tr key={item.id}>
                <td>{item.id}</td>
                <td>{item.gameType}</td>
                <td>{item.createdAt}</td>
                <td>
                  <details>
                    <summary>查看</summary>
                    <pre>{item.requestJson}</pre>
                  </details>
                </td>
                <td>
                  <details>
                    <summary>查看</summary>
                    <pre>{item.responseJson}</pre>
                  </details>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}
