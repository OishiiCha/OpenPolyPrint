import { useEffect, useState, useCallback, useMemo } from 'react'
import { Link } from 'react-router-dom'
import {
  ArrowLeft, Film, FileCode2, Sparkles, Loader2, ChevronLeft, ChevronRight,
  Thermometer, Clock, CheckCircle2, XCircle, Camera, Activity,
  Edit3, Save, Trash2, Bookmark, X,
} from 'lucide-react'
import { SessionTempGraph } from '../components/SessionTempGraph'
import { loadConfig } from '../config'

interface SavedSession {
  id: string
  printerName: string
  fileName: string
  startTime: string
  endTime?: string
  status: string
  progress: number
  sampleCount: number
  timelapseDir?: string
  hasGcode: boolean
  gcodeId?: string
}

interface TempSample {
  time: number
  nozzle: number
  targetNozzle: number
  bed: number
  targetBed: number
  progress: number
}

interface StatusEntry {
  time: number
  status: string
  progress: number
  file: string
}

interface SessionDetail {
  session: {
    printerId: string
    printerName: string
    fileName: string
    startTime: string
    endTime?: string
    status: string
    progress: number
    tempSamples: TempSample[]
    statusLog: StatusEntry[]
  }
  timelapseDir: string
  gcodeId: string
}

interface FrameMeta {
  frame: number
  timestamp: number  // unix millis
  elapsedSeconds: number
  cameraId: string
}

interface PromptPreset {
  id: string
  name: string
  prompt: string
  createdAt: string
}

