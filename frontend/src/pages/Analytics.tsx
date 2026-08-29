import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  ArrowLeft, BarChart3, TrendingUp, Clock, CheckCircle2, XCircle,
  DollarSign, Layers, Activity, Loader2, AlertTriangle,
} from 'lucide-react'

interface RecentPrint {
  id: string
  printer: string
  file: string
  result: string
  started: string
  duration: string
}

interface Stats {
  totalPrints: number
  successfulPrints: number
  failedPrints: number
  cancelledPrints: number
  successRate: number
  totalPrintTimeSeconds: number
  totalFilamentUsedG: number
  totalEstimatedCost: number
  avgPrintTimeSeconds: number
  printsPerPrinter: Record<string, number>
  printsPerFile: Record<string, number>
  printsPerDay: Record<string, number>
  filamentByType: Record<string, number>
  filamentByBrand: Record<string, number>
  recentFailures: RecentPrint[]
  longestPrints: RecentPrint[]
}

function formatTime(s: number): string {
  if (s <= 0) return '0s'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m`
  return `${Math.floor(s)}s`
}

function formatCost(c: number): string {
  if (c <= 0) return '$0.00'
  return `$${c.toFixed(2)}`
}

function formatWeight(g: number): string {
  if (g >= 1000) return `${(g / 1000).toFixed(2)} kg`
  return `${g.toFixed(0)} g`
}

export function Analytics() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetch('/api/analytics')
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((data: Stats) => {
        // Validate the response has the expected shape
        if (data && typeof data.totalPrints === 'number') {
          setStats(data)
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
      </div>
    )
  }

  if (!stats || stats.totalPrints === 0) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <Link to="/" className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white">
            <ArrowLeft className="h-4 w-4" /> Back
          </Link>
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Analytics</h1>
        </div>
        <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
          <BarChart3 className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-700" />
          <p className="font-mono text-sm text-slate-400">
            No print data yet. Complete some prints to see analytics.
          </p>
        </div>
      </div>
    )
  }

  const topFiles = Object.entries(stats.printsPerFile || {})
    .sort(([, a], [, b]) => b - a)
    .slice(0, 10)
  const topPrinters = Object.entries(stats.printsPerPrinter || {})
    .sort(([, a], [, b]) => b - a)

  // Simple bar chart for prints per day (last 14 days with data)
  const dayEntries = Object.entries(stats.printsPerDay || {}).sort(([a], [b]) => a.localeCompare(b))
  const recentDays = dayEntries.slice(-14)
  const maxDayCount = Math.max(1, ...recentDays.map(([, c]) => c))

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link to="/" className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white">
          <ArrowLeft className="h-4 w-4" /> Back
        </Link>
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Analytics</h1>
      </div>

      {/* Top-level stats */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {/* Total prints */}
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <div className="flex items-center gap-2 text-slate-400">
            <Activity className="h-4 w-4" />
            <span className="font-mono text-xs">Total Prints</span>
          </div>
          <p className="mt-2 text-3xl font-bold text-slate-900 dark:text-white">{stats.totalPrints}</p>
          <div className="mt-2 flex gap-3 text-xs">
            <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
              <CheckCircle2 className="h-3 w-3" /> {stats.successfulPrints}
            </span>
            <span className="flex items-center gap-1 text-rose-600 dark:text-rose-400">
              <XCircle className="h-3 w-3" /> {stats.failedPrints}
            </span>
            {stats.cancelledPrints > 0 && (
              <span className="text-slate-400">{stats.cancelledPrints} cancelled</span>
            )}
          </div>
        </div>

        {/* Success rate */}
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <div className="flex items-center gap-2 text-slate-400">
            <TrendingUp className="h-4 w-4" />
            <span className="font-mono text-xs">Success Rate</span>
          </div>
          <p className="mt-2 text-3xl font-bold text-slate-900 dark:text-white">{stats.successRate.toFixed(1)}%</p>
          <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
            <div
              className={`h-full rounded-full ${stats.successRate >= 80 ? 'bg-emerald-500' : stats.successRate >= 50 ? 'bg-amber-500' : 'bg-rose-500'}`}
              style={{ width: `${stats.successRate}%` }}
            />
          </div>
        </div>

        {/* Total print time */}
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <div className="flex items-center gap-2 text-slate-400">
            <Clock className="h-4 w-4" />
            <span className="font-mono text-xs">Total Print Time</span>
          </div>
          <p className="mt-2 text-3xl font-bold text-slate-900 dark:text-white">{formatTime(stats.totalPrintTimeSeconds)}</p>
          <p className="mt-2 font-mono text-xs text-slate-400">
            Avg: {formatTime(stats.avgPrintTimeSeconds)} / print
          </p>
        </div>

        {/* Filament + cost */}
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <div className="flex items-center gap-2 text-slate-400">
            <DollarSign className="h-4 w-4" />
            <span className="font-mono text-xs">Filament Used</span>
          </div>
          <p className="mt-2 text-3xl font-bold text-slate-900 dark:text-white">{formatWeight(stats.totalFilamentUsedG)}</p>
          <p className="mt-2 font-mono text-xs text-slate-400">
            Est. cost: {formatCost(stats.totalEstimatedCost)}
          </p>
        </div>
      </div>

      {/* Prints over time chart */}
      {recentDays.length > 0 && (
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Prints per day</h3>
          <div className="flex items-end gap-1.5" style={{ height: '120px' }}>
            {recentDays.map(([day, count]) => (
              <div key={day} className="flex flex-1 flex-col items-center gap-1">
                <div
                  className="w-full rounded-t bg-blue-500 transition-all hover:bg-blue-400"
                  style={{ height: `${(count / maxDayCount) * 100}%`, minHeight: count > 0 ? '4px' : '0' }}
                  title={`${day}: ${count} prints`}
                />
                <span className="font-mono text-[8px] text-slate-400">{day.slice(5)}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Top files */}
        {topFiles.length > 0 && (
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <h3 className="mb-3 font-semibold text-slate-900 dark:text-white">Most printed files</h3>
            <div className="space-y-2">
              {topFiles.map(([file, count], idx) => (
                <div key={file} className="flex items-center gap-3">
                  <span className="font-mono text-xs text-slate-400 w-5">{idx + 1}</span>
                  <span className="flex-1 truncate text-sm text-slate-700 dark:text-slate-300" title={file}>{file}</span>
                  <span className="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                    {count}x
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Printer usage */}
        {topPrinters.length > 0 && (
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <h3 className="mb-3 font-semibold text-slate-900 dark:text-white">Prints per printer</h3>
            <div className="space-y-2">
              {topPrinters.map(([printer, count]) => {
                const pct = (count / stats.totalPrints) * 100
                return (
                  <div key={printer} className="flex items-center gap-3">
                    <span className="w-24 truncate text-sm text-slate-700 dark:text-slate-300">{printer}</span>
                    <div className="h-4 flex-1 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
                      <div className="h-full rounded-full bg-purple-500" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="font-mono text-xs text-slate-400 w-8 text-right">{count}</span>
                  </div>
                )
              })}
            </div>
          </div>
        )}

        {/* Filament by type */}
        {Object.keys(stats.filamentByType || {}).length > 0 && (
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <h3 className="mb-3 flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
              <Layers className="h-4 w-4 text-orange-500" /> Filament by type
            </h3>
            <div className="space-y-2">
              {Object.entries(stats.filamentByType || {})
                .sort(([, a], [, b]) => b - a)
                .map(([type, grams]) => (
                  <div key={type} className="flex items-center justify-between">
                    <span className="text-sm text-slate-700 dark:text-slate-300">{type}</span>
                    <span className="font-mono text-xs text-slate-400">{formatWeight(grams)}</span>
                  </div>
                ))}
            </div>
          </div>
        )}

        {/* Recent failures */}
        {(stats.recentFailures?.length ?? 0) > 0 && (
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <h3 className="mb-3 flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
              <AlertTriangle className="h-4 w-4 text-rose-500" /> Recent failures
            </h3>
            <div className="space-y-2">
              {stats.recentFailures.map((p) => (
                <div key={p.id} className="flex items-center gap-2 text-sm">
                  <XCircle className="h-3 w-3 flex-shrink-0 text-rose-500" />
                  <span className="flex-1 truncate text-slate-700 dark:text-slate-300" title={p.file}>{p.file}</span>
                  <span className="font-mono text-xs text-slate-400">{p.duration}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Longest prints */}
        {(stats.longestPrints?.length ?? 0) > 0 && (
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
            <h3 className="mb-3 font-semibold text-slate-900 dark:text-white">Longest prints</h3>
            <div className="space-y-2">
              {stats.longestPrints.map((p) => (
                <div key={p.id} className="flex items-center gap-2 text-sm">
                  <Clock className="h-3 w-3 flex-shrink-0 text-slate-400" />
                  <span className="flex-1 truncate text-slate-700 dark:text-slate-300" title={p.file}>{p.file}</span>
                  <span className="font-mono text-xs text-slate-400">{p.duration}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
