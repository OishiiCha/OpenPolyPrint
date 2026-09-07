import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import {
  Cable, RefreshCw, Trash2, Download, Sparkles, Loader2, Search,
  ChevronDown, ChevronRight, MonitorSmartphone, FileCode2, Files,
  SlidersHorizontal, AlertTriangle, CheckCircle2, Copy, Check,
} from 'lucide-react'

interface BridgeInstance {
  instanceId: string
  hostname: string
  platform?: string
  orcaVersion?: string
  pluginVersion?: string
  capabilities?: string[]
  firstSeen: number
  lastSeen: number
  online: boolean
}

type ArtifactType = 'gcode' | 'presets' | 'settings'

interface Artifact {
  id: string
  instanceId: string
  instanceHost?: string
  type: ArtifactType
  name: string
  filename?: string
  size: number
  gcodeLines?: number
  settings?: Record<string, string>
  presets?: Record<string, unknown>
  analysis?: string
  analyzedAt?: number
  createdAt: number
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function timeAgo(ts: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - ts))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

const typeIcon = (t: ArtifactType) =>
  t === 'gcode' ? FileCode2 : t === 'presets' ? Files : SlidersHorizontal

const typeColor = (t: ArtifactType) =>
  t === 'gcode'
    ? 'text-purple-400 bg-purple-900/30'
    : t === 'presets'
      ? 'text-emerald-400 bg-emerald-900/30'
      : 'text-amber-400 bg-amber-900/30'

