import { useEffect, useRef } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'

export interface GCodePreviewProps {
  gcode: string
  bedWidth?: number
  bedDepth?: number
  bedHeight?: number
  className?: string
}

export function GCodePreview({
  gcode,
  bedWidth = 220,
  bedDepth = 220,
  bedHeight = 250,
  className = '',
}: GCodePreviewProps) {
  const mountRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!mountRef.current || !gcode) return

    const el = mountRef.current
    const width = el.clientWidth
    const height = el.clientHeight || width * 0.6

    const scene = new THREE.Scene()
    scene.background = new THREE.Color(0x0f172a)

    const camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 10000)
    camera.up.set(0, 0, 1)

    const renderer = new THREE.WebGLRenderer({ antialias: true })
    renderer.setSize(width, height)
    el.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true

    const bed = new THREE.GridHelper(bedWidth, 22, 0x1e293b, 0x1e293b)
    bed.rotation.x = Math.PI / 2
    bed.position.set(bedWidth / 2, bedDepth / 2, 0)
    scene.add(bed)

    const box = new THREE.Box3(
      new THREE.Vector3(0, 0, 0),
      new THREE.Vector3(bedWidth, bedDepth, bedHeight)
    )
    const edges = new THREE.EdgesGeometry(new THREE.BoxGeometry(bedWidth, bedDepth, bedHeight))
    const line = new THREE.LineSegments(
      edges,
      new THREE.LineBasicMaterial({ color: 0x334155 })
    )
    line.position.set(bedWidth / 2, bedDepth / 2, bedHeight / 2)
    scene.add(line)

    const positions: number[] = []
    const colors: number[] = []
    const colorTravel = new THREE.Color(0x64748b)
    const colorExtrude = new THREE.Color(0x3b82f6)

    let x = 0
    let y = 0
    let z = 0
    let e = 0
    let relative = false
    let prevX = x
    let prevY = y
    let prevZ = z

    const lines = gcode.split(/\r?\n/)
    for (const raw of lines) {
      const ln = raw.trim().toUpperCase()
      if (ln.startsWith(';')) continue
      if (ln === '') continue

      const tokens = ln.split(/\s+/)
      const cmd = tokens[0]

      if (cmd === 'G90') {
        relative = false
        continue
      }
      if (cmd === 'G91') {
        relative = true
        continue
      }
      if (cmd === 'G92') {
        for (const t of tokens.slice(1)) {
          const v = parseFloat(t.slice(1))
          if (t[0] === 'X') x = v
          else if (t[0] === 'Y') y = v
          else if (t[0] === 'Z') z = v
          else if (t[0] === 'E') e = v
        }
        continue
      }
      if (cmd === 'G28' || cmd.startsWith('G28')) {
        let nx = 0
        let ny = 0
        let nz = 0
        for (const t of tokens.slice(1)) {
          if (t[0] === 'X') nx = parseFloat(t.slice(1))
          else if (t[0] === 'Y') ny = parseFloat(t.slice(1))
          else if (t[0] === 'Z') nz = parseFloat(t.slice(1))
        }
        x = nx
        y = ny
        z = nz
        prevX = x
        prevY = y
        prevZ = z
        continue
      }
      if (cmd !== 'G0' && cmd !== 'G1') {
        continue
      }

      let nx = x
      let ny = y
      let nz = z
      let ne = e
      for (const t of tokens.slice(1)) {
        const v = parseFloat(t.slice(1))
        if (isNaN(v)) continue
        if (t[0] === 'X') nx = relative ? x + v : v
        else if (t[0] === 'Y') ny = relative ? y + v : v
        else if (t[0] === 'Z') nz = relative ? z + v : v
        else if (t[0] === 'E') ne = relative ? e + v : v
      }

      const extruding = ne > e
      const c = extruding ? colorExtrude : colorTravel
      positions.push(prevX, prevY, prevZ, nx, ny, nz)
      colors.push(c.r, c.g, c.b, c.r, c.g, c.b)

      x = nx
      y = ny
      z = nz
      e = ne
      prevX = x
      prevY = y
      prevZ = z
    }

    if (positions.length >= 6) {
      const geometry = new THREE.BufferGeometry()
      geometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
      geometry.setAttribute('color', new THREE.Float32BufferAttribute(colors, 3))
      const material = new THREE.LineBasicMaterial({
        vertexColors: true,
        transparent: true,
        opacity: 0.9,
      })
      const path = new THREE.LineSegments(geometry, material)
      scene.add(path)
    }

    const bounds = new THREE.Box3()
    if (positions.length > 0) {
      for (let i = 0; i < positions.length; i += 3) {
        bounds.expandByPoint(new THREE.Vector3(positions[i], positions[i + 1], positions[i + 2]))
      }
    } else {
      bounds.copy(box)
    }
    const center = bounds.getCenter(new THREE.Vector3())
    const size = bounds.getSize(new THREE.Vector3())
    const maxDim = Math.max(size.x, size.y, size.z, 1)
    const dist = maxDim * 1.5
    camera.position.set(center.x + dist, center.y - dist, center.z + dist * 0.5)
    controls.target.copy(center)
    controls.update()

    let raf = 0
    const animate = () => {
      raf = requestAnimationFrame(animate)
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
      cancelAnimationFrame(raf)
      window.removeEventListener('resize', handleResize)
      controls.dispose()
      renderer.dispose()
      el.removeChild(renderer.domElement)
    }
  }, [gcode, bedWidth, bedDepth, bedHeight])

  return <div ref={mountRef} className={`w-full ${className}`} style={{ minHeight: '18rem' }} />
}
