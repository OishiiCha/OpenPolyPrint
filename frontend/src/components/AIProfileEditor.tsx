import { useEffect, useState, useRef } from 'react'
import {
  Sparkles,
  X,
  Loader2,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Save,
  RefreshCw,
  Eye,
  ChevronDown,
  ChevronRight,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'

export interface ProfileSuggestion {
  key: string
  currentValue: string
  suggestedValue: string
  reason: string
  category: string
}

interface AIProfileEditorProps {
  open: boolean
  onClose: () => void
  content: string
  profileName: string
  profileType: string
  onSave: (newContent: string, newName: string) => Promise<void>
}

export function AIProfileEditor({
  open,
  onClose,
  content,
  profileName,
  profileType,
  onSave,
}: AIProfileEditorProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [suggestions, setSuggestions] = useState<ProfileSuggestion[]>([])
  const [rawText, setRawText] = useState('')
  const [accepted, setAccepted] = useState<Set<number>>(new Set())
  const [showPreview, setShowPreview] = useState(false)
  const [showRaw, setShowRaw] = useState(false)
  const [saving, setSaving] = useState(false)
  const [newName, setNewName] = useState('')
  const [saved, setSaved] = useState(false)
  const hasFetched = useRef(false)

  // Fetch suggestions when modal opens
  useEffect(() => {
    if (!open || hasFetched.current) return
    hasFetched.current = true
    fetchSuggestions()
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  // Reset state when modal closes
  useEffect(() => {
    if (!open) {
      hasFetched.current = false
      setSuggestions([])
      setAccepted(new Set())
      setError(null)
      setRawText('')
      setShowPreview(false)
      setShowRaw(false)
      setSaved(false)
      setNewName('')
    }
  }, [open])

  const fetchSuggestions = async () => {
    setLoading(true)
    setError(null)
    setSuggestions([])
    setAccepted(new Set())
    try {
      const res = await fetch('/api/ai/suggest-profile-edits', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          content,
          profileName,
          profileType,
        }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({ error: 'Failed to get suggestions' }))
        throw new Error(d.error || `Failed (HTTP ${res.status})`)
      }
      const data = await res.json()
      const sug = (data.suggestions || []) as ProfileSuggestion[]
      setSuggestions(sug)
      setRawText(data.rawText || '')
      if (sug.length === 0 && data.error) {
        setError(`AI returned no structured suggestions. ${data.error}`)
      }
      // Set default new name
      setNewName(`${profileName} (AI Optimized)`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to get suggestions')
    } finally {
      setLoading(false)
    }
  }

  const toggleAccept = (idx: number) => {
    setAccepted(prev => {
      const next = new Set(prev)
      if (next.has(idx)) next.delete(idx)
      else next.add(idx)
      return next
    })
  }

  const acceptAll = () => {
    setAccepted(new Set(suggestions.map((_, i) => i)))
  }

  const rejectAll = () => {
    setAccepted(new Set())
  }

  // Build the modified content by applying accepted suggestions
  const getModifiedContent = (): string => {
    let modified = content
    for (let i = 0; i < suggestions.length; i++) {
      if (!accepted.has(i)) continue
      const sug = suggestions[i]
      // Replace the value for this key in the INI content
      // Match: key = oldvalue  (with possible whitespace)
      const lines = modified.split('\n')
      for (let j = 0; j < lines.length; j++) {
        const line = lines[j]
        const trimmed = line.trim()
        if (trimmed.startsWith(';') || trimmed.startsWith('#') || trimmed.startsWith('[')) continue
        const eqIdx = trimmed.indexOf('=')
        if (eqIdx <= 0) continue
        const key = trimmed.slice(0, eqIdx).trim()
        if (key === sug.key) {
          // Preserve leading whitespace
          const leadingWs = line.slice(0, line.length - line.trimStart().length)
          lines[j] = `${leadingWs}${sug.key} = ${sug.suggestedValue}`
          break
        }
      }
      modified = lines.join('\n')
    }
    return modified
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      const modifiedContent = getModifiedContent()
      await onSave(modifiedContent, newName || `${profileName} (AI Optimized)`)
      setSaved(true)
      setTimeout(() => onClose(), 1500)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  if (!open) return null

  const modifiedContent = getModifiedContent()
  const acceptedCount = accepted.size
  const hasChanges = acceptedCount > 0

  return (
    <div className="fixed inset-0 z-[10001] flex items-center justify-center bg-black/85 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-3xl max-h-[90vh] flex-col overflow-hidden rounded-lg border-2 border-slate-700 bg-slate-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 p-2">
              <Sparkles className="h-5 w-5 text-white" />
            </div>
            <div>
              <h2 className="font-mono text-lg font-semibold text-blue-400">[ ai_profile_editor ]</h2>
              <p className="text-xs text-slate-500">{profileName} · {profileType}</p>
            </div>
          </div>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6">
          {/* Loading state */}
          {loading && (
            <div className="flex flex-col items-center justify-center py-16">
              <div className="mb-4 rounded-full bg-gradient-to-r from-blue-600 to-purple-600 p-4">
                <Sparkles className="h-8 w-8 animate-pulse text-white" />
              </div>
              <h3 className="mb-2 text-lg font-semibold text-slate-200">AI is analyzing the profile...</h3>
              <p className="text-sm text-slate-400">Generating setting suggestions</p>
              <Loader2 className="mt-4 h-6 w-6 animate-spin text-blue-400" />
            </div>
          )}

          {/* Error state */}
          {error && !loading && (
            <div className="mb-4 rounded-lg border border-rose-700 bg-rose-900/20 p-4">
              <div className="flex items-center gap-2 text-rose-400">
                <AlertTriangle className="h-4 w-4" />
                <span className="text-sm font-medium">Error</span>
              </div>
              <p className="mt-1 text-sm text-rose-300">{error}</p>
              <button
                onClick={fetchSuggestions}
                className="mt-3 flex items-center gap-2 rounded-lg bg-rose-800 px-3 py-1.5 text-xs text-rose-200 hover:bg-rose-700"
              >
                <RefreshCw className="h-3.5 w-3.5" /> Try Again
              </button>
            </div>
          )}

          {/* Suggestions */}
          {!loading && !error && suggestions.length > 0 && (
            <div className="space-y-4">
              {/* Summary bar */}
              <div className="flex items-center justify-between rounded-lg border border-slate-700 bg-slate-900 px-4 py-3">
                <div className="text-sm text-slate-300">
                  <span className="font-semibold text-blue-400">{suggestions.length}</span> suggestions
                  {acceptedCount > 0 && (
                    <>
                      {' · '}
                      <span className="font-semibold text-emerald-400">{acceptedCount}</span> accepted
                    </>
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={acceptAll}
                    className="rounded-lg bg-emerald-600/20 px-3 py-1.5 text-xs font-medium text-emerald-400 hover:bg-emerald-600/30"
                  >
                    Accept All
                  </button>
                  <button
                    onClick={rejectAll}
                    className="rounded-lg bg-slate-800 px-3 py-1.5 text-xs font-medium text-slate-400 hover:bg-slate-700"
                  >
                    Reject All
                  </button>
                </div>
              </div>

              {/* Suggestion list */}
              <div className="space-y-2">
                {suggestions.map((sug, i) => {
                  const isAccepted = accepted.has(i)
                  return (
                    <div
                      key={i}
                      className={`rounded-lg border p-4 transition-colors ${
                        isAccepted
                          ? 'border-emerald-600/50 bg-emerald-900/10'
                          : 'border-slate-700 bg-slate-900/50'
                      }`}
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          {/* Setting key */}
                          <div className="flex items-center gap-2">
                            <span className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-xs text-blue-400">
                              {sug.key}
                            </span>
                            <span className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-500">
                              {sug.category}
                            </span>
                          </div>
                          {/* Value change */}
                          <div className="mt-2 flex items-center gap-2 font-mono text-sm">
                            <span className="text-rose-400 line-through opacity-60">
                              {sug.currentValue || '(empty)'}
                            </span>
                            <span className="text-slate-500">→</span>
                            <span className="font-semibold text-emerald-400">
                              {sug.suggestedValue}
                            </span>
                          </div>
                          {/* Reason */}
                          <p className="mt-2 text-xs text-slate-400">{sug.reason}</p>
                        </div>
                        {/* Accept/Reject toggle */}
                        <button
                          onClick={() => toggleAccept(i)}
                          className={`flex shrink-0 items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors ${
                            isAccepted
                              ? 'bg-emerald-600 text-white hover:bg-emerald-500'
                              : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                          }`}
                        >
                          {isAccepted ? (
                            <>
                              <CheckCircle2 className="h-4 w-4" /> Accepted
                            </>
                          ) : (
                            <>
                              <XCircle className="h-4 w-4" /> Accept
                            </>
                          )}
                        </button>
                      </div>
                    </div>
                  )
                })}
              </div>

              {/* Preview toggle */}
              <div>
                <button
                  onClick={() => setShowPreview(!showPreview)}
                  className="flex items-center gap-2 text-sm text-slate-400 hover:text-slate-200"
                >
                  {showPreview ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                  <Eye className="h-4 w-4" /> Preview modified file
                </button>
                {showPreview && (
                  <pre className="mt-2 max-h-64 overflow-auto rounded-lg bg-slate-900 p-3 font-mono text-xs text-slate-300">
                    {modifiedContent}
                  </pre>
                )}
              </div>

              {/* Raw AI response toggle */}
              {rawText && (
                <div>
                  <button
                    onClick={() => setShowRaw(!showRaw)}
                    className="flex items-center gap-2 text-xs text-slate-500 hover:text-slate-300"
                  >
                    {showRaw ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                    View raw AI response
                  </button>
                  {showRaw && (
                    <div className="mt-2 max-h-48 overflow-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-400">
                      <ReactMarkdown>{rawText}</ReactMarkdown>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          {/* No suggestions */}
          {!loading && !error && suggestions.length === 0 && (
            <div className="flex flex-col items-center justify-center py-12">
              <CheckCircle2 className="mb-4 h-12 w-12 text-emerald-400" />
              <h3 className="mb-2 text-lg font-semibold text-slate-200">No changes suggested</h3>
              <p className="text-sm text-slate-400">
                The AI analyzed this profile and found no settings that need improvement.
              </p>
            </div>
          )}

          {/* Saved success */}
          {saved && (
            <div className="flex flex-col items-center justify-center py-12">
              <CheckCircle2 className="mb-4 h-12 w-12 text-emerald-400" />
              <h3 className="mb-2 text-lg font-semibold text-slate-200">Saved successfully!</h3>
              <p className="text-sm text-slate-400">The modified profile has been saved as a new file.</p>
            </div>
          )}
        </div>

        {/* Footer with save */}
        {!loading && !saved && suggestions.length > 0 && (
          <div className="border-t border-slate-800 px-6 py-4">
            <div className="flex items-center gap-3">
              <input
                type="text"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="New file name..."
                className="flex-1 rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
              <button
                onClick={handleSave}
                disabled={!hasChanges || saving}
                className="flex items-center gap-2 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 px-4 py-2 text-sm font-medium text-white hover:from-blue-500 hover:to-purple-500 disabled:opacity-50"
              >
                {saving ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Save className="h-4 w-4" />
                )}
                Save as New File
                {hasChanges && ` (${acceptedCount})`}
              </button>
            </div>
            {!hasChanges && (
              <p className="mt-2 text-xs text-slate-500">Accept at least one suggestion to save</p>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