export function SlicerBridge() {
  const [instances, setInstances] = useState<BridgeInstance[]>([])
  const [aiEnabled, setAiEnabled] = useState(true)
  const [artifacts, setArtifacts] = useState<Artifact[]>([])
  const [filter, setFilter] = useState<'all' | ArtifactType>('all')
  const [selected, setSelected] = useState<Artifact | null>(null)
  const [settingsQuery, setSettingsQuery] = useState('')
  const [analyzing, setAnalyzing] = useState(false)
  const [analysis, setAnalysis] = useState<string | null>(null)
  const [analyzeError, setAnalyzeError] = useState<string | null>(null)
  const [customPrompt, setCustomPrompt] = useState('')
  const [copied, setCopied] = useState(false)
  const selectedRef = useRef<string | null>(null)

  const load = useCallback(() => {
    fetch('/api/orca/instances')
      .then((r) => r.json())
      .then((data: { instances?: BridgeInstance[]; aiEnabled?: boolean }) => {
        setInstances(data.instances ?? [])
        if (data.aiEnabled !== undefined) setAiEnabled(data.aiEnabled)
      })
      .catch(() => {})
    fetch('/api/orca/artifacts')
      .then((r) => r.json())
      .then((data: { artifacts?: Artifact[] }) => setArtifacts(data.artifacts ?? []))
      .catch(() => {})
  }, [])

  useEffect(() => {
    load()
    const id = setInterval(load, 5000)
    return () => clearInterval(id)
  }, [load])

  // Refresh the selected artifact's detail (with analysis) when selection changes
  // or when the list reloads brings a new id.
  useEffect(() => {
    const id = selectedRef.current
    if (!id) return
    fetch(`/api/orca/artifacts/${id}`)
      .then((r) => (r.ok ? r.json() : null))
      .then((a: Artifact | null) => {
        if (a && selectedRef.current === a.id) {
          setSelected(a)
          if (!analyzing) setAnalysis(a.analysis || null)
        }
      })
      .catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [artifacts])

  const select = (a: Artifact) => {
    selectedRef.current = a.id
    setSelected(a)
    setAnalysis(a.analysis || null)
    setAnalyzeError(null)
    setSettingsQuery('')
    // The list endpoint strips heavy payloads (presets); fetch the full detail.
    fetch(`/api/orca/artifacts/${a.id}`)
      .then((r) => (r.ok ? r.json() : null))
      .then((full: Artifact | null) => {
        if (full && selectedRef.current === full.id) {
          setSelected(full)
          if (!analyzing) setAnalysis(full.analysis || null)
        }
      })
      .catch(() => {})
  }

  const filtered = useMemo(
    () => (filter === 'all' ? artifacts : artifacts.filter((a) => a.type === filter)),
    [artifacts, filter],
  )

  const settingEntries = useMemo(() => {
    const settings = selected?.settings ?? {}
    const q = settingsQuery.trim().toLowerCase()
    return Object.entries(settings)
      .filter(([k, v]) => !q || k.toLowerCase().includes(q) || v.toLowerCase().includes(q))
      .sort(([a], [b]) => a.localeCompare(b))
  }, [selected, settingsQuery])

  const runAnalysis = async () => {
    if (!selected) return
    setAnalyzing(true)
    setAnalyzeError(null)
    setAnalysis(null)
    try {
      const res = await fetch(`/api/orca/artifacts/${selected.id}/analyze`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ customPrompt: customPrompt.trim() || undefined }),
      })
      const data = await res.json()
      if (!res.ok) {
        setAnalyzeError(data.error || `analysis failed (${res.status})`)
      } else {
        setAnalysis(data.analysis ?? '')
      }
    } catch (e) {
      setAnalyzeError(String(e))
    } finally {
      setAnalyzing(false)
    }
  }

  const removeArtifact = async (id: string) => {
    if (!window.confirm('Delete this synced artifact?')) return
    await fetch(`/api/orca/artifacts/${id}`, { method: 'DELETE' }).catch(() => {})
    if (selectedRef.current === id) {
      selectedRef.current = null
      setSelected(null)
      setAnalysis(null)
    }
    load()
  }

  const filters: { id: 'all' | ArtifactType; label: string; count: number }[] = [
    { id: 'all', label: 'All', count: artifacts.length },
    { id: 'gcode', label: 'G-code', count: artifacts.filter((a) => a.type === 'gcode').length },
    { id: 'presets', label: 'Presets', count: artifacts.filter((a) => a.type === 'presets').length },
    { id: 'settings', label: 'Settings', count: artifacts.filter((a) => a.type === 'settings').length },
  ]

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold text-slate-900 dark:text-white">
            <Cable className="h-6 w-6 text-blue-500" />
            Slicer Bridge
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            OrcaSlicer instances synced via the OpenPolyPrint Bridge plugin — scan their G-code,
            presets and settings, and run AI checks.
          </p>
        </div>
        <button
          onClick={load}
          className="flex items-center gap-2 rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
        >
          <RefreshCw className="h-4 w-4" /> Refresh
        </button>
      </div>

      {/* Connect-a-slicer panel: the URL shown is how this browser reaches the
          server right now, so it is always a working network address for the
          plugin on the OrcaSlicer device. */}
      <section className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-sm font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
              Connect OrcaSlicer
            </h2>
            <p className="mt-1 text-sm text-slate-600 dark:text-slate-300">
              On the device running OrcaSlicer, point the OpenPolyPrint Bridge plugin at this
              server address (not localhost — OrcaSlicer is on another machine):
            </p>
          </div>
          <div className="flex items-center gap-2">
            <code className="rounded-lg bg-slate-100 px-3 py-2 text-sm font-medium text-slate-900 dark:bg-slate-800 dark:text-white">
              {window.location.origin}
            </code>
            <button
              onClick={() => {
                navigator.clipboard?.writeText(window.location.origin).then(
                  () => {
                    setCopied(true)
                    setTimeout(() => setCopied(false), 1500)
                  },
                  () => {},
                )
              }}
              className="flex items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-2 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
            >
              {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
              {copied ? 'Copied' : 'Copy'}
            </button>
          </div>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <a
            href="/api/orca/plugin"
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"
          >
            <Download className="h-4 w-4" /> Download plugin
          </a>
          <span className="text-xs text-slate-500 dark:text-slate-400">
            or download an installer and run it on the OrcaSlicer machine:
          </span>
          {(
            [
              ['windows', 'Windows'],
              ['mac', 'macOS'],
              ['linux', 'Linux'],
            ] as const
          ).map(([p, label]) => (
            <a
              key={p}
              href={`/api/orca/install/${p}`}
              className="flex items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-2 text-xs font-medium text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
            >
              <MonitorSmartphone className="h-3.5 w-3.5" /> {label} installer
            </a>
          ))}
        </div>
        <details className="mt-3 text-sm text-slate-600 dark:text-slate-400">
          <summary className="cursor-pointer font-medium text-slate-700 dark:text-slate-200">
            Install steps (Windows · macOS · Linux)
          </summary>
          <ol className="mt-3 list-decimal space-y-1.5 pl-5">
            <li>
              Easiest: download the installer for your platform (buttons above) on the OrcaSlicer
              machine and run it — it downloads the plugin straight from this server into the
              right <code>orca_plugins</code> folder, then prints the next steps.
            </li>
            <li>
              Manual alternative: <a
                href="/api/orca/plugin"
                className="font-medium text-blue-600 underline dark:text-blue-400"
              >
                download <code>plugin.py</code>
              </a>{' '}
              (pure Python, no dependencies — works on Windows, macOS and Linux, x64 and ARM) and
              save it as{' '}
              <code>&lt;OrcaSlicer data dir&gt;/orca_plugins/OpenPolyPrintBridge/plugin.py</code>:
              <ul className="mt-1 list-disc pl-5 text-xs">
                <li>
                  Windows: <code>%APPDATA%\OrcaSlicer\orca_plugins\</code>
                </li>
                <li>
                  macOS: <code>~/Library/Application Support/OrcaSlicer/orca_plugins/</code>
                </li>
                <li>
                  Linux: <code>~/.config/OrcaSlicer/orca_plugins/</code>
                </li>
              </ul>
            </li>
            <li>
              Restart OrcaSlicer, open File → Plugins → <em>OpenPolyPrint Bridge</em>, paste the
              server address above, and set the passcode if OpenPolyPrint uses one.
            </li>
            <li>
              Allow the network permission prompt, then run <em>Sync Now</em> to test the
              connection. The slicer appears above within seconds; every slice uploads
              automatically from then on.
            </li>
          </ol>
          <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
            The plugin only makes outbound connections to this server — no inbound ports or
            firewall rules are needed on the OrcaSlicer machine.
          </p>
        </details>
      </section>

      {/* AI availability banner */}
      {!aiEnabled && (
        <div className="flex items-start gap-3 rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" />
          <div>
            AI checks are disabled — no Gemini API key is configured on this server. Set one in{' '}
            <Link to="/settings" className="font-medium underline">
              Settings
            </Link>{' '}
            (or via the <code>GEMINI_API_KEY</code> env var) to enable G-code, preset and settings
            review.
          </div>
        </div>
      )}

      {/* Connected slicers */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
          Connected Slicers
        </h2>
        {instances.length === 0 ? (
          <div className="rounded-lg border border-dashed border-slate-300 p-6 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
            <MonitorSmartphone className="mx-auto mb-2 h-8 w-8 opacity-40" />
            No slicer has connected yet. Install the OpenPolyPrint Bridge plugin in OrcaSlicer —
            see the setup notes below.
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {instances.map((inst) => (
              <div
                key={inst.instanceId}
                className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900"
              >
                <div className="flex items-center justify-between">
                  <span className="font-medium text-slate-900 dark:text-white">{inst.hostname}</span>
                  <span
                    className={`flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ${
                      inst.online
                        ? 'bg-emerald-900/30 text-emerald-400'
                        : 'bg-slate-700/50 text-slate-400'
                    }`}
                  >
                    <span
                      className={`h-1.5 w-1.5 rounded-full ${inst.online ? 'bg-emerald-400' : 'bg-slate-500'}`}
                    />
                    {inst.online ? 'online' : 'offline'}
                  </span>
                </div>
                <dl className="mt-2 space-y-0.5 text-xs text-slate-500 dark:text-slate-400">
                  {inst.platform && <div>{inst.platform}</div>}
                  {inst.orcaVersion && <div>OrcaSlicer {inst.orcaVersion}</div>}
                  {inst.pluginVersion && <div>Bridge plugin v{inst.pluginVersion}</div>}
                  <div>Last heartbeat {timeAgo(inst.lastSeen)}</div>
                </dl>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Artifacts + detail */}
      <section className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        {/* List */}
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="mr-auto text-sm font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
              Synced Artifacts
            </h2>
            {filters.map((f) => (
              <button
                key={f.id}
                onClick={() => setFilter(f.id)}
                className={`rounded-full px-3 py-1 text-xs font-medium ${
                  filter === f.id
                    ? 'bg-blue-600 text-white'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700'
                }`}
              >
                {f.label} ({f.count})
              </button>
            ))}
          </div>
          {filtered.length === 0 ? (
            <div className="rounded-lg border border-dashed border-slate-300 p-6 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
              Nothing synced yet. Slice something in OrcaSlicer (G-code uploads automatically) or
              run the plugin's “Sync Now”.
            </div>
          ) : (
            <ul className="space-y-2">
              {filtered.map((a) => {
                const Icon = typeIcon(a.type)
                return (
                  <li key={a.id}>
                    <button
                      onClick={() => select(a)}
                      className={`flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors ${
                        selected?.id === a.id
                          ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/30'
                          : 'border-slate-200 bg-white hover:bg-slate-50 dark:border-slate-800 dark:bg-slate-900 dark:hover:bg-slate-800'
                      }`}
                    >
                      <span className={`rounded-lg p-2 ${typeColor(a.type)}`}>
                        <Icon className="h-4 w-4" />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium text-slate-900 dark:text-white">
                          {a.name}
                        </span>
                        <span className="block text-xs text-slate-500 dark:text-slate-400">
                          {a.type} · {a.instanceHost || a.instanceId.slice(0, 8)} ·{' '}
                          {timeAgo(a.createdAt)}
                        </span>
                      </span>
                      {a.analysis && (
                        <span className="rounded-full bg-emerald-900/30 px-2 py-0.5 text-xs text-emerald-400">
                          reviewed
                        </span>
                      )}
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </div>

        {/* Detail */}
        <div className="space-y-4">
          {!selected ? (
            <div className="flex h-64 items-center justify-center rounded-lg border border-dashed border-slate-300 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
              Select an artifact to inspect its settings
            </div>
          ) : (
            <>
              <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <h3 className="truncate text-lg font-semibold text-slate-900 dark:text-white">
                      {selected.name}
                    </h3>
                    <p className="text-xs text-slate-500 dark:text-slate-400">
                      {selected.type} · synced {timeAgo(selected.createdAt)} from{' '}
                      {selected.instanceHost || selected.instanceId.slice(0, 8)}
                      {selected.filename ? ` · ${selected.filename}` : ''}
                      {selected.size ? ` · ${formatSize(selected.size)}` : ''}
                      {selected.gcodeLines ? ` · ${selected.gcodeLines.toLocaleString()} lines` : ''}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <a
                      href={`/api/orca/artifacts/${selected.id}/content`}
                      className="flex items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-1.5 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                    >
                      <Download className="h-3.5 w-3.5" /> Download
                    </a>
                    <button
                      onClick={() => removeArtifact(selected.id)}
                      className="flex items-center gap-1.5 rounded-lg border border-red-300 px-3 py-1.5 text-xs text-red-600 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950/30"
                    >
                      <Trash2 className="h-3.5 w-3.5" /> Delete
                    </button>
                  </div>
                </div>

                {/* AI check */}
                <div className="mt-4 space-y-2 border-t border-slate-200 pt-4 dark:border-slate-800">
                  <div className="flex flex-wrap items-center gap-2">
                    <button
                      onClick={runAnalysis}
                      disabled={analyzing || !aiEnabled}
                      className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
                      title={aiEnabled ? '' : 'Set a Gemini API key in Settings first'}
                    >
                      {analyzing ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Sparkles className="h-4 w-4" />
                      )}
                      {analyzing ? 'Analyzing…' : 'AI Check'}
                    </button>
                    {selected.analyzedAt && !analyzing && (
                      <span className="text-xs text-slate-500 dark:text-slate-400">
                        last reviewed {timeAgo(selected.analyzedAt)}
                      </span>
                    )}
                  </div>
                  <input
                    value={customPrompt}
                    onChange={(e) => setCustomPrompt(e.target.value)}
                    placeholder="Optional focus for the review (e.g. 'check for PLA stringing')"
                    className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
                  />
                  {analyzeError && (
                    <p className="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-400">
                      {analyzeError}
                    </p>
                  )}
                  {analysis && (
                    <div className="max-h-96 overflow-y-auto rounded-lg bg-slate-50 p-4 text-sm leading-relaxed text-slate-800 dark:bg-slate-950 dark:text-slate-200">
                      <ReactMarkdown>{analysis}</ReactMarkdown>
                    </div>
                  )}
                </div>
              </div>

              {/* Settings table */}
              {selected.settings && Object.keys(selected.settings).length > 0 && (
                <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="text-sm font-semibold text-slate-900 dark:text-white">
                      Settings ({Object.keys(selected.settings).length})
                    </h3>
                    <div className="relative">
                      <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-slate-400" />
                      <input
                        value={settingsQuery}
                        onChange={(e) => setSettingsQuery(e.target.value)}
                        placeholder="Filter settings…"
                        className="w-56 rounded-lg border border-slate-300 bg-white py-2 pl-8 pr-3 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
                      />
                    </div>
                  </div>
                  <div className="mt-3 max-h-96 overflow-y-auto rounded-lg border border-slate-200 dark:border-slate-800">
                    <table className="w-full text-left text-xs">
                      <tbody>
                        {settingEntries.map(([k, v]) => (
                          <tr key={k} className="border-b border-slate-100 last:border-0 dark:border-slate-800">
                            <td className="w-1/2 break-all px-3 py-1.5 font-mono text-slate-500 dark:text-slate-400">
                              {k}
                            </td>
                            <td className="break-all px-3 py-1.5 font-mono text-slate-900 dark:text-slate-100">
                              {v}
                            </td>
                          </tr>
                        ))}
                        {settingEntries.length === 0 && (
                          <tr>
                            <td className="px-3 py-4 text-center text-slate-400" colSpan={2}>
                              no settings match “{settingsQuery}”
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {/* Presets viewer */}
              {selected.presets && <PresetsViewer presets={selected.presets} />}
            </>
          )}
        </div>
      </section>
    </div>
  )
}

// PresetsViewer renders a preset bundle snapshot: collections with counts and
// selected presets, plus raw JSON fallback for anything unexpected.
function PresetsViewer({ presets }: { presets: Record<string, unknown> }) {
  const error = typeof presets.error === 'string' ? presets.error : null
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
      <h3 className="text-sm font-semibold text-slate-900 dark:text-white">Preset Bundle</h3>
      {error ? (
        <p className="mt-2 flex items-center gap-2 text-sm text-amber-600 dark:text-amber-400">
          <AlertTriangle className="h-4 w-4" /> {error}
        </p>
      ) : (
        <div className="mt-3 space-y-2">
          {Object.entries(presets).map(([key, value]) => (
            <PresetSection key={key} name={key} value={value} />
          ))}
        </div>
      )}
    </div>
  )
}

function PresetSection({ name, value }: { name: string; value: unknown }) {
  const [open, setOpen] = useState(false)
  const isCollection =
    value !== null &&
    typeof value === 'object' &&
    !Array.isArray(value) &&
    Array.isArray((value as { presets?: unknown }).presets)

  if (!isCollection) {
    return (
      <div className="rounded-lg border border-slate-200 px-3 py-2 text-xs dark:border-slate-800">
        <div className="font-mono font-medium text-slate-700 dark:text-slate-200">{name}</div>
        <div className="mt-1 break-all font-mono text-slate-500 dark:text-slate-400">
          {JSON.stringify(value)}
        </div>
      </div>
    )
  }

  const collection = value as { count?: number; selected?: string; presets: { name?: string }[] }
  return (
    <div className="rounded-lg border border-slate-200 dark:border-slate-800">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs"
      >
        {open ? (
          <ChevronDown className="h-3.5 w-3.5 text-slate-400" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 text-slate-400" />
        )}
        <span className="font-mono font-medium text-slate-700 dark:text-slate-200">{name}</span>
        <span className="text-slate-400">
          ({collection.presets.length} {collection.presets.length === 1 ? 'preset' : 'presets'})
        </span>
        {collection.selected && (
          <span className="ml-auto flex items-center gap-1 rounded-full bg-blue-600/20 px-2 py-0.5 text-blue-500 dark:text-blue-400">
            <CheckCircle2 className="h-3 w-3" /> {collection.selected}
          </span>
        )}
      </button>
      {open && (
        <ul className="max-h-60 overflow-y-auto border-t border-slate-100 px-3 py-2 text-xs dark:border-slate-800">
          {collection.presets.map((p, i) => (
            <li key={i} className="py-0.5 font-mono text-slate-600 dark:text-slate-300">
              {p.name ?? JSON.stringify(p)}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
