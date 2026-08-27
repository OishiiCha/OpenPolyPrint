import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  ArrowLeft, Plus, Trash2, Edit3, Wrench, Clock,
  CheckCircle2, AlertTriangle, Loader2, Thermometer,
} from 'lucide-react'

interface Profile {
  id: string
  name: string
  filamentType: string
  nozzleTemp: number
  bedTemp: number
  fanSpeed: number
  printSpeed: number
  retraction: number
  notes: string
  createdAt: number
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

export function Profiles() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [reminders, setReminders] = useState<ReminderStatus[]>([])
  const [loading, setLoading] = useState(true)
  const [editingProfile, setEditingProfile] = useState<Profile | null>(null)
  const [showProfileForm, setShowProfileForm] = useState(false)
  const [editingReminder, setEditingReminder] = useState<Reminder | null>(null)
  const [showReminderForm, setShowReminderForm] = useState(false)

  const fetchAll = useCallback(() => {
    Promise.all([
      fetch('/api/profiles').then((r) => r.json()),
      fetch('/api/maintenance').then((r) => r.json()),
    ]).then(([p, r]) => {
      if (Array.isArray(p)) setProfiles(p)
      if (Array.isArray(r)) setReminders(r)
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [])

  useEffect(() => { fetchAll() }, [fetchAll])

  const saveProfile = async (p: Profile) => {
    if (p.id) {
      await fetch(`/api/profiles/${p.id}`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(p),
      })
    } else {
      await fetch('/api/profiles', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(p),
      })
    }
    setShowProfileForm(false)
    setEditingProfile(null)
    fetchAll()
  }

  const deleteProfile = async (id: string) => {
    await fetch(`/api/profiles/${id}`, { method: 'DELETE' })
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
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Profiles & Maintenance</h1>
      </div>

      {/* Print Profiles section */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="flex items-center gap-2 font-semibold text-slate-900 dark:text-white">
            <Thermometer className="h-5 w-5 text-orange-500" /> Print Profiles
          </h2>
          <button
            onClick={() => { setEditingProfile(null); setShowProfileForm(true) }}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-500"
          >
            <Plus className="h-4 w-4" /> New profile
          </button>
        </div>

        {profiles.length === 0 ? (
          <p className="font-mono text-sm text-slate-400 py-8 text-center">
            No profiles yet. Create temperature/speed presets for your filament types.
          </p>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {profiles.map((p) => (
              <div key={p.id} className="rounded-lg border border-slate-200 p-3 dark:border-slate-700">
                <div className="flex items-start justify-between">
                  <div>
                    <h3 className="font-medium text-slate-900 dark:text-white">{p.name}</h3>
                    <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-400">
                      {p.filamentType}
                    </span>
                  </div>
                  <div className="flex gap-1">
                    <button
                      onClick={() => { setEditingProfile(p); setShowProfileForm(true) }}
                      className="rounded p-1 text-slate-400 hover:text-blue-500"
                    >
                      <Edit3 className="h-3.5 w-3.5" />
                    </button>
                    <button
                      onClick={() => deleteProfile(p.id)}
                      className="rounded p-1 text-slate-400 hover:text-rose-500"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
                <div className="mt-3 grid grid-cols-2 gap-2 font-mono text-xs">
                  <div>
                    <span className="text-slate-400">Nozzle</span>
                    <p className="text-slate-700 dark:text-slate-300">{p.nozzleTemp}°C</p>
                  </div>
                  <div>
                    <span className="text-slate-400">Bed</span>
                    <p className="text-slate-700 dark:text-slate-300">{p.bedTemp}°C</p>
                  </div>
                  {p.fanSpeed >= 0 && (
                    <div>
                      <span className="text-slate-400">Fan</span>
                      <p className="text-slate-700 dark:text-slate-300">{p.fanSpeed}%</p>
                    </div>
                  )}
                  {p.printSpeed > 0 && (
                    <div>
                      <span className="text-slate-400">Speed</span>
                      <p className="text-slate-700 dark:text-slate-300">{p.printSpeed} mm/s</p>
                    </div>
                  )}
                  {p.retraction > 0 && (
                    <div>
                      <span className="text-slate-400">Retraction</span>
                      <p className="text-slate-700 dark:text-slate-300">{p.retraction} mm</p>
                    </div>
                  )}
                </div>
                {p.notes && (
                  <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">{p.notes}</p>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Maintenance section */}
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

      {/* Profile form modal */}
      {showProfileForm && (
        <ProfileForm
          profile={editingProfile}
          onSave={saveProfile}
          onCancel={() => { setShowProfileForm(false); setEditingProfile(null) }}
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

function ProfileForm({ profile, onSave, onCancel }: {
  profile: Profile | null
  onSave: (p: Profile) => void
  onCancel: () => void
}) {
  const [form, setForm] = useState<Profile>(profile || {
    id: '', name: '', filamentType: 'PLA', nozzleTemp: 210, bedTemp: 60,
    fanSpeed: 100, printSpeed: 0, retraction: 0, notes: '', createdAt: 0,
  })

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onCancel}>
      <div className="w-full max-w-md rounded-xl border border-slate-700 bg-slate-950 p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 font-semibold text-white">{profile ? 'Edit profile' : 'New profile'}</h2>
        <div className="space-y-3">
          <input className={inputClass} placeholder="Name *" value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <select className={inputClass} value={form.filamentType}
            onChange={(e) => setForm({ ...form, filamentType: e.target.value })}>
            {['PLA', 'PETG', 'ABS', 'TPU', 'ASA', 'PC', 'Nylon', 'Other'].map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-slate-400">Nozzle temp (°C)</label>
              <input type="number" className={inputClass} value={form.nozzleTemp}
                onChange={(e) => setForm({ ...form, nozzleTemp: +e.target.value })} />
            </div>
            <div>
              <label className="text-xs text-slate-400">Bed temp (°C)</label>
              <input type="number" className={inputClass} value={form.bedTemp}
                onChange={(e) => setForm({ ...form, bedTemp: +e.target.value })} />
            </div>
            <div>
              <label className="text-xs text-slate-400">Fan speed (%)</label>
              <input type="number" className={inputClass} value={form.fanSpeed}
                onChange={(e) => setForm({ ...form, fanSpeed: +e.target.value })} />
            </div>
            <div>
              <label className="text-xs text-slate-400">Print speed (mm/s)</label>
              <input type="number" className={inputClass} value={form.printSpeed}
                onChange={(e) => setForm({ ...form, printSpeed: +e.target.value })} />
            </div>
          </div>
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
