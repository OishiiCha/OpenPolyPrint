import { useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link, useParams } from 'react-router-dom'
import { loadConfig, saveConfig, type AppConfig, type ProviderConfig } from '../config'
import { Switch } from '../components/Switch'
import { BedPreview } from '../components/BedPreview'
import { GCodePreview } from '../components/GCodePreview'
import { TempChart } from '../components/TempChart'
import { usePushNotifications } from '../hooks/usePushNotifications'
import {
  Eye,
  EyeOff,
  FileCode2,
  Flame,
  Home,
  Layers,
  Lightbulb,
  MoreVertical,
  Pause,
  Plus,
  Printer as PrinterIcon,
  Search,
  Settings as SettingsIcon,
  SlidersHorizontal,
  Square,
  Thermometer,
  Timer,
  Trash2,
  Upload,
  Video,
  X,
} from 'lucide-react'
import { usePrinters } from '../hooks/usePrinters'
import { useGCodeFiles } from '../hooks/useGCodeFiles'
import { useCameras } from '../hooks/useCameras'
import { usePiReadings } from '../hooks/usePi'
import { isTest } from '../data/mock'
import { integrationIcons, integrations, testIntegration, type Integration } from '../integrations'
import type { Camera, GCodeFile, PrintRecord, Printer } from '../types'

const statusColor: Record<string, string> = {
  Idle: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900 dark:text-emerald-300',
  Printing: 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300',
  Paused: 'bg-amber-100 text-amber-700 dark:bg-amber-900 dark:text-amber-300',
  Offline: 'bg-slate-200 text-slate-700 dark:bg-slate-800 dark:text-slate-400',
  Heating: 'bg-rose-100 text-rose-700 dark:bg-rose-900 dark:text-rose-300',
}

const statusAccent: Record<string, string> = {
  Idle: '#10b981',
  Printing: '#3b82f6',
  Paused: '#f59e0b',
  Offline: '#64748b',
  Heating: '#f43f5e',
}

const resultColor: Record<string, string> = {
  Success: 'text-emerald-600 dark:text-emerald-400',
  Failed: 'text-rose-600 dark:text-rose-400',
  Cancelled: 'text-amber-600 dark:text-amber-400',
}

export function SectionTitle({ title, action }: { title: string; action?: React.ReactNode }) {
  return (
    <div className="mb-4 flex items-center justify-between">
      <h1 className="text-2xl font-mono font-semibold text-blue-600 dark:text-blue-400">
        <span className="mr-2 text-slate-500 dark:text-slate-400">&gt;</span>[ {title} ]
        <span className="ml-1 animate-pulse">_</span>
      </h1>
      {action}
    </div>
  )
}

export function Card({
  children,
  className = '',
  color,
}: {
  children: React.ReactNode
  className?: string
  color?: string
}) {
  return (
    <div
      className={`rounded-none border-2 border-slate-300 border-t-4 border-t-blue-500 bg-white p-6 shadow-md shadow-blue-500/10 transition-shadow hover:shadow-lg dark:border-slate-700 dark:bg-slate-950 dark:shadow-blue-500/20 ${className}`}
      style={color ? { borderTopColor: color, boxShadow: `0 4px 6px -1px ${color}20` } : undefined}
    >
      {children}
    </div>
  )
}

function ConfirmModal({
  title,
  message,
  onConfirm,
  onClose,
}: {
  title: string
  message: string
  onConfirm: () => void
  onClose: () => void
}) {
  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark w-full max-w-md rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-6 shadow-2xl shadow-blue-500/20"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-2 text-xl font-mono font-semibold text-blue-400">[ {title} ]</h2>
        <p className="mb-6 font-mono text-sm text-slate-300">{message}</p>
        <div className="flex justify-end gap-3">
          <button
            onClick={onClose}
            className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700"
          >
            cancel
          </button>
          <button
            onClick={onConfirm}
            className="rounded-lg bg-rose-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-rose-500"
          >
            confirm
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
}

function PrinterGCodePreview({ fileName }: { fileName: string }) {
  const [gcode, setGcode] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const listRes = await fetch('/api/gcode')
        if (!listRes.ok) { if (!cancelled) setError(true); return }
        const files = await listRes.json()
        const match = (Array.isArray(files) ? files : []).find(
          (f: any) => f.name === fileName || f.name === fileName + '.gcode' || fileName.endsWith(f.name)
        )
        if (!match) { if (!cancelled) { setError(true); setLoading(false) }; return }
        const contentRes = await fetch(`/api/gcode/${encodeURIComponent(match.id)}`)
        if (!contentRes.ok) { if (!cancelled) setError(true); return }
        const text = await contentRes.text()
        if (!cancelled) { setGcode(text); setLoading(false) }
      } catch {
        if (!cancelled) { setError(true); setLoading(false) }
      }
    }
    load()
    return () => { cancelled = true }
  }, [fileName])

  if (loading) {
    return (
      <div className="mt-4 h-40 rounded-lg border border-slate-700 bg-slate-950 flex items-center justify-center">
        <p className="font-mono text-xs text-slate-500">loading gcode preview...</p>
      </div>
    )
  }

  if (error || !gcode) {
    return null
  }

  return (
    <div className="mt-4">
      <div className="mb-2 flex items-center gap-2">
        <FileCode2 className="h-3.5 w-3.5 text-purple-400" />
        <span className="font-mono text-xs text-slate-400">gcode preview</span>
      </div>
      <div className="h-48 rounded-lg border border-slate-700 overflow-hidden">
        <GCodePreview gcode={gcode} className="w-full h-full" />
      </div>
    </div>
  )
}

function PrinterCard({ printer, onOpen }: { printer: Printer; onOpen?: () => void }) {
  const [showConfirm, setShowConfirm] = useState(false)
  const [showPauseConfirm, setShowPauseConfirm] = useState(false)

  return (
    <>
      <Card color={statusAccent[printer.status] || '#3b82f6'} className="relative">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="rounded-xl bg-slate-100 p-2.5 dark:bg-slate-800">
              <PrinterIcon className="h-6 w-6 text-slate-600 dark:text-slate-300" />
            </div>
            <div>
              <h3 className="font-semibold text-slate-900 dark:text-white">{printer.name}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 capitalize">{printer.type}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <span className={`rounded-full px-2.5 py-1 text-xs font-medium ${statusColor[printer.status]}`}>
              {printer.status}
            </span>
            {onOpen && (
              <button
                onClick={onOpen}
                className="flex items-center gap-1.5 rounded-lg bg-slate-100 px-2.5 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
              >
                <SlidersHorizontal className="h-3.5 w-3.5" /> Controls
              </button>
            )}
          </div>
        </div>

        <div className="mt-5 grid grid-cols-2 gap-4">
          <div className="flex items-center gap-2 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
            <Flame className="h-4 w-4 text-rose-500" />
            <div>
              <p className="text-xs text-slate-500 dark:text-slate-400">Nozzle</p>
              <p className="font-semibold text-slate-900 dark:text-white">
                {printer.temps.nozzle}° / {printer.temps.targetNozzle}°
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
            <Thermometer className="h-4 w-4 text-blue-500" />
            <div>
              <p className="text-xs text-slate-500 dark:text-slate-400">Bed</p>
              <p className="font-semibold text-slate-900 dark:text-white">
                {printer.temps.bed}° / {printer.temps.targetBed}°
              </p>
            </div>
          </div>
        </div>

        {printer.status === 'Printing' && (
          <div className="mt-5">
            <div className="mb-2 flex justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>{printer.currentFile}</span>
              <span>{printer.remainingTime} left</span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
              <div
                className="h-full rounded-full bg-blue-600 transition-all"
                style={{ width: `${printer.progress}%` }}
              />
            </div>
            <p className="mt-2 text-right text-sm font-semibold text-blue-600 dark:text-blue-400">
              {printer.progress}%
            </p>
          </div>
        )}

        {printer.status === 'Printing' && printer.currentFile && (
          <PrinterGCodePreview fileName={printer.currentFile} />
        )}

        <div className="mt-5 flex gap-2">
          <button
            onClick={(e) => {
              e.stopPropagation()
              setShowPauseConfirm(true)
            }}
            className="flex flex-1 items-center justify-center gap-2 rounded-lg bg-slate-100 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
          >
            <Pause className="h-4 w-4" /> Pause
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation()
              setShowConfirm(true)
            }}
            className="flex flex-1 items-center justify-center gap-2 rounded-lg bg-rose-100 px-3 py-2 text-sm font-medium text-rose-700 hover:bg-rose-200 dark:bg-rose-900/30 dark:text-rose-300 dark:hover:bg-rose-900/50"
          >
            <Square className="h-4 w-4" /> Stop
          </button>
        </div>
      </Card>

      {showConfirm && (
        <ConfirmModal
          title="stop print"
          message={`Are you sure you want to stop the current print on ${printer.name}?`}
          onConfirm={() => {
            setShowConfirm(false)
            if (isTest) {
              alert(`[test] stop ${printer.name}`)
              return
            }
            fetch(`/api/printers/${printer.id}/stop`, { method: 'POST' }).catch((err) =>
              alert(`Stop failed: ${err}`)
            )
          }}
          onClose={() => setShowConfirm(false)}
        />
      )}

      {showPauseConfirm && (
        <ConfirmModal
          title="pause print"
          message={`Are you sure you want to pause the current print on ${printer.name}?`}
          onConfirm={() => {
            setShowPauseConfirm(false)
            if (isTest) {
              alert(`[test] pause ${printer.name}`)
              return
            }
            fetch(`/api/printers/${printer.id}/pause`, { method: 'POST' }).catch((err) =>
              alert(`Pause failed: ${err}`)
            )
          }}
          onClose={() => setShowPauseConfirm(false)}
        />
      )}
    </>
  )
}

