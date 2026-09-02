import { useEffect, useRef, useState, useCallback } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { Box, Eye, Grid3x3, Layers, Maximize2, Minimize2, RotateCcw } from 'lucide-react'

interface Layer {
  z: number
  segments: { x1: number; y1: number; x2: number; y2: number; z: number }[]
}

export interface GCode3DViewerProps {
  gcode: string
  bedWidth?: number
  bedDepth?: number
  bedHeight?: number
  /** Print progress 0-1 (optional, for live tracking) */
  progress?: number
  /** Current layer from printer (optional) */
  currentLayer?: number
  /** Total layers from printer (optional) */
  totalLayerCount?: number
  className?: string
}

export function GCode3DViewer({
  gcode,
  bedWidth = 220,
  bedDepth = 220,
  progress = 0,
  currentLayer: externalLayer,
  totalLayerCount,
  className = '',
}: GCode3DViewerProps) {
  const mountRef = useRef<HTMLDivElement>(null)
  const sceneRef = useRef<THREE.Scene | null>(null)
  const cameraRef = useRef<THREE.PerspectiveCamera | null>(null)
  const rendererRef = useRef<THREE.WebGLRenderer | null>(null)
  const controlsRef = useRef<OrbitControls | null>(null)
  const printedGroupRef = useRef<THREE.Group | null>(null)
  const remainingGroupRef = useRef<THREE.Group | null>(null)
  const rafRef = useRef<number>(0)

  const layersRef = useRef<Layer[]>([])
  const totalSegmentsRef = useRef<number>(0)
  const [totalLayers, setTotalLayers] = useState(0)
  const [viewLayer, setViewLayer] = useState(0)
  const [isPreviewing, setIsPreviewing] = useState(false)
  const [followProgress, setFollowProgress] = useState(true)
  const [showBed, setShowBed] = useState(true)
  const [showPrinted] = useState(true)
  const [showRemaining] = useState(true)
  const [expanded, setExpanded] = useState(false)

  // Parse G-code into layers
  const parseGcode = useCallback((text: string): Layer[] => {
    const lines = text.split('\n')
    let curX = 0, curY = 0, curZ = 0, curE = 0
    let lastZ = -1
    let layerIdx = -1
    const parsedLayers: Layer[] = []
    let absPos = true

    for (const raw of lines) {
      const line = raw.trim()
      if (!line || line.startsWith(';') || line.startsWith('(')) continue

      const semi = line.indexOf(';')
      const clean = semi >= 0 ? line.substring(0, semi).trim() : line
      const parts = clean.split(/\s+/)
      const cmd = parts[0].toUpperCase()

      if (cmd === 'G90') { absPos = true; continue }
      if (cmd === 'G91') { absPos = false; continue }
      if (cmd === 'G28') { curX = 0; curY = 0; curZ = 0; curE = 0; continue }
      if (cmd === 'G92') {
        for (let j = 1; j < parts.length; j++) {
          const p = parts[j]
          if (p[0] === 'E') curE = parseFloat(p.substring(1))
          if (p[0] === 'Z') curZ = parseFloat(p.substring(1))
        }
        continue
      }
      if (cmd !== 'G0' && cmd !== 'G1') continue

      let newX = curX, newY = curY, newZ = curZ, newE = curE
      for (let k = 1; k < parts.length; k++) {
        const param = parts[k]
        const axis = param[0].toUpperCase()
        const val = parseFloat(param.substring(1))
        if (isNaN(val)) continue
        if (absPos) {
          if (axis === 'X') newX = val
          if (axis === 'Y') newY = val
          if (axis === 'Z') newZ = val
          if (axis === 'E') newE = val
        } else {
          if (axis === 'X') newX += val
          if (axis === 'Y') newY += val
          if (axis === 'Z') newZ += val
          if (axis === 'E') newE += val
        }
      }

      // Detect new layer by Z change
      if (Math.abs(newZ - lastZ) > 0.01) {
        lastZ = newZ
        layerIdx++
        parsedLayers.push({ z: newZ, segments: [] })
      }

      // Only draw extrusion moves
      const isExtruding = newE > curE
      if (isExtruding && layerIdx >= 0) {
        parsedLayers[layerIdx].segments.push({
          x1: curX, y1: curY, x2: newX, y2: newY, z: newZ,
        })
      }

      curX = newX; curY = newY; curZ = newZ; curE = newE
    }

    // Remove empty layers at start
    while (parsedLayers.length > 0 && parsedLayers[0].segments.length === 0) {
      parsedLayers.shift()
    }

    return parsedLayers
  }, [])

  // Initialize Three.js scene
  useEffect(() => {
    if (!mountRef.current) return
    const el = mountRef.current
    const width = el.clientWidth || 400
    const height = el.clientHeight || 300

    const scene = new THREE.Scene()
    scene.background = new THREE.Color(0x0f172a)
    sceneRef.current = scene

    const camera = new THREE.PerspectiveCamera(50, width / height, 0.1, 5000)
    camera.position.set(180, -180, 280)
    camera.lookAt(0, 0, 0)
    cameraRef.current = camera

    const renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: 'high-performance' })
    renderer.setSize(width, height)
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    rendererRef.current = renderer
    el.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controls.dampingFactor = 0.08
    controls.maxPolarAngle = Math.PI / 1.8
    controls.minPolarAngle = 0.1
    controls.minDistance = 30
    controls.maxDistance = 1000
    controls.target.set(0, 0, 0)
    controlsRef.current = controls

    // Lights
    scene.add(new THREE.AmbientLight(0xffffff, 0.6))
    const dirLight = new THREE.DirectionalLight(0xffffff, 0.8)
    dirLight.position.set(100, 200, 150)
    scene.add(dirLight)

    // Bed
    const bedGeo = new THREE.PlaneGeometry(bedWidth, bedDepth)
    const bedMat = new THREE.MeshBasicMaterial({
      color: 0x1a1a2e, side: THREE.DoubleSide, transparent: true, opacity: 0.5,
      polygonOffset: true, polygonOffsetFactor: 1, polygonOffsetUnits: 1,
    })
    const bedMesh = new THREE.Mesh(bedGeo, bedMat)
    bedMesh.rotation.x = -Math.PI / 2
    scene.add(bedMesh)

    const grid = new THREE.GridHelper(Math.max(bedWidth, bedDepth), 10, 0x334155, 0x1e293b)
    grid.position.y = 0.02
    ;(grid.material as THREE.Material).transparent = true
    ;(grid.material as THREE.Material).opacity = 0.5
    scene.add(grid)

    // Bed frame
    const frameGeo = new THREE.BufferGeometry()
    const hX = bedWidth / 2, hY = bedDepth / 2
    frameGeo.setAttribute('position', new THREE.Float32BufferAttribute([
      -hX, 0.02, -hY,  hX, 0.02, -hY,
       hX, 0.02, -hY,  hX, 0.02,  hY,
       hX, 0.02,  hY, -hX, 0.02,  hY,
      -hX, 0.02,  hY, -hX, 0.02, -hY,
    ], 3))
    const frame = new THREE.LineSegments(frameGeo, new THREE.LineBasicMaterial({ color: 0x475569 }))
    scene.add(frame)

    // Groups for printed and remaining
    const printedGroup = new THREE.Group()
    const remainingGroup = new THREE.Group()
    scene.add(printedGroup)
    scene.add(remainingGroup)
    printedGroupRef.current = printedGroup
    remainingGroupRef.current = remainingGroup

    const animate = () => {
      rafRef.current = requestAnimationFrame(animate)
      controls.update()
      renderer.render(scene, camera)
    }
    animate()

    const handleResize = () => {
      const w = el.clientWidth || 400
      const h = el.clientHeight || 300
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
  }, [bedWidth, bedDepth])

  // Parse G-code when it changes
  useEffect(() => {
    if (!gcode) return
    const layers = parseGcode(gcode)
    layersRef.current = layers
    let total = 0
    for (const l of layers) total += l.segments.length
    totalSegmentsRef.current = total
    setTotalLayers(layers.length)
    setViewLayer(0)
    setIsPreviewing(false)
  }, [gcode, parseGcode])

  // Render layers based on current view layer
  const renderLayers = useCallback((layerIdx: number, segCount?: number) => {
    const printedGroup = printedGroupRef.current
    const remainingGroup = remainingGroupRef.current
    if (!printedGroup || !remainingGroup) return

    // Clear previous
    while (printedGroup.children.length > 0) {
      const c = printedGroup.children[0] as THREE.LineSegments
      printedGroup.remove(c)
      c.geometry?.dispose()
      ;(c.material as THREE.Material)?.dispose()
    }
    while (remainingGroup.children.length > 0) {
      const c = remainingGroup.children[0] as THREE.LineSegments
      remainingGroup.remove(c)
      c.geometry?.dispose()
      ;(c.material as THREE.Material)?.dispose()
    }

    const layers = layersRef.current
    if (layers.length === 0) return

    const hX = bedWidth / 2, hY = bedDepth / 2

    for (let i = 0; i < layers.length; i++) {
      const layer = layers[i]
      if (layer.segments.length === 0) continue

      const positions: number[] = []
      let segsToDraw: number

      if (i < layerIdx) {
        segsToDraw = layer.segments.length
      } else if (i === layerIdx) {
        segsToDraw = segCount !== undefined ? Math.min(segCount, layer.segments.length) : layer.segments.length
      } else {
        // Remaining
        if (showRemaining) {
          for (const seg of layer.segments) {
            positions.push(seg.x1 - hX, seg.z, hY - seg.y1, seg.x2 - hX, seg.z, hY - seg.y2)
          }
          if (positions.length === 0) continue
          const geo = new THREE.BufferGeometry()
          geo.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
          const mat = new THREE.LineBasicMaterial({ color: 0x475569, transparent: true, opacity: 0.3 })
          remainingGroup.add(new THREE.LineSegments(geo, mat))
        }
        continue
      }

      if (!showPrinted && i < layerIdx) continue

      for (let s = 0; s < segsToDraw; s++) {
        const seg = layer.segments[s]
        positions.push(seg.x1 - hX, seg.z, hY - seg.y1, seg.x2 - hX, seg.z, hY - seg.y2)
      }
      if (positions.length === 0) continue

      const geo = new THREE.BufferGeometry()
      geo.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))

      // Color: completed layers use hue gradient, current partial layer is bright green
      let color: THREE.Color
      if (i === layerIdx && segsToDraw < layer.segments.length) {
        color = new THREE.Color(0x00ff88) // Active layer
      } else {
        const hue = (i / Math.max(layers.length, 1)) * 300
        color = new THREE.Color().setHSL(hue / 360, 0.7, 0.5)
      }
      const mat = new THREE.LineBasicMaterial({ color, linewidth: 2 })
      printedGroup.add(new THREE.LineSegments(geo, mat))
    }
  }, [bedWidth, bedDepth, showPrinted, showRemaining])

  // Re-render when view layer or toggles change
  useEffect(() => {
    if (layersRef.current.length === 0) return
    if (!isPreviewing && progress > 0 && progress < 1) {
      // Render with print progress
      const targetSeg = Math.floor(progress * totalSegmentsRef.current)
      let segCount = 0
      let renderLayer = 0
      let segInLayer = 0
      for (let i = 0; i < layersRef.current.length; i++) {
        const layerSegs = layersRef.current[i].segments.length
        if (segCount + layerSegs >= targetSeg) {
          renderLayer = i
          segInLayer = targetSeg - segCount
          break
        }
        segCount += layerSegs
        renderLayer = i
        segInLayer = layerSegs
      }
      renderLayers(renderLayer, segInLayer)
      setViewLayer(renderLayer)
    } else {
      renderLayers(viewLayer)
    }
  }, [viewLayer, isPreviewing, progress, renderLayers])

  // Follow live print progress
  useEffect(() => {
    if (!followProgress || isPreviewing) return
    if (externalLayer !== undefined && totalLayerCount && totalLayerCount > 0) {
      if (totalLayers > 0 && totalLayerCount !== totalLayers) {
        const scaled = Math.round((externalLayer / totalLayerCount) * totalLayers)
        setViewLayer(scaled)
      } else if (totalLayers > 0) {
        setViewLayer(Math.min(externalLayer, totalLayers - 1))
      }
    } else if (progress > 0 && progress < 1 && totalSegmentsRef.current > 0) {
      const targetSeg = Math.floor(progress * totalSegmentsRef.current)
      let segCount = 0
      for (let i = 0; i < layersRef.current.length; i++) {
        const layerSegs = layersRef.current[i].segments.length
        if (segCount + layerSegs >= targetSeg) {
          setViewLayer(i)
          break
        }
        segCount += layerSegs
      }
    }
  }, [externalLayer, totalLayerCount, totalLayers, progress, followProgress, isPreviewing])

  // Toggle bed visibility
  useEffect(() => {
    const scene = sceneRef.current
    if (!scene) return
    scene.traverse((obj) => {
      if (obj instanceof THREE.Mesh && obj.geometry instanceof THREE.PlaneGeometry) {
        obj.visible = showBed
      }
      if (obj instanceof THREE.GridHelper) {
        obj.visible = showBed
      }
    })
  }, [showBed])

  const setView = (view: 'top' | 'front' | 'iso') => {
    const camera = cameraRef.current
    const controls = controlsRef.current
    if (!camera || !controls) return
    if (view === 'top') {
      camera.position.set(0, 400, 0.1)
      controls.target.set(0, 0, 0)
    } else if (view === 'front') {
      camera.position.set(0, 50, 400)
      controls.target.set(0, 50, 0)
    } else {
      camera.position.set(180, -180, 280)
      controls.target.set(0, 0, 0)
    }
    controls.update()
  }

  const handleSlider = (idx: number) => {
    setIsPreviewing(true)
    setFollowProgress(false)
    setViewLayer(idx)
  }

  const resumeProgress = () => {
    setIsPreviewing(false)
    setFollowProgress(true)
  }

  if (!gcode) {
    return (
      <div className={`flex items-center justify-center bg-slate-950 rounded-lg ${className}`} style={{ minHeight: '20rem' }}>
        <div className="text-center">
          <Box className="mx-auto h-8 w-8 text-slate-600" />
          <p className="mt-2 text-xs text-slate-500">Waiting for print job...</p>
        </div>
      </div>
    )
  }

  return (
    <div className={`rounded-lg overflow-hidden border border-slate-700 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-700 bg-slate-900 px-3 py-1.5">
        <div className="flex items-center gap-2">
          <Box className="h-4 w-4 text-blue-400" />
          <span className="text-xs font-medium text-slate-300">3D Print Preview</span>
          <span className="rounded-full bg-slate-700 px-2 py-0.5 text-[10px] font-mono text-slate-300">
            {isPreviewing ? 'Preview ' : ''}Layer {viewLayer + 1} / {totalLayers}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <button onClick={() => setView('top')} title="Top view" className="rounded p-1 text-slate-400 hover:bg-slate-700 hover:text-white">
            <Grid3x3 className="h-3.5 w-3.5" />
          </button>
          <button onClick={() => setView('front')} title="Front view" className="rounded p-1 text-slate-400 hover:bg-slate-700 hover:text-white">
            <Eye className="h-3.5 w-3.5" />
          </button>
          <button onClick={() => setView('iso')} title="Isometric view" className="rounded p-1 text-slate-400 hover:bg-slate-700 hover:text-white">
            <Box className="h-3.5 w-3.5" />
          </button>
          <button onClick={() => { setShowBed(!showBed) }} title="Toggle bed" className={`rounded p-1 ${showBed ? 'text-blue-400' : 'text-slate-500'} hover:bg-slate-700`}>
            <Grid3x3 className="h-3.5 w-3.5" />
          </button>
          <button onClick={() => setExpanded(!expanded)} title={expanded ? 'Collapse' : 'Expand'} className="rounded p-1 text-slate-400 hover:bg-slate-700 hover:text-white">
            {expanded ? <Minimize2 className="h-3.5 w-3.5" /> : <Maximize2 className="h-3.5 w-3.5" />}
          </button>
        </div>
      </div>

      {/* 3D Canvas */}
      <div
        ref={mountRef}
        className="w-full bg-slate-950"
        style={{ height: expanded ? '500px' : '280px', touchAction: 'none', cursor: 'grab' }}
      />

      {/* Layer slider */}
      <div className="flex items-center gap-2 border-t border-slate-700 bg-slate-900 px-3 py-2">
        <button
          onClick={() => handleSlider(Math.max(0, viewLayer - 1))}
          disabled={viewLayer <= 0}
          className="rounded p-1 text-slate-400 hover:bg-slate-700 disabled:opacity-30"
        >
          <Layers className="h-3.5 w-3.5" />
        </button>
        <input
          type="range"
          min={0}
          max={Math.max(0, totalLayers - 1)}
          value={viewLayer}
          onChange={(e) => handleSlider(parseInt(e.target.value))}
          className="flex-1 accent-blue-600"
        />
        <label className="flex items-center gap-1 text-[10px] text-slate-400">
          <input
            type="checkbox"
            checked={followProgress}
            onChange={(e) => { setFollowProgress(e.target.checked); if (e.target.checked) resumeProgress() }}
            className="accent-blue-600"
          />
          Follow
        </label>
        {isPreviewing && (
          <button
            onClick={resumeProgress}
            className="rounded bg-amber-600 px-2 py-0.5 text-[10px] font-medium text-white hover:bg-amber-500"
          >
            <RotateCcw className="mr-1 inline h-3 w-3" />Resume
          </button>
        )}
      </div>
    </div>
  )
}
