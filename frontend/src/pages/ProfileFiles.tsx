import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  ArrowLeft, Upload, Trash2, Edit3, Download, Eye, Tag as TagIcon,
  Loader2, FileText, X, Search, Repeat, AlertTriangle, CheckCircle2,
  Sparkles, ChevronDown, ChevronRight, Settings as SettingsIcon,
  Printer as PrinterIcon, Layers, Box,
} from 'lucide-react'
import { AIAnalyzeModal } from '../components/AIAnalyzeModal'

type Category = 'filament' | 'print'

interface ProfileFile {
  id: string
  name: string
  filename: string
  category: Category
  size: number
  tags?: string[]
  slicer?: string
  notes?: string
  content?: string
  uploadedAt: number
}

const inputClass = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white'

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

// Parse INI content into sections for display.
// Handles both sectioned INI ([print:...]) and flat INI (eufyMake export).
function parseINI(content: string): { section: string; keys: { key: string; value: string }[] }[] {
  const sections: { section: string; keys: { key: string; value: string }[] }[] = []
  let current: { section: string; keys: { key: string; value: string }[] } | null = null
  const flatKeys: { key: string; value: string }[] = []
  let hasSections = false

  for (const line of content.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith(';') || trimmed.startsWith('#')) continue
    if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
      hasSections = true
      current = { section: trimmed.slice(1, -1), keys: [] }
      sections.push(current)
      continue
    }
    const idx = trimmed.indexOf('=')
    if (idx > 0) {
      const key = trimmed.slice(0, idx).trim()
      const value = trimmed.slice(idx + 1).trim()
      if (current) {
        current.keys.push({ key, value })
      } else {
        flatKeys.push({ key, value })
      }
    }
  }

  // If flat INI (no section headers), categorize settings into print/filament/printer
  if (!hasSections && flatKeys.length > 0) {
    const categorized = categorizeFlatINI(flatKeys)
    for (const cat of categorized) {
      if (cat.keys.length > 0) {
        sections.push(cat)
      }
    }
  }

  return sections
}

// Categorize flat INI settings into print/filament/printer groups.
// Mirrors the Go backend categorizeSetting logic.
function categorizeFlatINI(keys: { key: string; value: string }[]): { section: string; keys: { key: string; value: string }[] }[] {
  const print: { key: string; value: string }[] = []
  const filament: { key: string; value: string }[] = []
  const printer: { key: string; value: string }[] = []
  const meta: { key: string; value: string }[] = []

  // Filament-related prefixes and exact keys
  const filamentPrefixes = ['filament_', 'default_filament_profile']
  const filamentExact = new Set([
    'temperature', 'bed_temperature', 'first_layer_temperature', 'first_layer_bed_temperature',
    'cooling', 'max_fan_speed', 'min_fan_speed', 'disable_fan_first_layers', 'fan_always_on',
    'fan_below_layer_time', 'bridge_fan_speed', 'full_fan_speed_layer', 'enable_dynamic_fan_speeds',
    'overhang_fan_speed_0', 'overhang_fan_speed_1', 'overhang_fan_speed_2', 'overhang_fan_speed_3',
    'idle_temperature', 'standby_temperature_delta', 'high_current_on_filament_swap',
    'autoemit_temperature_commands', 'end_filament_gcode', 'start_filament_gcode',
    'extruder_colour', 'extruder_offset', 'max_volumetric_speed',
    'max_volumetric_extrusion_rate_slope_negative', 'max_volumetric_extrusion_rate_slope_positive',
    'single_extruder_multi_material', 'single_extruder_multi_material_priming',
    'extrusion_multiplier',
  ])

  // Printer-related prefixes and exact keys
  const printerPrefixes = ['machine_', 'printer_', 'bed_', 'nozzle_']
  const printerExact = new Set([
    'bed_shape', 'bed_custom_model', 'bed_custom_texture', 'max_print_height', 'nozzle_diameter',
    'printer_model', 'printer_vendor', 'printer_technology', 'printer_variant', 'printer_settings_id',
    'printer_notes', 'physical_printer_settings_id', 'gcode_flavor', 'gcode_resolution',
    'gcode_substitutions', 'gcode_comments', 'gcode_label_objects',
    'start_gcode', 'end_gcode', 'before_layer_gcode', 'layer_gcode', 'toolchange_gcode',
    'between_objects_gcode', 'color_change_gcode', 'pause_print_gcode', 'template_custom_gcode',
    'extruder_clearance_height', 'extruder_clearance_radius', 'extrusion_axis', 'extruder_count',
    'use_firmware_retraction', 'use_relative_e_distances', 'use_volumetric_e', 'variable_layer_height',
    'silent_mode', 'remaining_times', 'host_type', 'print_host', 'printhost_apikey', 'printhost_cafile',
    'thumbnails', 'thumbnails_format', 'output_filename_format', 'default_print_profile',
    'threads', 'z_offset', 'xy_hole_compensation', 'xy_size_compensation', 'hole_offset',
    'elefant_foot_compensation', 'lift_type', 'travel_speed_z', 'machine_limits_usage',
    'slice_closing_radius', 'slicing_mode', 'mmu_segmented_region_max_width', 'parking_pos_retraction',
    'wiping_volumes_extruders', 'wiping_volumes_matrix',
    'compatible_printers_condition_cummulative', 'compatible_prints_condition_cummulative',
    'inherits_cummulative', 'notes', 'colorprint_heights', 'post_process', 'enable_arc_fitting',
    'make_overhang_printable', 'make_overhang_printable_angle', 'make_overhang_printable_hole_size',
    'slow_down_layers',
  ])

  // Meta keys (not actual settings, just identifiers)
  const metaExact = new Set([
    'print_settings_id', 'filament_settings_id', 'inherits', 'from', 'version',
    'compatible_printers', 'compatible_prints', 'compatible_printers_condition',
    'compatible_prints_condition', 'renamed_from', 'parent',
  ])

  for (const { key, value } of keys) {
    // Check meta first
    if (metaExact.has(key)) {
      meta.push({ key, value })
      continue
    }

    // Check filament
    let isFilament = filamentExact.has(key)
    if (!isFilament) {
      for (const prefix of filamentPrefixes) {
        if (key.startsWith(prefix)) { isFilament = true; break }
      }
    }
    if (isFilament) {
      filament.push({ key, value })
      continue
    }

    // Check printer
    let isPrinter = printerExact.has(key)
    if (!isPrinter) {
      for (const prefix of printerPrefixes) {
        if (key.startsWith(prefix)) { isPrinter = true; break }
      }
    }
    if (isPrinter) {
      printer.push({ key, value })
      continue
    }

    // Default: print settings
    print.push({ key, value })
  }

  // Sort each category alphabetically by key
  const sortKeys = (arr: { key: string; value: string }[]) =>
    arr.sort((a, b) => a.key.localeCompare(b.key))

  sortKeys(print)
  sortKeys(filament)
  sortKeys(printer)
  sortKeys(meta)

  return [
    { section: 'meta', keys: meta },
    { section: 'print', keys: print },
    { section: 'filament', keys: filament },
    { section: 'printer', keys: printer },
  ]
}

