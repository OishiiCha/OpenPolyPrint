import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  Camera as CameraIcon,
  Printer as PrinterIcon,
  Settings as SettingsIcon,
  LayoutDashboard,
  Lightbulb,
  Sun,
  Moon,
  X,
  Bell as BellIcon,
  Play,
  Pause,
  Square,
  Thermometer,
  Gauge,
  Clock,
  Layers,
} from 'lucide-react'
import { loadConfig, saveConfig } from '../config'
import { AutoScrollText } from './AutoScrollText'
import { useCameras } from '../hooks/useCameras'
import { usePrinters } from '../hooks/usePrinters'
import { usePiReadings } from '../hooks/usePi'
import { usePushNotifications } from '../hooks/usePushNotifications'
import type { Printer } from '../types'

type Tab = 'dashboard' | 'cameras' | 'printer' | 'settings'

export function PhoneLayout({ children: _children }: { children: React.ReactNode }) {
  const [tab, setTab] = useState<Tab>('dashboard')
  const [isDark, setIsDark] = useState(() => loadConfig().dark)

  useEffect(() => {
    if (isDark) document.documentElement.classList.add('dark')
    else document.documentElement.classList.remove('dark')
  }, [isDark])

  useEffect(() => {
    const handler = () => {
      const cfg = loadConfig()
      setIsDark(cfg.dark)
    }
    window.addEventListener('openpolyprint-config-updated', handler)
    return () => window.removeEventListener('openpolyprint-config-updated', handler)
  }, [])

  const tabs: { id: Tab; icon: typeof LayoutDashboard; label: string }[] = [
    { id: 'dashboard', icon: LayoutDashboard, label: 'Home' },
    { id: 'cameras', icon: CameraIcon, label: 'Cameras' },
    { id: 'printer', icon: PrinterIcon, label: 'Printer' },
    { id: 'settings', icon: SettingsIcon, label: 'Settings' },
  ]

  return (
    <div className="flex h-[100dvh] flex-col bg-white dark:bg-slate-950">
      {/* Status bar space for notch */}
      <div className="h-[env(safe-area-inset-top)] bg-white dark:bg-slate-950" />

      {/* Content area */}
      <div className="flex-1 overflow-hidden">
        {tab === 'dashboard' && <PhoneDashboard />}
        {tab === 'cameras' && <PhoneCameras />}
        {tab === 'printer' && <PhonePrinter />}
        {tab === 'settings' && <PhoneSettings isDark={isDark} />}
      </div>

      {/* Bottom tab bar */}
      <nav className="flex items-center justify-around border-t border-slate-200 bg-white px-2 pb-[env(safe-area-inset-bottom)] pt-2 dark:border-slate-800 dark:bg-slate-950">
        {tabs.map((t) => {
          const Icon = t.icon
          const active = tab === t.id
          return (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`flex flex-1 flex-col items-center gap-1 rounded-lg py-1.5 transition-colors ${
                active
                  ? 'text-blue-600 dark:text-blue-400'
                  : 'text-slate-400 dark:text-slate-600'
              }`}
            >
              <Icon className="h-6 w-6" />
              <span className="text-[10px] font-medium">{t.label}</span>
            </button>
          )
        })}
      </nav>
    </div>
  )
}

// ─── Dashboard Tab ────────────────────────────────────────────────────────────

