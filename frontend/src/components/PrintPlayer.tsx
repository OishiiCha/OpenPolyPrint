import { useEffect, useRef, useState, useCallback, useMemo } from 'react'
import { Play, Pause, SkipBack, SkipForward, Sparkles, Loader2, Layers, Images } from 'lucide-react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'

interface Segment {
  lineNum: number
  layer: number
  x: number
  y: number
  z: number
  e: number
  extruding: boolean
  feedrate: number
  distance: number
  duration: number
  elapsedTime: number
  gcode: string
}

interface PrintPlayerProps {
  gcodeId: string
  timelapseDir: string  // e.g. "camera1-20240101-1200_frames"
  intervalSec: number  // timelapse capture interval
  printerName: string
  filename: string
  apiKey: string
}

export function PrintPlayer({
  gcodeId,
  timelapseDir,
  intervalSec,
  printerName,
  filename,
  apiKey,
}: PrintPlayerProps) {
  const [segments, setSegments] = useState<Segment[]>([])
  const [loading, setLoading] = useState(true)
  const [playing, setPlaying] = useState(false)
  const [progress, setProgress] = useState(0) // 0-1
  const [currentSegment, setCurrentSegment] = useState<Segment | null>(null)
  const [totalTime, setTotalTime] = useState(0)
  const [frameList, setFrameList] = useState<string[]>([])
  const [currentFrame, setCurrentFrame] = useState<string | null>(null)
  const [analyzing, setAnalyzing] = useState(false)
  const [analysis, setAnalysis] = useState<string | null>(null)
  const [analysisError, setAnalysisError] = useState<string | null>(null)
  const [selectedFrameIdx, setSelectedFrameIdx] = useState<number | null>(null)
  const [showFrames, setShowFrames] = useState(true)

  // Compute unique layers and their time ranges from segments
  const layerInfo = useMemo(() => {
    if (segments.length === 0) return []
    const map = new Map<number, { start: number; end: number; count: number }>()
    for (const seg of segments) {
      const existing = map.get(seg.layer)
      if (existing) {
        existing.end = seg.elapsedTime
        existing.count++
      } else {
        map.set(seg.layer, { start: seg.elapsedTime, end: seg.elapsedTime, count: 1 })
      }
    }
    return Array.from(map.entries())
      .map(([layer, info]) => ({ layer, ...info }))
      .sort((a, b) => a.layer - b.layer)
  }, [segments])

  const mountRef = useRef<HTMLDivElement>(null)
  const sceneRef = useRef<THREE.Scene | null>(null)
  const rendererRef = useRef<THREE.WebGLRenderer | null>(null)
  const cameraRef = useRef<THREE.PerspectiveCamera | null>(null)
  const controlsRef = useRef<OrbitControls | null>(null)
  const toolheadRef = useRef<THREE.Mesh | null>(null)
  const pathRef = useRef<THREE.LineSegments | null>(null)
  const playedPathRef = useRef<THREE.LineSegments | null>(null)
  const rafRef = useRef<number>(0)
  const playStartRef = useRef<number>(0)
  const playProgressRef = useRef<number>(0)
  const animationFrameRef = useRef<number>(0)

  // Load G-code timeline
  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const res = await fetch(`/api/gcode/${encodeURIComponent(gcodeId)}/timeline`)
        if (!res.ok) throw new Error('failed to load timeline')
        const data = await res.json()
        if (cancelled) return
        setSegments(data)
        if (data.length > 0) {
          setTotalTime(data[data.length - 1].elapsedTime)
          setCurrentSegment(data[0])
        }
        setLoading(false)
      } catch (e) {
        console.error(e)
        setLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [gcodeId])

  // Load frame list
  useEffect(() => {
    if (!timelapseDir) return
    fetch(`/api/timelapse-frames/${encodeURIComponent(timelapseDir)}`)
      .then((r) => r.json())
      .then((data) => {
        if (Array.isArray(data)) setFrameList(data)
      })
      .catch(() => {})
  }, [timelapseDir])

  // Initialize Three.js scene
  useEffect(() => {
    if (!mountRef.current || loading) return

    const el = mountRef.current
    const width = el.clientWidth
    const height = el.clientHeight || width * 0.6

    const scene = new THREE.Scene()
    scene.background = new THREE.Color(0x0f172a)
    sceneRef.current = scene

    const camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 10000)
    camera.up.set(0, 0, 1)
    cameraRef.current = camera

    const renderer = new THREE.WebGLRenderer({ antialias: true })
    renderer.setSize(width, height)
    rendererRef.current = renderer
    el.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controlsRef.current = controls

    // Bed grid
    const bed = new THREE.GridHelper(220, 22, 0x1e293b, 0x1e293b)
    bed.rotation.x = Math.PI / 2
    bed.position.set(110, 110, 0)
    scene.add(bed)

    // Toolhead indicator (small sphere)
    const toolheadGeo = new THREE.SphereGeometry(2, 16, 16)
    const toolheadMat = new THREE.MeshBasicMaterial({ color: 0xef4444 })
    const toolhead = new THREE.Mesh(toolheadGeo, toolheadMat)
    toolheadRef.current = toolhead
    scene.add(toolhead)

    // Set camera position
    camera.position.set(200, -200, 200)
    controls.target.set(110, 110, 50)
    controls.update()

    const animate = () => {
      rafRef.current = requestAnimationFrame(animate)
      controls.update()
      renderer.render(scene, camera)
    }
    animate()

    const handleResize = () => {
      const w = el.clientWidth
      const h = el.clientHeight || w * 0.6
      camera.aspect = w / h
      camera.updateProjectionMatrix()
      renderer.setSize(w, h)
    }
    window.addEventListener('resize', handleResize)

    return () => {
      cancelAnimationFrame(rafRef.current)
      window.removeEventListener('resize', handleResize)
      controls.dispose()
      renderer.dispose()
      if (renderer.domElement.parentNode) {
        el.removeChild(renderer.domElement)
      }
    }
  }, [loading])

  // Build path geometry from segments
  useEffect(() => {
    if (!sceneRef.current || segments.length === 0) return

    // Remove old paths
    if (pathRef.current) {
      sceneRef.current.remove(pathRef.current)
      pathRef.current.geometry.dispose()
    }
    if (playedPathRef.current) {
      sceneRef.current.remove(playedPathRef.current)
      playedPathRef.current.geometry.dispose()
    }

    const positions: number[] = []
    const colors: number[] = []
    const colorTravel = new THREE.Color(0x64748b)
    const colorExtrude = new THREE.Color(0x3b82f6)

    for (let i = 1; i < segments.length; i++) {
      const prev = segments[i - 1]
      const curr = segments[i]
      const c = curr.extruding ? colorExtrude : colorTravel
      positions.push(prev.x, prev.y, prev.z, curr.x, curr.y, curr.z)
      colors.push(c.r, c.g, c.b, c.r, c.g, c.b)
    }

    if (positions.length >= 6) {
      const geometry = new THREE.BufferGeometry()
      geometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
      geometry.setAttribute('color', new THREE.Float32BufferAttribute(colors, 3))
      const material = new THREE.LineBasicMaterial({
        vertexColors: true,
        transparent: true,
        opacity: 0.3,
      })
      const path = new THREE.LineSegments(geometry, material)
      pathRef.current = path
      sceneRef.current.add(path)
    }

    // Create played path (will be updated as progress changes)
    const playedGeo = new THREE.BufferGeometry()
    playedGeo.setAttribute('position', new THREE.Float32BufferAttribute([], 3))
    playedGeo.setAttribute('color', new THREE.Float32BufferAttribute([], 3))
    const playedMat = new THREE.LineBasicMaterial({
      vertexColors: true,
      transparent: true,
      opacity: 0.9,
    })
    const playedPath = new THREE.LineSegments(playedGeo, playedMat)
    playedPathRef.current = playedPath
    sceneRef.current.add(playedPath)
  }, [segments])

  // Update visualization based on progress
  const updateVisualization = useCallback((prog: number) => {
    if (segments.length === 0 || !toolheadRef.current) return

    const elapsed = prog * totalTime
    // Find current segment
    let segIdx = 0
    for (let i = 0; i < segments.length; i++) {
      if (segments[i].elapsedTime <= elapsed) {
        segIdx = i
      } else {
        break
      }
    }
    const seg = segments[segIdx]
    setCurrentSegment(seg)

    // Update toolhead position
    if (toolheadRef.current) {
      toolheadRef.current.position.set(seg.x, seg.y, seg.z)
    }

    // Update played path
    if (playedPathRef.current && pathRef.current) {
      const playedPositions: number[] = []
      const playedColors: number[] = []
      const colorTravel = new THREE.Color(0x64748b)
      const colorExtrude = new THREE.Color(0x3b82f6)

      for (let i = 1; i <= segIdx; i++) {
        const prev = segments[i - 1]
        const curr = segments[i]
        const c = curr.extruding ? colorExtrude : colorTravel
        playedPositions.push(prev.x, prev.y, prev.z, curr.x, curr.y, curr.z)
        playedColors.push(c.r, c.g, c.b, c.r, c.g, c.b)
      }

      if (playedPositions.length >= 6) {
        playedPathRef.current.geometry.setAttribute(
          'position',
          new THREE.Float32BufferAttribute(playedPositions, 3)
        )
        playedPathRef.current.geometry.setAttribute(
          'color',
          new THREE.Float32BufferAttribute(playedColors, 3)
        )
      } else {
        playedPathRef.current.geometry.setAttribute(
          'position',
          new THREE.Float32BufferAttribute([], 3)
        )
      }
    }

    // Update current frame
    if (frameList.length > 0 && timelapseDir) {
      const frameIdx = Math.floor(elapsed / intervalSec)
      const clampedIdx = Math.min(frameIdx, frameList.length - 1)
      if (clampedIdx >= 0) {
        setCurrentFrame(`/recordings/timelapse/${timelapseDir}/${frameList[clampedIdx]}`)
      }
    }
  }, [segments, totalTime, frameList, timelapseDir, intervalSec])

  // Playback loop
  useEffect(() => {
    if (!playing) {
      if (animationFrameRef.current) {
        cancelAnimationFrame(animationFrameRef.current)
      }
      return
    }

    playStartRef.current = performance.now()
    const startProgress = playProgressRef.current

    const tick = () => {
      const elapsed = (performance.now() - playStartRef.current) / 1000 // real seconds
      // Play at 10x speed for timelapse review
      const playSpeed = 10
      const printElapsed = elapsed * playSpeed
      const prog = Math.min(startProgress + printElapsed / totalTime, 1)

      setProgress(prog)
      playProgressRef.current = prog
      updateVisualization(prog)

      if (prog >= 1) {
        setPlaying(false)
        return
      }
      animationFrameRef.current = requestAnimationFrame(tick)
    }
    animationFrameRef.current = requestAnimationFrame(tick)

    return () => {
      if (animationFrameRef.current) {
        cancelAnimationFrame(animationFrameRef.current)
      }
    }
  }, [playing, totalTime, updateVisualization])

  // Update when scrubbing
  const handleScrub = (prog: number) => {
    setProgress(prog)
    playProgressRef.current = prog
    setSelectedFrameIdx(null) // clear manual frame selection when scrubbing
    updateVisualization(prog)
  }

  // Jump to a specific frame by index
  const jumpToFrame = (idx: number) => {
    if (frameList.length === 0) return
    const clampedIdx = Math.max(0, Math.min(idx, frameList.length - 1))
    setSelectedFrameIdx(clampedIdx)
    setPlaying(false)
    // Calculate progress from frame index
    const elapsed = clampedIdx * intervalSec
    const prog = totalTime > 0 ? Math.min(elapsed / totalTime, 1) : 0
    setProgress(prog)
    playProgressRef.current = prog
    updateVisualization(prog)
  }

  // Jump to a specific layer
  const jumpToLayer = (layer: number) => {
    const info = layerInfo.find((l) => l.layer === layer)
    if (!info || totalTime <= 0) return
    // Jump to the start of the layer
    const prog = Math.min(info.start / totalTime, 1)
    setPlaying(false)
    setSelectedFrameIdx(null)
    setProgress(prog)
    playProgressRef.current = prog
    updateVisualization(prog)
  }

  // AI analysis
  const analyzeFrame = async () => {
    if (!apiKey) {
      setAnalysisError('Please enter a Gemini API key in Settings first')
      return
    }
    setAnalyzing(true)
    setAnalysis(null)
    setAnalysisError(null)
    try {
      const elapsedSec = progress * totalTime
      const frameNum = selectedFrameIdx !== null ? selectedFrameIdx + 1 : undefined
      const res = await fetch('/api/ai/analyze', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          apiKey,
          frameDir: timelapseDir,
          elapsedSec,
          intervalSec,
          gcodeId,
          printerName,
          filename,
          ...(frameNum ? { frameNum } : {}),
        }),
      })
      if (!res.ok) {
        const err = await res.text()
        throw new Error(err)
      }
      const data = await res.json()
      setAnalysis(data.analysis || data.raw || 'No analysis returned')
    } catch (e) {
      setAnalysisError(e instanceof Error ? e.message : 'Analysis failed')
    } finally {
      setAnalyzing(false)
    }
  }

  const formatTime = (sec: number) => {
    const h = Math.floor(sec / 3600)
    const m = Math.floor((sec % 3600) / 60)
    const s = Math.floor(sec % 60)
    if (h > 0) return `${h}h ${m}m ${s}s`
    if (m > 0) return `${m}m ${s}s`
    return `${s}s`
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center rounded-xl border border-slate-200 dark:border-slate-800">
        <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Main view: video + 3D visualizer side by side */}
      <div className="grid gap-4 lg:grid-cols-2">
        {/* Camera frame */}
        <div className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
          <div className="border-b border-slate-200 bg-slate-50 px-3 py-2 dark:border-slate-800 dark:bg-slate-900">
            <span className="font-mono text-xs text-slate-500 dark:text-slate-400">Camera frame</span>
          </div>
          <div className="relative aspect-video bg-black">
            {currentFrame ? (
              <img src={currentFrame} alt="Print frame" className="h-full w-full object-contain" />
            ) : (
              <div className="flex h-full items-center justify-center text-slate-500">
                <p className="font-mono text-xs">No frames available</p>
              </div>
            )}
          </div>
        </div>

        {/* G-code visualizer */}
        <div className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
          <div className="border-b border-slate-200 bg-slate-50 px-3 py-2 dark:border-slate-800 dark:bg-slate-900">
            <span className="font-mono text-xs text-slate-500 dark:text-slate-400">G-code visualizer</span>
          </div>
          <div ref={mountRef} className="aspect-video bg-slate-950" />
        </div>
      </div>

      {/* Playback controls */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <div className="flex items-center gap-3">
          <button
            onClick={() => { handleScrub(0); setPlaying(false) }}
            className="rounded-lg p-2 text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            <SkipBack className="h-5 w-5" />
          </button>
          <button
            onClick={() => setPlaying(!playing)}
            className="rounded-lg bg-blue-600 p-2 text-white hover:bg-blue-500"
          >
            {playing ? <Pause className="h-5 w-5" /> : <Play className="h-5 w-5" />}
          </button>
          <button
            onClick={() => { handleScrub(1); setPlaying(false) }}
            className="rounded-lg p-2 text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            <SkipForward className="h-5 w-5" />
          </button>

          {/* Scrubber */}
          <input
            type="range"
            min={0}
            max={1}
            step={0.001}
            value={progress}
            onChange={(e) => { handleScrub(parseFloat(e.target.value)); setPlaying(false) }}
            className="flex-1 accent-blue-600"
          />

          {/* Time display */}
          <div className="font-mono text-xs text-slate-500 dark:text-slate-400 whitespace-nowrap">
            {formatTime(progress * totalTime)} / {formatTime(totalTime)}
          </div>
        </div>

        {/* Current segment info */}
        {currentSegment && (
          <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div className="rounded-lg bg-slate-100 px-3 py-1.5 dark:bg-slate-800">
              <p className="font-mono text-[10px] text-slate-400">Layer</p>
              <p className="font-mono text-sm font-bold text-slate-900 dark:text-white">{currentSegment.layer}</p>
            </div>
            <div className="rounded-lg bg-slate-100 px-3 py-1.5 dark:bg-slate-800">
              <p className="font-mono text-[10px] text-slate-400">Position</p>
              <p className="font-mono text-sm font-bold text-slate-900 dark:text-white">
                {currentSegment.x.toFixed(1)}, {currentSegment.y.toFixed(1)}, {currentSegment.z.toFixed(1)}
              </p>
            </div>
            <div className="rounded-lg bg-slate-100 px-3 py-1.5 dark:bg-slate-800">
              <p className="font-mono text-[10px] text-slate-400">Feedrate</p>
              <p className="font-mono text-sm font-bold text-slate-900 dark:text-white">{currentSegment.feedrate.toFixed(0)} mm/min</p>
            </div>
            <div className="rounded-lg bg-slate-100 px-3 py-1.5 dark:bg-slate-800">
              <p className="font-mono text-[10px] text-slate-400">Status</p>
              <p className={`font-mono text-sm font-bold ${currentSegment.extruding ? 'text-blue-500' : 'text-slate-400'}`}>
                {currentSegment.extruding ? 'Extruding' : 'Traveling'}
              </p>
            </div>
          </div>
        )}

        {/* Current G-code line */}
        {currentSegment && (
          <div className="mt-2 rounded-lg bg-slate-950 px-3 py-2">
            <p className="font-mono text-xs text-slate-400">
              <span className="text-slate-600">L{currentSegment.lineNum}:</span> {currentSegment.gcode}
            </p>
          </div>
        )}
      </div>

      {/* Frame gallery + layer selector */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <div className="mb-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Images className="h-5 w-5 text-blue-500" />
            <h3 className="font-semibold text-slate-900 dark:text-white">Frames</h3>
            <span className="font-mono text-xs text-slate-400">
              {frameList.length} frames · {layerInfo.length} layers
            </span>
          </div>
          <div className="flex items-center gap-3">
            {/* Layer selector */}
            {layerInfo.length > 0 && (
              <div className="flex items-center gap-2">
                <Layers className="h-4 w-4 text-slate-400" />
                <select
                  value={currentSegment?.layer ?? 0}
                  onChange={(e) => jumpToLayer(parseInt(e.target.value))}
                  className="rounded-lg border border-slate-300 px-2 py-1 text-xs dark:border-slate-700 dark:bg-slate-800 dark:text-white"
                >
                  {layerInfo.map((l) => (
                    <option key={l.layer} value={l.layer}>
                      Layer {l.layer} ({l.count} moves)
                    </option>
                  ))}
                </select>
              </div>
            )}
            <button
              onClick={() => setShowFrames(!showFrames)}
              className="rounded-lg border border-slate-300 px-3 py-1 text-xs text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
            >
              {showFrames ? 'Hide' : 'Show'} gallery
            </button>
          </div>
        </div>

        {showFrames && frameList.length > 0 && (
          <div className="grid max-h-64 grid-cols-4 gap-2 overflow-y-auto sm:grid-cols-6 md:grid-cols-8 lg:grid-cols-10">
            {frameList.map((frame, idx) => {
              const frameUrl = `/recordings/timelapse/${timelapseDir}/${frame}`
              const isSelected = selectedFrameIdx === idx
              const isCurrent = !isSelected && currentFrame === frameUrl
              return (
                <button
                  key={frame}
                  onClick={() => jumpToFrame(idx)}
                  className={`relative aspect-video overflow-hidden rounded-lg border-2 transition-all ${
                    isSelected
                      ? 'border-purple-500 ring-2 ring-purple-500/30'
                      : isCurrent
                        ? 'border-blue-500'
                        : 'border-slate-200 hover:border-blue-400 dark:border-slate-700'
                  }`}
                  title={`Frame ${idx + 1} · ${formatTime(idx * intervalSec)}`}
                >
                  <img
                    src={frameUrl}
                    alt={`Frame ${idx + 1}`}
                    loading="lazy"
                    className="h-full w-full object-cover"
                  />
                  <span className="absolute bottom-0 left-0 right-0 bg-black/60 px-1 py-0.5 text-center font-mono text-[8px] text-white">
                    {idx + 1}
                  </span>
                </button>
              )
            })}
          </div>
        )}

        {showFrames && frameList.length === 0 && (
          <p className="font-mono text-xs text-slate-400">No frames in this timelapse.</p>
        )}
      </div>

      {/* AI Analysis */}
      <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800">
        <div className="mb-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Sparkles className="h-5 w-5 text-purple-500" />
            <h3 className="font-semibold text-slate-900 dark:text-white">AI Analysis</h3>
          </div>
          <button
            onClick={analyzeFrame}
            disabled={analyzing || !currentFrame}
            className="flex items-center gap-2 rounded-lg bg-purple-600 px-4 py-2 text-sm font-medium text-white hover:bg-purple-500 disabled:opacity-50"
          >
            {analyzing ? (
              <><Loader2 className="h-4 w-4 animate-spin" /> Analyzing...</>
            ) : selectedFrameIdx !== null ? (
              <><Sparkles className="h-4 w-4" /> Analyze frame {selectedFrameIdx + 1}</>
            ) : (
              <><Sparkles className="h-4 w-4" /> Analyze current frame</>
            )}
          </button>
        </div>

        {!apiKey && (
          <p className="font-mono text-xs text-amber-500">
            No Gemini API key set. Add one in Settings to enable AI analysis.
          </p>
        )}

        {analysisError && (
          <div className="rounded-lg bg-rose-50 p-3 font-mono text-xs text-rose-600 dark:bg-rose-900/20 dark:text-rose-400">
            {analysisError}
          </div>
        )}

        {analysis && (
          <div className="rounded-lg bg-slate-50 p-4 dark:bg-slate-900">
            <pre className="whitespace-pre-wrap font-mono text-xs text-slate-700 dark:text-slate-300">
              {analysis}
            </pre>
          </div>
        )}
      </div>
    </div>
  )
}
