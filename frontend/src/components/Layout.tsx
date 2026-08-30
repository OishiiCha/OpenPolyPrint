import { useEffect, useRef, useState } from 'react'
import { NavLink, Link, Outlet, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Printer,
  FileCode2,
  Video,
  Film,
  Cpu,
  History,
  Terminal,
  Settings,
  HelpCircle,
  Sun,
  Moon,
  Hexagon,
  ListOrdered,
  Package,
  Plug as PlugIcon,
  Sparkles,
  BarChart3,
  Gauge,
  Menu,
  X,
} from 'lucide-react'
import { loadConfig, saveConfig } from '../config'
import { AIChatPane } from './AIChatSidebar'

const navItems = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/printers', label: 'Printers', icon: Printer },
  { to: '/gcode', label: 'G-code', icon: FileCode2 },
  { to: '/queue', label: 'Queue', icon: ListOrdered },
  { to: '/filament', label: 'Filament', icon: Package },
  { to: '/profiles', label: 'Profiles', icon: Gauge },
  { to: '/plugs', label: 'Plugs', icon: PlugIcon },
  { to: '/analysis', label: 'Analysis', icon: Sparkles },
  { to: '/cameras', label: 'Cameras', icon: Video },
  { to: '/recordings', label: 'Recordings', icon: Film },
  { to: '/pi', label: 'Pi', icon: Cpu },
  { to: '/history', label: 'History', icon: History },
  { to: '/analytics', label: 'Analytics', icon: BarChart3 },
  { to: '/terminal', label: 'Terminal', icon: Terminal },
  { to: '/settings', label: 'Settings', icon: Settings },
  { to: '/help', label: 'Help', icon: HelpCircle },
]

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

function MiniTerminal() {
  const [lines, setLines] = useState<string[]>([])
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const tick = () => {
      fetch('/api/logs')
        .then((r) => r.json())
        .then((data: any) => setLines(data.lines || []))
        .catch(() => {})
    }
    tick()
    const id = setInterval(tick, 1000)
    return () => clearInterval(id)
  }, [])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [lines])

  return (
    <div className="sensitive flex h-40 flex-col border-t border-slate-200 bg-slate-900 dark:border-slate-800">
      <div className="flex items-center justify-between border-b border-slate-800 px-3 py-1.5">
        <span className="font-mono text-xs text-slate-400">mini_log</span>
        <Link to="/terminal" className="font-mono text-xs text-blue-400 hover:underline">
          expand
        </Link>
      </div>
      <div className="flex-1 overflow-y-auto p-3 font-mono text-[10px] leading-4 text-slate-300">
        {lines.length === 0 ? (
          <p className="text-slate-500">no logs yet.</p>
        ) : (
          <>
            {lines.slice(-40).map((line, i) => (
              <div key={i} className={`whitespace-pre-wrap break-words ${lineColor(line)}`}>
                {line}
              </div>
            ))}
            <div ref={bottomRef} />
          </>
        )}
      </div>
    </div>
  )
}