function formatDate(s: string) {
  const d = new Date(s)
  return d.toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

function formatDuration(start: string, end?: string) {
  const ms = (end ? new Date(end).getTime() : Date.now()) - new Date(start).getTime()
  const totalSec = Math.floor(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function formatElapsed(sec: number) {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

export function PrintAnalysis() {
  const [sessions, setSessions] = useState<SavedSession[]>([])
  const [selectedSessionId, setSelectedSessionId] = useState('')
  const [sessionDetail, setSessionDetail] = useState<SessionDetail | null>(null)
  const [frames, setFrames] = useState<string[]>([])
  const [frameMetas, setFrameMetas] = useState<FrameMeta[]>([])
  const [currentFrameIdx, setCurrentFrameIdx] = useState(0)
  const [loading, setLoading] = useState(false)
  const [analyzing, setAnalyzing] = useState(false)
  const [analysis, setAnalysis] = useState<string | null>(null)
  const [analysisError, setAnalysisError] = useState<string | null>(null)
  const [apiKeyAvailable, setApiKeyAvailable] = useState(false)

  // Prompt state
  const [defaultPrompt, setDefaultPrompt] = useState('')
  const [promptOverride, setPromptOverride] = useState<string | null>(null) // null = use generated default
  const [editingPrompt, setEditingPrompt] = useState(false)
  const [customPrompt, setCustomPrompt] = useState('')
  const [presets, setPresets] = useState<PromptPreset[]>([])
  const [showSavePreset, setShowSavePreset] = useState(false)
  const [presetName, setPresetName] = useState('')
  const [presetTarget, setPresetTarget] = useState<'default' | 'custom'>('custom')

  // Fetch session list and presets
  useEffect(() => {
    fetch('/api/sessions')
      .then((r) => r.json())
      .then((data) => {
        if (data.sessions && Array.isArray(data.sessions)) setSessions(data.sessions)
      })
      .catch(() => {})

    fetch('/api/ai/prompts')
      .then((r) => r.json())
      .then((data) => {
        if (data.prompts && Array.isArray(data.prompts)) setPresets(data.prompts)
      })
      .catch(() => {})

    // Check if API key is configured
    const cfg = loadConfig()
    setApiKeyAvailable(!!cfg.geminiApiKey)
  }, [])

  // Fetch session detail when selected
  useEffect(() => {
    if (!selectedSessionId) {
      setSessionDetail(null)
      setFrames([])
      setFrameMetas([])
      setDefaultPrompt('')
      setPromptOverride(null)
      setEditingPrompt(false)
      return
    }
    setLoading(true)
    setAnalysis(null)
    setAnalysisError(null)
    setCurrentFrameIdx(0)
    setPromptOverride(null)
    setEditingPrompt(false)

    fetch(`/api/sessions/${encodeURIComponent(selectedSessionId)}`)
      .then((r) => r.json())
      .then((data: SessionDetail) => {
        setSessionDetail(data)
        if (data.timelapseDir) {
          Promise.all([
            fetch(`/api/timelapse-frames/${encodeURIComponent(data.timelapseDir)}`).then((r) => r.json()),
            fetch(`/api/timelapse-frames/${encodeURIComponent(data.timelapseDir)}/meta`).then((r) => r.json()),
          ]).then(([frameList, metas]) => {
            if (Array.isArray(frameList)) setFrames(frameList)
            if (Array.isArray(metas)) setFrameMetas(metas)
            setLoading(false)
          }).catch(() => setLoading(false))
        } else {
          setLoading(false)
        }
      })
      .catch(() => setLoading(false))
  }, [selectedSessionId])

  const currentFrame = frames.length > 0 ? frames[currentFrameIdx] : null
  const currentFrameUrl = currentFrame && sessionDetail?.timelapseDir
    ? `/recordings/timelapse/${sessionDetail.timelapseDir}/${currentFrame}`
    : null

  // Get the timestamp of the current frame for the temp graph marker
  const currentFrameTime = useMemo(() => {
    if (frameMetas.length === 0 || !frames[currentFrameIdx]) return null
    const frameNum = currentFrameIdx + 1
    const meta = frameMetas.find((m) => m.frame === frameNum)
    if (meta) return Math.floor(meta.timestamp / 1000)
    if (sessionDetail?.session.startTime && frameMetas.length > 0) {
      const interval = frameMetas.length > 1
        ? frameMetas[1].elapsedSeconds - frameMetas[0].elapsedSeconds
        : 1
      const elapsed = currentFrameIdx * interval
      return Math.floor(new Date(sessionDetail.session.startTime).getTime() / 1000 + elapsed)
    }
    return null
  }, [frameMetas, currentFrameIdx, frames, sessionDetail])

  const currentElapsedSec = useMemo(() => {
    if (frameMetas.length === 0 || !frames[currentFrameIdx]) return 0
    const frameNum = currentFrameIdx + 1
    const meta = frameMetas.find((m) => m.frame === frameNum)
    return meta?.elapsedSeconds ?? 0
  }, [frameMetas, currentFrameIdx, frames])

  // Fetch the generated default prompt whenever the frame or session changes
  // (only if the user hasn't overridden it)
  useEffect(() => {
    if (!sessionDetail) return
    if (promptOverride !== null) return // user has overridden, don't auto-update
    const body = {
      frameDir: sessionDetail.timelapseDir || '',
      frameNum: currentFrameIdx + 1,
      elapsedSec: currentElapsedSec,
      intervalSec: frameMetas.length > 1
        ? frameMetas[1].elapsedSeconds - frameMetas[0].elapsedSeconds
        : 1,
      gcodeId: sessionDetail.gcodeId || '',
      printerName: sessionDetail.session.printerName,
      filename: sessionDetail.session.fileName,
      sessionId: selectedSessionId,
    }
    fetch('/api/ai/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
      .then((r) => r.json())
      .then((data) => {
        if (data.prompt) setDefaultPrompt(data.prompt)
      })
      .catch(() => {})
  }, [sessionDetail, currentFrameIdx, currentElapsedSec, frameMetas, selectedSessionId, promptOverride])

  const goPrev = useCallback(() => {
    setCurrentFrameIdx((i) => Math.max(0, i - 1))
    setAnalysis(null)
    setAnalysisError(null)
  }, [])

  const goNext = useCallback(() => {
    setCurrentFrameIdx((i) => Math.min(frames.length - 1, i + 1))
    setAnalysis(null)
    setAnalysisError(null)
  }, [frames.length])

  const jumpToFrame = useCallback((idx: number) => {
    setCurrentFrameIdx(idx)
    setAnalysis(null)
    setAnalysisError(null)
  }, [])

  const startEditPrompt = () => {
    setPromptOverride(defaultPrompt)
    setEditingPrompt(true)
  }

  const savePromptEdit = () => {
    setEditingPrompt(false)
    // promptOverride stays set with the edited value
  }

  const cancelPromptEdit = () => {
    setEditingPrompt(false)
    setPromptOverride(null) // revert to generated default
  }

  const resetPromptToDefault = () => {
    setPromptOverride(null)
    setEditingPrompt(false)
  }

  const analyzeFrame = async () => {
    if (!sessionDetail || !currentFrame) return
    setAnalyzing(true)
    setAnalysis(null)
    setAnalysisError(null)
    try {
      const res = await fetch('/api/ai/analyze', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          frameDir: sessionDetail.timelapseDir,
          frameNum: currentFrameIdx + 1,
          elapsedSec: currentElapsedSec,
          intervalSec: frameMetas.length > 1
            ? frameMetas[1].elapsedSeconds - frameMetas[0].elapsedSeconds
            : 1,
          gcodeId: sessionDetail.gcodeId || '',
          printerName: sessionDetail.session.printerName,
          filename: sessionDetail.session.fileName,
          sessionId: selectedSessionId,
          promptOverride: promptOverride || '',
          customPrompt: customPrompt || '',
        }),
      })
      if (!res.ok) {
        const err = await res.text()
        try {
          const parsed = JSON.parse(err)
          throw new Error(parsed.error || err)
        } catch {
          throw new Error(err)
        }
      }
      const data = await res.json()
      setAnalysis(data.analysis || data.raw || 'No analysis returned')
    } catch (e) {
      setAnalysisError(e instanceof Error ? e.message : 'Analysis failed')
    } finally {
      setAnalyzing(false)
    }
  }

  const savePreset = async () => {
    if (!presetName.trim()) return
    const promptText = presetTarget === 'default'
      ? (promptOverride ?? defaultPrompt)
      : customPrompt
    if (!promptText.trim()) return
    const res = await fetch('/api/ai/prompts', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: presetName.trim(), prompt: promptText }),
    })
    if (res.ok) {
      const saved = await res.json()
      setPresets([...presets, saved])
      setShowSavePreset(false)
      setPresetName('')
    }
  }

  const deletePreset = async (id: string) => {
    const res = await fetch(`/api/ai/prompts/${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (res.ok) {
      setPresets(presets.filter((p) => p.id !== id))
    }
  }

  const loadPreset = (preset: PromptPreset) => {
    // Load into whichever box the user last interacted with, or custom by default
    setCustomPrompt(preset.prompt)
  }

  const loadPresetToDefault = (preset: PromptPreset) => {
    setPromptOverride(preset.prompt)
    setEditingPrompt(false)
  }

  const tempSamples = sessionDetail?.session.tempSamples ?? []
  const hasTimelapse = !!sessionDetail?.timelapseDir && frames.length > 0
  const hasGcode = sessionDetail?.gcodeId !== ''

  // The effective prompt shown in the default box
  const effectiveDefaultPrompt = promptOverride ?? defaultPrompt

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link
          to="/"
          className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white"
        >
          <ArrowLeft className="h-4 w-4" /> Back
        </Link>
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Print Analysis</h1>
      </div>

      <p className="font-mono text-sm text-slate-500 dark:text-slate-400">
        Select a recorded print session to review timelapse frames with synchronized temperature data.
        Use AI analysis to get quality assessments from Gemini.
      </p>

      {/* Session selector */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <label className="mb-2 flex items-center gap-2 font-mono text-xs text-slate-500 dark:text-slate-400">
          <Activity className="h-3.5 w-3.5" /> Print session
        </label>
        <select
          value={selectedSessionId}
          onChange={(e) => setSelectedSessionId(e.target.value)}
          className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
        >
          <option value="">Select a session...</option>
          {sessions.map((s) => (
            <option key={s.id} value={s.id}>
              {s.printerName} — {s.fileName} ({formatDate(s.startTime)})
            </option>
          ))}
        </select>
        {sessions.length === 0 && (
          <p className="mt-2 font-mono text-xs text-slate-400">
            No saved sessions yet. Sessions are recorded automatically when a print starts.
          </p>
        )}
      </div>

      {loading && (
        <div className="flex h-64 items-center justify-center rounded-xl border border-slate-200 dark:border-slate-800">
          <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
        </div>
      )}

      {/* Session detail */}
      {!loading && sessionDetail && (
        <>
          {/* Session metadata */}
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <div className="flex items-center gap-2">
                <Camera className="h-4 w-4 text-slate-400" />
                <div>
                  <p className="font-mono text-[10px] text-slate-400">Printer</p>
                  <p className="text-sm font-medium text-slate-900 dark:text-white">
                    {sessionDetail.session.printerName}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <FileCode2 className="h-4 w-4 text-slate-400" />
                <div>
                  <p className="font-mono text-[10px] text-slate-400">File</p>
                  <p className="truncate text-sm font-medium text-slate-900 dark:text-white" title={sessionDetail.session.fileName}>
                    {sessionDetail.session.fileName}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Clock className="h-4 w-4 text-slate-400" />
                <div>
                  <p className="font-mono text-[10px] text-slate-400">Duration</p>
                  <p className="text-sm font-medium text-slate-900 dark:text-white">
                    {formatDuration(sessionDetail.session.startTime, sessionDetail.session.endTime)}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {sessionDetail.session.status === 'Success' ? (
                  <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                ) : (
                  <XCircle className="h-4 w-4 text-rose-500" />
                )}
                <div>
                  <p className="font-mono text-[10px] text-slate-400">Status</p>
                  <p className={`text-sm font-medium ${
                    sessionDetail.session.status === 'Success'
                      ? 'text-emerald-600 dark:text-emerald-400'
                      : 'text-rose-600 dark:text-rose-400'
                  }`}>
                    {sessionDetail.session.status}
                  </p>
                </div>
              </div>
            </div>

            {/* G-code and timelapse status badges */}
            <div className="mt-4 flex flex-wrap gap-2">
              <span className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ${
                hasGcode
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                  : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-500'
              }`}>
                <FileCode2 className="h-3 w-3" />
                {hasGcode ? 'G-code attached' : 'No G-code'}
              </span>
              <span className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ${
                hasTimelapse
                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                  : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-500'
              }`}>
                <Film className="h-3 w-3" />
                {hasTimelapse ? `${frames.length} frames` : 'No timelapse'}
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-full bg-orange-100 px-3 py-1 text-xs font-medium text-orange-700 dark:bg-orange-900/30 dark:text-orange-400">
                <Thermometer className="h-3 w-3" />
                {tempSamples.length} temp samples
              </span>
            </div>
          </div>

          {/* Temp graph for the full session */}
          {tempSamples.length > 0 && (
            <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
              <div className="mb-2 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Thermometer className="h-4 w-4 text-orange-500" />
                  <h3 className="font-semibold text-slate-900 dark:text-white">Temperature history</h3>
                </div>
                <div className="flex items-center gap-3 font-mono text-[10px] text-slate-400">
                  <span className="flex items-center gap-1">
                    <span className="inline-block h-2 w-2 rounded-full bg-red-500" /> Nozzle
                  </span>
                  <span className="flex items-center gap-1">
                    <span className="inline-block h-2 w-2 rounded-full bg-orange-500" /> Bed
                  </span>
                  {currentFrameTime != null && (
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2 w-2 rounded-full bg-indigo-500" /> Current frame
                    </span>
                  )}
                </div>
              </div>
              <SessionTempGraph
                samples={tempSamples}
                currentTime={currentFrameTime}
                height={120}
              />
            </div>
          )}

          {/* Frame viewer */}
          {hasTimelapse ? (
            <>
              <div className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
                {/* Frame image */}
                <div className="relative aspect-video bg-black">
                  {currentFrameUrl ? (
                    <img src={currentFrameUrl} alt={`Frame ${currentFrameIdx + 1}`} className="h-full w-full object-contain" />
                  ) : (
                    <div className="flex h-full items-center justify-center text-slate-500">
                      <p className="font-mono text-xs">No frame</p>
                    </div>
                  )}

                  {/* Frame counter overlay */}
                  <div className="absolute top-3 left-3 rounded-lg bg-black/60 px-3 py-1.5 font-mono text-xs text-white">
                    Frame {currentFrameIdx + 1} / {frames.length}
                  </div>
                  {currentElapsedSec > 0 && (
                    <div className="absolute top-3 right-3 rounded-lg bg-black/60 px-3 py-1.5 font-mono text-xs text-white">
                      {formatElapsed(currentElapsedSec)}
                    </div>
                  )}
                </div>

                {/* Navigation controls */}
                <div className="flex items-center gap-3 border-t border-slate-200 p-3 dark:border-slate-800">
                  <button
                    onClick={goPrev}
                    disabled={currentFrameIdx === 0}
                    className="rounded-lg p-2 text-slate-600 hover:bg-slate-100 disabled:opacity-30 dark:text-slate-400 dark:hover:bg-slate-800"
                  >
                    <ChevronLeft className="h-5 w-5" />
                  </button>

                  {/* Scrubber */}
                  <input
                    type="range"
                    min={0}
                    max={frames.length - 1}
                    step={1}
                    value={currentFrameIdx}
                    onChange={(e) => jumpToFrame(parseInt(e.target.value))}
                    className="flex-1 accent-purple-600"
                  />

                  <button
                    onClick={goNext}
                    disabled={currentFrameIdx >= frames.length - 1}
                    className="rounded-lg p-2 text-slate-600 hover:bg-slate-100 disabled:opacity-30 dark:text-slate-400 dark:hover:bg-slate-800"
                  >
                    <ChevronRight className="h-5 w-5" />
                  </button>
                </div>
              </div>

              {/* Frame gallery */}
              <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
                <div className="mb-3 flex items-center gap-2">
                  <Film className="h-4 w-4 text-purple-500" />
                  <h3 className="font-semibold text-slate-900 dark:text-white">Frames</h3>
                  <span className="font-mono text-xs text-slate-400">{frames.length} total</span>
                </div>
                <div className="grid max-h-48 grid-cols-6 gap-2 overflow-y-auto sm:grid-cols-8 md:grid-cols-10 lg:grid-cols-12">
                  {frames.map((frame, idx) => {
                    const frameUrl = `/recordings/timelapse/${sessionDetail.timelapseDir}/${frame}`
                    const isSelected = idx === currentFrameIdx
                    return (
                      <button
                        key={frame}
                        onClick={() => jumpToFrame(idx)}
                        className={`relative aspect-video overflow-hidden rounded-lg border-2 transition-all ${
                          isSelected
                            ? 'border-purple-500 ring-2 ring-purple-500/30'
                            : 'border-slate-200 hover:border-purple-400 dark:border-slate-700'
                        }`}
                        title={`Frame ${idx + 1}`}
                      >
                        <img
                          src={frameUrl}
                          alt={`Frame ${idx + 1}`}
                          loading="lazy"
                          className="h-full w-full object-cover"
                        />
                        <span className="absolute bottom-0 left-0 right-0 bg-black/60 px-1 py-0.5 text-center font-mono text-[8px] text-white">
                          {idx + 1}
                        </span>
                      </button>
                    )
                  })}
                </div>
              </div>

              {/* Prompt configuration */}
              <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
                <div className="mb-3 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Sparkles className="h-5 w-5 text-purple-500" />
                    <h3 className="font-semibold text-slate-900 dark:text-white">AI Prompt</h3>
                  </div>
                  <div className="flex items-center gap-2">
                    {/* Preset selector */}
                    {presets.length > 0 && (
                      <select
                        onChange={(e) => {
                          const preset = presets.find((p) => p.id === e.target.value)
                          if (preset) loadPreset(preset)
                          e.target.value = ''
                        }}
                        className="rounded-lg border border-slate-300 px-2 py-1 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                        defaultValue=""
                      >
                        <option value="">Load preset...</option>
                        {presets.map((p) => (
                          <option key={p.id} value={p.id}>{p.name}</option>
                        ))}
                      </select>
                    )}
                    <button
                      onClick={() => setShowSavePreset(!showSavePreset)}
                      className="flex items-center gap-1 rounded-lg border border-slate-300 px-2 py-1 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                    >
                      <Bookmark className="h-3 w-3" /> Save preset
                    </button>
                  </div>
                </div>

                {/* Save preset form */}
                {showSavePreset && (
                  <div className="mb-3 rounded-lg bg-slate-50 p-3 dark:bg-slate-900">
                    <div className="flex items-center gap-2">
                      <input
                        type="text"
                        value={presetName}
                        onChange={(e) => setPresetName(e.target.value)}
                        placeholder="Preset name..."
                        className="flex-1 rounded-lg border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                      />
                      <select
                        value={presetTarget}
                        onChange={(e) => setPresetTarget(e.target.value as 'default' | 'custom')}
                        className="rounded-lg border border-slate-300 px-2 py-1.5 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                      >
                        <option value="custom">Custom instructions</option>
                        <option value="default">Default prompt</option>
                      </select>
                      <button
                        onClick={savePreset}
                        disabled={!presetName.trim()}
                        className="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50"
                      >
                        Save
                      </button>
                      <button
                        onClick={() => { setShowSavePreset(false); setPresetName('') }}
                        className="rounded-lg p-1.5 text-slate-400 hover:text-slate-600"
                      >
                        <X className="h-4 w-4" />
                      </button>
                    </div>
                  </div>
                )}

                {/* Saved presets list */}
                {presets.length > 0 && (
                  <div className="mb-3 flex flex-wrap gap-2">
                    {presets.map((p) => (
                      <div
                        key={p.id}
                        className="group flex items-center gap-1.5 rounded-full bg-slate-100 px-3 py-1 dark:bg-slate-800"
                      >
                        <button
                          onClick={() => loadPreset(p)}
                          className="text-xs font-medium text-slate-700 hover:text-blue-600 dark:text-slate-300 dark:hover:text-blue-400"
                          title={p.prompt.slice(0, 80) + '...'}
                        >
                          {p.name}
                        </button>
                        <button
                          onClick={() => loadPresetToDefault(p)}
                          className="text-[10px] text-slate-400 hover:text-blue-500"
                          title="Load into default prompt"
                        >
                          →default
                        </button>
                        <button
                          onClick={() => deletePreset(p.id)}
                          className="text-slate-400 hover:text-rose-500"
                          title="Delete preset"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </div>
                    ))}
                  </div>
                )}

                {/* Default prompt (read-only with edit button) */}
                <div className="mb-3">
                  <div className="mb-1.5 flex items-center justify-between">
                    <label className="flex items-center gap-1.5 font-mono text-xs text-slate-500 dark:text-slate-400">
                      Default prompt
                      {promptOverride !== null && (
                        <span className="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] text-amber-600 dark:bg-amber-900/30 dark:text-amber-400">
                          edited
                        </span>
                      )}
                    </label>
                    <div className="flex items-center gap-1">
                      {promptOverride !== null && !editingPrompt && (
                        <button
                          onClick={resetPromptToDefault}
                          className="rounded px-2 py-0.5 text-[10px] text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
                        >
                          Reset to auto
                        </button>
                      )}
                      {!editingPrompt ? (
                        <button
                          onClick={startEditPrompt}
                          className="flex items-center gap-1 rounded-lg border border-slate-300 px-2 py-1 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                        >
                          <Edit3 className="h-3 w-3" /> Edit
                        </button>
                      ) : (
                        <>
                          <button
                            onClick={savePromptEdit}
                            className="flex items-center gap-1 rounded-lg bg-emerald-600 px-2 py-1 text-xs text-white hover:bg-emerald-500"
                          >
                            <Save className="h-3 w-3" /> Save
                          </button>
                          <button
                            onClick={cancelPromptEdit}
                            className="flex items-center gap-1 rounded-lg border border-slate-300 px-2 py-1 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                          >
                            <X className="h-3 w-3" /> Cancel
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                  <textarea
                    value={effectiveDefaultPrompt}
                    onChange={(e) => setPromptOverride(e.target.value)}
                    readOnly={!editingPrompt}
                    rows={8}
                    className={`w-full rounded-lg border p-3 font-mono text-xs ${
                      editingPrompt
                        ? 'border-blue-400 bg-white dark:border-blue-500 dark:bg-slate-900 dark:text-white'
                        : 'border-slate-200 bg-slate-50 text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400'
                    }`}
                    placeholder="Default prompt will appear here when a frame is selected..."
                  />
                  {!editingPrompt && (
                    <p className="mt-1 font-mono text-[10px] text-slate-400">
                      Auto-generated based on available data (G-code, temps, etc.). Click Edit to customize.
                    </p>
                  )}
                </div>

                {/* Custom prompt (always editable) */}
                <div className="mb-3">
                  <label className="mb-1.5 block font-mono text-xs text-slate-500 dark:text-slate-400">
                    Additional instructions (always editable)
                  </label>
                  <textarea
                    value={customPrompt}
                    onChange={(e) => setCustomPrompt(e.target.value)}
                    rows={3}
                    className="w-full rounded-lg border border-slate-300 bg-white p-3 font-mono text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                    placeholder="Add specific instructions for this analysis, e.g. 'Focus on the first layer adhesion' or 'Check for stringing on the overhangs'..."
                  />
                </div>

                {/* Analyze button */}
                <div className="flex items-center justify-between">
                  {!apiKeyAvailable && (
                    <p className="font-mono text-xs text-amber-600 dark:text-amber-400">
                      No Gemini API key configured. Add one in Settings.
                    </p>
                  )}
                  <button
                    onClick={analyzeFrame}
                    disabled={analyzing || !currentFrameUrl}
                    className="ml-auto flex items-center gap-2 rounded-lg bg-purple-600 px-4 py-2 text-sm font-medium text-white hover:bg-purple-500 disabled:opacity-50"
                  >
                    {analyzing ? (
                      <><Loader2 className="h-4 w-4 animate-spin" /> Analyzing...</>
                    ) : (
                      <><Sparkles className="h-4 w-4" /> Analyze frame {currentFrameIdx + 1}</>
                    )}
                  </button>
                </div>

                {analysisError && (
                  <div className="mt-3 rounded-lg bg-rose-50 p-3 font-mono text-xs text-rose-600 dark:bg-rose-900/20 dark:text-rose-400">
                    {analysisError}
                  </div>
                )}

                {analysis && (
                  <div className="mt-3 rounded-lg bg-slate-50 p-4 dark:bg-slate-900">
                    <pre className="whitespace-pre-wrap font-mono text-xs text-slate-700 dark:text-slate-300">
                      {analysis}
                    </pre>
                  </div>
                )}
              </div>
            </>
          ) : (
            !loading && sessionDetail && (
              <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
                <Film className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-700" />
                <p className="font-mono text-sm text-slate-400">
                  No timelapse frames available for this session.
                </p>
              </div>
            )
          )}
        </>
      )}

      {/* Empty state */}
      {!loading && !sessionDetail && !selectedSessionId && (
        <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
          <Activity className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-700" />
          <p className="font-mono text-sm text-slate-400">
            Select a print session to start analyzing.
          </p>
        </div>
      )}
    </div>
  )
}
