import { useEffect, useState, useRef } from 'react'

interface Sample {
  time: number
  nozzle: number
  targetNozzle: number
  bed: number
  targetBed: number
}

interface TempChartProps {
  printerId: string
  height?: number
}

export function TempChart({ printerId, height = 120 }: TempChartProps) {
  const [samples, setSamples] = useState<Sample[]>([])
  const timerRef = useRef<ReturnType<typeof setInterval>>(undefined)

  useEffect(() => {
    const fetchTemps = async () => {
      try {
        const res = await fetch(`/api/temps/${encodeURIComponent(printerId)}`)
        if (!res.ok) return
        const data = await res.json()
        if (Array.isArray(data)) setSamples(data)
      } catch (e) {
        // ignore
      }
    }
    fetchTemps()
    timerRef.current = setInterval(fetchTemps, 3000)
    return () => clearInterval(timerRef.current)
  }, [printerId])

  if (samples.length < 2) {
    return (
      <div className="flex items-center justify-center text-xs text-slate-400" style={{ height }}>
        Collecting temperature data...
      </div>
    )
  }

  const width = 300
  const padding = 4
  const chartW = width - padding * 2
  const chartH = height - padding * 2

  // Find max temp for scaling
  const allTemps = samples.flatMap((s) => [s.nozzle, s.targetNozzle, s.bed, s.targetBed])
  const maxTemp = Math.max(...allTemps, 50)
  const minTemp = 0
  const tempRange = maxTemp - minTemp || 1

  const timeStart = samples[0].time
  const timeEnd = samples[samples.length - 1].time
  const timeRange = (timeEnd - timeStart) || 1

  const toX = (t: number) => padding + ((t - timeStart) / timeRange) * chartW
  const toY = (temp: number) => padding + chartH - ((temp - minTemp) / tempRange) * chartH

  const buildPath = (key: keyof Sample) => {
    return samples
      .map((s, i) => `${i === 0 ? 'M' : 'L'} ${toX(s.time).toFixed(1)} ${toY(s[key] as number).toFixed(1)}`)
      .join(' ')
  }

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full" style={{ height }} preserveAspectRatio="none">
      {/* Grid lines */}
      {[0, 0.25, 0.5, 0.75, 1].map((f) => (
        <line
          key={f}
          x1={padding}
          x2={width - padding}
          y1={padding + chartH * f}
          y2={padding + chartH * f}
          stroke="currentColor"
          strokeWidth="0.5"
          className="text-slate-200 dark:text-slate-800"
        />
      ))}

      {/* Bed target (dashed orange) */}
      <path d={buildPath('targetBed')} fill="none" stroke="#f97316" strokeWidth="1" strokeDasharray="3,2" opacity="0.6" />

      {/* Bed actual (orange) */}
      <path d={buildPath('bed')} fill="none" stroke="#f97316" strokeWidth="1.5" />

      {/* Nozzle target (dashed red) */}
      <path d={buildPath('targetNozzle')} fill="none" stroke="#ef4444" strokeWidth="1" strokeDasharray="3,2" opacity="0.6" />

      {/* Nozzle actual (red) */}
      <path d={buildPath('nozzle')} fill="none" stroke="#ef4444" strokeWidth="1.5" />

      {/* Current value labels */}
      <text x={width - padding - 4} y={toY(samples[samples.length - 1].nozzle) - 3} textAnchor="end" fontSize="8" fill="#ef4444" className="font-mono">
        {Math.round(samples[samples.length - 1].nozzle)}°
      </text>
      <text x={width - padding - 4} y={toY(samples[samples.length - 1].bed) - 3} textAnchor="end" fontSize="8" fill="#f97316" className="font-mono">
        {Math.round(samples[samples.length - 1].bed)}°
      </text>
    </svg>
  )
}
