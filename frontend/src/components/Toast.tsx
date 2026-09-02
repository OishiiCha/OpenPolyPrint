import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { Circle, X, AlertCircle, CheckCircle2 } from 'lucide-react'

interface ToastItem {
  id: number
  type: 'success' | 'error' | 'info'
  message: string
}

let toastId = 0

export function ToastContainer() {
  const [toasts, setToasts] = useState<ToastItem[]>([])

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail as { type: 'success' | 'error' | 'info'; message: string }
      const id = ++toastId
      setToasts((prev) => [...prev, { id, type: detail.type, message: detail.message }])
      // Auto-dismiss after 5 seconds
      setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id))
      }, 5000)
    }
    window.addEventListener('openpolyprint-toast', handler)
    return () => window.removeEventListener('openpolyprint-toast', handler)
  }, [])

  const dismiss = (id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }

  if (toasts.length === 0) return null

  return createPortal(
    <div className="fixed bottom-4 right-4 z-[9999] flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`flex items-center gap-3 rounded-lg border px-4 py-3 shadow-lg backdrop-blur-sm ${
            t.type === 'success'
              ? 'border-emerald-600/50 bg-emerald-950/90 text-emerald-200'
              : t.type === 'error'
              ? 'border-rose-600/50 bg-rose-950/90 text-rose-200'
              : 'border-blue-600/50 bg-blue-950/90 text-blue-200'
          }`}
          style={{ minWidth: '280px', maxWidth: '420px' }}
        >
          {t.type === 'success' ? (
            <CheckCircle2 className="h-5 w-5 flex-shrink-0 text-emerald-400" />
          ) : t.type === 'error' ? (
            <AlertCircle className="h-5 w-5 flex-shrink-0 text-rose-400" />
          ) : (
            <Circle className="h-5 w-5 flex-shrink-0 text-blue-400" />
          )}
          <p className="flex-1 font-mono text-sm">{t.message}</p>
          <button
            onClick={() => dismiss(t.id)}
            className="flex-shrink-0 text-slate-400 hover:text-white"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      ))}
    </div>,
    document.body
  )
}
