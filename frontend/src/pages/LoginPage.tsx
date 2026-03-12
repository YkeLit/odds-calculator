import { FormEvent, useState } from 'react'
import { api } from '../api/client'
import type { UserInfo } from '../types/api'

interface LoginPageProps {
  onAuthSuccess: (token: string, user: UserInfo) => void
}

export function LoginPage({ onAuthSuccess }: LoginPageProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const submit = async (mode: 'login' | 'register') => {
    setLoading(true)
    setError(null)
    try {
      const resp =
        mode === 'register' ? await api.register(username, password) : await api.login(username, password)
      onAuthSuccess(resp.accessToken, resp.user)
    } catch (err) {
      setError(err instanceof Error ? err.message : '请求失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <section>
      <h2>账号登录</h2>
      <form
        className="form-grid"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          void submit('login')
        }}
      >
        <label>
          用户名
          <input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="alice" required />
        </label>
        <label>
          密码
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="至少6位"
            required
          />
        </label>
        <div className="row-actions">
          <button type="submit" disabled={loading}>
            登录
          </button>
          <button type="button" disabled={loading} onClick={() => void submit('register')}>
            注册
          </button>
        </div>
        {error && <p className="error">{error}</p>}
      </form>
      <p className="hint">提示: 注册成功后会自动登录。</p>
    </section>
  )
}
