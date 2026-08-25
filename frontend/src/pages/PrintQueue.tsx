import { useEffect, useState, useCallback } from 'react'
import { Plus, Trash2, Play, X, ListOrdered } from 'lucide-react'
import { usePrinters } from '../hooks/usePrinters'
import { useGCodeFiles } from '../hooks/useGCodeFiles'

interface QueueItem {
  id: string
  printerId: string
  filename: string
  addedAt: number
  status: string
  startedAt?: number
  finishedAt?: number
  error?: string
}

export function PrintQueue() {
  const { printers } = usePrinters()
  const { files } = useGCodeFiles()
  const [queue, setQueue] = useState<QueueItem[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [selectedPrinter, setSelectedPrinter] = useState('')
  const [selectedFile, setSelectedFile] = useState('')

  const fetchQueue = useCallback(async () => {
    try {
      const res = await fetch('/api/queue')
      if (!res.ok) return
      const data = await res.json()
      setQueue(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error(e)
    }
  }, [])

  useEffect(() => {
    fetchQueue()
    const id = setInterval(fetchQueue, 3000)
    return () => clearInterval(id)
  }, [fetchQueue])

  const addToQueue = async () => {
    if (!selectedPrinter || !selectedFile) return
    try {
      await fetch('/api/queue', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ printerId: selectedPrinter, filename: selectedFile }),
      })
      setShowAdd(false)
      setSelectedFile('')
      fetchQueue()
    } catch (e) {
      console.error(e)
    }
  }

  const removeFromQueue = async (id: string) => {
    try {
      await fetch(`/api/queue/${encodeURIComponent(id)}`, { method: 'DELETE' })
      fetchQueue()
    } catch (e) {
      console.error(e)
    }
  }

  const startNow = async (id: string) => {
    try {
      await fetch(`/api/queue/${encodeURIComponent(id)}`, { method: 'POST' })
      fetchQueue()
    } catch (e) {
      console.error(e)
    }
  }

  const clearQueue = async () => {
    try {
      await fetch('/api/queue', { method: 'DELETE' })
      fetchQueue()
    } catch (e) {
      console.error(e)
    }
  }

  const printerName = (id: string) => printers.find((p) => p.id === id)?.name || id
  const statusColor = (status: string) => {
    switch (status) {
      case 'printing': return 'text-blue-600 dark:text-blue-400'
      case 'done': return 'text-emerald-600 dark:text-emerald-400'
      case 'failed': return 'text-rose-600 dark:text-rose-400'
      case 'skipped': return 'text-slate-400'
      default: return 'text-slate-500 dark:text-slate-400'
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <ListOrdered className="h-6 w-6 text-blue-500" />
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-white">Print Queue</h1>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => setShowAdd(true)}
            className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
          >
            <Plus className="h-4 w-4" /> Add file
          </button>
          {queue.length > 0 && (
            <button
              onClick={clearQueue}
              className="flex items-center gap-2 rounded-lg border border-slate-300 px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
            >
              <Trash2 className="h-4 w-4" /> Clear
            </button>
          )}
        </div>
      </div>

      {queue.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
          <ListOrdered className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-700" />
          <p className="font-mono text-sm text-slate-400">Queue is empty. Add a G-code file to get started.</p>
          <p className="mt-1 font-mono text-xs text-slate-400">When a print finishes, the next queued file starts automatically.</p>
        </div>
      ) : (
        <div className="space-y-2">
          {queue.map((item, i) => (
            <div
              key={item.id}
              className="flex items-center gap-4 rounded-xl border border-slate-200 p-4 dark:border-slate-800"
            >
              <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-slate-100 font-mono text-sm font-bold text-slate-600 dark:bg-slate-800 dark:text-slate-400">
                {i + 1}
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate font-mono text-sm font-medium text-slate-900 dark:text-white">{item.filename}</p>
                <p className="font-mono text-xs text-slate-500 dark:text-slate-400">
                  {printerName(item.printerId)} · {new Date(item.addedAt * 1000).toLocaleTimeString()}
                </p>
                {item.error && (
                  <p className="mt-1 font-mono text-xs text-rose-500">{item.error}</p>
                )}
              </div>
              <span className={`font-mono text-xs font-medium capitalize ${statusColor(item.status)}`}>
                {item.status}
              </span>
              <div className="flex gap-1">
                {item.status === 'pending' && (
                  <button
                    onClick={() => startNow(item.id)}
                    className="rounded-lg p-2 text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/20"
                    title="Start now"
                  >
                    <Play className="h-4 w-4" />
                  </button>
                )}
                <button
                  onClick={() => removeFromQueue(item.id)}
                  className="rounded-lg p-2 text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-900/20"
                  title="Remove"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Add to queue modal */}
      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-md rounded-2xl bg-white p-6 dark:bg-slate-900" onClick={(e) => e.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-slate-900 dark:text-white">Add to queue</h2>
              <button onClick={() => setShowAdd(false)} className="text-slate-400 hover:text-slate-900 dark:hover:text-white">
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="space-y-4">
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">Printer</label>
                <select
                  value={selectedPrinter}
                  onChange={(e) => setSelectedPrinter(e.target.value)}
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                >
                  <option value="">Select printer...</option>
                  {printers.map((p) => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500 dark:text-slate-400">G-code file</label>
                <select
                  value={selectedFile}
                  onChange={(e) => setSelectedFile(e.target.value)}
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                >
                  <option value="">Select file...</option>
                  {files.map((f) => (
                    <option key={f.id} value={f.name}>{f.name}</option>
                  ))}
                </select>
              </div>
              <button
                onClick={addToQueue}
                disabled={!selectedPrinter || !selectedFile}
                className="w-full rounded-lg bg-blue-600 py-2.5 font-medium text-white hover:bg-blue-500 disabled:opacity-50"
              >
                Add to queue
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
