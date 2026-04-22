import { createContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import { login as apiLogin, logout as apiLogout, setOnUnauthorized, setAuthToken } from '@/lib/api/client'
import type { User } from '@/lib/api/types'

interface AuthContextType {
  user: User | null
  isAuthenticated: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

export const AuthContext = createContext<AuthContextType | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(() => {
    const stored = sessionStorage.getItem('ct-user')
    return stored ? JSON.parse(stored) as User : null
  })

  const handleLogout = useCallback(async () => {
    try {
      await apiLogout()
    } catch {
      // ignore errors on logout
    }
    setUser(null)
    sessionStorage.removeItem('ct-user')
    setAuthToken(null)
  }, [])

  useEffect(() => {
    setOnUnauthorized(() => {
      setUser(null)
      sessionStorage.removeItem('ct-user')
      setAuthToken(null)
    })
  }, [])

  const handleLogin = useCallback(async (username: string, password: string) => {
    const result = await apiLogin({ username, password })
    setAuthToken(result.token)
    setUser(result.user)
    sessionStorage.setItem('ct-user', JSON.stringify(result.user))
  }, [])

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated: user !== null,
        login: handleLogin,
        logout: handleLogout,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

