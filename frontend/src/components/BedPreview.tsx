import { useEffect, useRef } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'

export interface BedPreviewProps {
  bedWidth?: number
  bedDepth?: number
  className?: string
}

export function BedPreview({
  bedWidth = 220,
  bedDepth = 220,
  className = '',
}: BedPreviewProps) {
  const mountRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!mountRef.current) return

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

    const grid = new THREE.GridHelper(bedWidth, 22, 0x3b82f6, 0x1e293b)
    grid.rotation.x = Math.PI / 2
    grid.position.set(bedWidth / 2, bedDepth / 2, 0)
    scene.add(grid)

    const mesh = new THREE.Mesh(
      new THREE.PlaneGeometry(bedWidth, bedDepth, 22, 22),
      new THREE.MeshBasicMaterial({
        color: 0x1e293b,
        wireframe: true,
        transparent: true,
        opacity: 0.25,
      })
    )
    mesh.position.set(bedWidth / 2, bedDepth / 2, 0)
    scene.add(mesh)

    const box = new THREE.BoxGeometry(bedWidth, bedDepth, 10)
    const edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(box),
      new THREE.LineBasicMaterial({ color: 0x334155 })
    )
    edges.position.set(bedWidth / 2, bedDepth / 2, 5)
    scene.add(edges)

    const center = new THREE.Vector3(bedWidth / 2, bedDepth / 2, 0)
    camera.position.set(center.x + bedWidth * 0.8, center.y - bedDepth * 0.8, bedDepth * 1.2)
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
  }, [bedWidth, bedDepth])

  return <div ref={mountRef} className={`w-full ${className}`} style={{ minHeight: '18rem' }} />
}
