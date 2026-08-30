import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Upload,
  Download,
  Trash2,
  Eye,
  Edit3,
  Search,
  Tag as TagIcon,
  X,
  Box,
  Loader2,
  Save,
} from 'lucide-react'
import { STLViewer } from '../components/STLViewer'
import { STLThumbnail } from '../components/STLViewer'

interface STLFile {
  id: string
  name: string
  filename: string
  size: number
  tags?: string[]
  notes?: string
  uploadedAt: number
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString()
}

const inputClass =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'

export function STLFiles() {
  const [files, setFiles] = useState<STLFile[]>([])
  const [allTags, setAllTags] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [tagFilter, setTagFilter] = useState<string | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [viewing, setViewing] = useState<STLFile | null>(null)
  const [editing, setEditing] = useState<STLFile | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const fetchAll = useCallback(() => {
    Promise.all([
      fetch('/api/stl-files').then((r) => r.json()),
      fetch('/api/stl-files/tags').then((r) => r.json()),
    ])
      .then(([f, t]) => {
        if (Array.isArray(f)) setFiles(f)
        if (Array.isArray(t)) setAllTags(t)
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this STL file?')) return
    await fetch(`/api/stl-files/${id}`, { method: 'DELETE' })
    fetchAll()
  }

  const handleDownload = async (id: string) => {
    const res = await fetch(`/api/stl-files/${id}`)
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const file = files.find((f) => f.id === id)
    a.download = file?.filename || 'model.stl'
    a.click()
    URL.revokeObjectURL(url)
  }

  const handleUpload = async (files: File[], tags: string, notes: string) => {
    for (let i = 0; i < files.length; i++) {
      const formData = new FormData()
      formData.append('file', files[i])
      formData.append('tags', tags)
      formData.append('notes', notes)
      await fetch('/api/stl-files', { method: 'POST', body: formData })
    }
    fetchAll()
  }

  const filtered = files.filter((f) => {
    const tags = f.tags || []
    if (tagFilter && !tags.includes(tagFilter)) return false
    if (
      search &&
      !f.name.toLowerCase().includes(search.toLowerCase()) &&
      !tags.some((t) => t.toLowerCase().includes(search.toLowerCase()))
    )
      return false
    return true
  })

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold text-slate-900 dark:text-white">
            <Box className="h-6 w-6 text-blue-500" /> STL Library
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            Store and preview 3D model files (.stl, .obj). Upload, tag, and download.
          </p>
        </div>
        <button
          onClick={() => setUploadOpen(true)}
          className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white shadow-sm hover:bg-blue-500"
        >
          <Upload className="h-4 w-4" /> Upload
        </button>
      </div>

      {/* Search + tag filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-[200px] flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or tag..."
            className={`${inputClass} pl-10`}
          />
        </div>
        {allTags.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {allTags.map((tag) => (
              <button
                key={tag}
                onClick={() => setTagFilter(tagFilter === tag ? null : tag)}
                className={`flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                  tagFilter === tag
                    ? 'bg-blue-600 text-white'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:hover:bg-slate-700'
                }`}
              >
                <TagIcon className="h-3 w-3" /> {tag}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* File grid */}
      {filtered.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 p-12 text-center dark:border-slate-700">
          <Box className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-600" />
          <p className="font-mono text-sm text-slate-400">
            No STL files yet. Upload 3D models to build your library.
          </p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {filtered.map((f) => (
            <div
              key={f.id}
              className="group cursor-pointer rounded-xl border border-slate-200 dark:border-slate-700"
              onClick={() => setViewing(f)}
            >
              {/* 3D preview thumbnail */}
              <div className="relative h-32 w-full overflow-hidden rounded-t-xl border-b border-slate-200 dark:border-slate-700">
                <STLThumbnail
                  url={`/api/stl-files/${f.id}`}
                  className="h-full w-full"
                />
                {/* Hover overlay with action buttons */}
                <div className="absolute inset-0 flex items-center justify-center gap-2 bg-slate-950/0 opacity-0 transition-all group-hover:bg-slate-950/40 group-hover:opacity-100">
                  <button
                    onClick={(e) => { e.stopPropagation(); setViewing(f) }}
                    className="rounded-lg bg-slate-800/90 p-2 text-slate-300 hover:text-blue-400"
                    title="View 3D"
                  >
                    <Eye className="h-4 w-4" />
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); handleDownload(f.id) }}
                    className="rounded-lg bg-slate-800/90 p-2 text-slate-300 hover:text-emerald-400"
                    title="Download"
                  >
                    <Download className="h-4 w-4" />
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); setEditing(f) }}
                    className="rounded-lg bg-slate-800/90 p-2 text-slate-300 hover:text-blue-400"
                    title="Edit"
                  >
                    <Edit3 className="h-4 w-4" />
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); handleDelete(f.id) }}
                    className="rounded-lg bg-slate-800/90 p-2 text-slate-300 hover:text-rose-400"
                    title="Delete"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>

              {/* File info */}
              <div className="p-3">
                <h3 className="truncate font-medium text-slate-900 dark:text-white">{f.name}</h3>
                <p className="truncate text-xs text-slate-400">{f.filename}</p>
                <div className="mt-1.5 flex items-center gap-2 text-xs text-slate-400">
                  <span>{formatSize(f.size)}</span>
                  <span>·</span>
                  <span>{formatDate(f.uploadedAt)}</span>
                </div>

                {f.tags && f.tags.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1">
                    {f.tags.map((tag) => (
                      <span
                        key={tag}
                        className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-400"
                      >
                        {tag}
                      </span>
                    ))}
                  </div>
                )}

                {f.notes && (
                  <p className="mt-2 line-clamp-2 text-xs text-slate-500 dark:text-slate-400">{f.notes}</p>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Upload modal */}
      {uploadOpen && (
        <UploadModal
          onClose={() => setUploadOpen(false)}
          onUpload={handleUpload}
          fileRef={fileRef}
        />
      )}

      {/* View modal */}
      {viewing && (
        <ViewModal file={viewing} onClose={() => setViewing(null)} />
      )}

      {/* Edit modal */}
      {editing && (
        <EditModal file={editing} onClose={() => setEditing(null)} onSaved={fetchAll} />
      )}
    </div>
  )
}

