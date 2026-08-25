import { useCallback, useEffect, useState } from 'react'

export interface PiSensorReading {
  id: number
  enabled: boolean
  name: string
  gpioPin: number
  filamentType: string
  color: string
  temp: number | null
  humidity: number | null
  error?: string
  updatedAt: number
  hasReading: boolean
}

export interface PiReadings {
  sensors: PiSensorReading[]
  lightRelayEnabled: boolean
  lightRelayGpio: number
  lightRelayOn: boolean
  gpioAvailable: boolean
  sensorManagerRunning: boolean
  os: string
}

export function usePiReadings() {
  const [readings, setReadings] = useState<PiReadings | null>(null)
  const [loading, setLoading] = useState(true)

  const fetchReadings = useCallback(async () => {
    try {
      const res = await fetch('/api/pi/readings')
      if (!res.ok) return
      const data = await res.json()
      setReadings({
        sensors: data.sensors || [],
        lightRelayEnabled: data.lightRelayEnabled ?? false,
        lightRelayGpio: data.lightRelayGpio ?? 0,
        lightRelayOn: data.lightRelayOn ?? false,
        gpioAvailable: data.gpioAvailable ?? false,
        sensorManagerRunning: data.sensorManagerRunning ?? false,
        os: data.os ?? '',
      })
    } catch (e) {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchReadings()
    const id = setInterval(fetchReadings, 5000)
    return () => clearInterval(id)
  }, [fetchReadings])

  const toggleLight = useCallback(async (on: boolean) => {
    try {
      const res = await fetch('/api/pi/light', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ on }),
      })
      const data = await res.json()
      if (data.success) {
        setReadings((prev) => (prev ? { ...prev, lightRelayOn: data.on } : prev))
      }
    } catch (e) {
      console.error(e)
    }
  }, [])

  return { readings, loading, toggleLight, refresh: fetchReadings }
}
