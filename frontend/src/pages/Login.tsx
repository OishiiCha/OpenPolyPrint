import { useState } from 'react'
import { Lock, Loader2 } from 'lucide-react'

interface LoginProps {
  onLogin: (passcode: string) => Promise<boolean>
}

export function Login({ onLogin }: LoginProps) {
  const [passcode, setPasscode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    const ok = await onLogin(passcode)
    if (!ok) {
      setError('Invalid passcode')
    }
    setLoading(false)
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-50 dark:bg-slate-950">
      <div className="w-full max-w-sm">
        <div className="rounded-2xl border border-slate-200 bg-white p-8 shadow-lg dark:border-slate-800 dark:bg-slate-900">
          <div className="mb-6 text-center">
            <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-blue-600">
              <Lock className="h-7 w-7 text-white" />
            </div>
            <h1 className="text-xl font-semibold text-slate-900 dark:text-white">OpenPolyPrint</h1>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">Enter your passcode to continue</p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <input
                type="password"
                value={passcode}
                onChange={(e) => setPasscode(e.target.value)}
                placeholder="Passcode"
                autoFocus
                className="w-full rounded-lg border border-slate-300 px-4 py-2.5 text-sm text-slate-900 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 dark:border-slate-700 dark:bg-slate-800 dark:text-white"
              />
            </div>

            {error && (
              <p className="text-center text-sm text-rose-500">{error}</p>
            )}

            <button
              type="submit"
              disabled={loading || !passcode}
              className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 py-2.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
            >
              {loading ? (
                <><Loader2 className="h-4 w-4 animate-spin" /> Authenticating...</>
              ) : (
                'Unlock'
              )}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
