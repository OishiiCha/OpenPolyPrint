import { useEffect, useState, useCallback } from 'react'

interface AuthState {
  enabled: boolean
  authenticated: boolean
  loading: boolean
}

export function useAuth() {
  const [state, setState] = useState<AuthState>({
    enabled: false,
    authenticated: false,
    loading: true,
  })

  const checkAuth = useCallback(async () => {
    try {
      // Check if auth is enabled
      const statusRes = await fetch('/api/auth/status')
      if (!statusRes.ok) {
        setState({ enabled: false, authenticated: true, loading: false })
        return
      }
      const status = await statusRes.json()
      if (!status.enabled) {
        setState({ enabled: false, authenticated: true, loading: false })
        return
      }
      // Auth is enabled — check if we're authenticated by trying a protected endpoint
      const testRes = await fetch('/api/config')
      if (testRes.ok) {
        setState({ enabled: true, authenticated: true, loading: false })
      } else {
        setState({ enabled: true, authenticated: false, loading: false })
      }
    } catch {
      setState({ enabled: false, authenticated: true, loading: false })
    }
  }, [])

  useEffect(() => {
    checkAuth()
  }, [checkAuth])

  const login = useCallback(async (passcode: string): Promise<boolean> => {
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ passcode }),
      })
      if (res.ok) {
        setState({ enabled: true, authenticated: true, loading: false })
        return true
      }
      return false
    } catch {
      return false
    }
  }, [])

  const logout = useCallback(async () => {
    try {
      await fetch('/api/auth/logout', { method: 'POST' })
    } catch {
      // ignore
    }
    setState({ enabled: true, authenticated: false, loading: false })
  }, [])

  return { ...state, login, logout, checkAuth }
}
