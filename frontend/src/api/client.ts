import type {
  AuthResponse,
  HistoryResponse,
  HoldemAllInEVRequest,
  HoldemAllInEVResponse,
  HoldemOddsRequest,
  HoldemOddsResponse,
  HoldemDecisionRequest,
  HoldemDecisionResponse,
  MahjongAnalyzeRequest,
  MahjongAnalyzeResponse
} from '../types/api'

const runtimeApiBase = window.__APP_CONFIG__?.VITE_API_BASE_URL?.trim()
const buildTimeApiBase = import.meta.env.VITE_API_BASE_URL?.trim()
const API_BASE = runtimeApiBase || buildTimeApiBase || ''

async function request<T>(path: string, init: RequestInit = {}, token?: string): Promise<T> {
  const headers = new Headers(init.headers)
  if (!headers.has('Content-Type') && init.body) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers
  })

  if (!response.ok) {
    const text = await response.text()
    let message = response.statusText
    if (text) {
      try {
        const parsed = JSON.parse(text) as { error?: string }
        message = parsed.error ?? text
      } catch {
        message = text
      }
    }
    throw new Error(message || `Request failed with ${response.status}`)
  }

  return (await response.json()) as T
}

export const api = {
  register: (username: string, password: string) =>
    request<AuthResponse>('/api/v1/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, password })
    }),

  login: (username: string, password: string) =>
    request<AuthResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password })
    }),

  calculateOdds: (payload: HoldemOddsRequest, token?: string) =>
    request<HoldemOddsResponse>(
      '/api/v1/holdem/odds',
      {
        method: 'POST',
        body: JSON.stringify(payload)
      },
      token
    ),

  calculateAllInEV: (payload: HoldemAllInEVRequest, token?: string) =>
    request<HoldemAllInEVResponse>(
      '/api/v1/holdem/allin-ev',
      {
        method: 'POST',
        body: JSON.stringify(payload)
      },
      token
    ),

  holdemDecision: (payload: HoldemDecisionRequest, token?: string) =>
    request<HoldemDecisionResponse>(
      '/api/v1/holdem/decision',
      {
        method: 'POST',
        body: JSON.stringify(payload)
      },
      token
    ),

  analyzeMahjong: (payload: MahjongAnalyzeRequest, token?: string) =>
    request<MahjongAnalyzeResponse>(
      '/api/v1/mahjong/analyze',
      {
        method: 'POST',
        body: JSON.stringify(payload)
      },
      token
    ),

  getHistory: (params: { page?: number; pageSize?: number; gameType?: string }, token: string) => {
    const query = new URLSearchParams()
    if (params.page) query.set('page', String(params.page))
    if (params.pageSize) query.set('pageSize', String(params.pageSize))
    if (params.gameType) query.set('gameType', params.gameType)
    return request<HistoryResponse>(`/api/v1/history?${query.toString()}`, { method: 'GET' }, token)
  }
}