export function ProfileFiles() {
  const [files, setFiles] = useState<ProfileFile[]>([])
  const [allTags, setAllTags] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<Category>('filament')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [viewing, setViewing] = useState<ProfileFile | null>(null)
  const [editing, setEditing] = useState<ProfileFile | null>(null)
  const [converting, setConverting] = useState<ProfileFile | null>(null)
  const [convertOpen, setConvertOpen] = useState(false)
  const [aiAnalyzing, setAiAnalyzing] = useState<ProfileFile | null>(null)
  const [tagFilter, setTagFilter] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const fetchAll = useCallback(() => {
    Promise.all([
      fetch(`/api/profile-files?category=${activeTab}`).then((r) => r.json()),
      fetch(`/api/profile-files/tags?category=${activeTab}`).then((r) => r.json()),
    ]).then(([f, t]) => {
      if (Array.isArray(f)) setFiles(f)
      if (Array.isArray(t)) setAllTags(t)
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [activeTab])

  useEffect(() => { fetchAll() }, [fetchAll])

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this profile file?')) return
    await fetch(`/api/profile-files/${id}`, { method: 'DELETE' })
    fetchAll()
  }

  const handleDownload = async (id: string) => {
    const res = await fetch(`/api/profile-files/${id}`)
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = files.find((f) => f.id === id)?.filename || 'profile.ini'
    a.click()
    URL.revokeObjectURL(url)
  }

  const handleView = async (id: string) => {
    const res = await fetch(`/api/profile-files/${id}/view`)
    const data = await res.json()
    setViewing(data)
  }

  const [aiContext, setAiContext] = useState('')
  const handleAskAI = async (f: ProfileFile) => {
    try {
      const res = await fetch(`/api/profile-files/${f.id}/view`)
      if (res.ok) {
        const data = await res.json()
        const content = data.content || data.text || ''
        const truncated = content.length > 30000 ? content.slice(0, 30000) + '\n... (truncated)' : content
        setAiContext(`Profile file: ${f.name}\nFilename: ${f.filename}\nSlicer: ${f.slicer || 'Unknown'}\nTags: ${(f.tags || []).join(', ') || 'None'}\nNotes: ${f.notes || 'None'}\n\n--- File Content ---\n${truncated}\n--- End File Content ---`)
      } else {
        setAiContext(`Profile file: ${f.name}\nFilename: ${f.filename}\nSlicer: ${f.slicer || 'Unknown'}\nTags: ${(f.tags || []).join(', ') || 'None'}\nNotes: ${f.notes || 'None'}\n(Could not load file content)`)
      }
    } catch {
      setAiContext(`Profile file: ${f.name}\nFilename: ${f.filename}\nSlicer: ${f.slicer || 'Unknown'}\nTags: ${(f.tags || []).join(', ') || 'None'}\nNotes: ${f.notes || 'None'}\n(Could not load file content)`)
    }
    setAiAnalyzing(f)
  }

  const filtered = files.filter((f) => {
    const tags = f.tags || []
    if (tagFilter && !tags.includes(tagFilter)) return false
    if (search && !f.name.toLowerCase().includes(search.toLowerCase()) && !tags.some((t) => t.toLowerCase().includes(search.toLowerCase()))) return false
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
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <Link to="/" className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-900 dark:hover:text-white">
            <ArrowLeft className="h-4 w-4" /> Back
          </Link>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-white sm:text-2xl">Profile Files</h1>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => { setConverting(null); setConvertOpen(true) }}
            className="flex items-center gap-2 rounded-lg bg-slate-200 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-300 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
          >
            <Repeat className="h-4 w-4" /> Convert
          </button>
          <button
            onClick={() => setUploadOpen(true)}
            className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
          >
            <Upload className="h-4 w-4" /> Upload profile
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-2">
        {(['filament', 'print'] as Category[]).map((cat) => (
          <button
            key={cat}
            onClick={() => { setActiveTab(cat); setTagFilter(null); setSearch('') }}
            className={`rounded-lg px-4 py-2 text-sm font-medium capitalize transition-colors ${
              activeTab === cat
                ? 'bg-blue-600 text-white'
                : 'bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:hover:bg-slate-700'
            }`}
          >
            {cat} Profiles
          </button>
        ))}
      </div>

      {/* Search + tag filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 min-w-[200px]">
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
          <FileText className="mx-auto mb-3 h-12 w-12 text-slate-300 dark:text-slate-600" />
          <p className="font-mono text-sm text-slate-400">
            No {activeTab} profile files yet. Upload slicer profiles to share them across software and computers.
          </p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((f) => (
            <div
              key={f.id}
              className="group rounded-xl border border-slate-200 p-4 dark:border-slate-700"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0 flex-1">
                  <h3 className="truncate font-medium text-slate-900 dark:text-white">{f.name}</h3>
                  <p className="truncate text-xs text-slate-400">{f.filename}</p>
                </div>
                <div className="flex shrink-0 gap-1">
                  <button onClick={() => handleView(f.id)} className="rounded p-1 text-slate-400 hover:text-blue-500" title="View details">
                    <Eye className="h-4 w-4" />
                  </button>
                  <button onClick={() => handleDownload(f.id)} className="rounded p-1 text-slate-400 hover:text-emerald-500" title="Download">
                    <Download className="h-4 w-4" />
                  </button>
                  <button onClick={() => { setConverting(f); setConvertOpen(true) }} className="rounded p-1 text-slate-400 hover:text-purple-500" title="Convert">
                    <Repeat className="h-4 w-4" />
                  </button>
                  <button onClick={() => handleAskAI(f)} className="rounded p-1 text-slate-400 hover:text-blue-500" title="Ask AI">
                    <Sparkles className="h-4 w-4" />
                  </button>
                  <button onClick={() => setEditing(f)} className="rounded p-1 text-slate-400 hover:text-blue-500" title="Edit">
                    <Edit3 className="h-4 w-4" />
                  </button>
                  <button onClick={() => handleDelete(f.id)} className="rounded p-1 text-slate-400 hover:text-rose-500" title="Delete">
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>

              {f.slicer && (
                <span className="mt-2 inline-block rounded-full bg-purple-100 px-2 py-0.5 text-xs text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">
                  {f.slicer}
                </span>
              )}

              {f.tags && f.tags.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1">
                  {f.tags.map((tag) => (
                    <span key={tag} className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-400">
                      {tag}
                    </span>
                  ))}
                </div>
              )}

              {f.notes && (
                <p className="mt-2 text-xs text-slate-500 dark:text-slate-400 line-clamp-2">{f.notes}</p>
              )}

              <div className="mt-3 flex items-center gap-3 font-mono text-[10px] text-slate-400">
                <span>{formatSize(f.size)}</span>
                <span>{formatDate(f.uploadedAt)}</span>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Upload modal */}
      {uploadOpen && (
        <UploadModal
          category={activeTab}
          onClose={() => setUploadOpen(false)}
          onUploaded={() => { setUploadOpen(false); fetchAll() }}
        />
      )}

      {/* View modal */}
      {viewing && (
        <ViewModal file={viewing} onClose={() => setViewing(null)} />
      )}

      {/* Edit modal */}
      {editing && (
        <EditModal
          file={editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); fetchAll() }}
        />
      )}

      {/* Convert modal */}
      {convertOpen && (
        <ConvertModal
          file={converting}
          category={activeTab}
          onClose={() => { setConvertOpen(false); setConverting(null) }}
          onConverted={() => { setConvertOpen(false); setConverting(null); fetchAll() }}
        />
      )}

      {/* AI analysis modal */}
      {aiAnalyzing && (
        <AIAnalyzeModal
          open={!!aiAnalyzing}
          onClose={() => { setAiAnalyzing(null); setAiContext('') }}
          title={`Analyze profile: ${aiAnalyzing.name}`}
          sourceType="profile"
          defaultMessage="Please analyze this slicer profile file. Review the settings, identify any potential issues or suboptimal values, and suggest improvements for print quality, speed, or reliability."
          contextText={aiContext}
        />
      )}
    </div>
  )
}

function UploadModal({ category, onClose, onUploaded }: { category: Category; onClose: () => void; onUploaded: () => void }) {
  const [name, setName] = useState('')
  const [slicer, setSlicer] = useState('')
  const [tags, setTags] = useState('')
  const [notes, setNotes] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!file) {
      setError('Please select a file')
      return
    }
    setUploading(true)
    setError(null)
    try {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('category', category)
      formData.append('name', name || file.name)
      formData.append('slicer', slicer)
      formData.append('tags', tags)
      formData.append('notes', notes)
      const res = await fetch('/api/profile-files', { method: 'POST', body: formData })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Upload failed' }))
        throw new Error(err.error)
      }
      onUploaded()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed')
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <form
        className="dark w-full max-w-md max-h-[90vh] overflow-y-auto rounded-lg border-2 border-slate-700 bg-slate-950 p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-mono text-lg font-semibold text-blue-400">[ upload_{category}_profile ]</h2>
          <button type="button" onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="space-y-4">
          {/* File drop zone */}
          <div
            onClick={() => fileRef.current?.click()}
            className="cursor-pointer rounded-lg border-2 border-dashed border-slate-600 p-6 text-center transition-colors hover:border-blue-500"
          >
            <Upload className="mx-auto mb-2 h-8 w-8 text-slate-500" />
            {file ? (
              <p className="text-sm text-slate-300">{file.name} ({formatSize(file.size)})</p>
            ) : (
              <p className="text-sm text-slate-500">Click to select a profile file (.ini, .json, .toml)</p>
            )}
            <input
              ref={fileRef}
              type="file"
              accept=".ini,.json,.toml,.cfg,.conf"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0]
                if (f) {
                  setFile(f)
                  if (!name) setName(f.name.replace(/\.[^.]+$/, ''))
                }
              }}
            />
          </div>

          <div>
            <label className="mb-1 block text-xs text-slate-400">Display name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. PLA Matte - 0.2mm" className={inputClass} />
          </div>

          <div>
            <label className="mb-1 block text-xs text-slate-400">Slicer</label>
            <select value={slicer} onChange={(e) => setSlicer(e.target.value)} className={inputClass}>
              <option value="">—</option>
              <option value="prusaslicer">PrusaSlicer</option>
              <option value="orcaslicer">OrcaSlicer</option>
              <option value="cura">Cura</option>
              <option value="ideamaker">IdeaMaker</option>
              <option value="lychee">Lychee</option>
              <option value="other">Other</option>
            </select>
          </div>

          <div>
            <label className="mb-1 block text-xs text-slate-400">Tags (comma-separated)</label>
            <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="e.g. PLA, 0.2mm, draft, fast" className={inputClass} />
          </div>

          <div>
            <label className="mb-1 block text-xs text-slate-400">Notes</label>
            <textarea value={notes} onChange={(e) => setNotes(e.target.value)} placeholder="Optional notes..." rows={2} className={inputClass} />
          </div>

          {error && <p className="text-xs text-rose-400">{error}</p>}

          <div className="flex gap-3">
            <button type="submit" disabled={uploading || !file} className="rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50">
              {uploading ? 'uploading...' : 'upload'}
            </button>
            <button type="button" onClick={onClose} className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700">
              cancel
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

