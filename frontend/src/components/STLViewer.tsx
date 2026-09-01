import { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { STLLoader } from 'three/examples/jsm/loaders/STLLoader.js'
import { Loader2, Box } from 'lucide-react'

// ── STL Screenshot Utility ─────────────────────────────────────────
// Renders an STL file off-screen and returns a base64 JPEG screenshot.
// Used by the "Ask AI" feature to send a model preview to Gemini.
export function renderSTLScreenshot(url: string, width = 512, height = 512): Promise<string> {
  return new Promise((resolve, reject) => {
    const scene = new THREE.Scene()
    scene.background = new THREE.Color(0x0f172a)

    const camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 100000)
    camera.up.set(0, 0, 1)

    const renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true })
    renderer.setSize(width, height)

    // Lights
    scene.add(new THREE.AmbientLight(0x6688aa, 0.8))
    const dir1 = new THREE.DirectionalLight(0xffffff, 1.2)
    dir1.position.set(1, 2, 3)
    scene.add(dir1)
    const dir2 = new THREE.DirectionalLight(0x8899ff, 0.5)
    dir2.position.set(-2, -1, 2)
    scene.add(dir2)

    // Grid
    const grid = new THREE.GridHelper(200, 20, 0x3b82f6, 0x1e291b)
    grid.rotation.x = Math.PI / 2
    scene.add(grid)

    const loader = new STLLoader()
    loader.load(
      url,
      (geometry) => {
        geometry.computeVertexNormals()
        geometry.center()
        const material = new THREE.MeshPhongMaterial({
          color: 0x6366f1,
          specular: 0x222244,
          shininess: 80,
          flatShading: false,
        })
        const mesh = new THREE.Mesh(geometry, material)

        const box = new THREE.Box3().setFromObject(mesh)
        const size = box.getSize(new THREE.Vector3())
        const maxDim = Math.max(size.x, size.y, size.z)
        const scale = 100 / maxDim
        mesh.scale.setScalar(scale)

        const scaledBox = new THREE.Box3().setFromObject(mesh)
        mesh.position.z -= scaledBox.min.z

        scene.add(mesh)

        // Frame camera
        const fittedBox = new THREE.Box3().setFromObject(mesh)
        const center = fittedBox.getCenter(new THREE.Vector3())
        const fittedSize = fittedBox.getSize(new THREE.Vector3())
        const maxFit = Math.max(fittedSize.x, fittedSize.y, fittedSize.z) * 1.5
        camera.position.set(center.x + maxFit, center.y - maxFit, center.z + maxFit)
        camera.lookAt(center)

        renderer.render(scene, camera)
        const dataUrl = renderer.domElement.toDataURL('image/jpeg', 0.85)
        const base64 = dataUrl.split(',')[1] || dataUrl

        // Cleanup
        mesh.geometry.dispose()
        ;(mesh.material as THREE.Material).dispose()
        renderer.dispose()

        resolve(base64)
      },
      undefined,
      (err) => {
        renderer.dispose()
        reject(err)
      }
    )
  })
}

export interface STLViewerProps {
  url: string
  className?: string
}

export function STLViewer({ url, className = '' }: STLViewerProps) {
  const mountRef = useRef<HTMLDivElement>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!mountRef.current) return
    const el = mountRef.current
    const width = el.clientWidth
    const height = el.clientHeight || 400

    const scene = new THREE.Scene()
    scene.background = new THREE.Color(0x0f172a)

    const camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 100000)
    camera.up.set(0, 0, 1)

    const renderer = new THREE.WebGLRenderer({ antialias: true })
    renderer.setSize(width, height)
    el.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true

    // Lights
    const ambient = new THREE.AmbientLight(0x6688aa, 0.8)
    scene.add(ambient)
    const dir1 = new THREE.DirectionalLight(0xffffff, 1.2)
    dir1.position.set(1, 2, 3)
    scene.add(dir1)
    const dir2 = new THREE.DirectionalLight(0x8899ff, 0.5)
    dir2.position.set(-2, -1, 2)
    scene.add(dir2)

    // Grid
    const grid = new THREE.GridHelper(200, 20, 0x3b82f6, 0x1e293b)
    grid.rotation.x = Math.PI / 2
    scene.add(grid)

    let mesh: THREE.Mesh | null = null
    let frameId = 0

    const loader = new STLLoader()
    loader.load(
      url,
      (geometry) => {
        geometry.computeVertexNormals()
        geometry.center()
        const material = new THREE.MeshPhongMaterial({
          color: 0x6366f1,
          specular: 0x222244,
          shininess: 80,
          flatShading: false,
        })
        mesh = new THREE.Mesh(geometry, material)

        // Auto-scale and position
        const box = new THREE.Box3().setFromObject(mesh)
        const size = box.getSize(new THREE.Vector3())
        const maxDim = Math.max(size.x, size.y, size.z)
        const scale = 100 / maxDim
        mesh.scale.setScalar(scale)

        // Place on grid
        const scaledBox = new THREE.Box3().setFromObject(mesh)
        mesh.position.z -= scaledBox.min.z

        scene.add(mesh)

        // Frame camera on the model
        const fittedBox = new THREE.Box3().setFromObject(mesh)
        const center = fittedBox.getCenter(new THREE.Vector3())
        const fittedSize = fittedBox.getSize(new THREE.Vector3())
        const maxFit = Math.max(fittedSize.x, fittedSize.y, fittedSize.z) * 1.5
        camera.position.set(center.x + maxFit, center.y - maxFit, center.z + maxFit)
        camera.lookAt(center)
        controls.target.copy(center)
        controls.update()

        setLoading(false)
      },
      undefined,
      (err) => {
        setError(err instanceof Error ? err.message : 'Failed to load STL')
        setLoading(false)
      }
    )

    const animate = () => {
      frameId = requestAnimationFrame(animate)
      controls.update()
      renderer.render(scene, camera)
    }
    animate()

    const handleResize = () => {
      const w = el.clientWidth
      const h = el.clientHeight || 400
      camera.aspect = w / h
      camera.updateProjectionMatrix()
      renderer.setSize(w, h)
    }
    window.addEventListener('resize', handleResize)

    return () => {
      cancelAnimationFrame(frameId)
      window.removeEventListener('resize', handleResize)
      controls.dispose()
      renderer.dispose()
      if (mesh) {
        mesh.geometry.dispose()
        ;(mesh.material as THREE.Material).dispose()
      }
      if (renderer.domElement.parentNode === el) {
        el.removeChild(renderer.domElement)
      }
    }
  }, [url])

  return (
    <div className={`relative ${className}`}>
      <div ref={mountRef} className="h-full w-full" style={{ minHeight: '400px' }} />
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center bg-slate-950/50">
          <Loader2 className="h-8 w-8 animate-spin text-blue-400" />
        </div>
      )}
      {error && (
        <div className="absolute inset-0 flex items-center justify-center bg-slate-950/80">
          <p className="font-mono text-sm text-rose-400">{error}</p>
        </div>
      )}
    </div>
  )
}

