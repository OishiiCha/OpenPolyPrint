import { useCallback, useEffect, useRef, useState } from 'react'
import { isTest, mockPrinter } from '../data/mock'
import type { Printer } from '../types'

export function usePrinters() {
  const [printers, setPrinters] = useState<Printer[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const initialRef = useRef(true)

  const refresh = useCallback(() => {
    if (isTest) {
      setPrinters([mockPrinter])
      setLoading(false)
      return
    }
    if (initialRef.current) {
      setLoading(true)
    }
    fetch('/api/printers')
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((data: Printer[]) => {
        setPrinters(data)
        setLoading(false)
        initialRef.current = false
      })
      .catch((err: any) => {
        setError(err.message)
        setLoading(false)
        initialRef.current = false
      })
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, 3000)
    return () => clearInterval(id)
  }, [refresh])

  return { printers, loading, error, refresh }
}