function PhoneDashboard() {
  const { cameras } = useCameras()
  const { printers } = usePrinters()
  const { readings, toggleLight } = usePiReadings()
  const [selectedCamera, setSelectedCamera] = useState(0)
  const [showExpand, setShowExpand] = useState(false)

  const enabledCameras = cameras.filter((c) => c.enabled)
  const activePrinters = printers.filter((p) => p.status === 'Printing')
  const mainPrinter = activePrinters[0] || printers[0]
  const mainCamera = enabledCameras[selectedCamera] || enabledCameras[0]

  return (
    <div className="flex h-full flex-col">
      {/* Camera view — takes most of the screen */}
      <div className="relative flex-1 bg-black" onClick={() => mainCamera && setShowExpand(true)}>
        {mainCamera ? (
          <img
            src={mainCamera.url}
            alt={mainCamera.name}
            className="h-full w-full object-contain"
          />
        ) : (
          <div className="flex h-full items-center justify-center">
            <CameraIcon className="h-12 w-12 text-slate-600" />
          </div>
        )}

        {/* Printer status overlay (top-left) */}
        {mainPrinter && (
          <div className="absolute left-2 top-2 max-w-[calc(100%-1rem)] rounded-lg bg-black/60 px-3 py-2 backdrop-blur-sm">
            <div className="flex items-center gap-2">
              <div className={`h-2 w-2 shrink-0 rounded-full ${mainPrinter.status === 'Printing' ? 'animate-pulse bg-blue-500' : mainPrinter.status === 'Offline' ? 'bg-rose-500' : 'bg-emerald-500'}`} />
              <AutoScrollText text={mainPrinter.name} className="font-mono text-xs font-medium text-white" />
            </div>
            {mainPrinter.status === 'Printing' && (
              <div className="mt-1">
                <div className="flex items-center justify-between gap-3 font-mono text-[10px] text-slate-300">
                  <span>{mainPrinter.progress}%</span>
                  {mainPrinter.remainingTime && <span>{mainPrinter.remainingTime}</span>}
                </div>
                <div className="mt-1 h-1 w-32 overflow-hidden rounded-full bg-slate-700">
                  <div className="h-full bg-blue-500 transition-all" style={{ width: `${mainPrinter.progress}%` }} />
                </div>
              </div>
            )}
            <div className="mt-1 flex gap-3 font-mono text-[10px] text-slate-400">
              <span className="flex items-center gap-1"><Thermometer className="h-3 w-3" />{Math.round(mainPrinter.temps.nozzle)}°</span>
              <span className="flex items-center gap-1"><Gauge className="h-3 w-3" />{Math.round(mainPrinter.temps.bed)}°</span>
            </div>
          </div>
        )}

        {/* Light button (top-right) */}
        {readings?.lightRelayEnabled && (
          <button
            onClick={(e) => { e.stopPropagation(); toggleLight(!readings.lightRelayOn) }}
            className={`absolute right-2 top-2 flex h-10 w-10 items-center justify-center rounded-full backdrop-blur-sm transition-colors ${
              readings.lightRelayOn ? 'bg-amber-500/80 text-white' : 'bg-black/60 text-slate-300'
            }`}
          >
            <Lightbulb className="h-5 w-5" />
          </button>
        )}

        {/* Expand icon */}
        {mainCamera && (
          <div className="absolute bottom-2 right-2 rounded bg-black/40 px-2 py-1 font-mono text-[10px] text-slate-300">
            tap to expand
          </div>
        )}
      </div>

      {/* Camera slider at bottom */}
      {enabledCameras.length > 1 && (
        <div className="no-scrollbar flex gap-2 overflow-x-auto bg-slate-900 px-2 py-2">
          {enabledCameras.map((cam, i) => (
            <button
              key={cam.id}
              onClick={() => setSelectedCamera(i)}
              className={`relative flex-shrink-0 overflow-hidden rounded-lg border-2 transition-all ${
                i === selectedCamera ? 'border-blue-500' : 'border-transparent opacity-60'
              }`}
              style={{ width: 64, height: 48 }}
            >
              <img src={cam.url} alt={cam.name} className="h-full w-full object-cover" />
              <div className="absolute bottom-0 left-0 right-0 truncate bg-black/60 px-1 font-mono text-[8px] text-white">
                {cam.name}
              </div>
            </button>
          ))}
        </div>
      )}

      {/* Expand modal */}
      {showExpand && mainCamera && createPortal(
        <div className="fixed inset-0 z-[9999] flex flex-col bg-black" onClick={() => setShowExpand(false)}>
          <div className="flex items-center justify-between p-4">
            <span className="font-mono text-sm text-white">{mainCamera.name}</span>
            <button onClick={() => setShowExpand(false)} className="text-slate-400 hover:text-white"><X className="h-6 w-6" /></button>
          </div>
          <div className="flex flex-1 items-center justify-center" onClick={(e) => e.stopPropagation()}>
            <img src={mainCamera.url} alt={mainCamera.name} className="max-h-full max-w-full object-contain" />
          </div>
        </div>,
        document.body
      )}
    </div>
  )
}

