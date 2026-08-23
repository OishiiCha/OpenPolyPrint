import { useEffect, useState } from 'react'
import { isTest, mockGCodeFiles } from '../data/mock'
import type { GCodeFile } from '../types'

export function useGCodeFiles() {
  const [files, setFiles] = useState<GCodeFile[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = async () => {
    if (isTest) {
      setFiles(mockGCodeFiles)
      setLoading(false)
      return
    }
    try {
      const res = await fetch('/api/gcode')
      if (!res.ok) throw new Error('failed to load')
      const data = await res.json()
      setFiles(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error(e)
      setFiles([])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  return { files, loading, refresh }
}
