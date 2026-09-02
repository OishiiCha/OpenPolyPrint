import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  ArrowLeft, Plus, Trash2, Edit3, Wrench, Clock,
  CheckCircle2, AlertTriangle, Loader2, Package,
  DollarSign, TrendingDown, ExternalLink, Minus, Plus as PlusIcon,
} from 'lucide-react'

interface Part {
  id: string
  name: string
  category: string
  printerModel: string
  stock: number
  minStock: number
  unitPrice: number
  currency: string
  supplier: string
  supplierUrl: string
  notes: string
  updatedAt: number
}

interface Reminder {
  id: string
  printerId: string
  printerName: string
  task: string
  intervalHours: number
  lastPerformed: number
  notes: string
}

interface ReminderStatus extends Reminder {
  hoursSince: number
  hoursUntilDue: number
  isDue: boolean
}

const inputClass = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'

const CATEGORIES = ['Belt', 'Nozzle', 'Fan', 'Filter', 'Lubricant', 'Sensor', 'Board', 'PSU', 'Extruder', 'Other']
const CURRENCIES = ['USD', 'EUR', 'GBP', 'CAD', 'AUD']

export function Maintenance() {
  const [parts, setParts] = useState<Part[]>([])
  const [reminders, setReminders] = useState<ReminderStatus[]>([])
  const [totalValue, setTotalValue] = useState(0)
  const [loading, setLoading] = useState(true)
  const [editingPart, setEditingPart] = useState<Part | null>(null)
  const [showPartForm, setShowPartForm] = useState(false)
  const [editingReminder, setEditingReminder] = useState<Reminder | null>(null)
  const [showReminderForm, setShowReminderForm] = useState(false)

  const fetchAll = useCallback(() => {
    Promise.all([
      fetch('/api/parts').then((r) => r.json()),
      fetch('/api/maintenance').then((r) => r.json()),
      fetch('/api/parts/value').then((r) => r.json()),
    ]).then(([p, r, v]) => {
      if (Array.isArray(p)) setParts(p)
      if (Array.isArray(r)) setReminders(r)
      if (v?.totalValue) setTotalValue(v.totalValue)
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [])

  useEffect(() => { fetchAll() }, [fetchAll])

  const savePart = async (p: Part) => {
    if (p.id) {
      await fetch(`/api/parts/${p.id}`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(p),
      })
    } else {
      await fetch('/api/parts', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(p),
      })
    }
    setShowPartForm(false)
    setEditingPart(null)
    fetchAll()
  }

  const deletePart = async (id: string) => {
    if (!confirm('Delete this part?')) return
    await fetch(`/api/parts/${id}`, { method: 'DELETE' })
    fetchAll()
  }

  const adjustStock = async (id: string, delta: number) => {
    await fetch(`/api/parts/${id}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ delta }),
    })
    fetchAll()
  }

  const saveReminder = async (r: Reminder) => {
    if (r.id) {
      await fetch(`/api/maintenance/${r.id}`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(r),
      })
    } else {
      await fetch('/api/maintenance', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(r),
      })
    }
    setShowReminderForm(false)
    setEditingReminder(null)
    fetchAll()
  }

  const deleteReminder = async (id: string) => {
    await fetch(`/api/maintenance/${id}`, { method: 'DELETE' })
    fetchAll()
  }

  const markPerformed = async (id: string) => {
    await fetch(`/api/maintenance/${id}`, { method: 'POST' })
    fetchAll()
  }

  // Group parts by category
  const grouped = parts.reduce((acc, p) => {
    const cat = p.category || 'Other'
    if (!acc[cat]) acc[cat] = []
    acc[cat].push(p)
    return acc
  }, {} as Record<string, Part[]>)

  const lowStockCount = parts.filter(p => p.stock <= p.minStock).length
  const dueCount = reminders.filter(r => r.isDue).length

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/" className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white">
          <ArrowLeft className="h-4 w-4" /> Back
        </Link>
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Maintenance</h1>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="rounded-xl border border-slate-200 p-3 dark:border-slate-800">
          <div className="flex items-center gap-2">
            <Package className="h-4 w-4 text-blue-500" />
            <span className="text-xs text-slate-500 dark:text-slate-400">Parts</span>
          </div>
          <p className="mt-1 text-lg font-bold text-slate-900 dark:text-white">{parts.length}</p>
        </div>
        <div className="rounded-xl border border-slate-200 p-3 dark:border-slate-800">
          <div className="flex items-center gap-2">
            <TrendingDown className="h-4 w-4 text-amber-500" />
            <span className="text-xs text-slate-500 dark:text-slate-400">Low stock</span>
          </div>
          <p className="mt-1 text-lg font-bold text-amber-600 dark:text-amber-400">{lowStockCount}</p>
        </div>
        <div className="rounded-xl border border-slate-200 p-3 dark:border-slate-800">
          <div className="flex items-center gap-2">
            <DollarSign className="h-4 w-4 text-emerald-500" />
            <span className="text-xs text-slate-500 dark:text-slate-400">Inventory value</span>
          </div>
          <p className="mt-1 text-lg font-bold text-emerald-600 dark:text-emerald-400">
            ${totalValue.toFixed(2)}
          </p>
        </div>
        <div className="rounded-xl border border-slate-200 p-3 dark:border-slate-800">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-rose-500" />
            <span className="text-xs text-slate-500 dark:text-slate-400">Due now</span>
          </div>
          <p className="mt-1 text-lg font-bold text-rose-600 dark:text-rose-400">{dueCount}</p>
        </div>
      </div>

      {/* Parts Inventory */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
            <Package className="h-5 w-5 text-blue-500" /> Parts Inventory
          </h2>
          <button
            onClick={() => { setEditingPart(null); setShowPartForm(true) }}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-500"
          >
            <Plus className="h-4 w-4" /> Add part
          </button>
        </div>

        {parts.length === 0 ? (
          <p className="font-mono text-sm text-slate-400 py-8 text-center">
            No parts in inventory. Add belts, nozzles, fans, and other consumables to track stock and costs.
          </p>
        ) : (
          <div className="space-y-4">
            {Object.entries(grouped).sort(([a], [b]) => a.localeCompare(b)).map(([category, catParts]) => (
              <div key={category}>
                <h3 className="mb-2 flex items-center gap-2 text-sm font-medium text-slate-600 dark:text-slate-400">
                  {category}
                  <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500 dark:bg-slate-800">
                    {catParts.length}
                  </span>
                </h3>
                <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  {catParts.map((p) => {
                    const lowStock = p.stock <= p.minStock
                    return (
                      <div
                        key={p.id}
                        className={`rounded-lg border p-3 ${
                          lowStock
                            ? 'border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/20'
                            : 'border-slate-200 dark:border-slate-700'
                        }`}
                      >
                        <div className="flex items-start justify-between">
                          <div className="flex-1 min-w-0">
                            <h4 className="font-medium text-slate-900 dark:text-white truncate">{p.name}</h4>
                            <div className="mt-0.5 flex flex-wrap gap-1">
                              {p.printerModel && (
                                <span className="rounded-full bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500 dark:bg-slate-800 dark:text-slate-400">
                                  {p.printerModel}
                                </span>
                              )}
                              {lowStock && (
                                <span className="rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900 dark:text-amber-300">
                                  Low stock
                                </span>
                              )}
                            </div>
                          </div>
                          <div className="flex gap-1">
                            <button
                              onClick={() => { setEditingPart(p); setShowPartForm(true) }}
                              className="rounded p-1 text-slate-400 hover:text-blue-500"
                            >
                              <Edit3 className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => deletePart(p.id)}
                              className="rounded p-1 text-slate-400 hover:text-rose-500"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        </div>

                        {/* Stock controls */}
                        <div className="mt-3 flex items-center gap-2">
                          <button
                            onClick={() => adjustStock(p.id, -1)}
                            className="rounded-lg border border-slate-300 p-1 text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                            title="Decrease stock"
                          >
                            <Minus className="h-3.5 w-3.5" />
                          </button>
                          <div className="flex-1 text-center">
                            <p className={`font-mono text-lg font-bold ${lowStock ? 'text-amber-600 dark:text-amber-400' : 'text-slate-900 dark:text-white'}`}>
                              {p.stock}
                            </p>
                            <p className="text-[10px] text-slate-400">in stock (min: {p.minStock})</p>
                          </div>
                          <button
                            onClick={() => adjustStock(p.id, 1)}
                            className="rounded-lg border border-slate-300 p-1 text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                            title="Increase stock"
                          >
                            <PlusIcon className="h-3.5 w-3.5" />
                          </button>
                        </div>

                        {/* Price + supplier */}
                        <div className="mt-2 flex items-center justify-between border-t border-slate-200 pt-2 dark:border-slate-700">
                          <span className="font-mono text-sm font-semibold text-emerald-600 dark:text-emerald-400">
                            {p.currency || 'USD'} {p.unitPrice.toFixed(2)}
                          </span>
                          {p.supplierUrl ? (
                            <a
                              href={p.supplierUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="flex items-center gap-1 text-xs text-blue-500 hover:text-blue-400"
                            >
                              {p.supplier || 'Buy'} <ExternalLink className="h-3 w-3" />
                            </a>
                          ) : p.supplier ? (
                            <span className="text-xs text-slate-400">{p.supplier}</span>
                          ) : null}
                        </div>

                        {p.notes && (
                          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{p.notes}</p>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Maintenance Reminders */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
            <Wrench className="h-5 w-5 text-amber-500" /> Maintenance Reminders
          </h2>
          <button
            onClick={() => { setEditingReminder(null); setShowReminderForm(true) }}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-500"
          >
            <Plus className="h-4 w-4" /> New reminder
          </button>
        </div>

        {reminders.length === 0 ? (
          <p className="font-mono text-sm text-slate-400 py-8 text-center">
            No maintenance reminders. Track when to lubricate, check belts, replace nozzles, etc.
          </p>
        ) : (
          <div className="space-y-2">
            {reminders.map((r) => (
              <div
                key={r.id}
                className={`flex items-center gap-3 rounded-lg border p-3 ${
                  r.isDue
                    ? 'border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/20'
                    : 'border-slate-200 dark:border-slate-700'
                }`}
              >
                <div className="flex-shrink-0">
                  {r.isDue ? (
                    <AlertTriangle className="h-5 w-5 text-amber-500" />
                  ) : (
                    <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                  )}
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-slate-900 dark:text-white">{r.task}</span>
                    <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500 dark:bg-slate-800">
                      {r.printerName || 'Any printer'}
                    </span>
                  </div>
                  <div className="mt-1 flex items-center gap-3 font-mono text-xs text-slate-400">
                    <span className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      Every {r.intervalHours}h
                    </span>
                    {r.isDue ? (
                      <span className="text-amber-600 dark:text-amber-400">
                        Overdue ({Math.abs(r.hoursUntilDue).toFixed(1)}h past)
                      </span>
                    ) : (
                      <span className="text-emerald-600 dark:text-emerald-400">
                        {r.hoursUntilDue.toFixed(1)}h remaining
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex gap-1">
                  <button
                    onClick={() => markPerformed(r.id)}
                    className="rounded-lg border border-slate-300 px-2 py-1 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                    title="Mark as performed"
                  >
                    Done
                  </button>
                  <button
                    onClick={() => { setEditingReminder(r); setShowReminderForm(true) }}
                    className="rounded p-1 text-slate-400 hover:text-blue-500"
                  >
                    <Edit3 className="h-3.5 w-3.5" />
                  </button>
                  <button
                    onClick={() => deleteReminder(r.id)}
                    className="rounded p-1 text-slate-400 hover:text-rose-500"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Part form modal */}
      {showPartForm && (
        <PartForm
          part={editingPart}
          onSave={savePart}
          onCancel={() => { setShowPartForm(false); setEditingPart(null) }}
        />
      )}

      {/* Reminder form modal */}
      {showReminderForm && (
        <ReminderForm
          reminder={editingReminder}
          onSave={saveReminder}
          onCancel={() => { setShowReminderForm(false); setEditingReminder(null) }}
        />
      )}
    </div>
  )
}

function PartForm({ part, onSave, onCancel }: {
  part: Part | null
  onSave: (p: Part) => void
  onCancel: () => void
}) {
  const [form, setForm] = useState<Part>(part || {
    id: '', name: '', category: 'Belt', printerModel: '', stock: 0, minStock: 1,
    unitPrice: 0, currency: 'USD', supplier: '', supplierUrl: '', notes: '', updatedAt: 0,
  })

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onCancel}>
      <div className="w-full max-w-md rounded-xl border border-slate-700 bg-slate-950 p-6 max-h-[90vh] overflow-y-auto" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 font-semibold text-white">{part ? 'Edit part' : 'Add part'}</h2>
        <div className="space-y-3">
          <input className={inputClass} placeholder="Name * (e.g. GT2 Belt 6mm)" value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-slate-400">Category</label>
              <select className={inputClass} value={form.category}
                onChange={(e) => setForm({ ...form, category: e.target.value })}>
                {CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-400">Printer model</label>
              <input className={inputClass} placeholder="M5C, M5, Universal" value={form.printerModel}
                onChange={(e) => setForm({ ...form, printerModel: e.target.value })} />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="text-xs text-slate-400">Stock</label>
              <input type="number" className={inputClass} value={form.stock}
                onChange={(e) => setForm({ ...form, stock: +e.target.value })} />
            </div>
            <div>
              <label className="text-xs text-slate-400">Min stock</label>
              <input type="number" className={inputClass} value={form.minStock}
                onChange={(e) => setForm({ ...form, minStock: +e.target.value })} />
            </div>
            <div>
              <label className="text-xs text-slate-400">Unit price</label>
              <input type="number" step="0.01" className={inputClass} value={form.unitPrice}
                onChange={(e) => setForm({ ...form, unitPrice: +e.target.value })} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-slate-400">Currency</label>
              <select className={inputClass} value={form.currency}
                onChange={(e) => setForm({ ...form, currency: e.target.value })}>
                {CURRENCIES.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-400">Supplier</label>
              <input className={inputClass} placeholder="Amazon, AliExpress..." value={form.supplier}
                onChange={(e) => setForm({ ...form, supplier: e.target.value })} />
            </div>
          </div>
          <input className={inputClass} placeholder="Supplier URL (optional)" value={form.supplierUrl}
            onChange={(e) => setForm({ ...form, supplierUrl: e.target.value })} />
          <input className={inputClass} placeholder="Notes (optional)" value={form.notes}
            onChange={(e) => setForm({ ...form, notes: e.target.value })} />
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <button onClick={onCancel} className="rounded-lg bg-slate-800 px-4 py-2 text-sm text-slate-300 hover:bg-slate-700">Cancel</button>
          <button onClick={() => onSave(form)} disabled={!form.name}
            className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50">
            Save
          </button>
        </div>
      </div>
    </div>
  )
}

function ReminderForm({ reminder, onSave, onCancel }: {
  reminder: Reminder | null
  onSave: (r: Reminder) => void
  onCancel: () => void
}) {
  const [form, setForm] = useState<Reminder>(reminder || {
    id: '', printerId: '', printerName: '', task: '', intervalHours: 100, lastPerformed: 0, notes: '',
  })

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onCancel}>
      <div className="w-full max-w-md rounded-xl border border-slate-700 bg-slate-950 p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 font-semibold text-white">{reminder ? 'Edit reminder' : 'New reminder'}</h2>
        <div className="space-y-3">
          <input className={inputClass} placeholder="Task * (e.g. Lubricate rods)" value={form.task}
            onChange={(e) => setForm({ ...form, task: e.target.value })} />
          <input className={inputClass} placeholder="Printer name (optional)" value={form.printerName}
            onChange={(e) => setForm({ ...form, printerName: e.target.value })} />
          <div>
            <label className="text-xs text-slate-400">Interval (print hours)</label>
            <input type="number" className={inputClass} value={form.intervalHours}
              onChange={(e) => setForm({ ...form, intervalHours: +e.target.value })} />
          </div>
          <input className={inputClass} placeholder="Notes (optional)" value={form.notes}
            onChange={(e) => setForm({ ...form, notes: e.target.value })} />
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <button onClick={onCancel} className="rounded-lg bg-slate-800 px-4 py-2 text-sm text-slate-300 hover:bg-slate-700">Cancel</button>
          <button onClick={() => onSave(form)} disabled={!form.task}
            className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50">
            Save
          </button>
        </div>
      </div>
    </div>
  )
}
