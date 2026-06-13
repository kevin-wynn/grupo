import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, type AuthStatus, type Me } from '../api'

type AuthState = {
  loading: boolean
  status: AuthStatus | null
  user: Me | null
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [loading, setLoading] = useState(true)
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [user, setUser] = useState<Me | null>(null)

  const refresh = useCallback(async () => {
    const s = await api.authStatus()
    setStatus(s)
    if (s.skip_github_auth || s.logged_in) {
      try {
        setUser(await api.me())
      } catch {
        setUser(null)
      }
    } else {
      setUser(null)
    }
  }, [])

  useEffect(() => {
    refresh()
      .catch(() => setStatus(null))
      .finally(() => setLoading(false))
  }, [refresh])

  return (
    <AuthContext.Provider value={{ loading, status, user, refresh }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