export function Dashboard() {
  const [cameraFilter, setCameraFilter] = useState('all')
  const { printers, loading } = usePrinters()
  const { cameras } = useCameras()
  const { readings: piReadings, toggleLight } = usePiReadings()
  const active = printers.filter((p) => p.status === 'Printing').length

  const uniquePrinterIds = [...new Set(cameras.map((c) => c.printerId))]
  const filteredCameras =
    cameraFilter === 'all'
      ? cameras
      : cameras.filter((c) => c.printerId === cameraFilter)

  const enabledSensors = piReadings?.sensors.filter((s) => s.enabled) ?? []
  const showLightButton = piReadings?.lightRelayEnabled ?? false
  const showSensors = enabledSensors.length > 0

  if (loading) {
    return (
      <div className="space-y-8">
        <SectionTitle title="Dashboard" />
        <p className="font-mono text-sm text-slate-500">loading printers...</p>
      </div>
    )
  }

  return (
    <div className="space-y-8">
      <SectionTitle
        title="Dashboard"
        action={
          <div className="flex flex-wrap items-center gap-2">
            {showLightButton && (
              <button
                onClick={() => toggleLight(!piReadings?.lightRelayOn)}
                className={`flex items-center gap-2 rounded-xl px-4 py-2 text-sm font-medium transition-colors ${
                  piReadings?.lightRelayOn
                    ? 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/40 dark:text-amber-300 dark:hover:bg-amber-900/60'
                    : 'bg-slate-100 text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700'
                }`}
                title={piReadings?.lightRelayOn ? 'Turn light off' : 'Turn light on'}
              >
                <Lightbulb className={`h-4 w-4 ${piReadings?.lightRelayOn ? 'fill-amber-400 text-amber-500' : ''}`} />
                {piReadings?.lightRelayOn ? 'light on' : 'light off'}
              </button>
            )}
            <div className="rounded-xl bg-emerald-50 px-4 py-2 text-sm font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
              {active} active print{active !== 1 ? 's' : ''}
            </div>
            <div className="rounded-xl bg-slate-100 px-4 py-2 text-sm font-medium text-slate-700 dark:bg-slate-800 dark:text-slate-300">
              {printers.length} printers
            </div>
          </div>
        }
      />

      {showSensors && (
        <div className="space-y-3">
          <h3 className="font-mono text-sm font-semibold text-slate-400">[ filament_box_sensors ]</h3>
          <div className="grid gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {enabledSensors.map((s) => (
              <div
                key={s.id}
                className="rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-950"
              >
                <div className="mb-3 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span
                      className="inline-block h-3 w-3 rounded-full"
                      style={{ backgroundColor: s.color || '#64748b' }}
                    />
                    <span className="font-mono text-sm font-semibold text-slate-900 dark:text-white">
                      {s.name || `Box ${s.id}`}
                    </span>
                  </div>
                  <span className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[10px] uppercase text-slate-500 dark:bg-slate-800 dark:text-slate-400">
                    {s.filamentType || '—'}
                  </span>
                </div>
                {s.error ? (
                  <p className="font-mono text-xs text-rose-500">{s.error}</p>
                ) : s.hasReading ? (
                  <div className="grid grid-cols-2 gap-2">
                    <div>
                      <p className="font-mono text-[10px] uppercase text-slate-400">temp</p>
                      <p className="font-mono text-lg font-semibold text-slate-900 dark:text-white">
                        {s.temp?.toFixed(1)}°<span className="text-xs text-slate-400">C</span>
                      </p>
                    </div>
                    <div>
                      <p className="font-mono text-[10px] uppercase text-slate-400">humidity</p>
                      <p className="font-mono text-lg font-semibold text-slate-900 dark:text-white">
                        {s.humidity?.toFixed(1)}<span className="text-xs text-slate-400">%</span>
                      </p>
                    </div>
                  </div>
                ) : (
                  <p className="font-mono text-xs text-slate-400">waiting for reading...</p>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-4">
        {printers.map((p) => (
          <PrinterCard key={p.id} printer={p} />
        ))}
      </div>

      {cameras.length > 0 && (
        <div className="space-y-4">
          <SectionTitle
            title="Live cameras"
            action={
              <select
                className="rounded-lg border border-slate-300 bg-slate-50 px-3 py-1.5 text-sm font-medium text-slate-900 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
                value={cameraFilter}
                onChange={(e) => setCameraFilter(e.target.value)}
              >
                <option value="all">All printers</option>
                {uniquePrinterIds.map((id) => (
                  <option key={id} value={id}>
                    {printers.find((p) => p.id === id)?.name ?? id}
                  </option>
                ))}
              </select>
            }
          />

          {filteredCameras.length === 0 ? (
            <p className="text-sm text-slate-500 dark:text-slate-400">No cameras for this selection.</p>
          ) : (
            <div className="space-y-6">
              {Object.entries(
                filteredCameras.reduce<Record<string, Camera[]>>((acc, c) => {
                  const key = c.printerId || 'unassigned'
                  ;(acc[key] ||= []).push(c)
                  return acc
                }, {})
              )
                .sort(([a], [b]) => (a === 'unassigned' ? 1 : b === 'unassigned' ? -1 : a.localeCompare(b)))
                .map(([printerId, groupCameras]) => {
                  const printer = printers.find((p) => p.id === printerId)
                  return (
                    <section key={printerId}>
                      <h4 className="mb-2 font-mono text-sm text-slate-400">
                        [{printer ? printer.name : 'unassigned'}]
                      </h4>
                      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                        {groupCameras.map((c) => (
                          <CameraCard key={c.id} camera={c} printers={printers} />
                        ))}
                      </div>
                    </section>
                  )
                })}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function PrinterModal({ printer, onClose }: { printer: Printer; onClose: () => void }) {
  const [tab, setTab] = useState<'controls' | 'leveling' | 'cameras' | 'gcode'>('controls')
  const { cameras: allCameras } = useCameras()
  const cameras = allCameras.filter((c) => c.printerId === printer.id)

  useEffect(() => {
    const handle = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handle)
    return () => window.removeEventListener('keydown', handle)
  }, [onClose])

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-4xl max-h-[90vh] flex-col overflow-hidden rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 shadow-2xl shadow-blue-500/20"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-800 bg-slate-900 px-6 py-4">
          <div>
            <h2 className="text-xl font-mono font-semibold text-blue-400">
              <span className="mr-2 text-slate-500">&gt;</span>[ {printer.name} ]
            </h2>
            <p className="font-mono text-sm text-slate-500">
              <span className="text-blue-400">status:</span> {printer.status.toLowerCase()}
            </p>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-2 text-slate-400 hover:bg-slate-800"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="flex border-b border-slate-800">
          <button
            onClick={() => setTab('controls')}
            className={`flex flex-1 items-center justify-center px-4 py-3 font-mono text-sm font-medium transition-colors ${
              tab === 'controls'
                ? 'bg-blue-600 text-white'
                : 'text-slate-400 hover:bg-slate-800'
            }`}
          >
            {tab === 'controls' ? '[ controls ]' : '  controls  '}
          </button>
          <button
            onClick={() => setTab('leveling')}
            className={`flex flex-1 items-center justify-center px-4 py-3 font-mono text-sm font-medium transition-colors ${
              tab === 'leveling'
                ? 'bg-blue-600 text-white'
                : 'text-slate-400 hover:bg-slate-800'
            }`}
          >
            {tab === 'leveling' ? '[ leveling ]' : '  leveling  '}
          </button>
          <button
            onClick={() => setTab('cameras')}
            className={`flex flex-1 items-center justify-center px-4 py-3 font-mono text-sm font-medium transition-colors ${
              tab === 'cameras'
                ? 'bg-blue-600 text-white'
                : 'text-slate-400 hover:bg-slate-800'
            }`}
          >
            {tab === 'cameras' ? `[ cameras[${cameras.length}] ]` : `  cameras[${cameras.length}]  `}
          </button>
          <button
            onClick={() => setTab('gcode')}
            className={`flex flex-1 items-center justify-center px-4 py-3 font-mono text-sm font-medium transition-colors ${
              tab === 'gcode'
                ? 'bg-blue-600 text-white'
                : 'text-slate-400 hover:bg-slate-800'
            }`}
          >
            {tab === 'gcode' ? '[ gcode ]' : '  gcode  '}
          </button>
        </div>

        <div className="flex-1 overflow-y-auto bg-slate-950 p-6">
          {tab === 'controls' && <ControlsView printer={printer} />}
          {tab === 'leveling' && <LevelingView printer={printer} />}
          {tab === 'cameras' && <CamerasView cameras={cameras} />}
          {tab === 'gcode' && <GCodeView printer={printer} />}
        </div>
      </div>
    </div>,
    document.body
  )
}

function ControlsView({ printer }: { printer: Printer }) {
  const post = async (path: string, body?: object) => {
    try {
      const res = await fetch(`/api/printers/${printer.id}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body ? JSON.stringify(body) : undefined,
      })
      if (!res.ok) throw new Error(await res.text())
    } catch (err) {
      console.error(err)
      alert(err instanceof Error ? err.message : 'Command failed')
    }
  }

  const jog = (axis: string, distance: number, feed: number) => {
    const gcode = `G91\nG0 ${axis}${distance} F${feed}\nG90`
    post('/gcode', { command: gcode })
  }

  return (
    <div className="space-y-6">
      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <h3 className="mb-4 font-mono font-semibold text-blue-400">[ temperatures ]</h3>
          <div className="space-y-4">
            <div>
              <div className="mb-1 flex justify-between text-sm">
                <span className="text-slate-600 dark:text-slate-300">Nozzle</span>
                <span className="font-semibold text-slate-900 dark:text-white">
                  {printer.temps.nozzle}° / {printer.temps.targetNozzle}°
                </span>
              </div>
              <div className="h-2 w-full rounded-full bg-slate-200 dark:bg-slate-800">
                <div
                  className="h-full rounded-full bg-rose-500"
                  style={{
                    width: `${Math.min((printer.temps.nozzle / (printer.temps.targetNozzle || 250)) * 100, 100)}%`,
                  }}
                />
              </div>
            </div>
            <div>
              <div className="mb-1 flex justify-between text-sm">
                <span className="text-slate-600 dark:text-slate-300">Bed</span>
                <span className="font-semibold text-slate-900 dark:text-white">
                  {printer.temps.bed}° / {printer.temps.targetBed}°
                </span>
              </div>
              <div className="h-2 w-full rounded-full bg-slate-200 dark:bg-slate-800">
                <div
                  className="h-full rounded-full bg-blue-500"
                  style={{
                    width: `${Math.min((printer.temps.bed / (printer.temps.targetBed || 110)) * 100, 100)}%`,
                  }}
                />
              </div>
            </div>
          </div>
        </Card>

        <Card>
          <h3 className="mb-4 font-mono font-semibold text-blue-400">[ macros ]</h3>
          <div className="grid grid-cols-3 gap-3">
            <button
              onClick={() => post('/home')}
              className="flex flex-col items-center justify-center gap-1 rounded-lg bg-slate-100 px-3 py-3 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
            >
              <Home className="h-5 w-5" /> Home all
            </button>
            <button
              onClick={() => post('/preheat', { nozzle: 200, bed: 60 })}
              className="flex flex-col items-center justify-center gap-1 rounded-lg bg-slate-100 px-3 py-3 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
            >
              <Flame className="h-5 w-5" /> Preheat
            </button>
            <button
              onClick={() => post('/cooldown')}
              className="flex flex-col items-center justify-center gap-1 rounded-lg bg-slate-100 px-3 py-3 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
            >
              <Thermometer className="h-5 w-5" /> Cooldown
            </button>
          </div>
        </Card>
      </div>

      <Card>
        <h3 className="mb-4 font-mono font-semibold text-blue-400">[ jog_controls ]</h3>
        <div className="grid grid-cols-3 gap-3 md:grid-cols-6">
          {['-X', '+X', '-Y', '+Y', '-Z', '+Z'].map((axis) => (
            <button
              key={axis}
              onClick={() => {
                const dir = axis[0] === '+' ? 1 : -1
                const a = axis[1]
                const dist = a === 'Z' ? 5 * dir : 10 * dir
                const feed = a === 'Z' ? 600 : 3000
                jog(a, dist, feed)
              }}
              className="rounded-lg bg-slate-100 px-4 py-6 text-lg font-semibold text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
            >
              {axis}
            </button>
          ))}
        </div>
      </Card>
    </div>
  )
}

function LevelingView({ printer }: { printer: Printer }) {
  const level = async () => {
    try {
      const res = await fetch(`/api/printers/${printer.id}/level`, { method: 'POST' })
      if (!res.ok) throw new Error(await res.text())
    } catch (err) {
      console.error(err)
      alert(err instanceof Error ? err.message : 'Command failed')
    }
  }

  return (
    <div className="space-y-6">
      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <h3 className="mb-4 font-mono font-semibold text-blue-400">[ bed_level_helpers ]</h3>
          <div className="grid grid-cols-2 gap-3">
            <button
              onClick={level}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
            >
              Auto bed level
            </button>
            <button className="rounded-lg bg-slate-100 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700">
              Probe Z offset
            </button>
            <button className="rounded-lg bg-slate-100 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700">
              Z- baby step
            </button>
            <button className="rounded-lg bg-slate-100 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700">
              Z+ baby step
            </button>
          </div>
        </Card>

        <Card>
          <h3 className="mb-4 font-mono font-semibold text-blue-400">[ 3d_bed_visualizer ]</h3>
          <div className="rounded-xl bg-slate-100 dark:bg-slate-800">
            <BedPreview className="aspect-video rounded-xl" />
          </div>
        </Card>
      </div>
    </div>
  )
}

function CamerasView({ cameras }: { cameras: Camera[] }) {
  if (cameras.length === 0) {
    return (
      <p className="text-center text-sm text-slate-500 dark:text-slate-400">
        No cameras are attached to this printer.
      </p>
    )
  }
  return (
    <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
      {cameras.map((camera) => (
        <CameraCard key={camera.id} camera={camera} />
      ))}
    </div>
  )
}

function GCodeView({ printer }: { printer: Printer }) {
  const [layer, setLayer] = useState(0)
  const activeFile = printer.currentFile?.trim() || 'none'

  return (
    <div className="space-y-6">
      <Card>
        <h3 className="mb-4 font-mono font-semibold text-blue-400">[ active_gcode ]</h3>
        <p className="font-mono text-sm text-slate-300">file: {activeFile}</p>
        {printer.status === 'Printing' && (
          <p className="mt-2 font-mono text-sm text-slate-500">
            progress: {printer.progress}% · {printer.remainingTime} remaining
          </p>
        )}
      </Card>

      <Card>
        <h3 className="mb-4 font-mono font-semibold text-blue-400">[ 3d_gcode_viewer ]</h3>
        <div className="flex aspect-video w-full items-center justify-center rounded-xl bg-slate-900 font-mono text-slate-400">
          <FileCode2 className="mr-2 h-8 w-8" /> Active 3D gcode preview placeholder
        </div>
        <div className="mt-4">
          <div className="mb-1 flex justify-between font-mono text-xs text-slate-400">
            <span>layer</span>
            <span>{layer} / 100</span>
          </div>
          <input
            type="range"
            min={0}
            max={100}
            value={layer}
            onChange={(e) => setLayer(Number(e.target.value))}
            className="w-full accent-blue-600"
          />
        </div>
        {printer.status === 'Printing' && (
          <div className="mt-4">
            <div className="mb-1 flex justify-between font-mono text-xs text-slate-500">
              <span>print progress</span>
              <span>{printer.progress}%</span>
            </div>
            <div className="h-2 w-full rounded-full bg-slate-800">
              <div
                className="h-full rounded-full bg-blue-500"
                style={{ width: `${printer.progress}%` }}
              />
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}

export function Printers() {
  const [selected, setSelected] = useState<Printer | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const { printers, loading, addPrinter } = usePrinters()

  if (loading) {
    return (
      <div className="space-y-6">
        <SectionTitle title="Printers" />
        <p className="font-mono text-sm text-slate-500">loading printers...</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <SectionTitle
        title="Printers"
        action={
          <button
            onClick={() => setAddOpen(true)}
            className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white shadow-sm hover:bg-blue-500"
          >
            <Plus className="h-4 w-4" /> add_printer
          </button>
        }
      />

      <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
        {printers.map((p) => (
          <PrinterCard key={p.id} printer={p} onOpen={() => setSelected(p)} />
        ))}
      </div>

      {selected && <PrinterModal printer={selected} onClose={() => setSelected(null)} />}
      <AddPrinterModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onAdd={addPrinter}
      />
    </div>
  )
}

export function PrinterDetail() {
  const { id } = useParams<{ id: string }>()
  const { printers, loading } = usePrinters()
  const printer = printers.find((p) => p.id === id)

  if (loading) {
    return <div className="p-8 text-center font-mono text-sm text-slate-500">loading printer...</div>
  }

  if (!printer) {
    return <div className="p-8 text-center text-slate-500 dark:text-slate-400">Printer not found</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">{printer.name}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 capitalize">{printer.type}</p>
        </div>
        <Link
          to={`/printers/${printer.id}/leveling`}
          className="flex items-center gap-2 rounded-lg bg-slate-100 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
        >
          <Layers className="h-4 w-4" /> Bed leveling
        </Link>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <h3 className="mb-4 font-mono font-semibold text-blue-400">[ temperatures ]</h3>
          <div className="space-y-4">
            <div>
              <div className="mb-1 flex justify-between text-sm">
                <span className="text-slate-600 dark:text-slate-300">Nozzle</span>
                <span className="font-semibold text-slate-900 dark:text-white">
                  {printer.temps.nozzle}° / {printer.temps.targetNozzle}°
                </span>
              </div>
              <div className="h-2 w-full rounded-full bg-slate-200 dark:bg-slate-800">
                <div
                  className="h-full rounded-full bg-rose-500"
                  style={{
                    width: `${Math.min((printer.temps.nozzle / (printer.temps.targetNozzle || 250)) * 100, 100)}%`,
                  }}
                />
              </div>
            </div>
            <div>
              <div className="mb-1 flex justify-between text-sm">
                <span className="text-slate-600 dark:text-slate-300">Bed</span>
                <span className="font-semibold text-slate-900 dark:text-white">
                  {printer.temps.bed}° / {printer.temps.targetBed}°
                </span>
              </div>
              <div className="h-2 w-full rounded-full bg-slate-200 dark:bg-slate-800">
                <div
                  className="h-full rounded-full bg-blue-500"
                  style={{
                    width: `${Math.min((printer.temps.bed / (printer.temps.targetBed || 110)) * 100, 100)}%`,
                  }}
                />
              </div>
            </div>
          </div>
          <div className="mt-4 border-t border-slate-200 pt-3 dark:border-slate-800">
            <div className="mb-1 flex items-center justify-between">
              <span className="font-mono text-xs text-slate-500 dark:text-slate-400">Temperature history</span>
              <div className="flex gap-3 font-mono text-[10px]">
                <span className="flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-full bg-rose-500" />Nozzle</span>
                <span className="flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-full bg-orange-500" />Bed</span>
              </div>
            </div>
            <TempChart printerId={printer.id} />
          </div>
        </Card>

        <Card>
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Quick controls</h3>
          <div className="grid grid-cols-2 gap-3">
            <button className="rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-500">
              Home all
            </button>
            <button className="rounded-lg bg-slate-100 px-4 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700">
              Preheat
            </button>
            <button className="rounded-lg bg-slate-100 px-4 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700">
              Cooldown
            </button>
            <button className="rounded-lg bg-slate-100 px-4 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700">
              Move Z
            </button>
          </div>
        </Card>
      </div>
    </div>
  )
}

function FileRow({ file, onDelete }: { file: GCodeFile; onDelete: (id: string) => void }) {
  const [menuOpen, setMenuOpen] = useState(false)

  return (
    <div
      className="flex items-center justify-between rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-4"
      style={{ borderTopColor: '#8b5cf6', boxShadow: '0 4px 6px -1px #8b5cf620' }}
    >
      <div className="flex items-center gap-4">
        <div className="rounded-lg bg-violet-900/30 p-3">
          <FileCode2 className="h-6 w-6 text-violet-400" />
        </div>
        <div>
          <h3 className="font-mono font-medium text-slate-100">{file.name}</h3>
          <p className="font-mono text-xs text-slate-400">
            {file.size}
            {file.estimatedTime ? ` · ${file.estimatedTime}` : ''}
            {file.filament ? ` · ${file.filament}` : ''}
          </p>
        </div>
      </div>
      <div className="relative flex items-center gap-2">
        <Link
          to={`/gcode/${file.id}`}
          className="rounded-lg bg-blue-600 px-3 py-1.5 font-mono text-sm font-medium text-white hover:bg-blue-500"
        >
          Preview
        </Link>
        <button
          type="button"
          onClick={() => setMenuOpen(!menuOpen)}
          className="rounded-lg bg-slate-800 p-2 text-slate-300 hover:bg-slate-700"
        >
          <MoreVertical className="h-4 w-4" />
        </button>
        {menuOpen && (
          <div className="absolute right-0 top-full z-10 mt-1 w-32 rounded-none border border-slate-700 bg-slate-950 shadow-xl">
            <button
              type="button"
              onClick={() => {
                setMenuOpen(false)
                onDelete(file.id)
              }}
              className="flex w-full items-center gap-2 px-3 py-2 font-mono text-sm text-rose-400 hover:bg-slate-900"
            >
              <Trash2 className="h-4 w-4" /> Delete
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

export function GCode() {
  const { files, loading, refresh } = useGCodeFiles()
  const { printers } = usePrinters()
  const [query, setQuery] = useState('')
  const [selectedPrinter, setSelectedPrinter] = useState('unassigned')
  const uploadRef = useRef<HTMLInputElement>(null)

  const inputClass =
    'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'

  const filtered = files.filter((f) => f.name.toLowerCase().includes(query.toLowerCase()))

  const handleUpload = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const form = new FormData()
    form.append('file', file)
    form.append('printer_id', selectedPrinter === 'all' ? 'unassigned' : selectedPrinter)
    try {
      const res = await fetch('/api/gcode', { method: 'POST', body: form })
      if (!res.ok) throw new Error('upload failed')
      await refresh()
    } catch (err) {
      console.error(err)
      alert('upload failed')
    }
    e.target.value = ''
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this G-code file?')) return
    try {
      const res = await fetch(`/api/gcode/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (!res.ok) throw new Error('delete failed')
      await refresh()
    } catch (err) {
      console.error(err)
      alert('delete failed')
    }
  }

  return (
    <div className="space-y-6">
      <SectionTitle
        title="G-code Library"
        action={
          <>
            <input
              ref={uploadRef}
              type="file"
              accept=".gcode,.gc,.nc"
              onChange={handleUpload}
              className="hidden"
            />
            <select
              value={selectedPrinter}
              onChange={(e) => setSelectedPrinter(e.target.value)}
              className="w-auto max-w-[12rem] rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
            >
              <option value="unassigned">Unassigned</option>
              {printers.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <button
              onClick={() => uploadRef.current?.click()}
              className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
            >
              <Upload className="h-4 w-4" /> Upload file
            </button>
          </>
        }
      />

      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Search G-code files..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className={inputClass + ' pl-10'}
          />
        </div>
      </div>

      {loading ? (
        <p className="font-mono text-sm text-slate-500">loading files...</p>
      ) : filtered.length === 0 ? (
        <p className="font-mono text-sm text-slate-500">
          {query ? 'no matching files' : 'no G-code files. upload one to start.'}
        </p>
      ) : (
        <div className="space-y-6">
          {Object.entries(
            filtered.reduce<Record<string, GCodeFile[]>>((acc, f) => {
              const key = f.printerId || 'unassigned'
              ;(acc[key] ||= []).push(f)
              return acc
            }, {})
          )
            .sort(([a], [b]) => (a === 'unassigned' ? 1 : b === 'unassigned' ? -1 : a.localeCompare(b)))
            .map(([printerId, groupFiles]) => {
              const printer = printers.find((p) => p.id === printerId)
              return (
                <div key={printerId} className="space-y-2">
                  <h3 className="font-mono text-sm font-semibold text-blue-400">
                    [{printer ? printer.name : 'unassigned'}]
                  </h3>
                  <div className="space-y-3">
                    {groupFiles.map((file) => (
                      <FileRow key={file.id} file={file} onDelete={handleDelete} />
                    ))}
                  </div>
                </div>
              )
            })}
        </div>
      )}
    </div>
  )
}

export function GCodeDetail() {
  const { id } = useParams<{ id: string }>()
  const { files, loading } = useGCodeFiles()
  const file = files?.find((f) => f.id === id)
  const [content, setContent] = useState<string>('')
  const [contentLoading, setContentLoading] = useState(false)

  useEffect(() => {
    if (!id) return
    setContentLoading(true)
    fetch(`/api/gcode/${encodeURIComponent(id)}`)
      .then((res) => res.text())
      .then((text) => setContent(text))
      .catch((err) => console.error('failed to load gcode', err))
      .finally(() => setContentLoading(false))
  }, [id])

  if (loading) {
    return <p className="font-mono text-sm text-slate-500">loading file...</p>
  }

  if (!file) {
    return <div className="p-8 text-center font-mono text-slate-500">File not found</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="font-mono text-2xl font-semibold text-slate-100">{file.name}</h1>
        <div className="flex gap-2">
          <Link
            to="/gcode"
            className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700"
          >
            Back
          </Link>
          <button
            disabled
            className="rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white opacity-50"
            title="send to printer coming soon"
          >
            Print
          </button>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="col-span-2">
          {contentLoading ? (
            <div className="flex aspect-video w-full items-center justify-center rounded-none bg-slate-900 text-slate-400">
              <Layers className="mr-2 h-6 w-6" /> loading preview...
            </div>
          ) : (
            <GCodePreview gcode={content} className="aspect-video rounded-none" />
          )}
        </Card>
        <Card>
          <h3 className="mb-4 font-mono font-semibold text-slate-100">File info</h3>
          <dl className="space-y-3 font-mono text-sm">
            <div className="flex justify-between">
              <dt className="text-slate-400">Size</dt>
              <dd className="text-slate-100">{file.size}</dd>
            </div>
            {file.estimatedTime && (
              <div className="flex justify-between">
                <dt className="text-slate-400">Estimated time</dt>
                <dd className="text-slate-100">{file.estimatedTime}</dd>
              </div>
            )}
            {file.filament && (
              <div className="flex justify-between">
                <dt className="text-slate-400">Filament</dt>
                <dd className="text-slate-100">{file.filament}</dd>
              </div>
            )}
          </dl>
        </Card>
      </div>
    </div>
  )
}

const RATES = [
  { label: '1 second per interval', value: 1 },
  { label: '2 seconds per interval', value: 2 },
  { label: '5 seconds per interval', value: 5 },
  { label: '10 seconds per interval', value: 10 },
  { label: '20 seconds per interval', value: 20 },
  { label: '30 seconds per interval', value: 30 },
  { label: '60 seconds per interval', value: 60 },
] as const

function formatDuration(s: number) {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}`
}

function timeUntil(s?: string) {
  if (!s) return 0
  const ms = new Date(s).getTime() - Date.now()
  return Math.max(0, Math.ceil(ms / 1000))
}

function CameraCard({
  camera,
  onEdit,
  onDelete,
  printers,
}: {
  camera: Camera
  onEdit?: () => void
  onDelete?: () => void
  printers?: Printer[]
}) {
  const recordable = camera.type === 'usb' || camera.type === 'mipi'
  const canRecord = recordable && printers !== undefined
  const [status, setStatus] = useState<{ video: any; timelapse: any } | null>(null)
  const [recordOpen, setRecordOpen] = useState(false)
  const [recordStep, setRecordStep] = useState<'type' | 'interval'>('type')
  const [recordInterval, setRecordInterval] = useState(1)
  const [confirmStop, setConfirmStop] = useState(false)
  const [busy, setBusy] = useState(false)
  const [expanded, setExpanded] = useState(false)

  const printer = printers?.find((p: Printer) => p.id === camera.printerId)
  const printerName = printer?.name || ''
  const gcode = printer?.currentFile || ''

  const active: 'video' | 'timelapse' | null =
    status?.timelapse?.active ? 'timelapse' : status?.video?.active ? 'video' : null

  useEffect(() => {
    if (!canRecord) return
    const load = () => {
      fetch(`/api/cameras/${camera.id}/record/status`)
        .then((r) => r.json().catch(() => ({ video: { active: false }, timelapse: { active: false } })))
        .then((data) => setStatus(data))
        .catch(() => {})
    }
    load()
    const id = setInterval(load, 1000)
    return () => clearInterval(id)
  }, [camera.id, canRecord])

  const startRecording = async (type: 'video' | 'timelapse', interval: number) => {
    setBusy(true)
    try {
      const body =
        type === 'video'
          ? JSON.stringify({ printer: printerName, gcode })
          : JSON.stringify({ printer: printerName, gcode, intervalSeconds: interval })
      const res = await fetch(`/api/cameras/${camera.id}/record${type === 'timelapse' ? '/timelapse' : ''}/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body,
      })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (!data.success) {
        alert(`Start ${type} failed: ${data.error || 'unknown'}`)
      }
    } catch (e: any) {
      alert(`Start ${type} error: ${e.message || e}`)
    } finally {
      setBusy(false)
    }
  }

  const stop = async (type: 'video' | 'timelapse') => {
    setBusy(true)
    try {
      const res = await fetch(`/api/cameras/${camera.id}/record${type === 'timelapse' ? '/timelapse' : ''}/stop`, { method: 'POST' })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (!data.success) {
        alert(`Stop ${type} failed: ${data.error || 'unknown'}`)
      }
    } catch (e: any) {
      alert(`Stop ${type} error: ${e.message || e}`)
    } finally {
      setBusy(false)
      setConfirmStop(false)
    }
  }

  const openRecord = () => {
    setRecordOpen(true)
    setRecordStep('type')
    setRecordInterval(1)
  }

  return (
    <>
      <Card className="space-y-3">
        <button
          type="button"
          onClick={() => camera.url && camera.enabled && setExpanded(true)}
          className={`relative flex aspect-video w-full items-center justify-center overflow-hidden rounded-xl bg-slate-100 text-slate-400 dark:bg-slate-800 ${camera.url && camera.enabled ? 'cursor-zoom-in hover:ring-2 hover:ring-blue-500' : 'cursor-default'}`}
          title={camera.url && camera.enabled ? 'Click to expand' : undefined}
        >
          {camera.url && camera.enabled ? (
            <img
              src={camera.url}
              alt={camera.name}
              className="h-full w-full object-cover"
              onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }}
            />
          ) : (
            <Video className="h-8 w-8" />
          )}
          {camera.enabled && (
            <span className="absolute right-2 top-2 h-2.5 w-2.5 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.6)]" />
          )}
          {active && (
            <div className="absolute left-2 top-2 flex items-center gap-1.5 rounded bg-rose-900/80 px-2 py-1 font-mono text-[10px] font-bold text-rose-100 animate-pulse">
              <span className="h-2 w-2 rounded-full bg-rose-500" />
              REC
            </div>
          )}
        </button>
        <div>
          <div className="flex items-center justify-between">
            <div>
              <p className="font-medium text-slate-900 dark:text-white">{camera.name}</p>
              <p className="text-xs text-slate-500 dark:text-slate-400 capitalize">{camera.type}</p>
            </div>
            {(onEdit || onDelete) && (
              <div className="flex gap-2">
                {onEdit && (
                  <button
                    onClick={onEdit}
                    className="rounded-lg bg-slate-800 p-2 text-slate-300 hover:bg-slate-700"
                    title="Edit"
                  >
                    <SettingsIcon className="h-4 w-4" />
                  </button>
                )}
                {onDelete && (
                  <button
                    onClick={onDelete}
                    className="rounded-lg bg-rose-900/30 p-2 text-rose-300 hover:bg-rose-900/50"
                    title="Delete"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                )}
              </div>
            )}
          </div>

          {canRecord && (
            <>
              {active ? (
                <div className="mt-3 space-y-2">
                  <button
                    onClick={() => setConfirmStop(true)}
                    disabled={busy}
                    className="flex w-full flex-1 animate-pulse items-center justify-center gap-1.5 rounded-lg bg-rose-600 px-2 py-1.5 text-xs font-medium text-white hover:bg-rose-500 disabled:opacity-50"
                  >
                    <Square className="h-3.5 w-3.5" />
                    {active === 'timelapse' ? 'Stop timelapse' : 'Stop recording'}
                  </button>
                  <div className="rounded bg-slate-900/50 p-2 font-mono text-[10px] text-slate-300">
                    {active === 'video' ? (
                      <p>
                        {formatDuration(status?.video?.elapsedSeconds ?? 0)} · {status?.video?.frames ?? 0} frames
                      </p>
                    ) : (
                      <p>
                        {formatDuration(status?.timelapse?.elapsedSeconds ?? 0)} · {status?.timelapse?.frames ?? 0} frames
                        {' · next '}
                        {timeUntil(status?.timelapse?.nextCapture)}s
                        {' · interval '}{status?.timelapse?.intervalSeconds ?? 0}s
                      </p>
                    )}
                  </div>
                </div>
              ) : (
                <button
                  onClick={openRecord}
                  disabled={busy}
                  className="mt-3 flex w-full items-center justify-center gap-1.5 rounded-lg bg-rose-600 px-2 py-1.5 text-xs font-medium text-white hover:bg-rose-500 disabled:opacity-50"
                >
                  <Video className="h-3.5 w-3.5" />
                  Record
                </button>
              )}
            </>
          )}
        </div>
      </Card>

      {recordOpen && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4"
          onClick={() => !busy && setRecordOpen(false)}
        >
          <div
            className="dark w-full max-w-md rounded-none border-2 border-slate-700 border-t-4 border-t-rose-600 bg-slate-950 p-6 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-xl font-mono font-semibold text-rose-500">[ record ]</h2>
              <button onClick={() => setRecordOpen(false)} disabled={busy} className="text-slate-400 hover:text-white disabled:opacity-50">
                <X className="h-5 w-5" />
              </button>
            </div>

            {recordStep === 'type' ? (
              <div className="space-y-3">
                <p className="font-mono text-sm text-slate-400">Choose a recording mode.</p>
                <div className="grid grid-cols-2 gap-3">
                  <button
                    onClick={() => {
                      setRecordOpen(false)
                      startRecording('video', 0)
                    }}
                    disabled={busy}
                    className="flex flex-col items-center gap-2 rounded-lg border border-slate-700 bg-slate-900/50 p-4 text-blue-400 hover:bg-slate-900 disabled:opacity-50"
                  >
                    <Video className="h-6 w-6" />
                    <span className="font-mono text-sm font-medium">Video</span>
                  </button>
                  <button
                    onClick={() => {
                      setRecordStep('interval')
                    }}
                    disabled={busy}
                    className="flex flex-col items-center gap-2 rounded-lg border border-slate-700 bg-slate-900/50 p-4 text-amber-400 hover:bg-slate-900 disabled:opacity-50"
                  >
                    <Timer className="h-6 w-6" />
                    <span className="font-mono text-sm font-medium">Timelapse</span>
                  </button>
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                <p className="font-mono text-sm text-slate-400">Set the interval between frames.</p>
                <label className="block font-mono text-xs text-slate-400">Frame interval</label>
                <select
                  value={recordInterval}
                  onChange={(e) => setRecordInterval(parseFloat(e.target.value))}
                  className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-rose-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
                >
                  {RATES.map((r) => (
                    <option key={r.value} value={r.value}>
                      {r.label}
                    </option>
                  ))}
                </select>
                <div className="flex justify-between gap-3">
                  <button
                    onClick={() => setRecordStep('type')}
                    disabled={busy}
                    className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700 disabled:opacity-50"
                  >
                    back
                  </button>
                  <button
                    onClick={() => {
                      setRecordOpen(false)
                      startRecording('timelapse', recordInterval)
                    }}
                    disabled={busy}
                    className="rounded-lg bg-rose-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-rose-500 disabled:opacity-50"
                  >
                    Start timelapse
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>,
        document.body
      )}

      {confirmStop && active && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4"
          onClick={() => setConfirmStop(false)}
        >
          <div
            className="dark w-full max-w-sm rounded-none border-2 border-slate-700 border-t-4 border-t-rose-600 bg-slate-950 p-6 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="mb-2 text-lg font-mono font-semibold text-rose-500">
              [ stop {active === 'timelapse' ? 'timelapse' : 'recording'} ]
            </h2>
            <p className="mb-4 font-mono text-sm text-slate-400">
              Stop the active {active === 'timelapse' ? 'timelapse' : 'recording'}? This will finalise the file.
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setConfirmStop(false)}
                disabled={busy}
                className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700 disabled:opacity-50"
              >
                cancel
              </button>
              <button
                onClick={() => stop(active)}
                disabled={busy}
                className="rounded-lg bg-rose-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-rose-500 disabled:opacity-50"
              >
                stop
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}

      {expanded && camera.url && camera.enabled && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/90 p-4"
          onClick={() => setExpanded(false)}
        >
          <div
            className="dark flex max-h-[95vh] w-full max-w-5xl flex-col overflow-hidden rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-slate-800 bg-slate-900 px-6 py-3">
              <div className="flex items-center gap-3">
                <Video className="h-5 w-5 text-blue-400" />
                <h2 className="font-mono text-sm font-semibold text-blue-400">[ {camera.name} ]</h2>
                <span className="font-mono text-xs text-slate-500 capitalize">{camera.type}</span>
                {active && (
                  <span className="flex items-center gap-1.5 rounded bg-rose-900/80 px-2 py-0.5 font-mono text-[10px] font-bold text-rose-100 animate-pulse">
                    <span className="h-2 w-2 rounded-full bg-rose-500" />
                    REC
                  </span>
                )}
              </div>
              <button
                type="button"
                onClick={() => setExpanded(false)}
                className="text-slate-400 hover:text-white"
                title="Close"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="flex items-center justify-center bg-black p-2">
              <img
                src={camera.url}
                alt={camera.name}
                className="max-h-[85vh] w-full object-contain"
                onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }}
              />
            </div>
          </div>
        </div>,
        document.body
      )}
    </>
  )
}