function ViewModal({ file, onClose }: { file: ProfileFile; onClose: () => void }) {
  const sections = file.content ? parseINI(file.content) : []
  const [search, setSearch] = useState('')
  const [activeCategory, setActiveCategory] = useState<string>('all')
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(new Set())
  const [showRaw, setShowRaw] = useState(false)

  // Detect format
  const isJSON = file.content?.trim().startsWith('{')
  const totalSettings = sections.reduce((sum, s) => sum + s.keys.length, 0)

  // Get category stats
  const categoryStats = sections.map(s => ({ section: s.section, count: s.keys.length }))

  // Filter settings based on search and active category
  const filteredSections = sections
    .filter(s => activeCategory === 'all' || s.section === activeCategory)
    .map(s => ({
      ...s,
      keys: search
        ? s.keys.filter(kv =>
            kv.key.toLowerCase().includes(search.toLowerCase()) ||
            kv.value.toLowerCase().includes(search.toLowerCase())
          )
        : s.keys,
    }))
    .filter(s => s.keys.length > 0)

  const toggleSection = (section: string) => {
    setCollapsedSections(prev => {
      const next = new Set(prev)
      if (next.has(section)) next.delete(section)
      else next.add(section)
      return next
    })
  }

  // Category icon + color
  const categoryStyle = (section: string): { icon: typeof Layers; color: string; bg: string } => {
    switch (section) {
      case 'print':
        return { icon: Layers, color: 'text-blue-400', bg: 'bg-blue-500/10 border-blue-500/30' }
      case 'filament':
        return { icon: Box, color: 'text-amber-400', bg: 'bg-amber-500/10 border-amber-500/30' }
      case 'printer':
        return { icon: PrinterIcon, color: 'text-emerald-400', bg: 'bg-emerald-500/10 border-emerald-500/30' }
      case 'meta':
        return { icon: SettingsIcon, color: 'text-slate-400', bg: 'bg-slate-500/10 border-slate-500/30' }
      default:
        return { icon: FileText, color: 'text-purple-400', bg: 'bg-purple-500/10 border-purple-500/30' }
    }
  }

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-3xl max-h-[90vh] flex-col overflow-hidden rounded-lg border-2 border-slate-700 bg-slate-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <div className="min-w-0 flex-1">
            <h2 className="truncate font-mono text-lg font-semibold text-blue-400">{file.name}</h2>
            <p className="truncate text-xs text-slate-500">
              {file.filename} · {formatSize(file.size)}
              {totalSettings > 0 && ` · ${totalSettings} settings`}
            </p>
          </div>
          <button onClick={onClose} className="shrink-0 rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Tags + metadata bar */}
        <div className="flex flex-wrap items-center gap-2 border-b border-slate-800/50 px-6 py-3">
          {file.slicer && (
            <span className="rounded-full bg-purple-100 px-2 py-0.5 text-xs text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">
              {file.slicer}
            </span>
          )}
          {file.tags && file.tags.map((tag) => (
            <span key={tag} className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-400">
              {tag}
            </span>
          ))}
          {file.notes && (
            <span className="truncate text-xs text-slate-500" title={file.notes}>
              {file.notes}
            </span>
          )}
        </div>

        <div className="flex-1 overflow-y-auto p-6">
          {/* Search + view toggle */}
          {!isJSON && sections.length > 0 && (
            <>
              <div className="mb-4 flex items-center gap-2">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                  <input
                    type="text"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Search settings..."
                    className="w-full rounded-lg border border-slate-700 bg-slate-900 py-2 pl-9 pr-3 text-sm text-slate-200 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
                  />
                </div>
                <button
                  onClick={() => setShowRaw(!showRaw)}
                  className={`shrink-0 rounded-lg px-3 py-2 text-xs font-medium transition-colors ${
                    showRaw
                      ? 'bg-blue-600 text-white'
                      : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                  }`}
                >
                  {showRaw ? 'Parsed' : 'Raw'}
                </button>
              </div>

              {/* Category tabs */}
              {!showRaw && (
                <div className="mb-4 flex flex-wrap gap-2">
                  <button
                    onClick={() => setActiveCategory('all')}
                    className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                      activeCategory === 'all'
                        ? 'bg-blue-600 text-white'
                        : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                    }`}
                  >
                    All ({totalSettings})
                  </button>
                  {categoryStats.map(cs => (
                    <button
                      key={cs.section}
                      onClick={() => setActiveCategory(cs.section)}
                      className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                        activeCategory === cs.section
                          ? 'bg-blue-600 text-white'
                          : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                      }`}
                    >
                      {cs.section} ({cs.count})
                    </button>
                  ))}
                </div>
              )}
            </>
          )}

          {/* Content display */}
          {isJSON ? (
            // JSON file display
            <div>
              <pre className="max-h-[60vh] overflow-auto rounded-lg bg-slate-900 p-4 font-mono text-xs text-slate-300">
                {file.content}
              </pre>
            </div>
          ) : showRaw ? (
            // Raw INI display
            <pre className="max-h-[60vh] overflow-auto rounded-lg bg-slate-900 p-4 font-mono text-xs text-slate-300">
              {file.content}
            </pre>
          ) : sections.length > 0 ? (
            // Parsed INI display with collapsible sections
            <div className="space-y-3">
              {filteredSections.map((sec) => {
                const style = categoryStyle(sec.section)
                const Icon = style.icon
                const isCollapsed = collapsedSections.has(sec.section)
                return (
                  <div key={sec.section} className={`rounded-lg border ${style.bg} overflow-hidden`}>
                    <button
                      onClick={() => toggleSection(sec.section)}
                      className="flex w-full items-center justify-between px-4 py-2.5 text-left"
                    >
                      <div className="flex items-center gap-2">
                        {isCollapsed ? (
                          <ChevronRight className="h-4 w-4 text-slate-500" />
                        ) : (
                          <ChevronDown className="h-4 w-4 text-slate-500" />
                        )}
                        <Icon className={`h-4 w-4 ${style.color}`} />
                        <span className={`font-mono text-sm font-semibold ${style.color}`}>
                          {sec.section}
                        </span>
                        <span className="text-xs text-slate-500">
                          ({sec.keys.length})
                        </span>
                      </div>
                    </button>
                    {!isCollapsed && (
                      <div className="border-t border-slate-800/50 p-3">
                        <div className="grid gap-1 sm:grid-cols-2">
                          {sec.keys.map((kv, i) => (
                            <div
                              key={i}
                              className="flex items-center gap-2 rounded px-2 py-1 font-mono text-xs hover:bg-slate-800/50"
                            >
                              <span className="shrink-0 text-slate-400">{kv.key}</span>
                              <span className="text-slate-600">=</span>
                              <span
                                className="truncate text-slate-200"
                                title={kv.value}
                              >
                                {kv.value || '(empty)'}
                              </span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )
              })}
              {filteredSections.length === 0 && search && (
                <div className="py-8 text-center text-sm text-slate-500">
                  No settings match "{search}"
                </div>
              )}
            </div>
          ) : file.content ? (
            <pre className="max-h-[60vh] overflow-auto rounded-lg bg-slate-900 p-4 font-mono text-xs text-slate-300">
              {file.content}
            </pre>
          ) : (
            <p className="text-sm text-slate-500">No content available</p>
          )}
        </div>
      </div>
    </div>
  )
}

function EditModal({ file, onClose, onSaved }: { file: ProfileFile; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(file.name)
  const [slicer, setSlicer] = useState(file.slicer || '')
  const [tags, setTags] = useState((file.tags || []).join(', '))
  const [notes, setNotes] = useState(file.notes || '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      const parsedTags = tags.split(',').map((t) => t.trim()).filter(Boolean)
      const res = await fetch(`/api/profile-files/${file.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, slicer, tags: parsedTags, notes }),
      })
      if (!res.ok) throw new Error('Failed to save')
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <form
        className="dark w-full max-w-md max-h-[90vh] overflow-y-auto rounded-lg border-2 border-slate-700 bg-slate-950 p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSave}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-mono text-lg font-semibold text-blue-400">[ edit_profile ]</h2>
          <button type="button" onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="mb-1 block text-xs text-slate-400">Display name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} className={inputClass} />
          </div>

          <div>
            <label className="mb-1 block text-xs text-slate-400">Slicer</label>
            <select value={slicer} onChange={(e) => setSlicer(e.target.value)} className={inputClass}>
              <option value="">—</option>
              <option value="prusaslicer">PrusaSlicer</option>
              <option value="orcaslicer">OrcaSlicer</option>
              <option value="cura">Cura</option>
              <option value="ideamaker">IdeaMaker</option>
              <option value="lychee">Lychee</option>
              <option value="other">Other</option>
            </select>
          </div>

          <div>
            <label className="mb-1 block text-xs text-slate-400">Tags (comma-separated)</label>
            <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="e.g. PLA, 0.2mm, draft" className={inputClass} />
          </div>

          <div>
            <label className="mb-1 block text-xs text-slate-400">Notes</label>
            <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={3} className={inputClass} />
          </div>

          {error && <p className="text-xs text-rose-400">{error}</p>}

          <div className="flex gap-3">
            <button type="submit" disabled={saving} className="rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50">
              {saving ? 'saving...' : 'save'}
            </button>
            <button type="button" onClick={onClose} className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700">
              cancel
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

interface ProfileOutput {
  type: 'print' | 'filament' | 'printer'
  name: string
  content: string
  filename: string
  settingCount: number
}

interface ConversionResult {
  content: string
  filename: string
  format: string
  warnings?: string[]
  unmapped?: string[]
  sections: number
  savedId?: string
  profiles?: ProfileOutput[]
}

function ConvertModal({ file, category, onClose, onConverted }: {
  file: ProfileFile | null
  category: Category
  onClose: () => void
  onConverted: () => void
}) {
  const [target, setTarget] = useState<'prusaslicer' | 'cura' | 'orcaslicer'>('prusaslicer')
  const [saveResult, setSaveResult] = useState(true)
  const [converting, setConverting] = useState(false)
  const [result, setResult] = useState<ConversionResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [uploadFile, setUploadFile] = useState<File | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const handleConvert = async () => {
    setConverting(true)
    setError(null)
    setResult(null)
    try {
      if (file) {
        const res = await fetch('/api/profile-files/convert', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: file.id, target, save: saveResult, category }),
        })
        if (!res.ok) {
          const err = await res.json().catch(() => ({ error: 'Conversion failed' }))
          throw new Error(err.error)
        }
        const data = await res.json()
        setResult(data)
        if (data.savedId) onConverted()
      } else if (uploadFile) {
        const formData = new FormData()
        formData.append('file', uploadFile)
        formData.append('target', target)
        formData.append('save', String(saveResult))
        formData.append('category', category)
        const res = await fetch('/api/profile-files/convert', { method: 'POST', body: formData })
        if (!res.ok) {
          const err = await res.json().catch(() => ({ error: 'Conversion failed' }))
          throw new Error(err.error)
        }
        const data = await res.json()
        setResult(data)
        if (data.savedId) onConverted()
      } else {
        setError('Select a file to convert or choose a profile from the list')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Conversion failed')
    } finally {
      setConverting(false)
    }
  }

  const handleDownloadResult = () => {
    if (!result) return
    const blob = new Blob([result.content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = result.filename
    a.click()
    URL.revokeObjectURL(url)
  }

  const handleDownloadProfile = (profile: ProfileOutput) => {
    const blob = new Blob([profile.content], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = profile.filename
    a.click()
    URL.revokeObjectURL(url)
  }

  const handleDownloadAllProfiles = () => {
    if (!result?.profiles) return
    result.profiles.forEach((p, i) => {
      setTimeout(() => handleDownloadProfile(p), i * 200)
    })
  }

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4" onClick={onClose}>
      <div
        className="dark flex w-full max-w-2xl max-h-[90vh] flex-col overflow-hidden rounded-lg border-2 border-slate-700 bg-slate-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <h2 className="font-mono text-lg font-semibold text-purple-400">[ profile_converter ]</h2>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-6 space-y-4">
          {/* Source */}
          <div>
            <label className="mb-1 block text-xs text-slate-400">Source</label>
            {file ? (
              <div className="rounded-lg bg-slate-900 p-3 text-sm text-slate-300">
                <span className="text-slate-400">From stored file:</span> {file.name}
                <span className="ml-2 text-xs text-slate-500">({file.filename})</span>
              </div>
            ) : (
              <div
                onClick={() => fileRef.current?.click()}
                className="cursor-pointer rounded-lg border-2 border-dashed border-slate-600 p-4 text-center transition-colors hover:border-purple-500"
              >
                {uploadFile ? (
                  <p className="text-sm text-slate-300">{uploadFile.name} ({formatSize(uploadFile.size)})</p>
                ) : (
                  <p className="text-sm text-slate-500">Click to select a profile file to convert</p>
                )}
                <input
                  ref={fileRef}
                  type="file"
                  accept=".ini,.json,.cfg,.conf,.toml"
                  className="hidden"
                  onChange={(e) => {
                    const f = e.target.files?.[0]
                    if (f) setUploadFile(f)
                  }}
                />
              </div>
            )}
          </div>

          {/* Target format */}
          <div>
            <label className="mb-1 block text-xs text-slate-400">Convert to</label>
            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => setTarget('prusaslicer')}
                className={`flex-1 rounded-lg border-2 px-4 py-2 text-sm font-medium transition-colors ${
                  target === 'prusaslicer'
                    ? 'border-orange-500 bg-orange-500/10 text-orange-400'
                    : 'border-slate-700 text-slate-400 hover:border-slate-600'
                }`}
              >
                PrusaSlicer / eufyMake Studio (.ini)
              </button>
              <button
                onClick={() => setTarget('orcaslicer')}
                className={`flex-1 rounded-lg border-2 px-4 py-2 text-sm font-medium transition-colors ${
                  target === 'orcaslicer'
                    ? 'border-emerald-500 bg-emerald-500/10 text-emerald-400'
                    : 'border-slate-700 text-slate-400 hover:border-slate-600'
                }`}
              >
                OrcaSlicer / BambuStudio (.json)
              </button>
              <button
                onClick={() => setTarget('cura')}
                className={`flex-1 rounded-lg border-2 px-4 py-2 text-sm font-medium transition-colors ${
                  target === 'cura'
                    ? 'border-blue-500 bg-blue-500/10 text-blue-400'
                    : 'border-slate-700 text-slate-400 hover:border-slate-600'
                }`}
              >
                Cura / AnkerMake Slicer (.inst.cfg)
              </button>
            </div>
          </div>

          {/* Save option */}
          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input
              type="checkbox"
              checked={saveResult}
              onChange={(e) => setSaveResult(e.target.checked)}
              className="h-4 w-4 rounded border-slate-600 text-purple-600"
            />
            Save converted file to Profile Files
          </label>

          {/* Convert button */}
          <div className="flex gap-3">
            <button
              onClick={handleConvert}
              disabled={converting || (!file && !uploadFile)}
              className="rounded-lg bg-purple-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-purple-500 disabled:opacity-50"
            >
              {converting ? 'converting...' : 'convert'}
            </button>
            <button onClick={onClose} className="rounded-lg bg-slate-800 px-4 py-2 font-mono text-sm font-medium text-slate-300 hover:bg-slate-700">
              close
            </button>
          </div>

          {error && <p className="text-xs text-rose-400">{error}</p>}

          {/* Result */}
          {result && (
            <div className="space-y-3 border-t border-slate-800 pt-4">
              <div className="flex items-center gap-2">
                <CheckCircle2 className="h-5 w-5 text-emerald-400" />
                <span className="text-sm text-emerald-400">Conversion complete</span>
                {result.savedId && (
                  <span className="rounded-full bg-emerald-900/30 px-2 py-0.5 text-xs text-emerald-300">saved to profile files</span>
                )}
              </div>

              {/* Multi-profile output (e.g. flat INI → OrcaSlicer) */}
              {result.profiles && result.profiles.length > 0 && (
                <div className="rounded-lg border border-purple-700/50 bg-purple-900/10 p-4">
                  <div className="mb-3 flex items-center justify-between">
                    <div>
                      <h4 className="text-sm font-semibold text-purple-300">Profiles to import</h4>
                      <p className="mt-0.5 text-xs text-slate-400">
                        Import each file separately in OrcaSlicer: File → Import → Import config
                      </p>
                    </div>
                    <button
                      onClick={handleDownloadAllProfiles}
                      className="flex items-center gap-1 rounded-lg bg-purple-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-purple-500"
                    >
                      <Download className="h-3.5 w-3.5" /> download all
                    </button>
                  </div>
                  <div className="space-y-2">
                    {result.profiles.map((p, i) => (
                      <div key={i} className="flex items-center gap-3 rounded-lg bg-slate-900 p-3">
                        <div className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                          p.type === 'print' ? 'bg-blue-900/50 text-blue-300' :
                          p.type === 'filament' ? 'bg-amber-900/50 text-amber-300' :
                          'bg-emerald-900/50 text-emerald-300'
                        }`}>
                          {p.type === 'print' ? 'P' : p.type === 'filament' ? 'F' : 'R'}
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <span className="truncate text-sm font-medium text-slate-200">{p.name}</span>
                            <span className="shrink-0 rounded-full bg-slate-800 px-1.5 py-0.5 text-[9px] text-slate-400">
                              {p.type}
                            </span>
                          </div>
                          <div className="mt-0.5 flex items-center gap-2 text-xs text-slate-500">
                            <span>{p.settingCount} settings</span>
                            <span>·</span>
                            <code className="truncate">{p.filename}</code>
                          </div>
                        </div>
                        <button
                          onClick={() => handleDownloadProfile(p)}
                          className="flex shrink-0 items-center gap-1 rounded bg-slate-700 px-2 py-1 text-xs text-slate-300 hover:bg-slate-600"
                        >
                          <Download className="h-3 w-3" /> download
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Single file output (when no multi-profile) */}
              {(!result.profiles || result.profiles.length === 0) && (
                <div className="rounded-lg bg-slate-900 p-3">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-xs text-slate-400">Output: {result.filename}</span>
                    <button
                      onClick={handleDownloadResult}
                      className="flex items-center gap-1 rounded bg-slate-700 px-2 py-1 text-xs text-slate-300 hover:bg-slate-600"
                    >
                      <Download className="h-3 w-3" /> download
                    </button>
                  </div>
                  <pre className="max-h-48 overflow-auto rounded bg-slate-950 p-2 font-mono text-[10px] text-slate-300">
                    {result.content.slice(0, 2000)}{result.content.length > 2000 ? '\n... (truncated)' : ''}
                  </pre>
                </div>
              )}

              {result.warnings && result.warnings.length > 0 && (
                <div className="rounded-lg border border-amber-700/50 bg-amber-900/20 p-3">
                  <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold text-amber-400">
                    <AlertTriangle className="h-3.5 w-3.5" /> Warnings ({result.warnings.length})
                  </div>
                  <ul className="ml-4 list-disc space-y-0.5 text-xs text-amber-300/80">
                    {result.warnings.map((w, i) => <li key={i}>{w}</li>)}
                  </ul>
                </div>
              )}

              {result.unmapped && result.unmapped.length > 0 && (
                <div className="rounded-lg border border-slate-700 bg-slate-900/50 p-3">
                  <div className="mb-1 text-xs font-semibold text-slate-400">
                    Unmapped settings ({result.unmapped.length})
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {result.unmapped.map((u, i) => (
                      <span key={i} className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-500">{u}</span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Info */}
          {!result && (
            <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-3 text-xs text-slate-400">
              <p className="mb-1 font-semibold text-slate-300">How it works:</p>
              <ul className="ml-4 list-disc space-y-0.5">
                <li>Upload or select a slicer profile file (.ini, .inst.cfg, .def.json)</li>
                <li>The source format is auto-detected from the file content</li>
                <li>Settings are mapped to the target format's equivalents</li>
                <li>Some settings may not have direct equivalents — these are listed as unmapped</li>
                <li>Review the warnings and unmapped settings before using the converted profile</li>
              </ul>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
