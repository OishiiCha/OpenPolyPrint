import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  ArrowLeft, Plus, Trash2, Edit3, FileText, Loader2, Save, X, Search,
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'

interface PlanFile {
  name: string
  size: number
  modified: number
  title: string
}

export function Planning() {
  const [files, setFiles] = useState<PlanFile[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<PlanFile | null>(null)
  const [content, setContent] = useState('')
  const [contentLoading, setContentLoading] = useState(false)
  const [editing, setEditing] = useState(false)
  const [editContent, setEditContent] = useState('')
  const [saving, setSaving] = useState(false)
  const [showNewForm, setShowNewForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [newContent, setNewContent] = useState('')
  const [search, setSearch] = useState('')
  const [error, setError] = useState<string | null>(null)

  const fetchFiles = useCallback(() => {
    fetch('/api/planning')
      .then((r) => r.json())
      .then((data) => {
        if (Array.isArray(data)) setFiles(data)
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [])

  useEffect(() => { fetchFiles() }, [fetchFiles])

  const loadFile = async (file: PlanFile) => {
    setSelected(file)
    setContentLoading(true)
    setEditing(false)
    setError(null)
    try {
      const res = await fetch(`/api/planning/${encodeURIComponent(file.name)}`)
      if (!res.ok) throw new Error('Failed to load')
      const text = await res.text()
      setContent(text)
    } catch {
      setError('Failed to load file')
      setContent('')
    } finally {
      setContentLoading(false)
    }
  }

  const startEdit = () => {
    setEditContent(content)
    setEditing(true)
  }

  const saveEdit = async () => {
    if (!selected) return
    setSaving(true)
    setError(null)
    try {
      const res = await fetch(`/api/planning/${encodeURIComponent(selected.name)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content: editContent }),
      })
      if (!res.ok) throw new Error('Failed to save')
      setContent(editContent)
      setEditing(false)
      window.dispatchEvent(new CustomEvent('openpolyprint-toast', {
        detail: { type: 'success', message: 'Planning doc saved' }
      }))
      fetchFiles()
    } catch {
      setError('Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const deleteFile = async (name: string) => {
    if (!confirm(`Delete "${name}"?`)) return
    await fetch(`/api/planning/${encodeURIComponent(name)}`, { method: 'DELETE' })
    if (selected?.name === name) {
      setSelected(null)
      setContent('')
    }
    fetchFiles()
  }

  const createFile = async () => {
    if (!newName.trim()) return
    setSaving(true)
    setError(null)
    try {
      const initialContent = `# ${newName.trim()}\n\n## Overview\n\nDescribe the plan here...\n\n## Tasks\n\n- [ ] Task 1\n- [ ] Task 2\n- [ ] Task 3\n\n## Notes\n\n`
      const res = await fetch('/api/planning', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newName.trim(), content: newContent || initialContent }),
      })
      if (!res.ok) throw new Error('Failed to create')
      const data = await res.json()
      setShowNewForm(false)
      setNewName('')
      setNewContent('')
      fetchFiles()
      // Load the new file
      loadFile({ name: data.name, size: 0, modified: Date.now() / 1000, title: newName.trim() })
    } catch {
      setError('Failed to create file')
    } finally {
      setSaving(false)
    }
  }

  const filtered = files.filter(f =>
    f.title.toLowerCase().includes(search.toLowerCase()) ||
    f.name.toLowerCase().includes(search.toLowerCase())
  )

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
      </div>
    )
  }

  return (
    <div className="flex h-[calc(100vh-3.5rem)] flex-col">
      {/* Header */}
      <div className="flex items-center gap-4 border-b border-slate-200 px-6 py-3 dark:border-slate-800">
        <Link to="/" className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white">
          <ArrowLeft className="h-4 w-4" /> Back
        </Link>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-white">Future Planning</h1>
        <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500 dark:bg-slate-800 dark:text-slate-400">
          {files.length} docs
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={() => setShowNewForm(true)}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-500"
          >
            <Plus className="h-4 w-4" /> New Doc
          </button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar: file list */}
        <div className="w-72 shrink-0 overflow-y-auto border-r border-slate-200 bg-slate-50 dark:border-slate-800 dark:bg-slate-950">
          <div className="p-3">
            <div className="relative mb-3">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-slate-400" />
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search docs..."
                className="w-full rounded-lg border border-slate-300 bg-white py-2 pl-8 pr-3 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-900 dark:text-white"
              />
            </div>
            <div className="space-y-1">
              {filtered.length === 0 ? (
                <p className="px-2 py-4 text-center text-xs text-slate-400">
                  No documents found
                </p>
              ) : (
                filtered.map((file) => (
                  <div
                    key={file.name}
                    className={`group flex items-center gap-2 rounded-lg border p-2.5 transition-colors cursor-pointer ${
                      selected?.name === file.name
                        ? 'border-blue-500 bg-blue-50 dark:border-blue-700 dark:bg-blue-900/20'
                        : 'border-transparent hover:border-slate-300 hover:bg-slate-100 dark:hover:border-slate-700 dark:hover:bg-slate-900'
                    }`}
                    onClick={() => loadFile(file)}
                  >
                    <FileText className="h-4 w-4 shrink-0 text-slate-400" />
                    <div className="flex-1 min-w-0">
                      <p className="truncate text-sm font-medium text-slate-900 dark:text-white">
                        {file.title}
                      </p>
                      <p className="truncate text-[10px] text-slate-400">
                        {file.name} · {formatDate(file.modified)}
                      </p>
                    </div>
                    <button
                      onClick={(e) => { e.stopPropagation(); deleteFile(file.name) }}
                      className="shrink-0 rounded p-1 text-slate-300 opacity-0 transition-opacity hover:text-rose-500 group-hover:opacity-100"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>

        {/* Main: content viewer/editor */}
        <div className="flex-1 overflow-y-auto bg-white dark:bg-slate-900">
          {error && (
            <div className="mx-6 mt-4 rounded-lg border border-rose-300 bg-rose-50 p-3 text-sm text-rose-600 dark:border-rose-700 dark:bg-rose-900/20 dark:text-rose-400">
              {error}
            </div>
          )}

          {!selected && !showNewForm && (
            <div className="flex h-full items-center justify-center">
              <div className="text-center">
                <FileText className="mx-auto h-12 w-12 text-slate-300 dark:text-slate-700" />
                <p className="mt-3 text-sm text-slate-400">Select a document or create a new one</p>
              </div>
            </div>
          )}

          {selected && contentLoading && (
            <div className="flex h-full items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
            </div>
          )}

          {selected && !contentLoading && !editing && (
            <div className="mx-auto max-w-4xl p-6">
              <div className="mb-4 flex items-center justify-between">
                <div>
                  <h2 className="text-lg font-semibold text-slate-900 dark:text-white">{selected.title}</h2>
                  <p className="font-mono text-xs text-slate-400">{selected.name}</p>
                </div>
                <button
                  onClick={startEdit}
                  className="flex items-center gap-1.5 rounded-lg bg-slate-100 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                >
                  <Edit3 className="h-4 w-4" /> Edit
                </button>
              </div>
              <div className="prose prose-sm dark:prose-invert max-w-none text-slate-700 dark:text-slate-300">
                <ReactMarkdown
                  components={{
                    h1: ({ children }) => <h1 className="mb-3 mt-4 text-xl font-bold text-slate-900 dark:text-white border-b border-slate-200 dark:border-slate-700 pb-2">{children}</h1>,
                    h2: ({ children }) => <h2 className="mb-2 mt-4 text-lg font-bold text-slate-900 dark:text-white">{children}</h2>,
                    h3: ({ children }) => <h3 className="mb-1.5 mt-3 text-base font-semibold text-slate-900 dark:text-white">{children}</h3>,
                    p: ({ children }) => <p className="mb-3 text-sm leading-relaxed">{children}</p>,
                    ul: ({ children }) => <ul className="mb-3 ml-5 list-disc space-y-1 text-sm">{children}</ul>,
                    ol: ({ children }) => <ol className="mb-3 ml-5 list-decimal space-y-1 text-sm">{children}</ol>,
                    li: ({ children }) => {
                      // Render checkboxes for task lists
                      const text = typeof children === 'string' ? children : ''
                      if (text.includes('[ ]') || text.includes('[x]')) {
                        return <li className="text-sm">{children}</li>
                      }
                      return <li className="text-sm">{children}</li>
                    },
                    strong: ({ children }) => <strong className="font-bold text-slate-900 dark:text-white">{children}</strong>,
                    em: ({ children }) => <em className="italic">{children}</em>,
                    code: ({ children }) => <code className="rounded bg-slate-200 px-1 py-0.5 text-xs dark:bg-slate-700">{children}</code>,
                    pre: ({ children }) => <pre className="mb-3 overflow-x-auto rounded-lg bg-slate-100 p-3 text-xs dark:bg-slate-800">{children}</pre>,
                    hr: () => <hr className="my-4 border-slate-200 dark:border-slate-700" />,
                    a: ({ children, href }) => <a href={href} target="_blank" rel="noopener noreferrer" className="text-blue-500 underline">{children}</a>,
                    blockquote: ({ children }) => <blockquote className="border-l-4 border-blue-500 pl-3 italic text-slate-600 dark:text-slate-400">{children}</blockquote>,
                    table: ({ children }) => <table className="mb-3 w-full border-collapse text-sm">{children}</table>,
                    th: ({ children }) => <th className="border border-slate-300 px-3 py-1.5 bg-slate-100 font-semibold text-left dark:border-slate-700 dark:bg-slate-800">{children}</th>,
                    td: ({ children }) => <td className="border border-slate-300 px-3 py-1.5 dark:border-slate-700">{children}</td>,
                  }}
                >
                  {content}
                </ReactMarkdown>
              </div>
            </div>
          )}

          {selected && !contentLoading && editing && (
            <div className="mx-auto max-w-4xl p-6">
              <div className="mb-4 flex items-center justify-between">
                <div>
                  <h2 className="text-lg font-semibold text-slate-900 dark:text-white">Editing: {selected.title}</h2>
                  <p className="font-mono text-xs text-slate-400">{selected.name}</p>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setEditing(false)}
                    className="flex items-center gap-1.5 rounded-lg bg-slate-100 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                  >
                    <X className="h-4 w-4" /> Cancel
                  </button>
                  <button
                    onClick={saveEdit}
                    disabled={saving}
                    className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
                  >
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                    Save
                  </button>
                </div>
              </div>
              <textarea
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                className="h-[calc(100vh-12rem)] w-full rounded-lg border border-slate-300 bg-white p-4 font-mono text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
                spellCheck={false}
              />
            </div>
          )}
        </div>
      </div>

      {/* New doc modal */}
      {showNewForm && (
        <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={() => setShowNewForm(false)}>
          <div className="w-full max-w-md rounded-xl border border-slate-700 bg-slate-950 p-6" onClick={(e) => e.stopPropagation()}>
            <h2 className="mb-4 font-semibold text-white">New Planning Document</h2>
            <div className="space-y-3">
              <div>
                <label className="mb-1 block text-xs text-slate-400">Document title</label>
                <input
                  type="text"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="e.g. Bambu Lab LAN Protocol"
                  className="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-white placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
                  onKeyDown={(e) => e.key === 'Enter' && createFile()}
                  autoFocus
                />
                <p className="mt-1 text-[10px] text-slate-500">
                  Will be saved as {newName.trim() ? newName.trim().toLowerCase().replace(/\s+/g, '-') : 'filename'}.md
                </p>
              </div>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button
                onClick={() => setShowNewForm(false)}
                className="rounded-lg bg-slate-800 px-4 py-2 text-sm text-slate-300 hover:bg-slate-700"
              >
                Cancel
              </button>
              <button
                onClick={createFile}
                disabled={!newName.trim() || saving}
                className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
              >
                {saving ? 'Creating...' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function formatDate(unix: number): string {
  try {
    const d = new Date(unix * 1000)
    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const days = Math.floor(diff / 86400000)
    if (days === 0) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    if (days === 1) return 'yesterday'
    if (days < 7) return `${days}d ago`
    return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
  } catch {
    return ''
  }
}
