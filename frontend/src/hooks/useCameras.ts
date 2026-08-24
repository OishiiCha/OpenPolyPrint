import { useCallback, useEffect, useState } from 'react'
import { isTest, mockCamera } from '../data/mock'
import type { Camera } from '../types'

const STORAGE_KEY = 'openpolyprint-cameras'

function loadLocal(): Camera[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      return JSON.parse(raw) as Camera[]
    }
  } catch (e) {
    // ignore
  }
  return []
}

function saveLocal(cameras: Camera[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(cameras))
}

function toCamera(c: any): Camera {
  const type: Camera['type'] = c.type === 'rpicam' ? 'mipi' : c.type
  let url = c.url
  if (c.type === 'usb') {
    url = `/api/cameras/usb/preview?deviceId=${encodeURIComponent(c.deviceId || '')}&deviceLabel=${encodeURIComponent(c.deviceLabel || '')}&flip=${encodeURIComponent(c.flip || '')}&brightness=${encodeURIComponent(c.brightness ?? 0)}`
  } else if (c.type === 'rpicam') {
    url = `/api/cameras/mipi/preview?deviceId=${encodeURIComponent(c.deviceId || '0')}&deviceLabel=${encodeURIComponent(c.deviceLabel || '')}&sensor=${encodeURIComponent(c.sensor || '')}&flip=${encodeURIComponent(c.flip || '')}&brightness=${encodeURIComponent(c.brightness ?? 0)}`
  }
  return {
    id: c.id,
    name: c.name,
    printerId: c.printerId || 'unassigned',
    type,
    enabled: c.enabled ?? true,
    url,
    deviceId: c.deviceId,
    deviceLabel: c.deviceLabel,
    sensor: c.sensor,
    brightness: c.brightness ?? 0,
    flip: c.flip ?? '',
  }
}

function fromCamera(c: Camera): any {
  return {
    id: c.id,
    name: c.name,
    type: c.type === 'mipi' ? 'rpicam' : c.type,
    printerId: c.printerId,
    enabled: c.enabled,
    url: c.type === 'stream' ? c.url : undefined,
    deviceId: c.deviceId,
    deviceLabel: c.deviceLabel,
    sensor: c.sensor,
    brightness: c.brightness,
    flip: c.flip,
  }
}

export function useCameras() {
  const [cameras, setCameras] = useState<Camera[]>(loadLocal)
  const [loading, setLoading] = useState(true)

  const fetchCameras = useCallback(async () => {
    if (isTest) {
      setCameras([mockCamera])
      setLoading(false)
      return
    }
    try {
      const res = await fetch('/api/cameras')
      if (!res.ok) throw new Error('failed to fetch cameras')
      const data = await res.json()
      const list = Array.isArray(data.cameras) ? data.cameras.map(toCamera) : []
      setCameras(list)
      saveLocal(list)
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchCameras()
    const handler = () => fetchCameras()
    window.addEventListener('openpolyprint-cameras-updated', handler)
    return () => window.removeEventListener('openpolyprint-cameras-updated', handler)
  }, [fetchCameras])

  const addCamera = useCallback(
    async (camera: Camera) => {
      const res = await fetch('/api/cameras', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(fromCamera(camera)),
      })
      if (!res.ok) {
        const msg = await res.text().catch(() => 'failed to add camera')
        throw new Error(msg)
      }
      await fetchCameras()
    },
    [fetchCameras]
  )

  const updateCamera = useCallback(
    async (camera: Camera) => {
      const res = await fetch('/api/cameras', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(fromCamera(camera)),
      })
      if (!res.ok) {
        const msg = await res.text().catch(() => 'failed to update camera')
        throw new Error(msg)
      }
      await fetchCameras()
    },
    [fetchCameras]
  )

  const removeCamera = useCallback(
    async (id: string) => {
      try {
        await fetch(`/api/cameras?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
      } catch (e) {
        console.error(e)
      }
      await fetchCameras()
    },
    [fetchCameras]
  )

  const refresh = fetchCameras

  return { cameras, loading, addCamera, updateCamera, removeCamera, refresh }
}
