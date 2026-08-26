import { apiFetch } from '../../lib/api'
import { clearSession, getAccessToken, readSession, writeSession, type StoredSession } from '../../lib/authStorage'

export type SessionResponse = {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
  user: {
    id: number
    email: string
    role: string
  }
}

export function toStoredSession(resp: SessionResponse): StoredSession {
  return {
    accessToken: resp.access_token,
    refreshToken: resp.refresh_token,
    expiresAt: Date.now() + resp.expires_in * 1000,
    user: resp.user,
  }
}

export async function fetchGoogleLoginUrl(): Promise<string> {
  const resp = await apiFetch<{ url: string }>('/api/v1/auth/google/login-url')
  return resp.url
}

export async function completeGoogleCallback(code: string, state: string): Promise<StoredSession> {
  const resp = await apiFetch<SessionResponse>('/api/v1/auth/google/callback', {
    method: 'POST',
    body: JSON.stringify({ code, state }),
  })
  const session = toStoredSession(resp)
  writeSession(session)
  return session
}

export async function refreshSession(): Promise<StoredSession | null> {
  const current = readSession()
  if (!current) return null
  const resp = await apiFetch<SessionResponse>('/api/v1/auth/refresh', {
    method: 'POST',
    body: JSON.stringify({ refresh_token: current.refreshToken }),
  })
  const session = toStoredSession(resp)
  writeSession(session)
  return session
}

export async function logout(): Promise<void> {
  const token = getAccessToken()
  if (token) {
    await apiFetch('/api/v1/auth/logout', { method: 'POST' }, token).catch(() => undefined)
  }
  clearSession()
}
