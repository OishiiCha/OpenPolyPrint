import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, RotateCw } from 'lucide-react'
import { usePrinters } from '../hooks/usePrinters'

interface Corner {
  id: string
  label: string
  x: number
  y: number
  icon: string
}

const corners: Corner[] = [
  { id: 'fl', label: 'Front Left', x: 30, y: 180, icon: '↖' },
  { id: 'fr', label: 'Front Right', x: 180, y: 180, icon: '↘' },
  { id: 'bl', label: 'Back Left', x: 30, y: 30, icon: '↖' },
  { id: 'br', label: 'Back Right', x: 180, y: 30, icon: '↗' },
  { id: 'center', label: 'Center', x: 105, y: 105, icon: '✕' },
]

export function BedLeveling() {
  const { id } = useParams<{ id: string }>()
  const { printers } = usePrinters()
  const printer = printers.find((p) => p.id === id)
  const [activeCorner, setActiveCorner] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [step, setStep] = useState(0)

  if (!printer) {
    return (
      <div className="p-8 text-center text-slate-500">
        Printer not found. <Link to="/printers" className="text-blue-500">Back to printers</Link>
      </div>
    )
  }

  const sendGcode = async (command: string) => {
    setBusy(true)
    try {
      await fetch(`/api/printers/${printer.id}/gcode`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command }),
      })
    } catch (e) {
      console.error(e)
    } finally {
      setBusy(false)
    }
  }

  const moveToCorner = async (corner: Corner) => {
    setActiveCorner(corner.id)
    // Home, then move to corner position
    await sendGcode('G28\nG0 X' + corner.x + ' Y' + corner.y + ' F3000')
  }

  const steps = [
    { title: 'Preparation', desc: 'Clean the bed and ensure it is at room temperature. Heat the bed to your typical print temperature (e.g. 60°C for PLA).' },
    { title: 'Home the printer', desc: 'Homing ensures the printer knows its position before moving to leveling points.' },
    { title: 'Level each corner', desc: 'Tap each corner to move the nozzle there. Adjust the bed knobs until a sheet of paper slides with slight resistance.' },
    { title: 'Verify center', desc: 'Move to the center and verify the nozzle height is consistent. Re-check corners if needed.' },
    { title: 'Done!', desc: 'Your bed is leveled. You can now start printing.' },
  ]

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="flex items-center gap-4">
        <Link
          to={`/printers/${printer.id}`}
          className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white"
        >
          <ArrowLeft className="h-4 w-4" /> Back
        </Link>
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Bed Leveling — {printer.name}</h1>
      </div>

      {/* Step indicator */}
      <div className="flex items-center gap-2">
        {steps.map((_, i) => (
          <div
            key={i}
            className={`h-2 flex-1 rounded-full transition-colors ${
              i <= step ? 'bg-blue-600' : 'bg-slate-200 dark:bg-slate-800'
            }`}
          />
        ))}
      </div>

      {/* Current step description */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <h3 className="mb-1 font-semibold text-slate-900 dark:text-white">
          Step {step + 1}: {steps[step].title}
        </h3>
        <p className="text-sm text-slate-600 dark:text-slate-400">{steps[step].desc}</p>
      </div>

      {/* Bed visualization */}
      <div className="rounded-xl border border-slate-200 p-6 dark:border-slate-800">
        <div className="mx-auto" style={{ maxWidth: 280 }}>
          <div className="relative mx-auto rounded-lg border-2 border-slate-300 bg-slate-50 dark:border-slate-700 dark:bg-slate-900" style={{ width: 240, height: 240 }}>
            {/* Bed grid */}
            <div className="absolute inset-4 grid grid-cols-3 grid-rows-3 gap-0">
              {Array.from({ length: 9 }).map((_, i) => (
                <div key={i} className="border border-slate-200 dark:border-slate-800" />
              ))}
            </div>

            {/* Corner buttons */}
            {corners.map((corner) => (
              <button
                key={corner.id}
                onClick={() => moveToCorner(corner)}
                disabled={busy || step < 2}
                className={`absolute flex h-10 w-10 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border-2 text-sm font-bold transition-all disabled:opacity-40 ${
                  activeCorner === corner.id
                    ? 'border-blue-500 bg-blue-500 text-white scale-110'
                    : 'border-slate-400 bg-white text-slate-600 hover:border-blue-400 dark:bg-slate-800 dark:text-slate-300'
                }`}
                style={{ left: corner.x, top: corner.y }}
                title={corner.label}
              >
                {corner.icon}
              </button>
            ))}
          </div>

          {/* Active corner label */}
          {activeCorner && (
            <p className="mt-3 text-center font-mono text-sm text-blue-600 dark:text-blue-400">
              Moved to: {corners.find((c) => c.id === activeCorner)?.label}
            </p>
          )}
        </div>
      </div>

      {/* Action buttons */}
      <div className="flex gap-3">
        {step === 0 && (
          <button
            onClick={() => { setStep(1); sendGcode('M140 S60') }}
            disabled={busy}
            className="flex-1 rounded-lg bg-blue-600 py-3 font-medium text-white hover:bg-blue-500 disabled:opacity-50"
          >
            Heat bed to 60°C
          </button>
        )}
        {step === 1 && (
          <button
            onClick={() => { setStep(2); sendGcode('G28') }}
            disabled={busy}
            className="flex-1 rounded-lg bg-blue-600 py-3 font-medium text-white hover:bg-blue-500 disabled:opacity-50"
          >
            Home printer (G28)
          </button>
        )}
        {step === 2 && (
          <button
            onClick={() => setStep(3)}
            className="flex-1 rounded-lg bg-blue-600 py-3 font-medium text-white hover:bg-blue-500"
          >
            I've leveled all corners →
          </button>
        )}
        {step === 3 && (
          <button
            onClick={() => setStep(4)}
            className="flex-1 rounded-lg bg-emerald-600 py-3 font-medium text-white hover:bg-emerald-500"
          >
            Verify center →
          </button>
        )}
        {step === 4 && (
          <Link
            to={`/printers/${printer.id}`}
            className="flex-1 rounded-lg bg-emerald-600 py-3 text-center font-medium text-white hover:bg-emerald-500"
          >
            Done — back to printer
          </Link>
        )}
        <button
          onClick={() => { setStep(0); setActiveCorner(null) }}
          className="flex items-center gap-2 rounded-lg border border-slate-300 px-4 py-3 text-sm text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
        >
          <RotateCw className="h-4 w-4" /> Restart
        </button>
      </div>
    </div>
  )
}