function UploadModal({
  onClose,
  onUpload,
  fileRef,
}: {
  onClose: () => void
  onUpload: (files: File[], tags: string, notes: string) => Promise<void>
  fileRef: React.RefObject<HTMLInputElement | null>
}) {
  const [files, setFiles] = useState<File[]>([])
  const [tags, setTags] = useState('')
  const [notes, setNotes] = useState('')
  const [uploading, setUploading] = useState(false)
  const [progress, setProgress] = useState({ current: 0, total: 0 })

  const handleUpload = async () => {
    if (files.length === 0) return
    setUploading(true)
    setProgress({ current: 0, total: files.length })
    await onUpload(files, tags, notes)
    setUploading(false)
    onClose()
  }

  const removeFile = (idx: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== idx))
  }

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-md flex-col overflow-hidden rounded-lg border-2 border-slate-700 bg-slate-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <h2 className="font-mono text-lg font-semibold text-blue-400">[ upload_stl ]</h2>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="space-y-4 p-6">
          <div
            onClick={() => fileRef.current?.click()}
            className="cursor-pointer rounded-lg border-2 border-dashed border-slate-600 p-6 text-center transition-colors hover:border-blue-500"
          >
            {files.length > 0 ? (
              <p className="text-sm text-slate-300">
                {files.length} file{files.length !== 1 ? 's' : ''} selected · click to add more
              </p>
            ) : (
              <p className="text-sm text-slate-500">Click to select STL / OBJ / 3MF files</p>
            )}
            <input
              ref={fileRef}
              type="file"
              accept=".stl,.obj,.3mf"
              multiple
              className="hidden"
              onChange={(e) => {
                const newFiles = Array.from(e.target.files || [])
                if (newFiles.length > 0) {
                  setFiles((prev) => [...prev, ...newFiles])
                }
                // Reset input so selecting the same file again still fires onChange
                e.target.value = ''
              }}
            />
          </div>

          {/* File list */}
          {files.length > 0 && (
            <div className="max-h-32 space-y-1 overflow-y-auto rounded-lg bg-slate-900 p-2">
              {files.map((f, i) => (
                <div key={i} className="flex items-center gap-2 rounded px-2 py-1 text-xs">
                  <Box className="h-3.5 w-3.5 shrink-0 text-blue-400" />
                  <span className="min-w-0 flex-1 truncate text-slate-300">{f.name}</span>
                  <span className="shrink-0 text-slate-500">{formatSize(f.size)}</span>
                  {!uploading && (
                    <button
                      onClick={() => removeFile(i)}
                      className="shrink-0 rounded p-0.5 text-slate-500 hover:text-rose-400"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  )}
                </div>
              ))}
            </div>
          )}

          <div>
            <label className="mb-1 block text-xs text-slate-400">
              Tags (comma-separated) — applied to all files
            </label>
            <input
              type="text"
              value={tags}
              onChange={(e) => setTags(e.target.value)}
              placeholder="e.g. vase, calibration, fun"
              className={inputClass}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-400">
              Notes — applied to all files
            </label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Optional notes about these models..."
              className={`${inputClass} h-20 resize-none`}
            />
          </div>
          <button
            onClick={handleUpload}
            disabled={files.length === 0 || uploading}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
          >
            {uploading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Uploading... ({progress.current}/{progress.total})
              </>
            ) : (
              <>
                <Upload className="h-4 w-4" />
                Upload {files.length > 0 ? `${files.length} file${files.length !== 1 ? 's' : ''}` : ''}
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  )
}

