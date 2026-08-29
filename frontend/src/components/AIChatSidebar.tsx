import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Send, Sparkles, Loader2, Trash2, Camera, Link2, Plus, MessageSquare, PanelRightClose, PanelRightOpen, FileCode, X, ArrowLeft, Clock } from 'lucide-react'
import { usePrinters } from '../hooks/usePrinters'
import { useGCodeFiles } from '../hooks/useGCodeFiles'
import type { GCodeFile } from '../types'

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
  collapsed: boolean
  onToggle: () => void
}

export function AIChatPane({ collapsed, onToggle }: AIChatPaneProps) {
  const { printers } = usePrinters()
  const { files: gcodeFiles } = useGCodeFiles()
  const [conversation, setConversation] = useState<ChatConversation | null>(null)
  const [loading, setLoading] = useState(false)
  const [input, setInput] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [linkedPrinterId, setLinkedPrinterId] = useState<string>('')
  const [showPrinterSelect, setShowPrinterSelect] = useState(false)
  const [attachSnapshot, setAttachSnapshot] = useState(false)
  const [selectedGcodeId, setSelectedGcodeId] = useState<string>('')
  const [showGcodeSelect, setShowGcodeSelect] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // View state: 'list' = chat history + new chat button, 'setup' = new chat setup, 'chat' = active conversation
  const [view, setView] = useState<'list' | 'setup' | 'chat'>('list')
  const [history, setHistory] = useState<ChatConversation[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)

  // Setup form state
  const [setupPrinterId, setSetupPrinterId] = useState<string>('')
  const [setupGcodeId, setSetupGcodeId] = useState<string>('')
  const [setupInitialMsg, setSetupInitialMsg] = useState('')

  const loadHistory = async () => {
    setHistoryLoading(true)
    try {
      const res = await fetch('/api/ai/chat')
      if (!res.ok) return
      const data = await res.json()
      setHistory(Array.isArray(data) ? data : [])
    } catch {
      setHistory([])
    } finally {
      setHistoryLoading(false)
    }
  }

  useEffect(() => {
    if (view === 'list') loadHistory()
  }, [view])

  // Start a new conversation from setup
  const startNewChat = async () => {
    setError(null)
    setLoading(true)
    try {
      const printer = printers.find((p) => p.id === setupPrinterId)
      const res = await fetch('/api/ai/chat/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          printerId: setupPrinterId || '',
          printerName: printer?.name || '',
        }),
      })
      if (!res.ok) throw new Error('Failed to start chat')
      const data = await res.json() as ChatConversation
      if (!data.messages) data.messages = []
      setConversation(data)
      setLinkedPrinterId(setupPrinterId)
      setView('chat')

      // If there's an initial message or gcode, send it immediately
      if (setupInitialMsg.trim() || setupGcodeId) {
        const gcodeFile = gcodeFiles.find((f) => f.id === setupGcodeId)
        const sendRes = await fetch(`/api/ai/chat/${data.id}/send`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            message: setupInitialMsg.trim() || 'Please analyze this G-code file.',
            attachSnapshot: false,
            printerId: setupPrinterId,
            gcodeFileId: setupGcodeId,
            gcodeFileName: gcodeFile?.name || '',
          }),
        })
        if (sendRes.ok) {
          const updated = await sendRes.json() as ChatConversation
          if (!updated.messages) updated.messages = []
          setConversation(updated)
        }
      }
      setSetupInitialMsg('')
      setSetupGcodeId('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to start chat')
    } finally {
      setLoading(false)
    }
  }

  // Load an existing conversation
  const loadConversation = async (id: string) => {
    setError(null)
    setLoading(true)
    try {
      const res = await fetch(`/api/ai/chat/${id}`)
      if (!res.ok) throw new Error('Failed to load chat')
      const data = await res.json() as ChatConversation
      if (!data.messages) data.messages = []
      setConversation(data)
      setLinkedPrinterId(data.printerId || '')
      setView('chat')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load chat')
    } finally {
      setLoading(false)
    }
  }

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [conversation?.messages])

  const sendMessage = async () => {
    if ((!input.trim() && !attachSnapshot && !selectedGcodeId) || !conversation || loading) return
    if (attachSnapshot && !linkedPrinterId) {
      setError('Link a printer first to attach a snapshot')
      return
    }
    const msg = input.trim()
    const gcodeId = selectedGcodeId
    const gcodeFile = gcodeFiles.find((f) => f.id === gcodeId)
    setInput('')
    setSelectedGcodeId('')
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
          gcodeFileId: gcodeId,
          gcodeFileName: gcodeFile?.name || '',
        }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({ error: `Failed (HTTP ${res.status})` }))
        throw new Error(d.error || `Failed (HTTP ${res.status})`)
      }
      const updated = await res.json() as ChatConversation
      if (!updated.messages) updated.messages = []
      setConversation(updated)
      setAttachSnapshot(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to send')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this conversation?')) return
    await fetch(`/api/ai/chat/${id}/delete`, { method: 'DELETE' })
    if (conversation?.id === id) {
      setConversation(null)
      setView('list')
    } else {
      loadHistory()
    }
  }

  const linkedPrinter = printers.find((p) => p.id === linkedPrinterId)

  // Collapsed state
  if (collapsed) {
    return (
      <aside className="flex w-12 flex-col items-center border-l border-slate-300 bg-slate-100 py-4 dark:border-slate-700 dark:bg-slate-950">
        <button
          onClick={onToggle}
          className="rounded-lg p-2 text-slate-500 hover:bg-slate-200 dark:text-slate-400 dark:hover:bg-slate-800"
          title="Open AI Chat"
        >
          <PanelRightOpen className="h-5 w-5" />
        </button>
        <div className="mt-4 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 p-1.5">
          <Sparkles className="h-4 w-4 text-white" />
        </div>
        <span className="mt-2 font-mono text-[9px] text-slate-400 [writing-mode:vertical-rl]">AI Chat</span>
      </aside>
    )
  }

  return (
    <aside className="flex w-80 flex-col border-l border-slate-300 bg-white dark:border-slate-700 dark:bg-slate-950">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-200 p-3 dark:border-slate-800">
        <div className="flex items-center gap-2">
          {view === 'chat' && (
            <button
              onClick={() => { setView('list'); setConversation(null) }}
              className="rounded-lg p-1 text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"
              title="Back to chat list"
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
          )}
          <div className="rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 p-1.5">
            <Sparkles className="h-4 w-4 text-white" />
          </div>
          <div>
            <h2 className="text-sm font-semibold text-slate-900 dark:text-white">
              {view === 'chat' ? 'Chat' : view === 'setup' ? 'New Chat' : 'AI Assistant'}
            </h2>
            <p className="text-xs text-slate-400">
              {view === 'chat' && conversation ? `${conversation.messages?.length ?? 0} messages` : 'Print analysis'}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-1">
          {view === 'chat' && conversation && (
            <>
              <button
                onClick={() => handleDelete(conversation.id)}
                className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-rose-500 dark:hover:bg-slate-800"
                title="Delete conversation"
              >
                <Trash2 className="h-4 w-4" />
              </button>
              <button
                onClick={() => { setConversation(null); setView('setup') }}
                className="flex items-center gap-1 rounded-lg bg-slate-100 px-2 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                title="New chat"
              >
                <Plus className="h-3.5 w-3.5" />
              </button>
            </>
          )}
          <button
            onClick={onToggle}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"
            title="Collapse"
          >
            <PanelRightClose className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* ═══ LIST VIEW: Chat history ═══ */}
      {view === 'list' && (
        <div className="flex-1 overflow-y-auto">
          {/* New chat button */}
          <div className="p-4">
            <button
              onClick={() => setView('setup')}
              className="flex w-full items-center justify-center gap-2 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 px-4 py-2.5 text-sm font-medium text-white hover:from-blue-500 hover:to-purple-500"
            >
              <Plus className="h-4 w-4" /> New Chat
            </button>
          </div>

          {error && (
            <div className="mx-4 mb-2 rounded-lg border border-rose-300 bg-rose-50 p-3 text-sm text-rose-600 dark:border-rose-700 dark:bg-rose-900/20 dark:text-rose-400">
              {error}
            </div>
          )}

          {/* History list */}
          <div className="px-4 pb-4">
            <h3 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase text-slate-400">
              <Clock className="h-3.5 w-3.5" /> Recent
            </h3>
            {historyLoading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-slate-400" />
              </div>
            ) : history.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-8 text-center">
                <MessageSquare className="h-8 w-8 text-slate-300 dark:text-slate-700" />
                <p className="mt-2 text-xs text-slate-400">No conversations yet</p>
              </div>
            ) : (
              <div className="space-y-1.5">
                {history.map((conv) => (
                  <div
                    key={conv.id}
                    className="group flex items-center gap-2 rounded-lg border border-slate-200 bg-white p-2.5 transition-colors hover:border-blue-300 dark:border-slate-700 dark:bg-slate-900 dark:hover:border-blue-700"
                  >
                    <button
                      onClick={() => loadConversation(conv.id)}
                      className="flex flex-1 flex-col items-start text-left"
                    >
                      <span className="text-xs font-medium text-slate-900 dark:text-white">
                        {conv.printerName || 'General'}
                      </span>
                      <span className="text-[10px] text-slate-400">
                        {conv.messages?.length ?? 0} msgs · {formatDate(conv.createdAt)}
                      </span>
                      {conv.messages?.[0]?.text && (
                        <span className="mt-0.5 line-clamp-1 text-[10px] text-slate-400">
                          {conv.messages[0].text.slice(0, 60)}
                        </span>
                      )}
                    </button>
                    <button
                      onClick={() => handleDelete(conv.id)}
                      className="shrink-0 rounded p-1 text-slate-300 opacity-0 transition-opacity hover:text-rose-500 group-hover:opacity-100"
                      title="Delete"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ═══ SETUP VIEW: New chat configuration ═══ */}
      {view === 'setup' && (
        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {error && (
            <div className="rounded-lg border border-rose-300 bg-rose-50 p-3 text-sm text-rose-600 dark:border-rose-700 dark:bg-rose-900/20 dark:text-rose-400">
              {error}
            </div>
          )}

          {/* Printer selection */}
          <div>
            <label className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-slate-600 dark:text-slate-300">
              <Link2 className="h-3.5 w-3.5" /> Link a printer (optional)
            </label>
            <select
              value={setupPrinterId}
              onChange={(e) => setSetupPrinterId(e.target.value)}
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
            >
              <option value="">No printer</option>
              {printers.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
            {setupPrinterId && (
              <p className="mt-1 text-[10px] text-slate-400">
                You'll be able to attach live snapshots from this printer
              </p>
            )}
          </div>

          {/* G-code file selection */}
          <div>
            <label className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-slate-600 dark:text-slate-300">
              <FileCode className="h-3.5 w-3.5" /> Attach G-code file (optional)
            </label>
            {gcodeFiles.length === 0 ? (
              <p className="rounded-lg border border-dashed border-slate-300 px-3 py-2 text-xs text-slate-400 dark:border-slate-700">
                No G-code files uploaded yet
              </p>
            ) : (
              <select
                value={setupGcodeId}
                onChange={(e) => setSetupGcodeId(e.target.value)}
                className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
              >
                <option value="">No file</option>
                {gcodeFiles.map((f: GCodeFile) => (
                  <option key={f.id} value={f.id}>{f.name} {f.size ? `(${f.size})` : ''}</option>
                ))}
              </select>
            )}
          </div>

          {/* Initial message */}
          <div>
            <label className="mb-1.5 block text-xs font-semibold text-slate-600 dark:text-slate-300">
              First message (optional)
            </label>
            <textarea
              value={setupInitialMsg}
              onChange={(e) => setSetupInitialMsg(e.target.value)}
              placeholder="e.g. Analyze this print for potential issues..."
              rows={3}
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
            />
          </div>

          {/* Start button */}
          <button
            onClick={startNewChat}
            disabled={loading}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 px-4 py-2.5 text-sm font-medium text-white hover:from-blue-500 hover:to-purple-500 disabled:opacity-50"
          >
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
            {loading ? 'Starting...' : 'Start Chat'}
          </button>

          <button
            onClick={() => setView('list')}
            className="w-full text-center text-xs text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
          >
            Cancel
          </button>
        </div>
      )}

      {/* ═══ CHAT VIEW: Active conversation ═══ */}
      {view === 'chat' && (
        <>
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
          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {loading && !conversation && (
              <div className="flex h-full items-center justify-center">
                <div className="text-center">
                  <Loader2 className="mx-auto h-8 w-8 animate-spin text-blue-500" />
                  <p className="mt-2 text-xs text-slate-400">Loading chat...</p>
                </div>
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
              {/* Snapshot toggle + Gcode selector */}
              <div className="mb-2 flex flex-wrap items-center gap-2">
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
                  {attachSnapshot ? 'Snapshot on' : 'Snapshot'}
                </button>
                {attachSnapshot && linkedPrinter && (
                  <span className="text-xs text-slate-400">{linkedPrinter.name}</span>
                )}
                {attachSnapshot && !linkedPrinterId && (
                  <span className="text-xs text-amber-500">Link printer first</span>
                )}
                <button
                  onClick={() => setShowGcodeSelect(!showGcodeSelect)}
                  className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors ${
                    selectedGcodeId
                      ? 'bg-blue-600 text-white'
                      : 'bg-slate-100 text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700'
                  }`}
                  title="Attach a G-code file for analysis"
                >
                  <FileCode className="h-3.5 w-3.5" />
                  G-code
                </button>
                {selectedGcodeId && (
                  <span className="flex items-center gap-1 text-xs text-slate-400">
                    {gcodeFiles.find((f) => f.id === selectedGcodeId)?.name || 'selected'}
                    <button onClick={() => setSelectedGcodeId('')} className="text-slate-400 hover:text-rose-500">
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                )}
              </div>
              {showGcodeSelect && (
                <div className="mb-2 max-h-40 overflow-y-auto rounded-lg border border-slate-200 bg-white p-1 dark:border-slate-700 dark:bg-slate-900">
                  {gcodeFiles.length === 0 ? (
                    <p className="px-2 py-1.5 text-xs text-slate-400">No G-code files uploaded</p>
                  ) : (
                    gcodeFiles.map((f) => (
                      <button
                        key={f.id}
                        onClick={() => { setSelectedGcodeId(f.id); setShowGcodeSelect(false) }}
                        className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
                      >
                        <FileCode className="h-3.5 w-3.5 shrink-0 text-slate-400" />
                        <span className="truncate">{f.name}</span>
                        {f.size && <span className="ml-auto shrink-0 text-slate-400">{f.size}</span>}
                      </button>
                    ))
                  )}
                </div>
              )}
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
                  disabled={loading || (!input.trim() && !attachSnapshot && !selectedGcodeId)}
                  className="rounded-lg bg-blue-600 p-2 text-white hover:bg-blue-500 disabled:opacity-50"
                >
                  <Send className="h-4 w-4" />
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </aside>
  )
}

function formatDate(iso: string): string {
  try {
    const d = new Date(iso)
    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const days = Math.floor(diff / 86400000)
    if (days === 0) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    if (days === 1) return 'yesterday'
    if (days < 7) return `${days}d ago`
    return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
  } catch {
    return ''
  }
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
