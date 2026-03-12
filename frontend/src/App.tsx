import { useMemo, useState } from 'react'
import type { UserInfo } from './types/api'
import { LoginPage } from './pages/LoginPage'
import { HoldemPage } from './pages/HoldemPage'
import { MahjongPage } from './pages/MahjongPage'
import { HistoryPage } from './pages/HistoryPage'

type TabKey = 'holdem' | 'mahjong' | 'history' | 'auth'

function readStoredUser(): UserInfo | null {
  const raw = localStorage.getItem('odds_user')
  if (!raw) return null
  try {
    return JSON.parse(raw) as UserInfo
  } catch {
    return null
  }
}

export default function App() {
  const [token, setToken] = useState<string | null>(localStorage.getItem('odds_token'))
  const [user, setUser] = useState<UserInfo | null>(readStoredUser())
  const [tab, setTab] = useState<TabKey>('holdem')

  const tabs = useMemo(
    () => [
      { key: 'holdem' as const, label: '德州' },
      { key: 'mahjong' as const, label: '四川麻将' },
      { key: 'history' as const, label: '历史记录' },
      { key: 'auth' as const, label: token ? '账号' : '登录' }
    ],
    [token]
  )

  const onAuthSuccess = (nextToken: string, nextUser: UserInfo) => {
    setToken(nextToken)
    setUser(nextUser)
    localStorage.setItem('odds_token', nextToken)
    localStorage.setItem('odds_user', JSON.stringify(nextUser))
    setTab('holdem')
  }

  const logout = () => {
    setToken(null)
    setUser(null)
    localStorage.removeItem('odds_token')
    localStorage.removeItem('odds_user')
    setTab('auth')
  }

  return (
    <div className="app-shell">
      <header className="app-header">
        <div>
          <h1>智能策略助手</h1>
          <p>德州 GTO 决策引擎 · 四川麻将期望分析</p>
        </div>
        <div className="user-box">
          {user ? (
            <>
              <span>用户: {user.username}</span>
              <button onClick={logout}>退出</button>
            </>
          ) : (
            <span>未登录</span>
          )}
        </div>
      </header>

      <nav className="tabs">
        {tabs.map((item) => (
          <button
            key={item.key}
            className={tab === item.key ? 'active' : ''}
            onClick={() => setTab(item.key)}
          >
            {item.label}
          </button>
        ))}
      </nav>

      <main className="panel">
        {tab === 'holdem' && <HoldemPage token={token ?? undefined} />}
        {tab === 'mahjong' && <MahjongPage token={token ?? undefined} />}
        {tab === 'history' && <HistoryPage token={token ?? undefined} />}
        {tab === 'auth' && <LoginPage onAuthSuccess={onAuthSuccess} />}
      </main>
    </div>
  )
}
