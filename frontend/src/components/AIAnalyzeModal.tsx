import { useEffect, useRef, useState, useCallback } from 'react'
import {
  Sparkles,
  Camera,
  X,
  Loader2,
  CheckCircle2,
  AlertTriangle,
  RefreshCw,
  MessageSquare,
  RotateCw,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'

export interface AIAnalysisResult {
  id: string
  messages: Array<{
    role: string
    text: string
    hasImage: boolean
    imagePaths?: string[]
    imageMime?: string
    timestamp: string
  }>
}

interface AIAnalyzeModalProps {
  open: boolean
  onClose: () => void
  /** Title shown in the modal header */
  title: string
  /** Source type: "printer", "profile", or "stl" */
  sourceType: 'printer' | 'profile' | 'stl'
  /** Printer ID (for printer source type — used to capture camera frames) */
  printerId?: string
  /** Printer name (for display and chat context) */
  printerName?: string
  /** Pre-captured images as base64 strings (for STL screenshots, etc.) */
  preCapturedImages?: string[]
  /** Extra context text to send to AI (e.g. printer data, file content) */
  contextText?: string
  /** Default message to send to AI */
  defaultMessage?: string
  /** Callback when a chat conversation is created (to update sidebar) */
  onConversationCreated?: (convId: string) => void
}

type Phase = 'capturing' | 'selecting' | 'analyzing' | 'result' | 'error'

export function AIAnalyzeModal({
  open,
  onClose,
  title,
  sourceType,
  printerId,
  printerName,
  preCapturedImages,
  contextText,
  defaultMessage,
  onConversationCreated,
}: AIAnalyzeModalProps) {
  const [phase, setPhase] = useState<Phase>('capturing')
  const [capturedPhotos, setCapturedPhotos] = useState<string[]>([]) // base64 strings
  const [selectedIndices, setSelectedIndices] = useState<Set<number>>(new Set())
  const [captureProgress, setCaptureProgress] = useState(0)
  const [result, setResult] = useState<AIAnalysisResult | null>(null)
  const [errorMsg, setErrorMsg] = useState('')
  const [customMessage, setCustomMessage] = useState('')
  const abortRef = useRef(false)

  const numPhotos = 5
  const captureInterval = 2000 // 2 seconds between photos

  // Capture photos from camera (for printer source type)
  const capturePhotos = useCallback(async () => {
    if (sourceType !== 'printer' || !printerId) {
      // For non-printer types, use pre-captured images
      if (preCapturedImages && preCapturedImages.length > 0) {
        setCapturedPhotos(preCapturedImages)
        setSelectedIndices(new Set([0]))
        setPhase('selecting')
      } else {
        // No images needed — go straight to analyzing with text only
        setPhase('analyzing')
        await runAnalysis([], defaultMessage || '')
      }
      return
    }

    abortRef.current = false
    setCapturedPhotos([])
    setCaptureProgress(0)
    setPhase('capturing')

    const photos: string[] = []
    for (let i = 0; i < numPhotos; i++) {
      if (abortRef.current) return
      try {
        const res = await fetch(`/api/ai/snapshot/${printerId}`, {
          cache: 'no-store',
        })
        if (res.ok) {
          const blob = await res.blob()
          const b64 = await blobToBase64(blob)
          photos.push(b64)
          setCapturedPhotos([...photos])
        }
      } catch {
        // Skip failed captures
      }
      setCaptureProgress(i + 1)
      if (i < numPhotos - 1 && !abortRef.current) {
        await sleep(captureInterval)
      }
    }

    if (abortRef.current) return

    if (photos.length === 0) {
      setErrorMsg('No camera frames could be captured. Make sure a camera is connected and enabled for this printer.')
      setPhase('error')
      return
    }

    // Auto-select the last photo (most recent)
    setSelectedIndices(new Set([photos.length - 1]))
    setPhase('selecting')
  }, [sourceType, printerId, preCapturedImages, defaultMessage])

  // Run AI analysis
  const runAnalysis = async (images: string[], message: string) => {
    setPhase('analyzing')
    setErrorMsg('')
    try {
      const res = await fetch('/api/ai/analyze-image', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          printerId: printerId || '',
          printerName: printerName || '',
          images,
          message: message || defaultMessage || 'Please analyze this and provide insights.',
          context: contextText || '',
          sourceType,
        }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({ error: 'Analysis failed' }))
        throw new Error(d.error || `Analysis failed (HTTP ${res.status})`)
      }
      const data = (await res.json()) as AIAnalysisResult
      setResult(data)
      if (data.id) onConversationCreated?.(data.id)
      setPhase('result')
    } catch (e) {
      setErrorMsg(e instanceof Error ? e.message : 'Analysis failed')
      setPhase('error')
    }
  }

  // Start capturing when modal opens
  useEffect(() => {
    if (open) {
      setResult(null)
      setErrorMsg('')
      setCustomMessage('')
      capturePhotos()
    }
    return () => {
      abortRef.current = true
    }
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  const togglePhoto = (idx: number) => {
    setSelectedIndices((prev) => {
      const next = new Set(prev)
      if (next.has(idx)) {
        next.delete(idx)
      } else {
        next.add(idx)
      }
      return next
    })
  }

  const handleSendToAI = () => {
    const selected = Array.from(selectedIndices)
      .sort((a, b) => a - b)
      .map((i) => capturedPhotos[i])
      .filter(Boolean)
    runAnalysis(selected, customMessage || defaultMessage || '')
  }

  const handleRetry = () => {
    setResult(null)
    setErrorMsg('')
    capturePhotos()
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-[10000] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-2xl max-h-[90vh] flex-col overflow-hidden rounded-lg border-2 border-slate-700 bg-slate-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 p-2">
              <Sparkles className="h-5 w-5 text-white" />
            </div>
            <div>
              <h2 className="font-mono text-lg font-semibold text-blue-400">[ ai_analysis ]</h2>
              <p className="text-xs text-slate-500">{title}</p>
            </div>
          </div>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6">
          {/* Phase: Capturing */}
          {phase === 'capturing' && (
            <div className="flex flex-col items-center justify-center py-12">
              <div className="relative mb-6">
                <Camera className="h-16 w-16 text-blue-400" />
                <div className="absolute -bottom-2 -right-2 rounded-full bg-blue-600 p-2">
                  <Loader2 className="h-4 w-4 animate-spin text-white" />
                </div>
              </div>
              <h3 className="mb-2 text-lg font-semibold text-slate-200">Capturing photos...</h3>
              <p className="mb-4 text-sm text-slate-400">
                Taking {numPhotos} photos over {Math.round((numPhotos * captureInterval) / 1000)} seconds
              </p>
              {/* Progress dots */}
              <div className="flex gap-2">
                {Array.from({ length: numPhotos }).map((_, i) => (
                  <div
                    key={i}
                    className={`h-3 w-3 rounded-full transition-colors ${
                      i < captureProgress ? 'bg-blue-500' : 'bg-slate-700'
                    }`}
                  />
                ))}
              </div>
              {/* Captured thumbnails */}
              {capturedPhotos.length > 0 && (
                <div className="mt-6 flex flex-wrap gap-2">
                  {capturedPhotos.map((p, i) => (
                    <img
                      key={i}
                      src={`data:image/jpeg;base64,${p}`}
                      alt={`Capture ${i + 1}`}
                      className="h-16 w-16 rounded-lg border border-slate-700 object-cover"
                    />
                  ))}
                </div>
              )}
              <button
                onClick={() => {
                  abortRef.current = true
                  onClose()
                }}
                className="mt-6 rounded-lg bg-slate-800 px-4 py-2 text-sm text-slate-400 hover:bg-slate-700"
              >
                Cancel
              </button>
            </div>
          )}

          {/* Phase: Selecting */}
          {phase === 'selecting' && (
            <div className="space-y-4">
              <div>
                <h3 className="mb-1 text-sm font-semibold text-slate-200">Select photo(s) to analyze</h3>
                <p className="text-xs text-slate-400">
                  {capturedPhotos.length} photo{capturedPhotos.length !== 1 ? 's' : ''} captured. Click to select/deselect.
                  Selected photos will be sent to AI for analysis.
                </p>
              </div>
              <div className="grid grid-cols-3 gap-3 sm:grid-cols-5">
                {capturedPhotos.map((p, i) => (
                  <button
                    key={i}
                    onClick={() => togglePhoto(i)}
                    className={`relative overflow-hidden rounded-lg border-2 transition-all ${
                      selectedIndices.has(i)
                        ? 'border-blue-500 ring-2 ring-blue-500/30'
                        : 'border-slate-700 hover:border-slate-500'
                    }`}
                  >
                    <img
                      src={`data:image/jpeg;base64,${p}`}
                      alt={`Photo ${i + 1}`}
                      className="aspect-square w-full object-cover"
                    />
                    {selectedIndices.has(i) && (
                      <div className="absolute right-1 top-1 rounded-full bg-blue-600 p-0.5">
                        <CheckCircle2 className="h-3.5 w-3.5 text-white" />
                      </div>
                    )}
                    <span className="absolute bottom-1 left-1 rounded bg-black/60 px-1.5 py-0.5 text-[9px] text-white">
                      #{i + 1}
                    </span>
                  </button>
                ))}
              </div>

              {/* Custom message */}
              <div>
                <label className="mb-1 block text-xs text-slate-400">Message to AI (optional)</label>
                <textarea
                  value={customMessage}
                  onChange={(e) => setCustomMessage(e.target.value)}
                  placeholder={defaultMessage || 'Please analyze this and provide insights.'}
                  className="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
                  rows={2}
                />
              </div>

              <div className="flex gap-2">
                <button
                  onClick={handleSendToAI}
                  disabled={selectedIndices.size === 0}
                  className="flex flex-1 items-center justify-center gap-2 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 px-4 py-2.5 text-sm font-medium text-white hover:from-blue-500 hover:to-purple-500 disabled:opacity-50"
                >
                  <Sparkles className="h-4 w-4" />
                  Analyze {selectedIndices.size > 0 ? `(${selectedIndices.size} photo${selectedIndices.size !== 1 ? 's' : ''})` : ''}
                </button>
                <button
                  onClick={handleRetry}
                  className="flex items-center gap-2 rounded-lg bg-slate-800 px-4 py-2.5 text-sm text-slate-400 hover:bg-slate-700"
                >
                  <RefreshCw className="h-4 w-4" /> Retake
                </button>
              </div>
            </div>
          )}

          {/* Phase: Analyzing */}
          {phase === 'analyzing' && (
            <div className="flex flex-col items-center justify-center py-16">
              <div className="mb-4 rounded-full bg-gradient-to-r from-blue-600 to-purple-600 p-4">
                <Sparkles className="h-8 w-8 animate-pulse text-white" />
              </div>
              <h3 className="mb-2 text-lg font-semibold text-slate-200">AI is analyzing...</h3>
              <p className="text-sm text-slate-400">This may take a few seconds</p>
              <Loader2 className="mt-4 h-6 w-6 animate-spin text-blue-400" />
            </div>
          )}

          {/* Phase: Result */}
          {phase === 'result' && result && (
            <div className="space-y-4">
              {/* AI Response */}
              <div className="rounded-lg border border-slate-700 bg-slate-900 p-4">
                <div className="mb-3 flex items-center gap-2">
                  <div className="rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 p-1.5">
                    <Sparkles className="h-4 w-4 text-white" />
                  </div>
                  <h3 className="text-sm font-semibold text-slate-200">AI Analysis</h3>
                </div>
                <div className="prose prose-sm prose-invert max-w-none text-sm text-slate-300">
                  <ReactMarkdown>
                    {result.messages.find((m) => m.role === 'model')?.text || 'No response from AI.'}
                  </ReactMarkdown>
                </div>
              </div>

              {/* Sent images preview */}
              {result.messages.find((m) => m.role === 'user' && m.hasImage) && (
                <div className="rounded-lg border border-slate-700 bg-slate-900/50 p-3">
                  <p className="mb-2 text-xs text-slate-400">Photos sent to AI:</p>
                  <div className="flex flex-wrap gap-2">
                    {result.messages
                      .filter((m) => m.role === 'user' && m.hasImage)
                      .flatMap((m) => m.imagePaths || [])
                      .map((path, i) => (
                        <img
                          key={i}
                          src={`/api/ai/chat/${result.id}/image/${path}`}
                          alt={`Sent ${i + 1}`}
                          className="h-16 w-16 rounded-lg border border-slate-700 object-cover"
                        />
                      ))}
                  </div>
                </div>
              )}

              {/* Actions */}
              <div className="flex gap-2">
                <button
                  onClick={() => {
                    // Dispatch event to open the chat sidebar with this conversation
                    window.dispatchEvent(
                      new CustomEvent('openpolyprint-open-chat', { detail: { conversationId: result.id } })
                    )
                    onClose()
                  }}
                  className="flex flex-1 items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-500"
                >
                  <MessageSquare className="h-4 w-4" />
                  Continue in Chat
                </button>
                <button
                  onClick={handleRetry}
                  className="flex items-center gap-2 rounded-lg bg-slate-800 px-4 py-2.5 text-sm text-slate-400 hover:bg-slate-700"
                >
                  <RotateCw className="h-4 w-4" /> New Analysis
                </button>
              </div>
            </div>
          )}

          {/* Phase: Error */}
          {phase === 'error' && (
            <div className="flex flex-col items-center justify-center py-12">
              <AlertTriangle className="mb-4 h-12 w-12 text-rose-400" />
              <h3 className="mb-2 text-lg font-semibold text-slate-200">Analysis Failed</h3>
              <p className="mb-6 max-w-md text-center text-sm text-slate-400">{errorMsg}</p>
              <div className="flex gap-2">
                <button
                  onClick={handleRetry}
                  className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
                >
                  <RefreshCw className="h-4 w-4" /> Try Again
                </button>
                <button
                  onClick={onClose}
                  className="rounded-lg bg-slate-800 px-4 py-2 text-sm text-slate-400 hover:bg-slate-700"
                >
                  Close
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// ── Helpers ────────────────────────────────────────────────────────

function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onloadend = () => {
      const result = reader.result as string
      // Remove "data:image/jpeg;base64," prefix
      const base64 = result.split(',')[1] || result
      resolve(base64)
    }
    reader.onerror = reject
    reader.readAsDataURL(blob)
  })
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
