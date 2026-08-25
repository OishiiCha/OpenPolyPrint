import { useEffect, useState, useCallback } from 'react'
import { Plus, Trash2, Edit3, X, Package } from 'lucide-react'

interface Spool {
  id: string
  brand: string
  type: string
  color: string
  colorName: string
  weightG: number
  remainingG: number
  diameter: number
  cost: number
  addedAt: number
}

const emptySpool: Omit<Spool, 'id' | 'addedAt'> = {
  brand: '',
  type: 'PLA',
  color: '#22c55e',
  colorName: '',
  weightG: 1000,
  remainingG: 1000,
  diameter: 1.75,
  cost: 0,
}

export function Filament() {
  const [spools, setSpools] = useState<Spool[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [editing, setEditing] = useState<Spool | null>(null)
  const [form, setForm] = useState(emptySpool)

  const fetchSpools = useCallback(async () => {
    try {
      const res = await fetch('/api/filament')
      if (!res.ok) return
      const data = await res.json()
      setSpools(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error(e)
    }
  }, [])

  useEffect(() => {
    fetchSpools()
  }, [fetchSpools])

  const save = async () => {
    try {
      if (editing) {
        await fetch(`/api/filament/${encodeURIComponent(editing.id)}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(form),
        })
      } else {
        await fetch('/api/filament', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(form),
        })
      }
      setShowAdd(false)
      setEditing(null)
      setForm(emptySpool)
      fetchSpools()
    } catch (e) {
      console.error(e)
    }
  }

  const remove = async (id: string) => {
    try {
      await fetch(`/api/filament/${encodeURIComponent(id)}`, { method: 'DELETE' })
      fetchSpools()
    } catch (e) {
      console.error(e)
    }
  }

  const startEdit = (spool: Spool) => {
    setEditing(spool)
    setForm({
      brand: spool.brand,
      type: spool.type,
      color: spool.color,
      colorName: spool.colorName,
      weightG: spool.weightG,
      remainingG: spool.remainingG,
      diameter: spool.diameter,
      cost: spool.cost,
    })
    setShowAdd(true)
  }

  const pct = (s: Spool) => s.weightG > 0 ? Math.round((s.remainingG / s.weightG) * 100) : 0
  const totalRemaining = spools.reduce((sum, s) => sum + s.remainingG, 0)
  const lowSpools = spools.filter((s) => pct(s) < 20)

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Package className="h-6 w-6 text-blue-500" />
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Filament</h1>
        </div>
        <button
          onClick={() => { setEditing(null); setForm(emptySpool); setShowAdd(true) }}
          className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
        >
          <Plus className="h-4 w-4" /> Add spool
        </button>
      </div>

      {/* Summary */}
      <div className="grid grid-cols-3 gap-4">
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <p className="font-mono text-xs text-slate-500 dark:text-slate-400">Total spools</p>
          <p className="mt-1 text-2xl font-bold text-slate-900 dark:text-white">{spools.length}</p>
        </div>
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <p className="font-mono text-xs text-slate-500 dark:text-slate-400">Remaining</p>
          <p className="mt-1 text-2xl font-bold text-slate-900 dark:text-white">{Math.round(totalRemaining)}g</p>
        </div>
        <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
          <p className="font-mono text-xs text-slate-500 dark:text-slate-400">Low stock</p>
          <p className={`mt-1 text-2xl font-bold ${lowSpools.length > 0 ? 'text-amber-500' : 'text-slate-900 dark:text-white'}`}>
            {lowSpools.length}
          </p>
        </div>
      </div>

      {/* Spool grid */}
      {spools.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
          <Package className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-700" />
          <p className="font-mono text-sm text-slate-400">No filament in inventory. Add a spool to get started.</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {spools.map((spool) => (
            <div key={spool.id} className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div
                    className="h-10 w-10 rounded-full border-2 border-slate-300 dark:border-slate-700"
                    style={{ backgroundColor: spool.color }}
                  />
                  <div>
                    <p className="font-mono text-sm font-medium text-slate-900 dark:text-white">
                      {spool.brand || 'Unknown'} {spool.type}
                    </p>
                    <p className="font-mono text-xs text-slate-500 dark:text-slate-400">
                      {spool.colorName || spool.color} · {spool.diameter}mm
                    </p>
                  </div>
                </div>
                <div className="flex gap-1">
                  <button
                    onClick={() => startEdit(spool)}
                    className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-900 dark:hover:bg-slate-800"
                  >
                    <Edit3 className="h-4 w-4" />
                  </button>
                  <button
                    onClick={() => remove(spool.id)}
                    className="rounded-lg p-1.5 text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-900/20"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>

              {/* Remaining bar */}
              <div className="mt-3">
                <div className="mb-1 flex justify-between font-mono text-xs">
                  <span className="text-slate-500 dark:text-slate-400">{Math.round(spool.remainingG)}g / {Math.round(spool.weightG)}g</span>
                  <span className={pct(spool) < 20 ? 'font-bold text-amber-500' : 'text-slate-500 dark:text-slate-400'}>{pct(spool)}%</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
                  <div
                    className="h-full transition-all"
                    style={{
                      width: `${pct(spool)}%`,
                      backgroundColor: spool.color,
                      opacity: pct(spool) < 20 ? 0.6 : 1,
                    }}
                  />
                </div>
              </div>

              {spool.cost > 0 && (
                <p className="mt-2 font-mono text-xs text-slate-400">
                  ${spool.cost.toFixed(2)} · ${(spool.cost / spool.weightG * 1000).toFixed(2)}/kg
                </p>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Add/Edit modal */}
      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-md rounded-2xl bg-white p-6 dark:bg-slate-900" onClick={(e) => e.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-slate-900 dark:text-white">{editing ? 'Edit spool' : 'Add spool'}</h2>
              <button onClick={() => setShowAdd(false)} className="text-slate-400 hover:text-slate-900 dark:hover:text-white">
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Brand</label>
                  <input
                    type="text"
                    value={form.brand}
                    onChange={(e) => setForm({ ...form, brand: e.target.value })}
                    placeholder="e.g. Polymaker"
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
                    {['PLA', 'PETG', 'ABS', 'ASA', 'TPU', 'Nylon', 'PC', 'PVA', 'HIPS'].map((t) => (
                      <option key={t} value={t}>{t}</option>
                    ))}
                  </select>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Color</label>
                  <input
                    type="color"
                    value={form.color}
                    onChange={(e) => setForm({ ...form, color: e.target.value })}
                    className="h-10 w-full rounded-lg border border-slate-300 dark:border-slate-700"
                  />
                </div>
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Color name</label>
                  <input
                    type="text"
                    value={form.colorName}
                    onChange={(e) => setForm({ ...form, colorName: e.target.value })}
                    placeholder="e.g. Lava Red"
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                  />
                </div>
              </div>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Total (g)</label>
                  <input
                    type="number"
                    value={form.weightG}
                    onChange={(e) => setForm({ ...form, weightG: parseFloat(e.target.value) || 0 })}
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                  />
                </div>
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Remaining (g)</label>
                  <input
                    type="number"
                    value={form.remainingG}
                    onChange={(e) => setForm({ ...form, remainingG: parseFloat(e.target.value) || 0 })}
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                  />
                </div>
                <div>
                  <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Diameter</label>
                  <select
                    value={form.diameter}
                    onChange={(e) => setForm({ ...form, diameter: parseFloat(e.target.value) })}
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                  >
                    <option value={1.75}>1.75mm</option>
                    <option value={2.85}>2.85mm</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Cost ($)</label>
                <input
                  type="number"
                  step="0.01"
                  value={form.cost}
                  onChange={(e) => setForm({ ...form, cost: parseFloat(e.target.value) || 0 })}
                  placeholder="0.00"
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                />
              </div>
              <button
                onClick={save}
                className="w-full rounded-lg bg-blue-600 py-2.5 font-medium text-white hover:bg-blue-500"
              >
                {editing ? 'Save changes' : 'Add spool'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