function ViewModal({ file, onClose }: { file: STLFile; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-4xl max-h-[90vh] flex-col overflow-hidden rounded-lg border-2 border-slate-700 bg-slate-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <div>
            <h2 className="font-mono text-lg font-semibold text-blue-400">{file.name}</h2>
            <p className="text-xs text-slate-500">{file.filename} · {formatSize(file.size)}</p>
          </div>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="flex-1 overflow-hidden p-4">
          <STLViewer url={`/api/stl-files/${file.id}`} className="h-[60vh] w-full rounded-lg border border-slate-800" />
        </div>
        {file.notes && (
          <div className="border-t border-slate-800 px-6 py-3">
            <p className="text-sm text-slate-400">{file.notes}</p>
          </div>
        )}
      </div>
    </div>
  )
}

function EditModal({
  file,
  onClose,
  onSaved,
}: {
  file: STLFile
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(file.name)
  const [tags, setTags] = useState((file.tags || []).join(', '))
  const [notes, setNotes] = useState(file.notes || '')
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    setSaving(true)
    const parsedTags = tags
      .split(',')
      .map((t) => t.trim())
      .filter((t) => t !== '')
    await fetch(`/api/stl-files/${file.id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, tags: parsedTags, notes }),
    })
    setSaving(false)
    onSaved()
    onClose()
  }

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-md flex-col overflow-hidden rounded-lg border-2 border-slate-700 bg-slate-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <h2 className="font-mono text-lg font-semibold text-blue-400">[ edit_stl ]</h2>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="space-y-4 p-6">
          <div>
            <label className="mb-1 block text-xs text-slate-400">Name</label>
            <input type="text" value={name} onChange={(e) => setName(e.target.value)} className={inputClass} />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-400">Tags (comma-separated)</label>
            <input type="text" value={tags} onChange={(e) => setTags(e.target.value)} className={inputClass} />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-400">Notes</label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className={`${inputClass} h-20 resize-none`}
            />
          </div>
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
          >
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            Save
          </button>
        </div>
      </div>
    </div>
  )
}
