import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { logout as logoutRequest } from './api'
import { clearSession, readSession, type StoredSession } from '../../lib/authStorage'

type AuthContextValue = {
  session: StoredSession | null
  setSession: (session: StoredSession | null) => void
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSessionState] = useState<StoredSession | null>(() => readSession())

  const setSession = useCallback((next: StoredSession | null) => {
    setSessionState(next)
  }, [])

  const logout = useCallback(async () => {
    await logoutRequest()
    clearSession()
    setSessionState(null)
  }, [])

  const value = useMemo(
    () => ({
      session,
      setSession,
      logout,
    }),
    [session, setSession, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
