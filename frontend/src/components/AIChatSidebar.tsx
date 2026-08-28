import { useEffect, useRef, useState } from 'react'
import { X, Send, Sparkles, Loader2, Trash2 } from 'lucide-react'

interface ChatMessage {
  role: string
  text: string
  hasImage: boolean
  imagePath?: string
  imageMime?: string
  timestamp: string
}

interface ChatConversation {
  id: string
  printerId: string
  printerName: string
  file: string
  createdAt: string
  messages: ChatMessage[]
}

interface AIChatSidebarProps {
  printerId: string
  printerName: string
  onClose: () => void
}

export function AIChatSidebar({ printerId, printerName, onClose }: AIChatSidebarProps) {
  const [conversation, setConversation] = useState<ChatConversation | null>(null)
  const [loading, setLoading] = useState(false)
  const [input, setInput] = useState('')
  const [error, setError] = useState<string | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // Start a new conversation when the sidebar opens
  useEffect(() => {
    setError(null)
    setLoading(true)
    fetch('/api/ai/chat/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ printerId, printerName }),
    })
      .then((r) => {
        if (!r.ok) return r.json().then((d) => { throw new Error(d.error || 'Failed to start analysis') })
        return r.json()
      })
      .then((data: ChatConversation) => setConversation(data))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [printerId, printerName])

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [conversation?.messages])

  const sendMessage = async () => {
    if (!input.trim() || !conversation || loading) return
    const msg = input.trim()
    setInput('')
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`/api/ai/chat/${conversation.id}/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: msg }),
      })
      if (!res.ok) {
        const d = await res.json()
        throw new Error(d.error || 'Failed to send message')
      }
      const updated = await res.json() as ChatConversation
      setConversation(updated)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to send')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async () => {
    if (!conversation) return
    if (!confirm('Delete this conversation?')) return
    await fetch(`/api/ai/chat/${conversation.id}/delete`, { method: 'DELETE' })
    onClose()
  }

  return (
    <div className="fixed right-0 top-0 z-50 flex h-full w-full max-w-md flex-col border-l border-slate-200 bg-white shadow-2xl dark:border-slate-800 dark:bg-slate-950">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-200 p-4 dark:border-slate-800">
        <div className="flex items-center gap-2">
          <Sparkles className="h-5 w-5 text-blue-500" />
          <div>
            <h2 className="text-sm font-semibold text-slate-900 dark:text-white">AI Print Analysis</h2>
            <p className="text-xs text-slate-400">{printerName}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {conversation && (
            <button
              onClick={handleDelete}
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-rose-500 dark:hover:bg-slate-800"
              title="Delete conversation"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {loading && !conversation && (
          <div className="flex h-full items-center justify-center">
            <div className="text-center">
              <Loader2 className="mx-auto h-8 w-8 animate-spin text-blue-500" />
              <p className="mt-2 text-xs text-slate-400">Capturing frame and analyzing...</p>
            </div>
          </div>
        )}

        {error && !conversation && (
          <div className="rounded-lg border border-rose-300 bg-rose-50 p-3 text-sm text-rose-600 dark:border-rose-700 dark:bg-rose-900/20 dark:text-rose-400">
            {error}
          </div>
        )}

        {conversation?.messages.map((msg, idx) => (
          <div
            key={idx}
            className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            <div
              className={`max-w-[85%] rounded-lg p-3 text-sm ${
                msg.role === 'user'
                  ? 'bg-blue-600 text-white'
                  : 'bg-slate-100 text-slate-900 dark:bg-slate-800 dark:text-slate-100'
              }`}
            >
              {/* Image if present */}
              {msg.hasImage && msg.imagePath && (
                <img
                  src={`/api/ai/chat/${conversation.id}/image?path=${encodeURIComponent(msg.imagePath)}`}
                  alt="Captured frame"
                  className="mb-2 rounded-lg max-h-48 w-full object-cover"
                />
              )}
              {/* Text — render markdown-ish (split by ** for bold) */}
              <div className="whitespace-pre-wrap break-words">
                {renderText(msg.text)}
              </div>
            </div>
          </div>
        ))}

        {loading && conversation && (
          <div className="flex justify-start">
            <div className="rounded-lg bg-slate-100 p-3 dark:bg-slate-800">
              <Loader2 className="h-4 w-4 animate-spin text-slate-400" />
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      {conversation && (
        <div className="border-t border-slate-200 p-4 dark:border-slate-800">
          {error && (
            <p className="mb-2 text-xs text-rose-500">{error}</p>
          )}
          <div className="flex gap-2">
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && sendMessage()}
              placeholder="Ask about the print..."
              disabled={loading}
              className="flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
            />
            <button
              onClick={sendMessage}
              disabled={loading || !input.trim()}
              className="rounded-lg bg-blue-600 p-2 text-white hover:bg-blue-500 disabled:opacity-50"
            >
              <Send className="h-4 w-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// renderText renders simple markdown-like formatting (bold, headers, lists)
function renderText(text: string): React.ReactNode {
  const lines = text.split('\n')
  return lines.map((line, idx) => {
    // Render **bold** segments
    const parts = line.split(/(\*\*[^*]+\*\*)/g)
    return (
      <span key={idx}>
        {parts.map((part, i) => {
          if (part.startsWith('**') && part.endsWith('**')) {
            return <strong key={i}>{part.slice(2, -2)}</strong>
          }
          return <span key={i}>{part}</span>
        })}
        {idx < lines.length - 1 && '\n'}
      </span>
    )
  })
}
