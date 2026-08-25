import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft, Film, FileCode2, Sparkles } from 'lucide-react'
import { PrintPlayer } from '../components/PrintPlayer'
import { useGCodeFiles } from '../hooks/useGCodeFiles'
import { loadConfig, saveConfig } from '../config'

export function PrintAnalysis() {
  const { files } = useGCodeFiles()
  const [timelapseDirs, setTimelapseDirs] = useState<string[]>([])
  const [selectedGcode, setSelectedGcode] = useState('')
  const [selectedTimelapse, setSelectedTimelapse] = useState('')
  const [intervalSec, setIntervalSec] = useState(1)
  const [apiKey, setApiKey] = useState('')

  useEffect(() => {
    // Load API key from config
    const cfg = loadConfig()
    if (cfg.geminiApiKey) setApiKey(cfg.geminiApiKey)

    // Fetch timelapse frame directories
    fetch('/api/timelapse-frames')
      .then((r) => r.json())
      .then((data) => {
        if (Array.isArray(data)) setTimelapseDirs(data)
      })
      .catch(() => {})
  }, [])

  const saveApiKey = (key: string) => {
    setApiKey(key)
    const cfg = loadConfig()
    saveConfig({ ...cfg, geminiApiKey: key })
  }

  const canPlay = selectedGcode && selectedTimelapse

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link
          to="/"
          className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white"
        >
          <ArrowLeft className="h-4 w-4" /> Back
        </Link>
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Print Analysis</h1>
      </div>

      <p className="font-mono text-sm text-slate-500 dark:text-slate-400">
        Sync a timelapse recording with the G-code visualizer to scrub through a print.
        Use the AI analysis feature to send frames to Gemini for quality assessment.
      </p>

      {/* Configuration */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <h3 className="mb-4 font-semibold text-slate-900 dark:text-white">Select print session</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1 flex items-center gap-2 font-mono text-xs text-slate-500 dark:text-slate-400">
              <FileCode2 className="h-3.5 w-3.5" /> G-code file
            </label>
            <select
              value={selectedGcode}
              onChange={(e) => setSelectedGcode(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
            >
              <option value="">Select G-code...</option>
              {files.map((f) => (
                <option key={f.id} value={f.id}>{f.name}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 flex items-center gap-2 font-mono text-xs text-slate-500 dark:text-slate-400">
              <Film className="h-3.5 w-3.5" /> Timelapse frames
            </label>
            <select
              value={selectedTimelapse}
              onChange={(e) => setSelectedTimelapse(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
            >
              <option value="">Select timelapse...</option>
              {timelapseDirs.map((dir) => (
                <option key={dir} value={dir}>{dir}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">
              Timelapse interval (seconds)
            </label>
            <input
              type="number"
              min={0.5}
              step={0.5}
              value={intervalSec}
              onChange={(e) => setIntervalSec(parseFloat(e.target.value) || 1)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
            />
          </div>

          <div>
            <label className="mb-1 flex items-center gap-2 font-mono text-xs text-slate-500 dark:text-slate-400">
              <Sparkles className="h-3.5 w-3.5" /> Gemini API key (BYOK)
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => saveApiKey(e.target.value)}
              placeholder="AIza..."
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
            />
          </div>
        </div>
      </div>

      {/* Player */}
      {canPlay ? (
        <PrintPlayer
          gcodeId={selectedGcode}
          timelapseDir={selectedTimelapse}
          intervalSec={intervalSec}
          printerName=""
          filename={files.find((f) => f.id === selectedGcode)?.name || ''}
          apiKey={apiKey}
        />
      ) : (
        <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
          <Film className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-700" />
          <p className="font-mono text-sm text-slate-400">
            Select a G-code file and timelapse to start analyzing.
          </p>
        </div>
      )}
    </div>
  )
}
