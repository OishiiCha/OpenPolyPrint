interface TempSample {
  time: number
  nozzle: number
  targetNozzle: number
  bed: number
  targetBed: number
  progress: number
}

interface SessionTempGraphProps {
  samples: TempSample[]
  currentTime: number | null  // unix timestamp of the current frame, or null
  height?: number
}

export function SessionTempGraph({ samples, currentTime, height = 100 }: SessionTempGraphProps) {
  if (samples.length < 2) {
    return (
      <div className="flex items-center justify-center text-xs text-slate-400" style={{ height }}>
        No temperature data in this session
      </div>
    )
  }

  const width = 600
  const padding = 6
  const chartW = width - padding * 2
  const chartH = height - padding * 2

  const allTemps = samples.flatMap((s) => [s.nozzle, s.targetNozzle, s.bed, s.targetBed])
  const maxTemp = Math.max(...allTemps, 50)
  const minTemp = 0
  const tempRange = maxTemp - minTemp || 1

  const timeStart = samples[0].time
  const timeEnd = samples[samples.length - 1].time
  const timeRange = (timeEnd - timeStart) || 1

  const toX = (t: number) => padding + ((t - timeStart) / timeRange) * chartW
  const toY = (temp: number) => padding + chartH - ((temp - minTemp) / tempRange) * chartH

  const buildPath = (key: keyof TempSample) => {
    return samples
      .map((s, i) => `${i === 0 ? 'M' : 'L'} ${toX(s.time).toFixed(1)} ${toY(s[key] as number).toFixed(1)}`)
      .join(' ')
  }

  const markerX = currentTime != null ? toX(currentTime) : null
  const clampedMarkerX = markerX != null ? Math.max(padding, Math.min(width - padding, markerX)) : null

  // Find the temps at the current time for display
  let currentNozzle: number | null = null
  let currentBed: number | null = null
  if (currentTime != null) {
    let best = samples[0]
    let bestDiff = Infinity
    for (const s of samples) {
      const diff = Math.abs(s.time - currentTime)
      if (diff < bestDiff) {
        bestDiff = diff
        best = s
      }
    }
    currentNozzle = best.nozzle
    currentBed = best.bed
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
      <path d={buildPath('targetBed')} fill="none" stroke="#f97316" strokeWidth="1" strokeDasharray="3,2" opacity="0.5" />

      {/* Bed actual (orange) */}
      <path d={buildPath('bed')} fill="none" stroke="#f97316" strokeWidth="1.5" />

      {/* Nozzle target (dashed red) */}
      <path d={buildPath('targetNozzle')} fill="none" stroke="#ef4444" strokeWidth="1" strokeDasharray="3,2" opacity="0.5" />

      {/* Nozzle actual (red) */}
      <path d={buildPath('nozzle')} fill="none" stroke="#ef4444" strokeWidth="1.5" />

      {/* Current time marker */}
      {clampedMarkerX != null && (
        <>
          <line
            x1={clampedMarkerX}
            x2={clampedMarkerX}
            y1={padding}
            y2={height - padding}
            stroke="#6366f1"
            strokeWidth="1.5"
            strokeDasharray="2,2"
          />
          <circle cx={clampedMarkerX} cy={padding} r="3" fill="#6366f1" />
        </>
      )}

      {/* Current value labels */}
      {currentNozzle != null && (
        <text x={width - padding - 4} y={toY(currentNozzle) - 3} textAnchor="end" fontSize="9" fill="#ef4444" className="font-mono">
          {Math.round(currentNozzle)}C
        </text>
      )}
      {currentBed != null && (
        <text x={width - padding - 4} y={toY(currentBed) - 3} textAnchor="end" fontSize="9" fill="#f97316" className="font-mono">
          {Math.round(currentBed)}C
        </text>
      )}
    </svg>
  )
}
