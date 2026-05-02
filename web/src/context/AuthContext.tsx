import React, { createContext, useContext, useEffect, useState, useCallback } from 'react'
import { authApi, User } from '@/api/auth'

interface AuthContextValue {
  user: User | null
  isLoading: boolean
  login: (email: string, password: string) => Promise<User>
  logout: () => Promise<void>
  refreshMe: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    authApi.me()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setIsLoading(false))
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    const u = await authApi.login(email, password)
    setUser(u)
    return u
  }, [])

  const logout = useCallback(async () => {
    await authApi.logout().catch(() => {})
    setUser(null)
  }, [])

  const refreshMe = useCallback(async () => {
    const u = await authApi.me()
    setUser(u)
  }, [])

  return (
    <AuthContext.Provider value={{ user, isLoading, login, logout, refreshMe }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
