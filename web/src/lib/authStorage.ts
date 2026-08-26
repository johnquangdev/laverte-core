const STORAGE_KEY = 'laverte_session'

export type StoredSession = {
  accessToken: string
  refreshToken: string
  expiresAt: number
  user: {
    id: number
    email: string
    role: string
  }
}

export function readSession(): StoredSession | null {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as StoredSession
  } catch {
    return null
  }
}

export function writeSession(session: StoredSession): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
}

export function clearSession(): void {
  localStorage.removeItem(STORAGE_KEY)
}

export function getAccessToken(): string | null {
  return readSession()?.accessToken ?? null
}
