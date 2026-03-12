import { useState } from 'react'
import { api } from '../api/client'
import type { MahjongAnalyzeResponse } from '../types/api'

interface MahjongPageProps {
  token?: string
}

function parseTiles(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((x) => x.trim())
    .filter(Boolean)
}

function parseMelds(raw: string): string[][] {
  return raw
    .split('\n')
    .map((line) => parseTiles(line))
    .filter((line) => line.length > 0)
}

export function MahjongPage({ token }: MahjongPageProps) {
  const [handInput, setHandInput] = useState('m1 m1 m2 m2 m3 m3 p4 p4 p5 p5 s6 s6 s7 s8')
  const [meldInput, setMeldInput] = useState('')
  const [visibleInput, setVisibleInput] = useState('')
  const [missingSuit, setMissingSuit] = useState('')
  const [result, setResult] = useState<MahjongAnalyzeResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const runAnalyze = async () => {
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const resp = await api.analyzeMahjong(
        {
          handTiles: parseTiles(handInput),
          melds: parseMelds(meldInput),
          visibleTiles: parseTiles(visibleInput),
          missingSuit
        },
        token
      )
      setResult(resp)
    } catch (err) {
      setError(err instanceof Error ? err.message : '分析失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <section>
      <h2>四川麻将（血战到底 + 缺一门）</h2>
      <p className="hint">手牌输入支持 `m/p/s + 1..9`，例如 `m1 p9 s5`。</p>
      <div className="form-grid">
        <label>
          手牌(13或14张)
          <textarea rows={4} value={handInput} onChange={(e) => setHandInput(e.target.value)} />
        </label>
        <label>
          副露(每行一组, 可选)
          <textarea rows={3} value={meldInput} onChange={(e) => setMeldInput(e.target.value)} placeholder="m1 m1 m1" />
        </label>
        <label>
          可见牌(可选)
          <textarea rows={3} value={visibleInput} onChange={(e) => setVisibleInput(e.target.value)} />
        </label>
        <label>
          缺门
          <select value={missingSuit} onChange={(e) => setMissingSuit(e.target.value)}>
            <option value="">无</option>
            <option value="m">万(m)</option>
            <option value="p">筒(p)</option>
            <option value="s">条(s)</option>
          </select>
        </label>
      </div>

      <div className="row-actions">
        <button onClick={runAnalyze} disabled={loading}>
          分析两摸番数期望
        </button>
      </div>
      {error && <p className="error">{error}</p>}

      {result && (
        <div>
          <h3>分析结果</h3>
          <p>
            是否听牌: {result.isTing ? '是' : '否'} | 两摸胡牌概率: {(result.nextTwoDrawHuProb * 100).toFixed(2)}% | 耗时:{' '}
            {result.elapsedMs} ms
          </p>
          <p>有效牌: {result.winningTiles.join(' ') || '无'}</p>
          <table className="data-table">
            <thead>
              <tr>
                <th>推荐打牌</th>
                <th>ExpectedFan</th>
                <th>HuProb</th>
                <th>番型贡献</th>
              </tr>
            </thead>
            <tbody>
              {result.discardRecommendations.map((item) => (
                <tr key={item.tile}>
                  <td>{item.tile}</td>
                  <td>{item.expectedFan.toFixed(4)}</td>
                  <td>{(item.huProb * 100).toFixed(2)}%</td>
                  <td>
                    {item.topFanBreakdown.map((f) => `${f.fan}:${f.contribution.toFixed(4)}`).join(' | ') || '无'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