export function Layout() {
  const [theme, setTheme] = useState(() => loadConfig().theme)
  const [showMini, setShowMini] = useState(() => loadConfig().showMiniTerminal)
  const [geminiEnabled, setGeminiEnabled] = useState(() => loadConfig().geminiEnabled)
  const [analyticsEnabled, setAnalyticsEnabled] = useState(() => loadConfig().analyticsEnabled)
  const [hiddenNavItems, setHiddenNavItems] = useState<string[]>(() => loadConfig().hiddenNavItems ?? [])
  const [chatCollapsed, setChatCollapsed] = useState(() => {
    try { return localStorage.getItem('aiChatCollapsed') === 'true' } catch { return false }
  })
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const location = useLocation()

  const toggleChat = () => {
    const next = !chatCollapsed
    setChatCollapsed(next)
    try { localStorage.setItem('aiChatCollapsed', String(next)) } catch {}
  }

  // Close mobile sidebar on route change
  useEffect(() => {
    setSidebarOpen(false)
  }, [location.pathname])

  useEffect(() => {
    const el = document.documentElement
    // Remove all theme classes
    el.classList.remove('dark', 'theme-polygon')
    if (theme === 'dark') {
      el.classList.add('dark')
    } else if (theme === 'polygon') {
      el.classList.add('theme-polygon')
    }
  }, [theme])

  useEffect(() => {
    const handler = () => {
      const cfg = loadConfig()
      setTheme(cfg.theme)
      setShowMini(cfg.showMiniTerminal)
      setGeminiEnabled(cfg.geminiEnabled)
      setAnalyticsEnabled(cfg.analyticsEnabled)
      setHiddenNavItems(cfg.hiddenNavItems ?? [])
    }
    window.addEventListener('openpolyprint-config-updated', handler)
    return () => window.removeEventListener('openpolyprint-config-updated', handler)
  }, [])

  return (
    <div className="flex h-screen w-full overflow-hidden">
      {/* Mobile sidebar overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      <aside className={`fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-slate-300 bg-slate-100 transition-transform duration-200 dark:border-slate-700 dark:bg-slate-950 lg:static lg:translate-x-0 ${sidebarOpen ? 'translate-x-0' : '-translate-x-full'}`}>
        <div className="flex items-center justify-between px-4 py-5">
          <Link to="/" className="flex items-center gap-3">
            <img src="/logo.svg" alt="OpenPolyPrint" className="h-8 w-auto lg:h-10" />
            <span className="text-xl font-mono font-bold tracking-tight text-blue-600 dark:text-blue-400 lg:text-2xl">
              OpenPolyPrint
            </span>
          </Link>
          <button
            onClick={() => setSidebarOpen(false)}
            className="rounded-lg p-1.5 text-slate-500 hover:bg-slate-200 dark:hover:bg-slate-800 lg:hidden"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-2">
          {navItems.filter((item) =>
            (item.to !== '/analysis' || geminiEnabled) &&
            (item.to !== '/analytics' || analyticsEnabled) &&
            !hiddenNavItems.includes(item.to)
          ).map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors ${
                  isActive
                    ? 'bg-blue-600 text-white'
                    : 'text-slate-600 hover:bg-slate-200 dark:text-slate-400 dark:hover:bg-slate-800'
                }`
              }
            >
              <item.icon className="h-5 w-5 shrink-0" />
              <span className="truncate">{item.label}</span>
            </NavLink>
          ))}
        </nav>

        {showMini && <MiniTerminal />}

        <div className="border-t border-slate-200 p-4 dark:border-slate-800">
          <button
            onClick={() => {
              const cfg = loadConfig()
              const themes: Array<'light' | 'dark' | 'polygon'> = ['light', 'dark', 'polygon']
              const currentIdx = themes.indexOf(cfg.theme)
              const nextTheme = themes[(currentIdx + 1) % themes.length]
              saveConfig({ ...cfg, theme: nextTheme, dark: nextTheme === 'dark' })
            }}
            className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-200 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            {theme === 'dark' ? <Sun className="h-5 w-5" /> : theme === 'polygon' ? <Hexagon className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
            {theme === 'dark' ? 'Light mode' : theme === 'polygon' ? 'Polygon' : 'Dark mode'}
          </button>
        </div>
      </aside>

      <div className="flex flex-1 flex-col overflow-hidden bg-white dark:bg-slate-950">
        {/* Mobile top bar with hamburger */}
        <div className="flex items-center gap-3 border-b border-slate-200 px-4 py-3 dark:border-slate-800 lg:hidden">
          <button
            onClick={() => setSidebarOpen(true)}
            className="rounded-lg p-1.5 text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            <Menu className="h-6 w-6" />
          </button>
          <Link to="/" className="flex items-center gap-2">
            <img src="/logo.svg" alt="OpenPolyPrint" className="h-6 w-auto" />
            <span className="text-lg font-mono font-bold tracking-tight text-blue-600 dark:text-blue-400">
              OpenPolyPrint
            </span>
          </Link>
        </div>

        <main className="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8">
          <Outlet />
        </main>
      </div>

      {geminiEnabled && (
        <AIChatPane collapsed={chatCollapsed} onToggle={toggleChat} />
      )}
    </div>
  )
}