// ── STLThumbnail ───────────────────────────────────────────────────
// Lightweight non-interactive 3D preview for grid cards. Auto-rotates
// slowly. Disposes resources when the element unmounts.

export interface STLThumbnailProps {
  url: string
  className?: string
}

export function STLThumbnail({ url, className = '' }: STLThumbnailProps) {
  const mountRef = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState<'loading' | 'ok' | 'error'>('loading')

  useEffect(() => {
    if (!mountRef.current) return
    const el = mountRef.current
    const width = el.clientWidth || 200
    const height = el.clientHeight || 150

    const scene = new THREE.Scene()
    scene.background = new THREE.Color(0x1e293b)

    const camera = new THREE.PerspectiveCamera(40, width / height, 0.1, 100000)
    camera.up.set(0, 0, 1)

    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
    renderer.setSize(width, height)
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    el.appendChild(renderer.domElement)

    // Lights
    scene.add(new THREE.AmbientLight(0x6688aa, 0.7))
    const dir1 = new THREE.DirectionalLight(0xffffff, 1.0)
    dir1.position.set(1, 2, 3)
    scene.add(dir1)
    const dir2 = new THREE.DirectionalLight(0x8899ff, 0.4)
    dir2.position.set(-2, -1, 2)
    scene.add(dir2)

    let mesh: THREE.Mesh | null = null
    let frameId = 0
    let rotationSpeed = 0.003

    const loader = new STLLoader()
    loader.load(
      url,
      (geometry) => {
        geometry.computeVertexNormals()
        geometry.center()
        const material = new THREE.MeshPhongMaterial({
          color: 0x6366f1,
          specular: 0x222244,
          shininess: 60,
          flatShading: false,
        })
        mesh = new THREE.Mesh(geometry, material)

        // Auto-scale to fit
        const box = new THREE.Box3().setFromObject(mesh)
        const size = box.getSize(new THREE.Vector3())
        const maxDim = Math.max(size.x, size.y, size.z)
        if (maxDim > 0) {
          mesh.scale.setScalar(50 / maxDim)
        }

        // Place on virtual bed
        const scaledBox = new THREE.Box3().setFromObject(mesh)
        mesh.position.z -= scaledBox.min.z

        scene.add(mesh)

        // Frame camera
        const fittedBox = new THREE.Box3().setFromObject(mesh)
        const center = fittedBox.getCenter(new THREE.Vector3())
        const fittedSize = fittedBox.getSize(new THREE.Vector3())
        const maxFit = Math.max(fittedSize.x, fittedSize.y, fittedSize.z) * 1.8
        camera.position.set(center.x + maxFit * 0.7, center.y - maxFit * 0.7, center.z + maxFit * 0.5)
        camera.lookAt(center)

        setStatus('ok')
      },
      undefined,
      () => setStatus('error')
    )

    const animate = () => {
      frameId = requestAnimationFrame(animate)
      if (mesh) {
        mesh.rotation.z += rotationSpeed
      }
      renderer.render(scene, camera)
    }
    animate()

    const handleResize = () => {
      const w = el.clientWidth || 200
      const h = el.clientHeight || 150
      camera.aspect = w / h
      camera.updateProjectionMatrix()
      renderer.setSize(w, h)
    }
    window.addEventListener('resize', handleResize)

    return () => {
      cancelAnimationFrame(frameId)
      window.removeEventListener('resize', handleResize)
      renderer.dispose()
      if (mesh) {
        mesh.geometry.dispose()
        ;(mesh.material as THREE.Material).dispose()
      }
      if (renderer.domElement.parentNode === el) {
        el.removeChild(renderer.domElement)
      }
    }
  }, [url])

  return (
    <div className={`relative overflow-hidden ${className}`}>
      <div ref={mountRef} className="h-full w-full" />
      {status === 'loading' && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-slate-500" />
        </div>
      )}
      {status === 'error' && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Box className="h-8 w-8 text-slate-600" />
        </div>
      )}
    </div>
  )
}
