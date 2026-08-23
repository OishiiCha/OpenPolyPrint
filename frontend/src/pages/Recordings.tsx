import { useEffect, useState } from 'react'
import { Card, SectionTitle } from './index'

export function Recordings() {
  const [videos, setVideos] = useState<string[]>([])
  const [timelapses, setTimelapses] = useState<string[]>([])
  const [names, setNames] = useState<Record<string, string>>({})
  const [editingNames, setEditingNames] = useState<Record<string, string>>({})
  const [details, setDetails] = useState<string | null>(null)
  const [playing, setPlaying] = useState<string | null>(null)
  const [tab, setTab] = useState<'videos' | 'timelapses'>('videos')
  const [converting, setConverting] = useState<string | null>(null)

  const loadVideos = async () => {
    try {
      const res = await fetch('/api/recordings/videos')
      const data = await res.json()
      setVideos(Array.isArray(data.recordings) ? data.recordings : [])
    } catch (e) {
      console.error(e)
    }
  }

  const loadTimelapses = async () => {
    try {
      const res = await fetch('/api/recordings/timelapses')
      const data = await res.json()
      setTimelapses(Array.isArray(data.recordings) ? data.recordings : [])
    } catch (e) {
      console.error(e)
    }
  }

  const loadNames = async () => {
    try {
      const res = await fetch('/api/recordings/names')
      const data = await res.json()
      setNames(data.names || {})
    } catch (e) {
      console.error(e)
    }
  }

  const loadAll = () => {
    loadVideos()
    loadTimelapses()
    loadNames()
  }

  useEffect(() => {
    loadAll()
  }, [])

  const saveName = async (folder: string, filename: string, name: string) => {
    try {
      await fetch(`/api/recordings/${folder}/${encodeURIComponent(filename)}/name`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      })
      await loadNames()
    } catch (e) {
      console.error(e)
    }
  }

  const displayName = (folder: string, filename: string) => {
    return names[`${folder}/${filename}`] || filename
  }

  const convertToTimelapse = async (filename: string) => {
    setConverting(filename)
    try {
      const res = await fetch(`/api/recordings/videos/${encodeURIComponent(filename)}/convert/timelapse`, { method: 'POST' })
      const data = await res.json().catch(() => ({ error: 'invalid response' }))
      if (data.success) {
        await loadTimelapses()
        setTab('timelapses')
      } else {
        alert(`Convert failed: ${data.error || 'unknown'}`)
      }
    } catch (e: any) {
      alert(`Convert error: ${e.message || e}`)
    } finally {
      setConverting(null)
    }
  }

  const files = tab === 'videos' ? videos : timelapses
  const folder = tab === 'videos' ? 'videos' : 'timelapse'

  return (
    <div className="space-y-6">
      <SectionTitle title="Recordings" />

      <div className="flex gap-4 border-b border-slate-200 dark:border-slate-800">
        <button
          onClick={() => setTab('videos')}
          className={`border-b-2 px-3 py-2 text-sm font-medium ${
            tab === 'videos'
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-300'
          }`}
        >
          Videos
        </button>
        <button
          onClick={() => setTab('timelapses')}
          className={`border-b-2 px-3 py-2 text-sm font-medium ${
            tab === 'timelapses'
              ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
              : 'border-transparent text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-300'
          }`}
        >
          Timelapses
        </button>
      </div>

      {files.length === 0 ? (
        <p className="font-mono text-sm text-slate-500">
          {tab === 'videos' ? 'No video recordings yet. Use the dashboard to start recording.' : 'No timelapse recordings yet.'}
        </p>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {files.map((r) => {
            const itemKey = `${folder}/${r}`
            const display = displayName(folder, r)
            return (
              <div key={r} className="flex flex-col gap-2 rounded-lg border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-950">
                <div className="relative aspect-video w-full overflow-hidden rounded-lg border border-slate-200 bg-slate-100 dark:border-slate-700 dark:bg-slate-800">
                  <img
                    src={`/api/recordings/${folder}/${encodeURIComponent(r)}/thumb`}
                    alt={r}
                    className="h-full w-full object-cover"
                    onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }}
                  />
                </div>
                {itemKey in editingNames ? (
                  <input
                    value={editingNames[itemKey]}
                    onChange={(e) => setEditingNames((prev) => ({ ...prev, [itemKey]: e.target.value }))}
                    onBlur={() => {
                      const v = editingNames[itemKey]
                      if (v !== undefined) {
                        saveName(folder, r, v)
                        setEditingNames((prev) => {
                          const next = { ...prev }
                          delete next[itemKey]
                          return next
                        })
                      }
                    }}
                    onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }}
                    className="rounded-lg border border-slate-300 bg-white px-2 py-1.5 font-mono text-sm text-slate-900 focus:border-blue-500 focus:outline-none dark:border-slate-700 dark:bg-slate-950 dark:text-white"
                  />
                ) : (
                  <div className="truncate font-mono text-sm text-slate-900 dark:text-white" title={display}>
                    {display}
                  </div>
                )}
                <div className="mt-auto flex flex-wrap items-center gap-2">
                  {tab === 'videos' && (
                    <button
                      onClick={() => convertToTimelapse(r)}
                      disabled={converting === r}
                      className="flex-1 rounded-lg bg-amber-100 px-2 py-1 font-mono text-xs font-medium text-amber-700 hover:bg-amber-200 disabled:opacity-50 dark:bg-amber-900/30 dark:text-amber-300 dark:hover:bg-amber-900/50"
                    >
                      {converting === r ? '...' : 'timelapse'}
                    </button>
                  )}
                  {!(itemKey in editingNames) && (
                    <>
                      <button
                        onClick={() => setEditingNames((prev) => ({ ...prev, [itemKey]: display }))}
                        className="rounded-lg bg-slate-100 px-2 py-1 font-mono text-xs font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                      >
                        edit
                      </button>
                      <button
                        onClick={() => setDetails(details === r ? null : r)}
                        className="rounded-lg bg-slate-100 px-2 py-1 font-mono text-xs font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                      >
                        details
                      </button>
                    </>
                  )}
                  <button
                    onClick={() => setPlaying(`/recordings/${folder}/${encodeURIComponent(r)}`)}
                    className="flex-1 rounded-lg bg-blue-100 px-2 py-1 font-mono text-xs font-medium text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-300 dark:hover:bg-blue-900/50"
                  >
                    play
                  </button>
                  <a
                    href={`/recordings/${folder}/${encodeURIComponent(r)}`}
                    download
                    className="rounded-lg bg-slate-100 px-2 py-1 font-mono text-xs font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                  >
                    download
                  </a>
                </div>
                {details === r && (
                  <div className="mt-1 rounded bg-slate-50 p-2 font-mono text-[10px] text-slate-500 dark:bg-slate-900 dark:text-slate-400">
                    <p>original: {r}</p>
                    <p>path: /recordings/{folder}/{encodeURIComponent(r)}</p>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {playing && (
        <Card>
          <div className="mb-3 flex items-center justify-between">
            <h3 className="font-semibold text-slate-900 dark:text-white">Player</h3>
            <button onClick={() => setPlaying(null)} className="text-sm text-slate-500 hover:text-slate-300">
              close
            </button>
          </div>
          <video controls className="w-full rounded-lg" src={playing}>
            Your browser does not support the video tag.
          </video>
        </Card>
      )}
    </div>
  )
}
