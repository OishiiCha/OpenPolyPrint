import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Send, Sparkles, Loader2, Trash2, Camera, Link2, Plus, MessageSquare } from 'lucide-react'
import type { Printer } from '../types'

interface ChatMessage {
  role: string
  text: string
  hasImage: boolean
  imagePaths?: string[]
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

interface AIChatPaneProps {
  printers: Printer[]
}

export function AIChatPane({ printers }: AIChatPaneProps) {
  const [conversation, setConversation] = useState<ChatConversation | null>(null)
  const [loading, setLoading] = useState(false)
  const [input, setInput] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [linkedPrinterId, setLinkedPrinterId] = useState<string>('')
  const [showPrinterSelect, setShowPrinterSelect] = useState(false)
  const [attachSnapshot, setAttachSnapshot] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // Start a new conversation
  const startNewChat = async () => {
    setError(null)
    setLoading(true)
    setConversation(null)
    try {
      const res = await fetch('/api/ai/chat/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ printerId: linkedPrinterId || '' }),
      })
      if (!res.ok) throw new Error('Failed to start chat')
      const data = await res.json() as ChatConversation
      setConversation(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to start chat')
    } finally {
      setLoading(false)
    }
  }

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [conversation?.messages])

  const sendMessage = async () => {
    if (!input.trim() || !conversation || loading) return
    if (attachSnapshot && !linkedPrinterId) {
      setError('Link a printer first to attach a snapshot')
      return
    }
    const msg = input.trim()
    setInput('')
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`/api/ai/chat/${conversation.id}/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: msg,
          attachSnapshot,
          printerId: linkedPrinterId,
        }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({ error: 'Failed to send' }))
        throw new Error(d.error || 'Failed to send')
      }
      const updated = await res.json() as ChatConversation
      setConversation(updated)
      setAttachSnapshot(false) // reset after sending
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
    setConversation(null)
  }

  const linkedPrinter = printers.find((p) => p.id === linkedPrinterId)

  return (
    <div className="flex flex-col rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-200 p-4 dark:border-slate-800">
        <div className="flex items-center gap-2">
          <div className="rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 p-1.5">
            <Sparkles className="h-4 w-4 text-white" />
          </div>
          <div>
            <h2 className="text-sm font-semibold text-slate-900 dark:text-white">AI Print Assistant</h2>
            <p className="text-xs text-slate-400">
              {conversation ? `${conversation.messages.length} messages` : 'No active chat'}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {conversation && (
            <>
              <button
                onClick={handleDelete}
                className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-rose-500 dark:hover:bg-slate-800"
                title="Delete conversation"
              >
                <Trash2 className="h-4 w-4" />
              </button>
              <button
                onClick={startNewChat}
                className="flex items-center gap-1.5 rounded-lg bg-slate-100 px-2.5 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                title="New chat"
              >
                <Plus className="h-3.5 w-3.5" /> New
              </button>
            </>
          )}
        </div>
      </div>

      {/* Printer linking bar */}
      <div className="flex items-center gap-2 border-b border-slate-200 px-4 py-2 dark:border-slate-800">
        <Link2 className="h-4 w-4 text-slate-400" />
        {linkedPrinter ? (
          <span className="flex items-center gap-1.5 text-xs font-medium text-slate-700 dark:text-slate-300">
            <span className="h-2 w-2 rounded-full bg-emerald-500" />
            {linkedPrinter.name}
            <button
              onClick={() => setShowPrinterSelect(!showPrinterSelect)}
              className="ml-1 text-blue-500 hover:underline"
            >
              change
            </button>
          </span>
        ) : (
          <button
            onClick={() => setShowPrinterSelect(!showPrinterSelect)}
            className="text-xs font-medium text-blue-500 hover:underline"
          >
            Link a printer
          </button>
        )}
        {showPrinterSelect && (
          <div className="relative z-10 ml-auto">
            <select
              value={linkedPrinterId}
              onChange={(e) => { setLinkedPrinterId(e.target.value); setShowPrinterSelect(false) }}
              className="rounded-lg border border-slate-300 bg-white px-2 py-1 text-xs text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
            >
              <option value="">No printer</option>
              {printers.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
          </div>
        )}
      </div>

      {/* Messages area */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4" style={{ minHeight: '300px', maxHeight: '500px' }}>
        {!conversation && !loading && (
          <div className="flex h-full flex-col items-center justify-center text-center">
            <MessageSquare className="h-10 w-10 text-slate-300 dark:text-slate-700" />
            <p className="mt-3 text-sm text-slate-400">Start a new chat to analyze your prints</p>
            <button
              onClick={startNewChat}
              className="mt-4 flex items-center gap-2 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 px-4 py-2 text-sm font-medium text-white hover:from-blue-500 hover:to-purple-500"
            >
              <Sparkles className="h-4 w-4" /> Start AI Chat
            </button>
          </div>
        )}

        {loading && !conversation && (
          <div className="flex h-full items-center justify-center">
            <div className="text-center">
              <Loader2 className="mx-auto h-8 w-8 animate-spin text-blue-500" />
              <p className="mt-2 text-xs text-slate-400">Starting chat...</p>
            </div>
          </div>
        )}

        {error && !conversation && (
          <div className="rounded-lg border border-rose-300 bg-rose-50 p-3 text-sm text-rose-600 dark:border-rose-700 dark:bg-rose-900/20 dark:text-rose-400">
            {error}
          </div>
        )}

        {conversation?.messages?.map((msg, idx) => (
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
              {/* Images if present */}
              {msg.hasImage && msg.imagePaths && msg.imagePaths.length > 0 && (
                <div className="mb-2 space-y-1">
                  {msg.imagePaths.map((imgPath, imgIdx) => (
                    <img
                      key={imgIdx}
                      src={`/api/ai/chat/${conversation.id}/image?path=${encodeURIComponent(imgPath)}`}
                      alt={`Frame ${imgIdx + 1}`}
                      className="rounded-lg max-h-48 w-full object-cover"
                    />
                  ))}
                </div>
              )}
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

      {/* Input area */}
      {conversation && (
        <div className="border-t border-slate-200 p-4 dark:border-slate-800">
          {error && (
            <p className="mb-2 text-xs text-rose-500">{error}</p>
          )}
          {/* Snapshot toggle */}
          <div className="mb-2 flex items-center gap-2">
            <button
              onClick={() => setAttachSnapshot(!attachSnapshot)}
              className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors ${
                attachSnapshot
                  ? 'bg-blue-600 text-white'
                  : 'bg-slate-100 text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700'
              }`}
              title="Attach live camera frame + printer data"
            >
              <Camera className="h-3.5 w-3.5" />
              {attachSnapshot ? 'Snapshot attached' : 'Attach snapshot'}
            </button>
            {attachSnapshot && linkedPrinter && (
              <span className="text-xs text-slate-400">
                from {linkedPrinter.name}
              </span>
            )}
            {attachSnapshot && !linkedPrinterId && (
              <span className="text-xs text-amber-500">Link a printer first</span>
            )}
          </div>
          <div className="flex gap-2">
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && sendMessage()}
              placeholder="Ask about your print..."
              disabled={loading}
              className="flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
            />
            <button
              onClick={sendMessage}
              disabled={loading || (!input.trim() && !attachSnapshot)}
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
function renderText(text: string): ReactNode {
  const lines = text.split('\n')
  return lines.map((line, idx) => {
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
