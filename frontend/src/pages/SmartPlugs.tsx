import { useEffect, useState, useCallback } from 'react'
import { Plus, Trash2, Power, X, Plug as PlugIcon } from 'lucide-react'
import { usePrinters } from '../hooks/usePrinters'

interface Plug {
  id: string
  name: string
  type: string
  host: string
  port: number
  password: string
  on: boolean
  printerId: string
  autoOff: boolean
}

const emptyPlug: Omit<Plug, 'id' | 'on'> = {
  name: '',
  type: 'tasmota',
  host: '',
  port: 80,
  password: '',
  printerId: '',
  autoOff: false,
}

export function SmartPlugs() {
  const { printers } = usePrinters()
  const [plugs, setPlugs] = useState<Plug[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState(emptyPlug)
  const [busy, setBusy] = useState<string | null>(null)

  const fetchPlugs = useCallback(async () => {
    try {
      const res = await fetch('/api/plugs')
      if (!res.ok) return
      const data = await res.json()
      setPlugs(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error(e)
    }
  }, [])

  useEffect(() => {
    fetchPlugs()
    const id = setInterval(fetchPlugs, 5000)
    return () => clearInterval(id)
  }, [fetchPlugs])

  const add = async () => {
    try {
      await fetch('/api/plugs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      setShowAdd(false)
      setForm(emptyPlug)
      fetchPlugs()
    } catch (e) {
      console.error(e)
    }
  }

  const remove = async (id: string) => {
    try {
      await fetch(`/api/plugs/${encodeURIComponent(id)}`, { method: 'DELETE' })
      fetchPlugs()
    } catch (e) {
      console.error(e)
    }
  }

  const toggle = async (plug: Plug) => {
    setBusy(plug.id)
    try {
      await fetch(`/api/plugs/${encodeURIComponent(plug.id)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ on: !plug.on }),
      })
      fetchPlugs()
    } catch (e) {
      console.error(e)
    } finally {
      setBusy(null)
    }
  }

  const printerName = (id: string) => printers.find((p) => p.id === id)?.name || 'Unassigned'

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <PlugIcon className="h-6 w-6 text-blue-500" />
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Smart Plugs</h1>
        </div>
        <button
          onClick={() => { setForm(emptyPlug); setShowAdd(true) }}
          className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
        >
          <Plus className="h-4 w-4" /> Add plug
        </button>
      </div>

      <p className="font-mono text-xs text-slate-500 dark:text-slate-400">
        Control power to your printers via TP-Link Kasa, Shelly, or Tasmota smart plugs.
        Enable "Auto-off" to automatically turn off the plug 60 seconds after a print completes.
      </p>

      {plugs.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
          <PlugIcon className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-700" />
          <p className="font-mono text-sm text-slate-400">No smart plugs configured. Add one to control printer power.</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {plugs.map((plug) => (
            <div key={plug.id} className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
              <div className="flex items-start justify-between">
                <div>
                  <p className="font-mono text-sm font-medium text-slate-900 dark:text-white">{plug.name}</p>
                  <p className="font-mono text-xs text-slate-500 dark:text-slate-400">
                    {plug.type} · {plug.host}:{plug.port}
                  </p>
                  <p className="mt-1 font-mono text-xs text-slate-400">
                    Printer: {printerName(plug.printerId)}
                  </p>
                </div>
                <button
                  onClick={() => remove(plug.id)}
                  className="rounded-lg p-1.5 text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-900/20"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>

              {plug.autoOff && (
                <div className="mt-2 inline-block rounded-full bg-amber-100 px-2 py-0.5 font-mono text-[10px] text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
                  Auto-off after print
                </div>
              )}

              <button
                onClick={() => toggle(plug)}
                disabled={busy === plug.id}
                className={`mt-3 flex w-full items-center justify-center gap-2 rounded-lg py-3 font-medium transition-colors disabled:opacity-50 ${
                  plug.on
                    ? 'bg-emerald-600 text-white hover:bg-emerald-500'
                    : 'bg-slate-200 text-slate-700 hover:bg-slate-300 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700'
                }`}
              >
                <Power className="h-5 w-5" />
                {plug.on ? 'ON' : 'OFF'}
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Add modal */}
      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-md rounded-2xl bg-white p-6 dark:bg-slate-900" onClick={(e) => e.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-slate-900 dark:text-white">Add smart plug</h2>
              <button onClick={() => setShowAdd(false)} className="text-slate-400 hover:text-slate-900 dark:hover:text-white">
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Name</label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="e.g. Printer Power"
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                />
              </div>
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Type</label>
                <select
                  value={form.type}
                  onChange={(e) => setForm({ ...form, type: e.target.value })}
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                >
                  <option value="tasmota">Tasmota</option>
                  <option value="shelly">Shelly</option>
                  <option value="tplink">TP-Link Kasa</option>
                </select>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Host / IP</label>
                  <input
                    type="text"
                    value={form.host}
                    onChange={(e) => setForm({ ...form, host: e.target.value })}
                    placeholder="192.168.1.100"
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                  />
                </div>
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Port</label>
                  <input
                    type="number"
                    value={form.port}
                    onChange={(e) => setForm({ ...form, port: parseInt(e.target.value) || 0 })}
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                  />
                </div>
              </div>
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Password (optional)</label>
                <input
                  type="password"
                  value={form.password}
                  onChange={(e) => setForm({ ...form, password: e.target.value })}
                  placeholder="For Tasmota/Shelly auth"
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                />
              </div>
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Associated printer</label>
                <select
                  value={form.printerId}
                  onChange={(e) => setForm({ ...form, printerId: e.target.value })}
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                >
                  <option value="">None</option>
                  {printers.map((p) => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </select>
              </div>
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input
                  type="checkbox"
                  checked={form.autoOff}
                  onChange={(e) => setForm({ ...form, autoOff: e.target.checked })}
                  className="h-4 w-4 rounded border-slate-300 text-blue-600"
                />
                Auto-off after print completes (60s delay)
              </label>
              <button
                onClick={add}
                disabled={!form.name || !form.host}
                className="w-full rounded-lg bg-blue-600 py-2.5 font-medium text-white hover:bg-blue-500 disabled:opacity-50"
              >
                Add plug
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