function CameraModal({
  printers,
  open,
  onClose,
  onSave,
  editing,
}: {
  printers: Printer[]
  open: boolean
  onClose: () => void
  onSave: (camera: Camera) => Promise<void>
  editing?: Camera | null
}) {
  const [name, setName] = useState('')
  const [printerId, setPrinterId] = useState('unassigned')
  const [type, setType] = useState<Camera['type']>('stream')
  const [url, setUrl] = useState('')
  const [usbDevices, setUsbDevices] = useState<{ name: string; deviceId: string; deviceLabel: string }[]>([])
  const [selectedDevice, setSelectedDevice] = useState('')
  const [mipiDevices, setMipiDevices] = useState<{ index: string; sensor: string; name: string }[]>([])
  const [selectedMipi, setSelectedMipi] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [brightness, setBrightness] = useState(0)
  const [flip, setFlip] = useState('')
  const [sensor, setSensor] = useState('')
  const [loadingDevices, setLoadingDevices] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [previewError, setPreviewError] = useState(false)
  const inputClass =
    'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'
  const btnClass =
    'rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white shadow-sm hover:bg-blue-500 disabled:opacity-50'
  const ghostClass =
    'rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700'

  useEffect(() => {
    if (open && (type === 'usb' || type === 'mipi')) {
      setLoadingDevices(true)
      const endpoint = type === 'usb' ? '/api/cameras/usb/list' : '/api/cameras/mipi/list'
      fetch(endpoint)
        .then((r) => r.json())
        .then((data) => {
          if (type === 'usb') {
            const devices = Array.isArray(data.devices) ? data.devices : []
            setUsbDevices(devices)
            if (editing?.deviceId && devices.some((d: any) => d.deviceId === editing.deviceId)) {
              setSelectedDevice(editing.deviceId)
            } else if (devices.length > 0) {
              setSelectedDevice(devices[0].deviceId)
            }
          } else {
            const devices = Array.isArray(data.devices) ? data.devices : []
            setMipiDevices(devices)
            if (editing?.deviceId && devices.some((d: any) => d.index === editing.deviceId)) {
              setSelectedMipi(editing.deviceId)
            } else if (devices.length > 0) {
              setSelectedMipi(devices[0].index)
            }
          }
        })
        .catch(console.error)
        .finally(() => setLoadingDevices(false))
    }
  }, [open, type, editing])

  useEffect(() => {
    if (open && editing) {
      setName(editing.name)
      setPrinterId(editing.printerId)
      setType(editing.type)
      setUrl(editing.url || '')
      setEnabled(editing.enabled)
      setBrightness(editing.brightness ?? 0)
      setFlip(editing.flip || '')
      setSensor(editing.sensor || '')
      setSelectedDevice(editing.deviceId || '')
      setSelectedMipi(editing.deviceId || '')
      setError(null)
    } else if (open) {
      setName('')
      setPrinterId('unassigned')
      setType('stream')
      setUrl('')
      setEnabled(true)
      setBrightness(0)
      setFlip('')
      setSensor('')
      setSelectedDevice('')
      setSelectedMipi('')
      setError(null)
    }
  }, [open, editing])

  const previewUrl = useMemo(() => {
    if (type === 'usb') {
      const dev = usbDevices.find((d) => d.deviceId === selectedDevice)
      if (!dev) return ''
      return `/api/cameras/usb/preview?deviceId=${encodeURIComponent(dev.deviceId)}&deviceLabel=${encodeURIComponent(dev.deviceLabel)}&flip=${encodeURIComponent(flip)}&brightness=${encodeURIComponent(brightness)}&_=${Date.now()}`
    }
    if (type === 'mipi') {
      const dev = mipiDevices.find((d) => d.index === selectedMipi)
      if (!dev) return ''
      return `/api/cameras/mipi/preview?deviceId=${encodeURIComponent(dev.index)}&deviceLabel=${encodeURIComponent(dev.name)}&sensor=${encodeURIComponent(dev.sensor)}&flip=${encodeURIComponent(flip)}&brightness=${encodeURIComponent(brightness)}&_=${Date.now()}`
    }
    return ''
  }, [type, selectedDevice, usbDevices, selectedMipi, mipiDevices, flip, brightness])

  if (!open) return null

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!name || !printerId) {
      setError('Please enter a camera name and select a printer')
      return
    }
    if ((type === 'usb' && !selectedDevice) || (type === 'mipi' && !selectedMipi)) {
      setError('Please select a camera device')
      return
    }
    const base: Camera = {
      id: editing?.id || `cam_${Math.random().toString(36).slice(2)}_${Date.now().toString(36)}`,
      name,
      printerId,
      type,
      enabled,
      brightness,
      flip,
      sensor,
    }
    if (type === 'stream') {
      base.url = url || undefined
      base.deviceId = undefined
      base.deviceLabel = undefined
    } else if (type === 'usb') {
      const dev = usbDevices.find((d) => d.deviceId === selectedDevice)
      if (!dev) return
      base.deviceId = dev.deviceId
      base.deviceLabel = dev.deviceLabel
      base.url = `/api/cameras/usb/preview?deviceId=${encodeURIComponent(dev.deviceId)}&deviceLabel=${encodeURIComponent(dev.deviceLabel)}`
    } else if (type === 'mipi') {
      const dev = mipiDevices.find((d) => d.index === selectedMipi)
      if (!dev) return
      base.deviceId = dev.index
      base.deviceLabel = dev.name
      base.sensor = dev.sensor
      base.url = `/api/cameras/mipi/preview?deviceId=${encodeURIComponent(dev.index)}&deviceLabel=${encodeURIComponent(dev.name)}&sensor=${encodeURIComponent(dev.sensor)}`
    }
    try {
      await onSave(base)
      setName('')
      setPrinterId('unassigned')
      setUrl('')
      setSelectedDevice('')
      setSelectedMipi('')
      setEnabled(true)
      setBrightness(0)
      setFlip('')
      setSensor('')
      setType('stream')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed to save camera')
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark w-full max-w-md max-h-[90vh] overflow-y-auto rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 font-mono text-xl font-semibold text-blue-400">[ add_camera ]</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            type="text"
            placeholder="Camera name *"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={inputClass}
          />
          <select
            value={printerId}
            onChange={(e) => setPrinterId(e.target.value)}
            className={inputClass}
          >
            <option value="unassigned">Unassigned</option>
            {printers.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <select
            value={type}
            onChange={(e) => setType(e.target.value as Camera['type'])}
            className={inputClass}
            disabled={!!editing}
          >
            <option value="stream">Stream URL</option>
            <option value="usb">USB camera</option>
            <option value="mipi">Raspberry Pi MIPI</option>
          </select>
          {type === 'stream' && (
            <input
              type="url"
              placeholder="Stream URL (optional)"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              className={inputClass}
            />
          )}
          {type === 'usb' && (
            <select
              value={selectedDevice}
              onChange={(e) => setSelectedDevice(e.target.value)}
              className={inputClass}
              disabled={loadingDevices}
            >
              {usbDevices.length === 0 ? (
                <option value="">{loadingDevices ? 'scanning...' : 'no devices found'}</option>
              ) : (
                usbDevices.map((d) => (
                  <option key={d.deviceId} value={d.deviceId}>
                    {d.name}
                  </option>
                ))
              )}
            </select>
          )}
          {type === 'mipi' && (
            <select
              value={selectedMipi}
              onChange={(e) => setSelectedMipi(e.target.value)}
              className={inputClass}
              disabled={loadingDevices}
            >
              {mipiDevices.length === 0 ? (
                <option value="">{loadingDevices ? 'scanning...' : 'no devices found'}</option>
              ) : (
                mipiDevices.map((d) => (
                  <option key={d.index} value={d.index}>
                    {d.name || d.sensor} ({d.index})
                  </option>
                ))
              )}
            </select>
          )}
          {editing && type === 'mipi' && (
            <input
              type="text"
              placeholder="Sensor (e.g. imx219)"
              value={sensor}
              onChange={(e) => setSensor(e.target.value)}
              className={inputClass}
            />
          )}
          {(type === 'usb' || type === 'mipi') && (
            <div className="rounded-lg border border-slate-700 bg-slate-900/50 p-3">
              <label className="mb-2 block font-mono text-xs text-slate-400">Preview</label>
              {previewUrl ? (
                <>
                  <img
                    src={previewUrl}
                    alt="camera preview"
                    className="aspect-video w-full rounded bg-slate-950 object-cover"
                    onLoad={() => setPreviewError(false)}
                    onError={() => setPreviewError(true)}
                  />
                  {previewError && (
                    <p className="mt-2 font-mono text-xs text-rose-400">Preview failed to load</p>
                  )}
                </>
              ) : (
                <p className="font-mono text-sm text-slate-500">Select a device to see preview</p>
              )}
            </div>
          )}
          <div className="flex items-center justify-between rounded-lg border border-slate-700 bg-slate-900/50 p-3">
            <span className="font-mono text-sm text-slate-300">Enabled</span>
            <Switch checked={enabled} onChange={setEnabled} />
          </div>
          <div>
            <label className="mb-1 block font-mono text-xs text-slate-400">Brightness {brightness.toFixed(1)}</label>
            <input
              type="range"
              min={-1}
              max={1}
              step={0.1}
              value={brightness}
              onChange={(e) => setBrightness(parseFloat(e.target.value))}
              className="w-full accent-blue-600"
            />
          </div>
          <select value={flip} onChange={(e) => setFlip(e.target.value)} className={inputClass}>
            <option value="">Normal orientation</option>
            <option value="horizontal">Flip horizontal</option>
            <option value="vertical">Flip vertical</option>
            <option value="both">Flip both</option>
            <option value="90">Rotate 90°</option>
            <option value="270">Rotate 270°</option>
          </select>
          {error && (
            <p className="rounded-lg border border-rose-600 bg-rose-950/30 p-3 font-mono text-sm text-rose-400">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-3">
            <button type="button" onClick={onClose} className={ghostClass}>
              cancel
            </button>
            <button type="submit" className={btnClass}>
              {editing ? 'save' : 'add'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body
  )
}

export function Cameras() {
  const { printers } = usePrinters()
  const { cameras, addCamera, updateCamera, removeCamera } = useCameras()
  const [addOpen, setAddOpen] = useState(false)
  const [editing, setEditing] = useState<Camera | null>(null)

  const handleSave = async (camera: Camera) => {
    if (editing) {
      await updateCamera(camera)
    } else {
      await addCamera(camera)
    }
  }

  if (cameras.length === 0) {
    return (
      <div className="space-y-10">
        <SectionTitle
          title="Cameras"
          action={
            <button
              type="button"
              onClick={() => setAddOpen(true)}
              className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
            >
              <Plus className="h-4 w-4" /> add camera
            </button>
          }
        />
        <p className="font-mono text-sm text-slate-500">No cameras added yet.</p>
        <CameraModal
          printers={printers}
          open={addOpen}
          onClose={() => setAddOpen(false)}
          onSave={handleSave}
          editing={null}
        />
      </div>
    )
  }

  const printerIds = [...new Set(cameras.map((c) => c.printerId))]
  return (
    <div className="space-y-10">
      <SectionTitle
        title="Cameras"
        action={
          <button
            type="button"
            onClick={() => setAddOpen(true)}
            className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
          >
            <Plus className="h-4 w-4" /> add camera
          </button>
        }
      />
      {printerIds.map((id) => {
        const printerCameras = cameras.filter((c) => c.printerId === id)
        const printer = printers.find((p) => p.id === id)
        return (
          <section key={id}>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-slate-900 dark:text-white">
                {printer?.name ?? id}
              </h2>
              <span className="text-sm text-slate-500 dark:text-slate-400">
                {printerCameras.length} camera{printerCameras.length !== 1 ? 's' : ''}
              </span>
            </div>
            <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
              {printerCameras.map((camera) => (
                <CameraCard
                  key={camera.id}
                  camera={camera}
                  onEdit={() => setEditing(camera)}
                  onDelete={() => removeCamera(camera.id)}
                />
              ))}
            </div>
          </section>
        )
      })}
      <CameraModal
        printers={printers}
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onSave={handleSave}
        editing={null}
      />
      <CameraModal
        printers={printers}
        open={editing !== null}
        onClose={() => setEditing(null)}
        onSave={async (c) => { await updateCamera(c); setEditing(null) }}
        editing={editing}
      />
    </div>
  )
}

export function Recordings() {
  const { cameras } = useCameras()
  const { printers } = usePrinters()
  const [selected, setSelected] = useState(cameras[0]?.id || '')
  const [recording, setRecording] = useState(false)
  const [timelapsing, setTimelapsing] = useState(false)
  const [tab, setTab] = useState<'videos' | 'timelapses'>('videos')
  const [videos, setVideos] = useState<string[]>([])
  const [timelapses, setTimelapses] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [playing, setPlaying] = useState<string | null>(null)
  const [tlInterval, setTlInterval] = useState(1)
  const [status, setStatus] = useState<{ video?: any; timelapse?: any }>({})
  const [confirmStop, setConfirmStop] = useState<'video' | 'timelapse' | null>(null)

  const recordable = cameras.filter((c) => c.type === 'usb' || c.type === 'mipi')
  const recordings = tab === 'videos' ? videos : timelapses

  useEffect(() => {
    if (recordable.length && !recordable.find((c) => c.id === selected)) {
      setSelected(recordable[0].id)
    }
  }, [cameras, selected])

  const loadRecordings = async () => {
    try {
      const [vRes, tRes] = await Promise.all([
        fetch('/api/recordings/videos'),
        fetch('/api/recordings/timelapses'),
      ])
      const vData = await vRes.json()
      const tData = await tRes.json()
      setVideos(Array.isArray(vData.recordings) ? vData.recordings : [])
      setTimelapses(Array.isArray(tData.recordings) ? tData.recordings : [])
    } catch (e) {
      console.error(e)
    }
  }

  const loadStatus = async () => {
    if (!selected) return
    try {
      const res = await fetch(`/api/cameras/${selected}/record/status`)
      const data = await res.json()
      setStatus(data)
      setRecording(data.video?.active || false)
      setTimelapsing(data.timelapse?.active || false)
    } catch (e) {
      // ignore
    }
  }

  useEffect(() => {
    loadRecordings()
    loadStatus()
    const id = setInterval(loadStatus, 2000)
    return () => clearInterval(id)
  }, [selected])

  const startRecording = async () => {
    if (!selected) return
    const camera = recordable.find((c) => c.id === selected)
    const printer = printers.find((p) => p.id === camera?.printerId)
    const printerName = printer?.name || ''
    const gcode = printer?.currentFile || ''

    setBusy(true)
    try {
      const res = await fetch(`/api/cameras/${selected}/record/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ printer: printerName, gcode }),
      })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (data.success) {
        setRecording(true)
      } else {
        alert(`Recording start failed: ${data.error || 'unknown'}`)
      }
    } catch (e: any) {
      alert(`Recording start error: ${e.message || e}`)
    } finally {
      setBusy(false)
    }
  }

  const stopRecording = async () => {
    setBusy(true)
    setConfirmStop(null)
    try {
      const res = await fetch(`/api/cameras/${selected}/record/stop`, { method: 'POST' })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (!data.success) {
        alert(`Recording stop failed: ${data.error || 'unknown'}`)
      }
      setRecording(false)
      await loadRecordings()
    } catch (e: any) {
      alert(`Recording stop error: ${e.message || e}`)
    } finally {
      setBusy(false)
    }
  }

  const startTimelapse = async () => {
    if (!selected) return
    const camera = recordable.find((c) => c.id === selected)
    const printer = printers.find((p) => p.id === camera?.printerId)
    const printerName = printer?.name || ''
    const gcode = printer?.currentFile || ''

    setBusy(true)
    try {
      const res = await fetch(`/api/cameras/${selected}/record/timelapse/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ printer: printerName, gcode, intervalSeconds: tlInterval }),
      })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (data.success) {
        setTimelapsing(true)
      } else {
        alert(`Timelapse start failed: ${data.error || 'unknown'}`)
      }
    } catch (e: any) {
      alert(`Timelapse start error: ${e.message || e}`)
    } finally {
      setBusy(false)
    }
  }

  const stopTimelapse = async () => {
    setBusy(true)
    setConfirmStop(null)
    try {
      const res = await fetch(`/api/cameras/${selected}/record/timelapse/stop`, { method: 'POST' })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (!data.success) {
        alert(`Timelapse stop failed: ${data.error || 'unknown'}`)
      }
      setTimelapsing(false)
      await loadRecordings()
    } catch (e: any) {
      alert(`Timelapse stop error: ${e.message || e}`)
    } finally {
      setBusy(false)
    }
  }

  const convertToTimelapse = async (filename: string) => {
    setBusy(true)
    try {
      const res = await fetch(`/api/recordings/videos/${encodeURIComponent(filename)}/convert/timelapse`, { method: 'POST' })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (!data.success) {
        alert(`Convert failed: ${data.error || 'unknown'}`)
      }
      await loadRecordings()
      setTab('timelapses')
    } catch (e: any) {
      alert(`Convert error: ${e.message || e}`)
    } finally {
      setBusy(false)
    }
  }

  const deleteRecording = async (filename: string) => {
    if (!confirm(`Delete ${filename}?`)) return
    setBusy(true)
    try {
      const res = await fetch(`/api/recordings/${tab}/${encodeURIComponent(filename)}/delete`, { method: 'POST' })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (!data.success) {
        alert(`Delete failed: ${data.error || 'unknown'}`)
      }
      await loadRecordings()
    } catch (e: any) {
      alert(`Delete error: ${e.message || e}`)
    } finally {
      setBusy(false)
    }
  }

  const fmtElapsed = (s: number) => {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    const sec = Math.floor(s % 60)
    return h > 0 ? `${h}h ${m}m ${sec}s` : `${m}m ${sec}s`
  }

  return (
    <div className="space-y-6">
      <SectionTitle title="Recordings" />
      {recordable.length === 0 ? (
        <p className="font-mono text-sm text-slate-500">No recordable cameras (USB/MIPI). Add a camera to start recording.</p>
      ) : (
        <Card>
          <h3 className="mb-4 font-mono font-semibold text-blue-400">[ capture ]</h3>
          <div className="space-y-4">
            <div>
              <label className="mb-1 block font-mono text-xs text-slate-400">Camera</label>
              <select
                value={selected}
                onChange={(e) => setSelected(e.target.value)}
                className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
              >
                {recordable.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="rounded-lg border border-slate-200 p-4 dark:border-slate-800">
                <h4 className="mb-2 font-mono text-sm font-semibold text-slate-700 dark:text-slate-300">Video recording</h4>
                <p className="mb-3 font-mono text-xs text-slate-500">Capture full-speed MJPEG video as MKV.</p>
                {recording && status.video ? (
                  <div className="mb-3 space-y-1 font-mono text-xs text-slate-400">
                    <p>elapsed: {fmtElapsed(status.video.elapsedSeconds || 0)}</p>
                    <p>frames: {status.video.frames || 0}</p>
                  </div>
                ) : null}
                <button
                  type="button"
                  onClick={() => (recording ? setConfirmStop('video') : startRecording())}
                  disabled={busy || timelapsing}
                  className={`w-full rounded-lg px-4 py-2 font-mono text-sm font-medium text-white disabled:opacity-50 ${
                    recording ? 'bg-rose-600 hover:bg-rose-500' : 'bg-blue-600 hover:bg-blue-500'
                  }`}
                >
                  {recording ? 'Stop recording' : 'Start recording'}
                </button>
              </div>
              <div className="rounded-lg border border-slate-200 p-4 dark:border-slate-800">
                <h4 className="mb-2 font-mono text-sm font-semibold text-slate-700 dark:text-slate-300">Timelapse</h4>
                <p className="mb-2 font-mono text-xs text-slate-500">Capture one frame at a set interval.</p>
                <div className="mb-3">
                  <label className="mb-1 block font-mono text-xs text-slate-400">
                    Interval: {tlInterval}s {tlInterval >= 60 ? `(${(tlInterval / 60).toFixed(1)} min)` : ''}
                  </label>
                  <input
                    type="range"
                    min={1}
                    max={300}
                    step={1}
                    value={tlInterval}
                    onChange={(e) => setTlInterval(Number(e.target.value))}
                    className="w-full accent-blue-600"
                    disabled={timelapsing}
                  />
                </div>
                {timelapsing && status.timelapse ? (
                  <div className="mb-3 space-y-1 font-mono text-xs text-slate-400">
                    <p>elapsed: {fmtElapsed(status.timelapse.elapsedSeconds || 0)}</p>
                    <p>frames: {status.timelapse.frames || 0}</p>
                    <p>next: {new Date(status.timelapse.nextCapture).toLocaleTimeString()}</p>
                  </div>
                ) : null}
                <button
                  type="button"
                  onClick={() => (timelapsing ? setConfirmStop('timelapse') : startTimelapse())}
                  disabled={busy || recording}
                  className={`w-full rounded-lg px-4 py-2 font-mono text-sm font-medium text-white disabled:opacity-50 ${
                    timelapsing ? 'bg-rose-600 hover:bg-rose-500' : 'bg-blue-600 hover:bg-blue-500'
                  }`}
                >
                  {timelapsing ? 'Stop timelapse' : 'Start timelapse'}
                </button>
              </div>
            </div>
          </div>
        </Card>
      )}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={() => setTab('videos')}
          className={`rounded-lg px-4 py-2 font-mono text-sm font-medium ${
            tab === 'videos'
              ? 'bg-blue-600 text-white'
              : 'bg-slate-100 text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700'
          }`}
        >
          videos ({videos.length})
        </button>
        <button
          type="button"
          onClick={() => setTab('timelapses')}
          className={`rounded-lg px-4 py-2 font-mono text-sm font-medium ${
            tab === 'timelapses'
              ? 'bg-blue-600 text-white'
              : 'bg-slate-100 text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700'
          }`}
        >
          timelapses ({timelapses.length})
        </button>
      </div>

      {recordings.length === 0 ? (
        <p className="font-mono text-sm text-slate-500">No {tab} yet.</p>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {recordings.map((r) => (
            <div key={r} className="overflow-hidden rounded-lg border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
              <div className="relative aspect-video bg-slate-900">
                <img
                  src={`/api/recordings/${tab}/${encodeURIComponent(r)}/thumb`}
                  alt={r}
                  className="h-full w-full object-cover"
                  onError={(e) => {
                    (e.target as HTMLImageElement).style.display = 'none'
                  }}
                />
              </div>
              <div className="p-3">
                <p className="mb-2 truncate font-mono text-xs text-slate-700 dark:text-slate-300" title={r}>
                  {r}
                </p>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    onClick={() => setPlaying(`/recordings/${tab}/${r}`)}
                    className="rounded-lg bg-blue-100 px-3 py-1 font-mono text-xs font-medium text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-300 dark:hover:bg-blue-900/50"
                  >
                    play
                  </button>
                  <a
                    href={`/recordings/${tab}/${r}`}
                    download
                    className="rounded-lg bg-slate-100 px-3 py-1 font-mono text-xs font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                  >
                    download
                  </a>
                  {tab === 'videos' && (
                    <button
                      type="button"
                      onClick={() => convertToTimelapse(r)}
                      disabled={busy}
                      className="rounded-lg bg-purple-100 px-3 py-1 font-mono text-xs font-medium text-purple-700 hover:bg-purple-200 disabled:opacity-50 dark:bg-purple-900/30 dark:text-purple-300 dark:hover:bg-purple-900/50"
                    >
                      to timelapse
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => deleteRecording(r)}
                    disabled={busy}
                    className="rounded-lg bg-rose-100 px-3 py-1 font-mono text-xs font-medium text-rose-700 hover:bg-rose-200 disabled:opacity-50 dark:bg-rose-900/30 dark:text-rose-300 dark:hover:bg-rose-900/50"
                  >
                    delete
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {playing && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4"
          onClick={() => setPlaying(null)}
        >
          <div
            className="dark w-full max-w-3xl rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-6 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-3 flex items-center justify-between">
              <h3 className="font-mono text-sm font-semibold text-blue-400">[ player ]</h3>
              <button type="button" onClick={() => setPlaying(null)} className="text-sm text-slate-500 hover:text-slate-300">
                close
              </button>
            </div>
            <video controls autoPlay className="w-full rounded-lg" src={playing}>
              Your browser does not support the video tag.
            </video>
          </div>
        </div>,
        document.body
      )}

      {confirmStop && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4"
          onClick={() => setConfirmStop(null)}
        >
          <div
            className="dark w-full max-w-sm rounded-none border-2 border-slate-700 border-t-4 border-t-rose-600 bg-slate-950 p-6 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="mb-2 font-mono text-lg font-semibold text-rose-500">
              [ stop {confirmStop === 'video' ? 'recording' : 'timelapse'} ]
            </h2>
            <p className="mb-4 font-mono text-sm text-slate-400">
              Stop the active {confirmStop === 'video' ? 'recording' : 'timelapse'}? This will finalise the file.
            </p>
            <div className="flex justify-end gap-3">
              <button
                type="button"
                onClick={() => setConfirmStop(null)}
                disabled={busy}
                className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700 disabled:opacity-50"
              >
                cancel
              </button>
              <button
                type="button"
                onClick={() => (confirmStop === 'video' ? stopRecording() : stopTimelapse())}
                disabled={busy}
                className="rounded-lg bg-rose-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-rose-500 disabled:opacity-50"
              >
                stop
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}
    </div>
  )
}

function HistoryRow({ h, onDelete }: { h: PrintRecord; onDelete: (id: string) => void }) {
  return (
    <tr className="border-b border-slate-100 last:border-0 dark:border-slate-800">
      <td className="py-3 text-sm font-medium text-slate-900 dark:text-white">{h.file}</td>
      <td className="py-3 text-sm text-slate-500 dark:text-slate-400">{h.printer}</td>
      <td className="py-3 text-sm text-slate-500 dark:text-slate-400">{h.started}</td>
      <td className="py-3 text-sm text-slate-500 dark:text-slate-400">{h.duration}</td>
      <td className={`py-3 text-sm font-semibold ${resultColor[h.result] || 'text-slate-600 dark:text-slate-400'}`}>{h.result}</td>
      <td className="py-3 text-right">
        <button
          type="button"
          onClick={() => onDelete(h.id)}
          className="rounded-lg bg-rose-100 px-2 py-1 font-mono text-xs font-medium text-rose-700 hover:bg-rose-200 dark:bg-rose-900/30 dark:text-rose-300 dark:hover:bg-rose-900/50"
        >
          delete
        </button>
      </td>
    </tr>
  )
}

export function History() {
  const { printers } = usePrinters()
  const [history, setHistory] = useState<PrintRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [addOpen, setAddOpen] = useState(false)

  const loadHistory = async () => {
    try {
      const res = await fetch('/api/history')
      const data = await res.json()
      setHistory(Array.isArray(data.history) ? data.history : [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadHistory()
    const id = setInterval(loadHistory, 5000)
    return () => clearInterval(id)
  }, [])

  const clearAll = async () => {
    if (!confirm('Clear all print history?')) return
    try {
      const res = await fetch('/api/history', { method: 'DELETE' })
      if (res.ok) setHistory([])
    } catch (e) {
      console.error(e)
    }
  }

  const deleteRow = async (id: string) => {
    try {
      const res = await fetch(`/api/history/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (res.ok) setHistory((h) => h.filter((r) => r.id !== id))
    } catch (e) {
      console.error(e)
    }
  }

  const addEntry = async (entry: { printer: string; file: string; result: string; startedAt: string; endedAt: string }) => {
    try {
      const res = await fetch('/api/history', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(entry),
      })
      if (res.ok) {
        setAddOpen(false)
        await loadHistory()
      }
    } catch (e) {
      console.error(e)
    }
  }

  return (
    <div className="space-y-6">
      <SectionTitle
        title="Print History"
        action={
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setAddOpen(true)}
              className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
            >
              <Plus className="h-4 w-4" /> add entry
            </button>
            {history.length > 0 && (
              <button
                type="button"
                onClick={clearAll}
                className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700"
              >
                clear
              </button>
            )}
          </div>
        }
      />
      {loading ? (
        <p className="font-mono text-sm text-slate-500">loading history...</p>
      ) : history.length === 0 ? (
        <p className="font-mono text-sm text-slate-500">No print history yet. History is recorded automatically when prints finish, or add an entry manually.</p>
      ) : (
        <Card className="overflow-x-auto">
          <table className="w-full text-left">
            <thead>
              <tr className="border-b border-slate-200 text-xs font-medium text-slate-500 dark:border-slate-800 dark:text-slate-400">
                <th className="pb-3">File</th>
                <th className="pb-3">Printer</th>
                <th className="pb-3">Started</th>
                <th className="pb-3">Duration</th>
                <th className="pb-3">Result</th>
                <th className="pb-3"></th>
              </tr>
            </thead>
            <tbody>
              {history.map((h) => (
                <HistoryRow key={h.id} h={h} onDelete={deleteRow} />
              ))}
            </tbody>
          </table>
        </Card>
      )}
      {addOpen && (
        <AddHistoryModal
          printers={printers}
          onClose={() => setAddOpen(false)}
          onAdd={addEntry}
        />
      )}
    </div>
  )
}

function AddHistoryModal({
  printers,
  onClose,
  onAdd,
}: {
  printers: Printer[]
  onClose: () => void
  onAdd: (entry: { printer: string; file: string; result: string; startedAt: string; endedAt: string }) => void
}) {
  const [printer, setPrinter] = useState(printers[0]?.name || '')
  const [file, setFile] = useState('')
  const [result, setResult] = useState('Success')
  const [duration, setDuration] = useState(60)
  const inputClass =
    'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'
  const btnClass =
    'rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white shadow-sm hover:bg-blue-500 disabled:opacity-50'
  const ghostClass =
    'rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700'

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!file || !printer) return
    const now = new Date()
    const start = new Date(now.getTime() - duration * 1000)
    onAdd({
      printer,
      file,
      result,
      startedAt: start.toISOString(),
      endedAt: now.toISOString(),
    })
  }

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark w-full max-w-md max-h-[90vh] overflow-y-auto rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 font-mono text-xl font-semibold text-blue-400">[ add_history ]</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            type="text"
            placeholder="G-code file name *"
            value={file}
            onChange={(e) => setFile(e.target.value)}
            className={inputClass}
          />
          <select value={printer} onChange={(e) => setPrinter(e.target.value)} className={inputClass}>
            {printers.map((p) => (
              <option key={p.id} value={p.name}>
                {p.name}
              </option>
            ))}
            <option value="Manual">Manual / Other</option>
          </select>
          <select value={result} onChange={(e) => setResult(e.target.value)} className={inputClass}>
            <option value="Success">Success</option>
            <option value="Failed">Failed</option>
            <option value="Cancelled">Cancelled</option>
          </select>
          <div>
            <label className="mb-1 block font-mono text-xs text-slate-400">Duration (minutes): {Math.round(duration / 60)}</label>
            <input
              type="range"
              min={60}
              max={36000}
              step={60}
              value={duration}
              onChange={(e) => setDuration(Number(e.target.value))}
              className="w-full accent-blue-600"
            />
          </div>
          <div className="flex justify-end gap-3">
            <button type="button" onClick={onClose} className={ghostClass}>
              cancel
            </button>
            <button type="submit" disabled={!file || !printer} className={btnClass}>
              add
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body
  )
}

function ProviderSection({
  title,
  enabled,
  onToggle,
  children,
}: {
  title: string
  enabled: boolean
  onToggle: (enabled: boolean) => void
  children?: ReactNode
}) {
  return (
    <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
      <div className="mb-3 flex items-center justify-between">
        <h4 className="font-medium text-slate-900 dark:text-white">{title}</h4>
        <Switch checked={enabled} onChange={onToggle} />
      </div>
      {enabled && <div className="space-y-3 border-t border-slate-100 pt-3 dark:border-slate-800">{children}</div>}
    </div>
  )
}

function AnkerLoginModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [tab, setTab] = useState<'login' | 'import' | 'auto'>('auto')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [region, setRegion] = useState('eu')
  const [captchaId, setCaptchaId] = useState('')
  const [captchaImg, setCaptchaImg] = useState('')
  const [captchaAnswer, setCaptchaAnswer] = useState('')
  const [verificationCode, setVerificationCode] = useState('')
  const [verificationData, setVerificationData] = useState<any>(null)
  const [file, setFile] = useState<File | null>(null)
  const [autoPath, setAutoPath] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<any>(null)
  const [elapsed, setElapsed] = useState(0)
  const [spinner, setSpinner] = useState(0)
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (tab === 'auto') {
      fetch('/api/anker/detect')
        .then((r) => r.json())
        .then((data: any) => setAutoPath(data.found ? data.path : null))
        .catch(() => setAutoPath(null))
    }
  }, [tab])

  useEffect(() => {
    if (!loading) {
      setElapsed(0)
      setSpinner(0)
      return
    }
    const tick = setInterval(() => {
      setElapsed((e) => e + 1)
      setSpinner((s) => (s + 1) % 4)
    }, 1000)
    return () => clearInterval(tick)
  }, [loading])

  const inputClass =
    'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'
  const btnClass =
    'rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white shadow-sm hover:bg-blue-500 disabled:opacity-50'
  const ghostClass =
    'rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700'

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError(null)
    setCaptchaImg('')
    const controller = new AbortController()
    const timeout = setTimeout(() => controller.abort(), 60000)
    try {
      const res = await fetch('/api/anker/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email,
          password,
          region,
          captcha_id: captchaId,
          captcha_answer: captchaAnswer,
          verification_code: verificationCode,
          verification_data: verificationData,
        }),
        signal: controller.signal,
      })
      let data: any = {}
      try {
        data = await res.json()
      } catch {
        data = { success: false, message: `HTTP ${res.status}` }
      }

      if (data.success) {
        setResult(data)
      } else if (data.captcha_img) {
        setCaptchaId(data.captcha_id || '')
        setCaptchaImg(data.captcha_img)
        setCaptchaAnswer('')
        setVerificationCode('')
        setError(data.message || 'CAPTCHA required')
      } else if (data.code && data.code !== 0) {
        // Any non-zero code may require an email/2FA/verification code.
        setVerificationData(data.verification_data || {})
        setVerificationCode('')
        setError(data.message || 'Check your email for a verification code.')
      } else {
        setError(data.message || `Login failed: ${res.status}`)
      }
    } catch (err: any) {
      if (err.name === 'AbortError') {
        setError('login timed out after 60s. check terminal for details.')
      } else {
        setError(err.message)
      }
    } finally {
      clearTimeout(timeout)
      setLoading(false)
    }
  }

  const handleImport = async (e: FormEvent) => {
    e.preventDefault()
    if (!file) {
      setError('select a login.json file')
      return
    }
    setLoading(true)
    setError(null)
    try {
      const text = await file.text()
      const res = await fetch('/api/anker/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: text,
      })
      const data = await res.json().catch(() => ({ success: false, message: `HTTP ${res.status}` }))
      if (data.success) {
        setResult(data)
      } else {
        setError(data.message || 'Import failed')
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleAuto = async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch('/api/anker/auto-import', { method: 'POST' })
      const data = await res.json().catch(() => ({ success: false, message: `HTTP ${res.status}` }))
      if (data.success) {
        setResult(data)
      } else {
        setError(data.message || 'Auto-import failed')
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  if (!open) return null

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-6 shadow-2xl shadow-blue-500/20"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-xl font-mono font-semibold text-blue-400">[ anker_auth ]</h2>

        {!result ? (
          <>
            <div className="mb-4 flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => {
                  setTab('auto')
                  setError(null)
                }}
                className={tab === 'auto' ? btnClass : ghostClass}
              >
                Auto-detect
              </button>
              <button
                type="button"
                onClick={() => {
                  setTab('login')
                  setError(null)
                }}
                className={tab === 'login' ? btnClass : ghostClass}
              >
                Cloud login
              </button>
              <button
                type="button"
                onClick={() => {
                  setTab('import')
                  setError(null)
                }}
                className={tab === 'import' ? btnClass : ghostClass}
              >
                Import login.json
              </button>
            </div>

            {tab === 'auto' ? (
              <div className="space-y-4">
                {autoPath ? (
                  <p className="font-mono text-sm text-slate-300">
                    found: <span className="text-blue-400">{autoPath}</span>
                  </p>
                ) : (
                  <p className="font-mono text-sm text-slate-500">
                    no login.json found. try the cloud login or upload the file manually.
                  </p>
                )}
                {error && <p className="font-mono text-sm text-rose-400">error: {error}</p>}
                <div className="flex justify-end gap-3">
                  <button type="button" onClick={onClose} className={ghostClass}>
                    cancel
                  </button>
                  <button
                    onClick={handleAuto}
                    disabled={loading || !autoPath}
                    className={btnClass}
                  >
                    {loading ? 'importing...' : 'import'}
                  </button>
                </div>
              </div>
            ) : tab === 'login' ? (
              <form onSubmit={handleLogin} className="sensitive space-y-4">
                <input
                  type="email"
                  placeholder="Email"
                  className={inputClass}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
                <div className="relative">
                  <input
                    type={showPassword ? 'text' : 'password'}
                    placeholder="Password"
                    className={inputClass + ' pr-10'}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-200"
                    tabIndex={-1}
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                <select
                  value={region}
                  onChange={(e) => setRegion(e.target.value)}
                  className={inputClass}
                >
                  <option value="eu">Europe</option>
                  <option value="us">United States</option>
                </select>

                {captchaImg && (
                  <div className="space-y-2">
                    <img
                      src={captchaImg.startsWith('data:') ? captchaImg : `data:image/png;base64,${captchaImg}`}
                      alt="captcha"
                      className="h-16 w-auto rounded border border-slate-700"
                    />
                    <input
                      type="text"
                      placeholder="CAPTCHA answer"
                      className={inputClass}
                      value={captchaAnswer}
                      onChange={(e) => setCaptchaAnswer(e.target.value)}
                      required
                    />
                  </div>
                )}

                {verificationData && (
                  <div className="space-y-2">
                    <p className="font-mono text-xs text-slate-400">
                      account may be locked. enter the code sent to your email, then log in again.
                    </p>
                    <input
                      type="text"
                      placeholder="Verification code"
                      className={inputClass}
                      value={verificationCode}
                      onChange={(e) => setVerificationCode(e.target.value)}
                    />
                  </div>
                )}

                {error && <p className="font-mono text-sm text-rose-400">error: {error}</p>}
                <div className="flex justify-end gap-3">
                  <button type="button" onClick={onClose} className={ghostClass}>
                    cancel
                  </button>
                  <button type="submit" disabled={loading} className={btnClass}>
                    {loading ? (
                      <span className="font-mono">
                        {['|', '/', '-', '\\'][spinner]} authenticating...
                        {' '}
                        [{Math.floor(elapsed / 60)
                          .toString()
                          .padStart(2, '0')}:
                        {(elapsed % 60).toString().padStart(2, '0')}]
                      </span>
                    ) : (
                      'log in'
                    )}
                  </button>
                </div>
              </form>
            ) : (
              <form onSubmit={handleImport} className="space-y-4">
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".json"
                  onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                  className={inputClass}
                />
                {file && <p className="font-mono text-xs text-slate-400">selected: {file.name}</p>}
                {error && <p className="font-mono text-sm text-rose-400">error: {error}</p>}
                <div className="flex justify-end gap-3">
                  <button type="button" onClick={onClose} className={ghostClass}>
                    cancel
                  </button>
                  <button type="submit" disabled={loading || !file} className={btnClass}>
                    {loading ? 'importing...' : 'import'}
                  </button>
                </div>
              </form>
            )}
          </>
        ) : (
          <div className="space-y-4">
            <p className="font-mono text-sm text-emerald-400">{result.message || 'success'}</p>
            <div className="flex justify-end">
              <button onClick={onClose} className={btnClass}>
                done
              </button>
            </div>
          </div>
        )}
      </div>
    </div>,
    document.body
  )
}

function AddPrinterModal({
  open,
  onClose,
  onAdd,
}: {
  open: boolean
  onClose: () => void
  onAdd: (printer: Partial<Printer> & { name: string; type: string; host?: string; apiKey?: string }) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [type, setType] = useState('klipper')
  const [host, setHost] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const inputClass =
    'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'
  const btnClass =
    'rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white shadow-sm hover:bg-blue-500 disabled:opacity-50'
  const ghostClass =
    'rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700'

  useEffect(() => {
    if (open) {
      setName('')
      setType('klipper')
      setHost('')
      setApiKey('')
      setError(null)
      setSaving(false)
    }
  }, [open])

  if (!open) return null

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!name || !type) {
      setError('Name and type are required')
      return
    }
    setSaving(true)
    try {
      await onAdd({ name, type, host: host || undefined, apiKey: apiKey || undefined })
      setName('')
      setType('klipper')
      setHost('')
      setApiKey('')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed to add printer')
    } finally {
      setSaving(false)
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark w-full max-w-md max-h-[90vh] overflow-y-auto rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 font-mono text-xl font-semibold text-blue-400">[ add_printer ]</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            type="text"
            placeholder="Printer name *"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={inputClass}
          />
          <select
            value={type}
            onChange={(e) => setType(e.target.value)}
            className={inputClass}
          >
            <option value="klipper">Klipper / Moonraker</option>
            <option value="flashforge">FlashForge</option>
            <option value="other">Other / Generic</option>
          </select>
          <input
            type="text"
            placeholder="Host / IP (optional)"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            className={`${inputClass} sensitive`}
          />
          <input
            type="text"
            placeholder="API key (optional)"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            className={`${inputClass} sensitive`}
          />
          <p className="font-mono text-xs text-slate-500">
            For AnkerMake printers, log in via Settings to auto-discover.
          </p>
          {error && (
            <p className="rounded-lg border border-rose-600 bg-rose-950/30 p-3 font-mono text-sm text-rose-400">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-3">
            <button type="button" onClick={onClose} className={ghostClass}>
              cancel
            </button>
            <button type="submit" disabled={saving} className={btnClass}>
              {saving ? 'adding...' : 'add'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body
  )
}

function IntegrationModal({
  integration,
  enabled,
  values,
  onClose,
  onSave,
  onTest,
}: {
  integration: Integration | null
  enabled: boolean
  values: Record<string, string>
  onClose: () => void
  onSave: (enabled: boolean, values: Record<string, string>) => void
  onTest: (values: Record<string, string>) => void
}) {
  const inputClass =
    'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'
  const btnClass =
    'rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white shadow-sm hover:bg-blue-500 disabled:opacity-50'
  const ghostClass =
    'rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700'

  const [localEnabled, setLocalEnabled] = useState(enabled)
  const [localValues, setLocalValues] = useState<Record<string, string>>(values)

  useEffect(() => {
    setLocalEnabled(enabled)
    setLocalValues(values)
  }, [integration?.id, enabled, values])

  if (!integration) return null
  const ModalIcon = integrationIcons[integration.icon]

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        style={{
          borderTopColor: integration.color,
          boxShadow: `0 25px 50px -12px ${integration.color}20`,
        }}
      >
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            {ModalIcon && <ModalIcon className="h-5 w-5" style={{ color: integration.color }} />}
            <h2 className="text-xl font-mono font-semibold" style={{ color: integration.color }}>
              [ {integration.name} ]
            </h2>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>
        <p className="mb-2 font-mono text-sm text-slate-300">{integration.description}</p>
        <p className="mb-4 font-mono text-sm text-slate-400">{integration.longDesc}</p>
        {integration.url && (
          <a
            href={integration.url}
            target="_blank"
            rel="noreferrer"
            className="mb-4 inline-block font-mono text-sm text-blue-400 hover:underline"
          >
            {integration.urlLabel}
          </a>
        )}

        {/* Slicer setup instructions */}
        {(integration.id === 'prusaslicer' || integration.id === 'orcaslicer') && (
          <div className="mb-6 rounded-lg border border-slate-700 bg-slate-900/50 p-4">
            <h4 className="mb-3 font-mono text-sm font-semibold text-slate-200">Setup instructions</h4>
            <ol className="ml-4 list-decimal space-y-2 font-mono text-xs text-slate-400">
              <li>Open {integration.name} → Settings → Physical Printers → Add</li>
              <li>
                Set the API URL to your OpenPolyPrint address:
                <code className="ml-1 block rounded bg-slate-800 px-2 py-1 text-blue-300">
                  http://{'<openpolyprint-ip>'}:8080
                </code>
              </li>
              <li>Leave the API key field blank</li>
              <li>Set the default target printer in OpenPolyPrint → Settings → Slicer upload target</li>
              <li>
                For per-printer routing, use this URL instead:
                <code className="ml-1 block rounded bg-slate-800 px-2 py-1 text-blue-300">
                  http://{'<ip>'}:8080/api/files/{'<printer_name>'}/local
                </code>
              </li>
              <li>Upload G-code from the slicer — it will be sent to the printer via PPPP</li>
            </ol>
          </div>
        )}

        {integration.id === 'cura' && (
          <div className="mb-6 rounded-lg border border-slate-700 bg-slate-900/50 p-4">
            <h4 className="mb-3 font-mono text-sm font-semibold text-slate-200">Setup instructions</h4>
            <ol className="ml-4 list-decimal space-y-2 font-mono text-xs text-slate-400">
              <li>Open Cura → Marketplace → search for "OctoPrint" → install the plugin</li>
              <li>Restart Cura</li>
              <li>Settings → Printer → Add Printer → Add by OctoPrint URL</li>
              <li>
                Enter your OpenPolyPrint address:
                <code className="ml-1 block rounded bg-slate-800 px-2 py-1 text-blue-300">
                  http://{'<openpolyprint-ip>'}:8080
                </code>
              </li>
              <li>Leave the API key blank</li>
              <li>Set the default target printer in OpenPolyPrint → Settings → Slicer upload target</li>
            </ol>
          </div>
        )}

        <div className="mb-6 flex items-center justify-between rounded-lg border border-slate-700 bg-slate-900/50 p-3">
          <span className="font-mono text-sm text-slate-300">
            {integration.alwaysActive ? 'Always enabled' : 'Enabled'}
          </span>
          <Switch
            checked={localEnabled}
            onChange={integration.alwaysActive ? () => {} : (v: boolean) => setLocalEnabled(v)}
            disabled={integration.alwaysActive}
          />
        </div>

        {integration.fields.length > 0 && (
          <div className="sensitive space-y-4">
            {integration.fields.map((f) => (
              <div key={f.id}>
                <label className="mb-1 block font-mono text-xs text-slate-400">{f.label}</label>
                <input
                  type={f.type}
                  placeholder={f.placeholder}
                  value={localValues[f.id] ?? ''}
                  onChange={(e) => setLocalValues((prev) => ({ ...prev, [f.id]: e.target.value }))}
                  className={inputClass}
                />
              </div>
            ))}
          </div>
        )}

        {(integration.id === 'telegram' || integration.id === 'discord') && integration.fields.length > 0 && (
          <div className="mt-6 flex items-center gap-3">
            <button onClick={() => onTest(localValues)} className={btnClass}>
              Test
            </button>
          </div>
        )}

        <div className="mt-6 flex justify-end gap-3">
          <button onClick={onClose} className={ghostClass}>
            close
          </button>
          <button
            onClick={() => {
              onSave(localEnabled, localValues)
              onClose()
            }}
            className={btnClass}
          >
            save
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
}

export function Terminal() {
  const [lines, setLines] = useState<string[]>([])
  const [paused, setPaused] = useState(false)
  const [mini, setMini] = useState(() => loadConfig().showMiniTerminal)
  const [search, setSearch] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (paused) return
    const tick = () => {
      fetch('/api/logs')
        .then((r) => r.json())
        .then((data: any) => setLines(data.lines || []))
        .catch((err) => console.error(err))
    }
    tick()
    const id = setInterval(tick, 1000)
    return () => clearInterval(id)
  }, [paused])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [lines, paused])

  const clear = async () => {
    await fetch('/api/logs', { method: 'POST' })
    setLines([])
  }

  const download = () => {
    window.location.href = '/api/logs/download'
  }

  const [filter, setFilter] = useState<'all' | 'cameras' | 'printers' | 'pi' | 'gcode' | 'server'>('all')

  const filters = [
    { id: 'all', label: 'all', color: 'text-slate-300', bg: 'bg-slate-800', active: 'bg-slate-700' },
    { id: 'cameras', label: 'cameras', color: 'text-emerald-400', bg: 'bg-emerald-900/30', active: 'bg-emerald-900/50' },
    { id: 'printers', label: 'printers', color: 'text-blue-400', bg: 'bg-blue-900/30', active: 'bg-blue-900/50' },
    { id: 'pi', label: 'pi', color: 'text-amber-400', bg: 'bg-amber-900/30', active: 'bg-amber-900/50' },
    { id: 'gcode', label: 'gcode', color: 'text-purple-400', bg: 'bg-purple-900/30', active: 'bg-purple-900/50' },
    { id: 'server', label: 'server', color: 'text-cyan-400', bg: 'bg-cyan-900/30', active: 'bg-cyan-900/50' },
  ] as const

  const lineCategory = (line: string) => {
    const l = line.toLowerCase()
    if (l.includes('usb') || l.includes('streamer') || l.includes('camera') || l.includes('mjpeg') || l.includes('ffmpeg')) return 'cameras'
    if (l.includes('anker') || l.includes('printer') || l.includes('printing') || l.includes('pstate')) return 'printers'
    if (l.includes('pi') || l.includes('gpio') || l.includes('dht') || l.includes('pigpio')) return 'pi'
    if (l.includes('gcode')) return 'gcode'
    if (l.includes('api') || l.includes('http') || l.includes('listening on') || l.includes('settings') || l.includes('config')) return 'server'
    return 'all'
  }

  const lineColor = (line: string) => {
    const cat = lineCategory(line)
    const f = filters.find((x) => x.id === cat)
    return f?.color ?? 'text-slate-300'
  }

  const filteredLines = lines.filter((line) => {
    if (filter !== 'all' && lineCategory(line) !== filter) return false
    if (search && !line.toLowerCase().includes(search.toLowerCase())) return false
    return true
  })

  return (
    <div className="flex h-[calc(100vh-4rem)] flex-col gap-4">
      <div className="flex items-center justify-between">
        <SectionTitle title="Terminal" />
        <div className="flex gap-2">
          <button
            onClick={() => setPaused(!paused)}
            className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700"
          >
            {paused ? 'resume' : 'pause'}
          </button>
          <button
            onClick={clear}
            className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700"
          >
            clear
          </button>
          <button
            onClick={download}
            className="rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
          >
            download
          </button>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {filters.map((f) => (
          <button
            key={f.id}
            onClick={() => setFilter(f.id)}
            className={`rounded-lg px-3 py-1.5 font-mono text-xs font-medium ${f.color} ${filter === f.id ? f.active : f.bg} border border-slate-700`}
          >
            {f.label}
          </button>
        ))}
        <div className="relative ml-auto">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
          <input
            type="text"
            placeholder="search logs..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-48 rounded-lg border border-slate-700 bg-slate-900 py-1.5 pl-8 pr-3 font-mono text-xs text-slate-300 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>
        <span className="font-mono text-xs text-slate-500">
          {filteredLines.length} line{filteredLines.length !== 1 ? 's' : ''}
        </span>
      </div>

      <Card className="py-3">
        <div className="flex items-center justify-between">
          <span className="font-mono text-sm text-slate-300">show mini terminal in sidebar</span>
          <Switch
            checked={mini}
            onChange={(v) => {
              setMini(v)
              const cfg = loadConfig()
              saveConfig({ ...cfg, showMiniTerminal: v })
            }}
          />
        </div>
      </Card>
      <div className="flex-1 overflow-hidden rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-4 font-mono text-xs text-slate-300">
        <div className="h-full overflow-y-auto">
          {filteredLines.length === 0 ? (
            <p className="text-slate-500">
              {lines.length === 0 ? 'no logs yet.' : 'no logs for the selected filter.'}
            </p>
          ) : (
            <>
              {filteredLines.map((line, i) => (
                <div key={i} className={`whitespace-pre-wrap break-words py-0.5 ${lineColor(line)}`}>
                  {line}
                </div>
              ))}
              <div ref={bottomRef} />
            </>
          )}
        </div>
      </div>
    </div>
  )
}

interface AnkerConfig {
  account?: {
    user_id: string
    auth_token: string
    email: string
    region: string
  } | null
  printers: {
    id: string
    sn: string
    name: string
    model: string
    create_time: number
    update_time: number
    wifi_mac: string
    ip_addr: string
    api_hosts: string[]
    p2p_hosts: string[]
    p2p_duid: string
  }[]
}

function redact(s: string) {
  return s.length > 10 ? `${s.slice(0, 10)}...<REDACTED>` : '...<REDACTED>'
}

function prettyMAC(mac: string) {
  const parts: string[] = []
  for (let i = 0; i < mac.length; i += 2) {
    parts.push(mac.slice(i, i + 2))
  }
  return parts.join(':')
}

export function Settings() {
  const [config, setConfig] = useState<AppConfig>(loadConfig)
  const [saved, setSaved] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [ankerLoginOpen, setAnkerLoginOpen] = useState(false)
  const [anker, setAnker] = useState<AnkerConfig | null>(null)
  const [integrationOpen, setIntegrationOpen] = useState<string | null>(null)
  const { printers } = usePrinters()
  const push = usePushNotifications()

  const setUnsaved = () => setDirty(true)

  const update = (patch: Partial<AppConfig>) => {
    setConfig((c) => ({ ...c, ...patch }))
    setUnsaved()
  }

  const updateProvider = (key: keyof AppConfig['providers'], patch: Partial<ProviderConfig>) => {
    setConfig((c) => ({
      ...c,
      providers: { ...c.providers, [key]: { ...c.providers[key], ...patch } },
    }))
    setUnsaved()
  }

  const handleSave = () => {
    // Merge with the latest localStorage config so changes made outside
    // the Settings page (e.g. mini terminal toggle on the Terminal page)
    // are not overwritten by stale state.
    const latest = loadConfig()
    const merged = { ...latest, ...config }
    saveConfig(merged)
    setConfig(merged)
    setDirty(false)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  const loadAnker = async () => {
    try {
      const res = await fetch('/api/anker/config')
      if (!res.ok) throw new Error()
      setAnker(await res.json())
    } catch {
      setAnker(null)
    }
  }

  const handleLogout = async () => {
    await fetch('/api/anker/config', { method: 'DELETE' })
    setAnker(null)
  }

  useEffect(() => {
    loadAnker()
  }, [ankerLoginOpen])

  const inputClass =
    'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'

  return (
    <div className={`space-y-6 ${dirty && !saved ? 'rounded-xl outline-2 outline-amber-500/60 outline-dashed' : ''}`}>
      <SectionTitle title="Settings" />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Appearance</h3>
          <div className="space-y-4">
            <label className="flex items-center justify-between text-sm text-slate-700 dark:text-slate-300">
              Use dark theme
              <input
                type="checkbox"
                checked={config.dark}
                onChange={(e) => update({ dark: e.target.checked })}
                className="h-4 w-4 rounded border-slate-300 text-blue-600"
              />
            </label>
            <label className="flex items-center justify-between text-sm text-slate-700 dark:text-slate-300">
              Compact mode
              <input
                type="checkbox"
                checked={config.compact}
                onChange={(e) => update({ compact: e.target.checked })}
                className="h-4 w-4 rounded border-slate-300 text-blue-600"
              />
            </label>
          </div>
        </Card>
        <Card>
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Slicer upload target</h3>
          <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">
            When a slicer (PrusaSlicer, OrcaSlicer, Cura) uploads G-code via the OctoPrint API,
            it goes to this printer by default. You can also override per-upload by using
            <code className="mx-1 rounded bg-slate-100 px-1 dark:bg-slate-800">/api/files/{"{printer_name}"}/local</code>
            as the upload URL.
          </p>
          <select
            value={config.slicerTarget}
            onChange={(e) => update({ slicerTarget: e.target.value })}
            className={inputClass}
          >
            <option value="">Auto (first available printer)</option>
            {printers.map((p) => (
              <option key={p.id} value={p.name}>{p.name}</option>
            ))}
          </select>
        </Card>
        <Card>
          <div className="mb-2 flex items-center justify-between">
            <h3 className="font-semibold text-slate-900 dark:text-white">AI Analysis (Gemini)</h3>
            <Switch checked={config.geminiEnabled} onChange={(v) => update({ geminiEnabled: v })} />
          </div>
          <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">
            Bring your own Gemini API key to enable AI-powered print analysis.
            The key is stored locally and sent directly to Google's Gemini API.
            Get a key at <a href="https://aistudio.google.com/apikey" target="_blank" rel="noopener" className="text-blue-500 underline">aistudio.google.com/apikey</a>.
          </p>
          <input
            type="password"
            value={config.geminiApiKey}
            onChange={(e) => update({ geminiApiKey: e.target.value })}
            placeholder="AIza..."
            className={inputClass}
          />
        </Card>
        {push.supported && (
          <Card>
            <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Push notifications</h3>
            <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">
              Get notified on your phone when a print finishes or fails, even if the browser is closed.
              Requires HTTPS or localhost for background push.
            </p>
            {push.subscribed ? (
              <div className="flex items-center justify-between">
                <span className="flex items-center gap-2 text-sm text-emerald-600 dark:text-emerald-400">
                  <span className="h-2 w-2 rounded-full bg-emerald-500" /> Push notifications enabled
                </span>
                <button
                  onClick={() => push.unsubscribe()}
                  className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                >
                  Disable
                </button>
              </div>
            ) : (
              <button
                onClick={() => push.subscribe()}
                className="w-full rounded-lg bg-blue-600 py-2.5 text-sm font-medium text-white hover:bg-blue-500"
              >
                Enable push notifications
              </button>
            )}
          </Card>
        )}
        <Card>
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Notifications</h3>
          <div className="space-y-4">
            <label className="flex items-center justify-between text-sm text-slate-700 dark:text-slate-300">
              Print finished
              <input
                type="checkbox"
                checked={config.notifyFinished}
                onChange={(e) => update({ notifyFinished: e.target.checked })}
                className="h-4 w-4 rounded border-slate-300 text-blue-600"
              />
            </label>
            <label className="flex items-center justify-between text-sm text-slate-700 dark:text-slate-300">
              Print failed
              <input
                type="checkbox"
                checked={config.notifyFailed}
                onChange={(e) => update({ notifyFailed: e.target.checked })}
                className="h-4 w-4 rounded border-slate-300 text-blue-600"
              />
            </label>
          </div>
        </Card>

        <Card className="lg:col-span-2">
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Printer providers</h3>
          <div className="grid gap-4 md:grid-cols-2">
            <ProviderSection
              title="Anker (M5 / M5C)"
              enabled={config.providers.anker.enabled}
              onToggle={(v) => updateProvider('anker', { enabled: v })}
            >
              {anker?.account ? (
                <div className="space-y-4">
                  <div className="sensitive rounded border border-slate-700 bg-slate-950 p-3 font-mono text-xs text-slate-300">
                    <h4 className="mb-2 font-semibold text-blue-400">AnkerMake M5C Config</h4>
                    <p className="mb-1 text-slate-400">Account:</p>
                    <div className="mb-3 pl-2 text-slate-300">
                      <p>user_id: {redact(anker.account.user_id)}</p>
                      <p>auth_token: {redact(anker.account.auth_token)}</p>
                      <p>email: {anker.account.email}</p>
                      <p>region: {(anker.account.region || '').toUpperCase()}</p>
                    </div>
                    <p className="mb-1 text-slate-400">Printers:</p>
                    <div className="space-y-3">
                      {anker.printers.map((p, i) => (
                        <div key={p.id} className="pl-2">
                          <p>printer: {i}</p>
                          <p>id: {p.id}</p>
                          <p>name: {p.name}</p>
                          <p>duid: {p.p2p_duid}</p>
                          <p>sn: {p.sn}</p>
                          <p>model: {p.model}</p>
                          <p>created: {new Date(p.create_time * 1000).toLocaleString()}</p>
                          <p>updated: {new Date(p.update_time * 1000).toLocaleString()}</p>
                          <p>ip: {p.ip_addr || ''}</p>
                          <p>wifi_mac: {prettyMAC(p.wifi_mac)}</p>
                          <p>api_hosts:</p>
                          {p.api_hosts.length === 0 ? (
                            <p className="pl-2 text-slate-500">-</p>
                          ) : (
                            p.api_hosts.map((h) => <p key={h} className="pl-2">- {h}</p>)
                          )}
                          <p>p2p_hosts:</p>
                          {p.p2p_hosts.length === 0 ? (
                            <p className="pl-2 text-slate-500">-</p>
                          ) : (
                            p.p2p_hosts.map((h) => <p key={h} className="pl-2">- {h}</p>)
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <a
                      href="/api/anker/config/download"
                      className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700"
                    >
                      export default.json
                    </a>
                    <a
                      href="/api/anker/login.json"
                      className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700"
                    >
                      export login.json
                    </a>
                    <button
                      onClick={handleLogout}
                      className="rounded-lg bg-rose-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-rose-500"
                    >
                      log out
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <p className="text-xs text-slate-500 dark:text-slate-400">
                    Enable the AnkerMake protocol driver. Log in to fetch your printers from the cloud.
                  </p>
                  <button
                    type="button"
                    onClick={() => setAnkerLoginOpen(true)}
                    className="mt-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
                  >
                    Log in to Anker
                  </button>
                </>
              )}
            </ProviderSection>

            <div className="rounded-lg border border-slate-200 p-4 dark:border-slate-800">
              <h4 className="mb-2 font-mono text-sm font-semibold text-slate-700 dark:text-slate-300">
                Other printers
              </h4>
              <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">
                FlashForge, Klipper/Moonraker, and other printers are added via the
                add-printer wizard on the Printers page.
              </p>
              <Link
                to="/printers"
                className="inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
              >
                <Plus className="h-4 w-4" /> add printer
              </Link>
            </div>
          </div>
        </Card>

        <Card className="lg:col-span-2">
          <h3 className="mb-4 font-semibold text-slate-100">Integrations</h3>
          <div className="space-y-6">
            <div>
              <h4 className="mb-2 font-mono text-sm text-emerald-400">Enabled</h4>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {integrations
                  .filter((i) => i.alwaysActive || (config.integrations[i.id]?.enabled ?? false))
                  .map((i) => {
                    const Icon = integrationIcons[i.icon]
                    return (
                      <div
                        key={i.id}
                        onClick={() => setIntegrationOpen(i.id)}
                        className="cursor-pointer rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-4 shadow-md transition hover:shadow-lg"
                        style={{
                          borderTopColor: i.color,
                          boxShadow: `0 4px 6px -1px ${i.color}20`,
                        }}
                      >
                        <div className="mb-2 flex items-start justify-between">
                          <div className="flex items-start gap-2">
                            {Icon && <Icon className="mt-0.5 h-4 w-4" style={{ color: i.color }} />}
                            <div>
                              <h4 className="font-mono font-medium text-slate-100">{i.name}</h4>
                              <p className="font-mono text-xs text-slate-400">{i.category}</p>
                            </div>
                          </div>
                          <span className="h-2.5 w-2.5 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.6)]" />
                        </div>
                        <p className="font-mono text-sm text-slate-300">{i.description}</p>
                      </div>
                    )
                  })}
              </div>
            </div>
            <div>
              <h4 className="mb-2 font-mono text-sm text-slate-500">Available</h4>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {integrations
                  .filter((i) => !i.alwaysActive && !(config.integrations[i.id]?.enabled ?? false))
                  .map((i) => {
                    const Icon = integrationIcons[i.icon]
                    return (
                      <div
                        key={i.id}
                        onClick={() => setIntegrationOpen(i.id)}
                        className="cursor-pointer rounded-none border-2 border-slate-700 border-t-4 border-t-blue-500 bg-slate-950 p-4 shadow-md transition hover:shadow-lg"
                        style={{
                          borderTopColor: i.color,
                          boxShadow: `0 4px 6px -1px ${i.color}20`,
                        }}
                      >
                        <div className="mb-2 flex items-start justify-between">
                          <div className="flex items-start gap-2">
                            {Icon && <Icon className="mt-0.5 h-4 w-4" style={{ color: i.color }} />}
                            <div>
                              <h4 className="font-mono font-medium text-slate-100">{i.name}</h4>
                              <p className="font-mono text-xs text-slate-400">{i.category}</p>
                            </div>
                          </div>
                          <span className="h-2.5 w-2.5 rounded-full bg-slate-600" />
                        </div>
                        <p className="font-mono text-sm text-slate-300">{i.description}</p>
                      </div>
                    )
                  })}
              </div>
            </div>
          </div>
        </Card>
        <Card>
          <h3 className="mb-1 font-semibold text-slate-900 dark:text-white">Auto Record</h3>
          <p className="mb-4 font-mono text-xs text-slate-500 dark:text-slate-400">
            Automatically start recording when a print begins and stop when it ends.
            Uses cameras assigned to the printer that support recording (USB / MIPI).
          </p>
          <div className="space-y-4">
            <label className="flex items-center justify-between text-sm text-slate-700 dark:text-slate-300">
              <span>Enable auto-record on print start</span>
              <Switch
                checked={config.autoRecord.enabled}
                onChange={(v) => update({ autoRecord: { ...config.autoRecord, enabled: v } })}
              />
            </label>
            <div className={`space-y-4 ${config.autoRecord.enabled ? '' : 'opacity-50 pointer-events-none'}`}>
              <label className="block text-sm text-slate-700 dark:text-slate-300">
                Recording mode
                <select
                  value={config.autoRecord.mode}
                  onChange={(e) => update({ autoRecord: { ...config.autoRecord, mode: e.target.value as any } })}
                  className={inputClass}
                >
                  <option value="video">Video (continuous)</option>
                  <option value="timelapse">Timelapse (frame interval)</option>
                </select>
              </label>
              {config.autoRecord.mode === 'timelapse' && (
                <label className="block text-sm text-slate-700 dark:text-slate-300">
                  Frame interval (time between captures)
                  <select
                    value={config.autoRecord.interval}
                    onChange={(e) => update({ autoRecord: { ...config.autoRecord, interval: parseFloat(e.target.value) } })}
                    className={inputClass}
                  >
                    {RATES.map((r) => (
                      <option key={r.value} value={r.value}>
                        {r.label}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {config.autoRecord.enabled && (
                <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-slate-800 dark:bg-slate-900/50">
                  <p className="font-mono text-xs text-slate-500 dark:text-slate-400">
                    {config.autoRecord.mode === 'timelapse'
                      ? `> timelapse: 1 frame every ${config.autoRecord.interval}s, auto-start on print, auto-stop on finish`
                      : '> video: continuous recording, auto-start on print, auto-stop on finish'}
                  </p>
                </div>
              )}
            </div>
          </div>
        </Card>
        <Card>
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Offline keys</h3>
          <button
            onClick={async () => {
              try {
                const res = await fetch('/api/anker/export-keys')
                if (!res.ok) throw new Error(`status ${res.status}`)
                const data = await res.json()
                const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
                const url = URL.createObjectURL(blob)
                const a = document.createElement('a')
                a.href = url
                a.download = 'openpolyprint-offline-keys.json'
                a.click()
                URL.revokeObjectURL(url)
              } catch (e) {
                console.error(e)
                alert('Failed to export offline keys.')
              }
            }}
            className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
          >
            Export offline keys
          </button>
        </Card>
      </div>

      <AnkerLoginModal open={ankerLoginOpen} onClose={() => setAnkerLoginOpen(false)} />

      <IntegrationModal
        integration={
          integrationOpen
            ? integrations.find((i) => i.id === integrationOpen) ?? null
            : null
        }
        enabled={config.integrations[integrationOpen ?? '']?.enabled ?? false}
        values={config.integrations[integrationOpen ?? '']?.fields ?? {}}
        onClose={() => setIntegrationOpen(null)}
        onSave={(enabled, values) => {
          const id = integrationOpen ?? ''
          setConfig((c) => ({
            ...c,
            integrations: {
              ...c.integrations,
              [id]: { ...(c.integrations[id] ?? { enabled: false, fields: {} }), enabled, fields: values },
            },
          }))
          setUnsaved()
        }}
        onTest={(values) => {
          const id = integrationOpen ?? ''
          testIntegration(id, values).then(async (r) => {
            const data = await r.json().catch(() => ({} as any))
            if (!r.ok) {
              alert(`test failed: ${data.error || r.statusText}`)
            } else {
              alert('test message sent')
            }
          })
        }}
      />

      {createPortal(
        <div className="fixed bottom-6 right-6 z-[9999] flex items-center gap-3">
          {dirty && !saved && (
            <span className="rounded-lg bg-amber-500/90 px-3 py-2 font-mono text-xs font-medium text-white shadow-lg">
              unsaved changes
            </span>
          )}
          {saved && (
            <span className="rounded-lg bg-emerald-600/90 px-3 py-2 font-mono text-xs font-medium text-white shadow-lg">
              saved
            </span>
          )}
          <button
            onClick={handleSave}
            className={`flex items-center gap-2 rounded-xl px-5 py-3 text-sm font-semibold shadow-xl transition-all ${
              dirty && !saved
                ? 'bg-amber-500 text-white hover:bg-amber-400 ring-4 ring-amber-500/30'
                : saved
                  ? 'bg-emerald-600 text-white hover:bg-emerald-500'
                  : 'bg-blue-600 text-white hover:bg-blue-500'
            }`}
          >
            {dirty && !saved ? 'Save changes' : saved ? 'Saved' : 'Save settings'}
          </button>
        </div>,
        document.body
      )}
    </div>
  )
}

interface PiSettings {
  lightRelayEnabled: boolean
  lightRelayGpio: number
  lightRelayOn: boolean
  filamentSensors: any[]
}

export function Pi() {
  const [settings, setSettings] = useState<PiSettings>({ lightRelayEnabled: false, lightRelayGpio: 0, lightRelayOn: false, filamentSensors: [] })
  const [readings, setReadings] = useState<any[]>([])
  const [status, setStatus] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [saved, setSaved] = useState(false)

  const input = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'
  // BCM GPIOs exposed on the 40-pin header that are safe for general use
  // (excludes I2C 2/3, SPI 7-11, UART 14/15, and non-header pins).
  const PINS = [4, 5, 6, 12, 13, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27]

  const loadSettings = async () => {
    try {
      const res = await fetch('/api/pi')
      const data = await res.json()
      setSettings({
        lightRelayEnabled: data.lightRelayEnabled ?? false,
        lightRelayGpio: data.lightRelayGpio ?? 0,
        lightRelayOn: data.lightRelayOn ?? false,
        filamentSensors: data.filamentSensors ?? [],
      })
    } catch (e) {
      console.error(e)
    }
  }

  const loadReadings = async () => {
    try {
      const res = await fetch('/api/pi/readings')
      const data = await res.json()
      setReadings(data.sensors || [])
      setStatus({
        gpioAvailable: data.gpioAvailable,
        os: data.os,
        lightRelayEnabled: data.lightRelayEnabled,
        lightRelayGpio: data.lightRelayGpio,
        lightRelayOn: data.lightRelayOn,
        sensorManagerRunning: data.sensorManagerRunning,
      })
    } catch (e) {
      console.error(e)
    }
  }

  const load = async () => {
    await loadSettings()
    await loadReadings()
    setLoading(false)
  }

  useEffect(() => {
    load()
    const id = setInterval(loadReadings, 5000)
    return () => clearInterval(id)
  }, [])

  const save = async () => {
    const body = {
      lightRelayEnabled: settings.lightRelayEnabled,
      lightRelayGpio: Number(settings.lightRelayGpio),
      filamentSensors: settings.filamentSensors,
    }
    try {
      const res = await fetch('/api/pi', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      if (!res.ok) throw new Error('save failed')
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
      load()
    } catch (e) {
      alert('Save failed: ' + e)
    }
  }

  const toggleLight = async () => {
    try {
      const res = await fetch('/api/pi/light', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ on: !status?.lightRelayOn }),
      })
      const data = await res.json()
      if (data.success) setStatus((s: any) => ({ ...s, lightRelayOn: data.on }))
    } catch (e) {
      console.error(e)
    }
  }

  const updateSensor = (id: number, patch: Partial<any>) => {
    setSettings((s) => ({
      ...s,
      filamentSensors: s.filamentSensors.map((x: any) => (x.id === id ? { ...x, ...patch } : x)),
    }))
  }

  const addSensor = () => {
    const next = settings.filamentSensors.length ? Math.max(...settings.filamentSensors.map((s: any) => s.id)) + 1 : 1
    setSettings((s) => ({
      ...s,
      filamentSensors: [
        ...s.filamentSensors,
        { id: next, enabled: true, name: `Box ${next}`, gpioPin: 0, filamentType: 'PLA', color: '#3b82f6' },
      ],
    }))
  }

  const removeSensor = (id: number) => {
    setSettings((s) => ({ ...s, filamentSensors: s.filamentSensors.filter((x: any) => x.id !== id) }))
  }

  if (loading) {
    return <div className="p-8 font-mono text-slate-500">Loading Pi settings...</div>
  }

  return (
    <div className="space-y-6">
      <SectionTitle
        title="Pi"
        action={
          <div className="flex items-center gap-3">
            {saved && <span className="text-sm font-medium text-emerald-600 dark:text-emerald-400">Saved</span>}
            <button onClick={save} className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500">
              Save settings
            </button>
          </div>
        }
      />
      {status && (
        <p className="font-mono text-sm text-slate-500">
          GPIO {status.gpioAvailable ? 'available' : 'not available'} · {status.os} · sensor manager {status.sensorManagerRunning ? 'running' : 'stopped'}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Light relay</h3>
          <div className="space-y-4">
            <label className="flex items-center justify-between text-sm text-slate-700 dark:text-slate-300">
              Enable relay
              <input
                type="checkbox"
                checked={settings.lightRelayEnabled}
                onChange={(e) => setSettings((s) => ({ ...s, lightRelayEnabled: e.target.checked }))}
                className="h-4 w-4 rounded border-slate-300 text-blue-600"
              />
            </label>
            {settings.lightRelayEnabled && (
              <>
                <label className="block text-sm text-slate-700 dark:text-slate-300">
                  GPIO pin
                  <select
                    value={settings.lightRelayGpio}
                    onChange={(e) => setSettings((s) => ({ ...s, lightRelayGpio: Number(e.target.value) }))}
                    className={input}
                  >
                    {PINS.filter(
                      (p) => p === settings.lightRelayGpio || !settings.filamentSensors.some((x: any) => x.gpioPin === p)
                    ).map((p) => (
                      <option key={p} value={p}>
                        {p}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  onClick={toggleLight}
                  disabled={!status?.gpioAvailable}
                  className="rounded-lg bg-amber-600 px-4 py-2 text-sm font-medium text-white hover:bg-amber-500 disabled:opacity-50"
                >
                  {status?.lightRelayOn ? 'Turn light off' : 'Turn light on'}
                </button>
              </>
            )}
          </div>
        </Card>
        <Card>
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">DHT22 filament sensors</h3>
          <div className="space-y-4">
            {settings.filamentSensors.map((s: any) => {
              const r = readings.find((x) => x.id === s.id)
              return (
                <div key={s.id} className="rounded-lg border border-slate-200 p-3 dark:border-slate-800">
                  <div className="flex items-start gap-2">
                    <input
                      type="checkbox"
                      checked={s.enabled}
                      onChange={(e) => updateSensor(s.id, { enabled: e.target.checked })}
                      className="mt-1 h-4 w-4"
                    />
                    <div className="flex-1 space-y-2">
                      <div className="grid grid-cols-2 gap-2">
                        <input value={s.name} onChange={(e) => updateSensor(s.id, { name: e.target.value })} className={input} placeholder="Name" />
                        <select value={s.gpioPin} onChange={(e) => updateSensor(s.id, { gpioPin: Number(e.target.value) })} className={input}>
                          {PINS.filter(
                            (p) => p === s.gpioPin || !settings.filamentSensors.some((x: any) => x.id !== s.id && x.gpioPin === p)
                          ).map((p) => (
                            <option key={p} value={p}>
                              {p}
                            </option>
                          ))}
                        </select>
                      </div>
                      <div className="grid grid-cols-2 gap-2">
                        <input value={s.filamentType} onChange={(e) => updateSensor(s.id, { filamentType: e.target.value })} className={input} placeholder="Type" />
                        <input type="color" value={s.color} onChange={(e) => updateSensor(s.id, { color: e.target.value })} className="h-10 w-full rounded border border-slate-300 bg-white p-1 dark:border-slate-700 dark:bg-slate-950" />
                      </div>
                      {r?.hasReading && (
                        <p className="font-mono text-xs text-slate-500">
                          {r.error ? `Error: ${r.error}` : `${r.temp?.toFixed(1) ?? '--'}°C · ${r.humidity?.toFixed(1) ?? '--'}%`}
                        </p>
                      )}
                    </div>
                    <button onClick={() => removeSensor(s.id)} className="text-rose-600 hover:text-rose-500" title="remove">
                      ×
                    </button>
                  </div>
                </div>
              )
            })}
            {settings.filamentSensors.length < 5 && (
              <button
                onClick={addSensor}
                className="w-full rounded-lg border-2 border-dashed border-slate-300 py-2 text-sm font-medium text-slate-600 hover:border-blue-500 hover:text-blue-600 dark:border-slate-700 dark:text-slate-400 dark:hover:border-blue-500 dark:hover:text-blue-400"
              >
                + Add sensor
              </button>
            )}
          </div>
        </Card>
      </div>
    </div>
  )
}

export function Help() {
  return (
    <div className="space-y-6">
      <SectionTitle title="Help & About" />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <h3 className="mb-3 font-semibold text-slate-900 dark:text-white">About OpenPolyPrint</h3>
          <p className="text-sm text-slate-600 dark:text-slate-300">
            Multi-vendor 3D printer control built from the Anker protocol code. Designed for desktop first.
          </p>
          <p className="mt-2 text-xs text-slate-400">Version 0.1.0 · Prototype</p>
        </Card>
        <Card>
          <h3 className="mb-3 font-semibold text-slate-900 dark:text-white">Slicer setup (OctoPrint API)</h3>
          <p className="mb-3 text-sm text-slate-600 dark:text-slate-300">
            OpenPolyPrint exposes OctoPrint-compatible endpoints so slicers can upload G-code
            and start prints directly. Set the slicer target printer in Settings → Slicer upload target.
          </p>
          <div className="space-y-4">
            <div>
              <h4 className="mb-1 font-mono text-sm font-semibold text-slate-700 dark:text-slate-300">PrusaSlicer / OrcaSlicer</h4>
              <ol className="ml-4 list-decimal space-y-1 text-xs text-slate-500 dark:text-slate-400">
                <li>Open Settings → Physical Printers → Add</li>
                <li>Set the API URL to <code className="rounded bg-slate-100 px-1 dark:bg-slate-800">http://&lt;openpolyprint-ip&gt;:8080</code></li>
                <li>Leave the API key blank</li>
                <li>Uploads go to the printer set in Settings → Slicer upload target</li>
                <li>For per-printer routing, use <code className="rounded bg-slate-100 px-1 dark:bg-slate-800">http://&lt;ip&gt;:8080/api/files/&lt;printer_name&gt;/local</code> as the URL</li>
              </ol>
            </div>
            <div>
              <h4 className="mb-1 font-mono text-sm font-semibold text-slate-700 dark:text-slate-300">Cura</h4>
              <ol className="ml-4 list-decimal space-y-1 text-xs text-slate-500 dark:text-slate-400">
                <li>Install the OctoPrint plugin from Cura Marketplace</li>
                <li>Add a printer with the OpenPolyPrint address as the OctoPrint URL</li>
                <li>Leave the API key blank</li>
                <li>Uploads go to the printer set in Settings → Slicer upload target</li>
              </ol>
            </div>
            <div>
              <h4 className="mb-1 font-mono text-sm font-semibold text-slate-700 dark:text-slate-300">API endpoints</h4>
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-slate-200 text-slate-500 dark:border-slate-800 dark:text-slate-400">
                    <th className="pb-1 pr-4">Method</th>
                    <th className="pb-1 pr-4">Endpoint</th>
                    <th className="pb-1">Purpose</th>
                  </tr>
                </thead>
                <tbody className="text-slate-600 dark:text-slate-300">
                  <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1 pr-4 font-mono">GET</td><td className="pr-4 font-mono">/api/version</td><td>API version</td></tr>
                  <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1 pr-4 font-mono">GET</td><td className="pr-4 font-mono">/api/printer</td><td>State + temps</td></tr>
                  <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1 pr-4 font-mono">GET</td><td className="pr-4 font-mono">/api/job</td><td>Current job</td></tr>
                  <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1 pr-4 font-mono">POST</td><td className="pr-4 font-mono">/api/files/local</td><td>Upload (default printer)</td></tr>
                  <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1 pr-4 font-mono">POST</td><td className="pr-4 font-mono">/api/files/{"{name}"}/local</td><td>Upload (specific printer)</td></tr>
                  <tr><td className="py-1 pr-4 font-mono">POST</td><td className="pr-4 font-mono">/api/files/{"{name}"}</td><td>Select/start print</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </Card>
        <Card>
          <h3 className="mb-3 font-semibold text-slate-900 dark:text-white">Anker M5 and M5C MQTT command types</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
                  <th className="pb-2 pr-4">commandType</th>
                  <th className="pb-2 pr-4">Name</th>
                  <th className="pb-2">Fields</th>
                </tr>
              </thead>
              <tbody className="text-slate-700 dark:text-slate-300">
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1000</td><td className="pr-4">Event Notify</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">event_type</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1001</td><td className="pr-4">Print Schedule</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">schedule_type</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1002</td><td className="pr-4">Firmware Version</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1003</td><td className="pr-4">Nozzle Temp</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (°C)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1004</td><td className="pr-4">Hotbed Temp</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (°C)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1005</td><td className="pr-4">Fan Speed</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (0-255)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1006</td><td className="pr-4">Print Speed</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (%)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1007</td><td className="pr-4">Auto Leveling</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (1=start)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1008</td><td className="pr-4">Print Control</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">control (0=pause, 1=resume, 2=stop)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1009</td><td className="pr-4">File List Request</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1010</td><td className="pr-4">Gcode File Request</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">filename</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1011</td><td className="pr-4">Allow Firmware Update</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1012</td><td className="pr-4">Gcode File Download</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">filename</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1013</td><td className="pr-4">Z-Axis Recoup</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (1=start)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1014</td><td className="pr-4">Extrusion Step</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1015</td><td className="pr-4">Enter/Quit Material</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1016</td><td className="pr-4">Move Step</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">axis, step, speed</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1017</td><td className="pr-4">Move Direction</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">axis, direction</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1018</td><td className="pr-4">Move Zero (Home)</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (0=all)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1019</td><td className="pr-4">App Query Status</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1020</td><td className="pr-4">Online Notify</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1021</td><td className="pr-4">Recover Factory</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1023</td><td className="pr-4">BLE On/Off</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (0=off, 1=on)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1024</td><td className="pr-4">Delete Gcode File</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">filename</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1025</td><td className="pr-4">Reset Gcode Param</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1026</td><td className="pr-4">Device Name Set</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">name</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1027</td><td className="pr-4">Device Log Upload</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1028</td><td className="pr-4">On/Off Modal</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1029</td><td className="pr-4">Motor Lock</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1030</td><td className="pr-4">Preheat Config</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">nozzle_temp, bed_temp</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1031</td><td className="pr-4">Break Point</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1032</td><td className="pr-4">AI Calibration</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1033</td><td className="pr-4">Video On/Off</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value (0=off, 1=on)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1034</td><td className="pr-4">Advanced Parameters</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1035</td><td className="pr-4">Gcode Command</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">gcode (string)</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1036</td><td className="pr-4">Preview Image URL</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">url</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1041</td><td className="pr-4">System Check</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1042</td><td className="pr-4">AI Switch</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1043</td><td className="pr-4">AI Info Check</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1044</td><td className="pr-4">Model Layer</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1045</td><td className="pr-4">Model DL Process</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">—</td></tr>
                <tr className="border-b border-slate-100 dark:border-slate-800"><td className="py-1.5 pr-4 font-mono">1047</td><td className="pr-4">Print Max Speed</td><td className="font-mono text-xs text-slate-500 dark:text-slate-400">value</td></tr>
              </tbody>
            </table>
          </div>
        </Card>
      </div>
    </div>
  )
}