// ─── Cameras Tab ──────────────────────────────────────────────────────────────

function PhoneCameras() {
  const { cameras } = useCameras()
  const [selected, setSelected] = useState(0)
  const enabledCameras = cameras.filter((c) => c.enabled)
  const cam = enabledCameras[selected] || enabledCameras[0]

  return (
    <div className="flex h-full flex-col bg-black">
      {/* Main camera view */}
      <div className="flex flex-1 items-center justify-center">
        {cam ? (
          <img src={cam.url} alt={cam.name} className="max-h-full max-w-full object-contain" />
        ) : (
          <div className="flex flex-col items-center gap-2 text-slate-600">
            <CameraIcon className="h-12 w-12" />
            <p className="font-mono text-sm">No cameras configured</p>
          </div>
        )}
      </div>

      {/* Camera name + type */}
      {cam && (
        <div className="flex items-center justify-between px-4 py-2">
          <span className="font-mono text-sm text-white">{cam.name}</span>
          <span className="rounded bg-slate-800 px-2 py-0.5 font-mono text-[10px] text-slate-400">{cam.type}</span>
        </div>
      )}

      {/* Camera selector — horizontal scroll */}
      {enabledCameras.length > 1 && (
        <div className="no-scrollbar flex gap-2 overflow-x-auto px-4 pb-4">
          {enabledCameras.map((c, i) => (
            <button
              key={c.id}
              onClick={() => setSelected(i)}
              className={`flex-shrink-0 rounded-lg border-2 px-3 py-2 font-mono text-xs transition-all ${
                i === selected
                  ? 'border-blue-500 bg-blue-500/20 text-white'
                  : 'border-slate-700 bg-slate-900 text-slate-400'
              }`}
            >
              {c.name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Printer Tab ──────────────────────────────────────────────────────────────

function PhonePrinter() {
  const { printers } = usePrinters()
  const [selectedId, setSelectedId] = useState('')

  const activePrinter = printers.find((p) => p.status === 'Printing')
  const printer = printers.find((p) => p.id === selectedId) || activePrinter || printers[0]

  useEffect(() => {
    if (printer && !selectedId) setSelectedId(printer.id)
  }, [printer, selectedId])

  if (printers.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-2 text-slate-400">
          <PrinterIcon className="h-12 w-12" />
          <p className="font-mono text-sm">No printers configured</p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-y-auto bg-white p-4 dark:bg-slate-950">
      {/* Printer selector */}
      {printers.length > 1 && (
        <div className="no-scrollbar mb-4 flex gap-2 overflow-x-auto">
          {printers.map((p) => (
            <button
              key={p.id}
              onClick={() => setSelectedId(p.id)}
              className={`flex-shrink-0 rounded-lg border-2 px-3 py-1.5 font-mono text-xs transition-all ${
                p.id === printer?.id
                  ? 'border-blue-500 bg-blue-500/10 text-blue-600 dark:text-blue-400'
                  : 'border-slate-200 text-slate-500 dark:border-slate-800 dark:text-slate-400'
              }`}
            >
              {p.name}
            </button>
          ))}
        </div>
      )}

      {printer && <PrinterCard printer={printer} />}
    </div>
  )
}

function PrinterCard({ printer }: { printer: Printer }) {
  const [busy, setBusy] = useState(false)

  const send = async (path: string) => {
    setBusy(true)
    try {
      await fetch(`/api/printers/${printer.id}${path}`, { method: 'POST' })
    } catch (e) {
      console.error(e)
    } finally {
      setBusy(false)
    }
  }

  const isPrinting = printer.status === 'Printing'
  const isPaused = printer.status === 'Paused'

  return (
    <div className="space-y-4">
      {/* Status header */}
      <div className="rounded-2xl border border-slate-200 p-4 dark:border-slate-800">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className={`h-3 w-3 rounded-full ${
              isPrinting ? 'animate-pulse bg-blue-500' :
              isPaused ? 'bg-amber-500' :
              printer.status === 'Offline' ? 'bg-rose-500' : 'bg-emerald-500'
            }`} />
            <span className="font-mono text-sm font-semibold text-slate-900 dark:text-white">{printer.name}</span>
          </div>
          <span className="font-mono text-xs text-slate-500">{printer.status}</span>
        </div>
        <p className="mt-1 font-mono text-xs text-slate-400">{printer.type}</p>
      </div>

      {/* Progress */}
      {isPrinting && (
        <div className="rounded-2xl border border-slate-200 p-4 dark:border-slate-800">
          <div className="flex items-center justify-between font-mono text-xs text-slate-500 dark:text-slate-400">
            <span className="flex items-center gap-1"><Clock className="h-3 w-3" />{printer.remainingTime || '—'}</span>
            <span>{printer.progress}%</span>
          </div>
          <div className="mt-2 h-2 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
            <div className="h-full bg-blue-500 transition-all" style={{ width: `${printer.progress}%` }} />
          </div>
          {printer.currentFile && (
            <p className="mt-2 truncate font-mono text-[10px] text-slate-400">{printer.currentFile}</p>
          )}

          {/* Layer + time info */}
          <div className="mt-3 grid grid-cols-2 gap-2">
            {(printer.layerCount ?? 0) > 0 && (
              <div className="flex items-center gap-1.5 font-mono text-xs text-slate-600 dark:text-slate-300">
                <Layers className="h-3 w-3 text-slate-400" />
                <span>{printer.layerNum || 0} / {printer.layerCount}</span>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Temperatures */}
      <div className="grid grid-cols-2 gap-3">
        <div className="rounded-2xl border border-slate-200 p-4 dark:border-slate-800">
          <div className="flex items-center gap-2 text-slate-500 dark:text-slate-400">
            <Thermometer className="h-4 w-4" />
            <span className="font-mono text-xs">Nozzle</span>
          </div>
          <p className="mt-1 font-mono text-2xl font-bold text-slate-900 dark:text-white">
            {Math.round(printer.temps.nozzle)}°
          </p>
          {printer.temps.targetNozzle > 0 && (
            <p className="font-mono text-[10px] text-slate-400">→ {Math.round(printer.temps.targetNozzle)}°</p>
          )}
        </div>
        <div className="rounded-2xl border border-slate-200 p-4 dark:border-slate-800">
          <div className="flex items-center gap-2 text-slate-500 dark:text-slate-400">
            <Gauge className="h-4 w-4" />
            <span className="font-mono text-xs">Bed</span>
          </div>
          <p className="mt-1 font-mono text-2xl font-bold text-slate-900 dark:text-white">
            {Math.round(printer.temps.bed)}°
          </p>
          {printer.temps.targetBed > 0 && (
            <p className="font-mono text-[10px] text-slate-400">→ {Math.round(printer.temps.targetBed)}°</p>
          )}
        </div>
      </div>

      {/* Controls */}
      <div className="grid grid-cols-3 gap-3">
        {isPrinting ? (
          <button
            onClick={() => send('/pause')}
            disabled={busy}
            className="flex flex-col items-center gap-1 rounded-2xl bg-amber-500 py-4 text-white disabled:opacity-50"
          >
            <Pause className="h-6 w-6" />
            <span className="font-mono text-xs">Pause</span>
          </button>
        ) : isPaused ? (
          <button
            onClick={() => send('/pause')}
            disabled={busy}
            className="flex flex-col items-center gap-1 rounded-2xl bg-blue-600 py-4 text-white disabled:opacity-50"
          >
            <Play className="h-6 w-6" />
            <span className="font-mono text-xs">Resume</span>
          </button>
        ) : (
          <div className="flex flex-col items-center gap-1 rounded-2xl bg-slate-100 py-4 text-slate-400 dark:bg-slate-900">
            <Pause className="h-6 w-6" />
            <span className="font-mono text-xs">Idle</span>
          </div>
        )}
        <button
          onClick={() => send('/stop')}
          disabled={busy || (!isPrinting && !isPaused)}
          className="flex flex-col items-center gap-1 rounded-2xl bg-rose-600 py-4 text-white disabled:opacity-50"
        >
          <Square className="h-6 w-6" />
          <span className="font-mono text-xs">Stop</span>
        </button>
        <button
          onClick={() => send('/home')}
          disabled={busy || isPrinting}
          className="flex flex-col items-center gap-1 rounded-2xl bg-slate-200 py-4 text-slate-700 disabled:opacity-50 dark:bg-slate-800 dark:text-slate-300"
        >
          <LayoutDashboard className="h-6 w-6" />
          <span className="font-mono text-xs">Home</span>
        </button>
      </div>
    </div>
  )
}

// ─── Settings Tab ─────────────────────────────────────────────────────────────

function PhoneSettings({ isDark }: { isDark: boolean }) {
  const push = usePushNotifications()

  const toggle = (key: 'dark', value: boolean) => {
    const cfg = loadConfig()
    saveConfig({ ...cfg, [key]: value })
  }

  return (
    <div className="h-full overflow-y-auto bg-white p-4 dark:bg-slate-950">
      <h2 className="mb-4 font-mono text-lg font-bold text-slate-900 dark:text-white">Settings</h2>

      {/* Quick toggles */}
      <div className="space-y-3">
        <button
          onClick={() => toggle('dark', !isDark)}
          className="flex w-full items-center justify-between rounded-2xl border border-slate-200 p-4 dark:border-slate-800"
        >
          <div className="flex items-center gap-3">
            {isDark ? <Sun className="h-5 w-5 text-amber-500" /> : <Moon className="h-5 w-5 text-slate-600" />}
            <span className="font-mono text-sm text-slate-900 dark:text-white">{isDark ? 'Light mode' : 'Dark mode'}</span>
          </div>
          <div className={`h-6 w-11 rounded-full transition-colors ${isDark ? 'bg-blue-600' : 'bg-slate-300'}`}>
            <div className={`h-5 w-5 rounded-full bg-white transition-transform ${isDark ? 'translate-x-5' : 'translate-x-0.5'} mt-0.5`} />
          </div>
        </button>

        {/* Push notifications */}
        {push.supported && (
          <button
            onClick={() => push.subscribed ? push.unsubscribe() : push.subscribe()}
            className="flex w-full items-center justify-between rounded-2xl border border-slate-200 p-4 dark:border-slate-800"
          >
            <div className="flex items-center gap-3">
              <BellIcon className="h-5 w-5 text-slate-600 dark:text-slate-400" />
              <span className="font-mono text-sm text-slate-900 dark:text-white">Push notifications</span>
            </div>
            <div className={`h-6 w-11 rounded-full transition-colors ${push.subscribed ? 'bg-blue-600' : 'bg-slate-300'}`}>
              <div className={`h-5 w-5 rounded-full bg-white transition-transform ${push.subscribed ? 'translate-x-5' : 'translate-x-0.5'} mt-0.5`} />
            </div>
          </button>
        )}
      </div>

      <div className="mt-6 space-y-2">
        <p className="font-mono text-xs text-slate-400">Full settings available at:</p>
        <p className="rounded-lg bg-slate-100 px-3 py-2 font-mono text-xs text-slate-600 dark:bg-slate-900 dark:text-slate-400">
          Open OpenPolyPrint on a desktop browser for all settings, integrations, G-code management, history, and terminal.
        </p>
      </div>
    </div>
  )
}
