import { useEffect, useRef, useState } from 'react'

interface AutoScrollTextProps {
  text: string
  className?: string
  /** Speed in pixels per second */
  speed?: number
  /** Pause at start and end in ms */
  pauseDuration?: number
}

/**
 * Displays text that shows the start, then auto-scrolls to reveal the rest
 * if the text overflows its container. If the text fits, it stays static.
 *
 * The animation cycle is:
 * 1. Show start (pause)
 * 2. Scroll to end
 * 3. Pause at end
 * 4. Scroll back to start
 * 5. Repeat
 */
export function AutoScrollText({
  text,
  className = '',
  speed = 30,
  pauseDuration = 1500,
}: AutoScrollTextProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLSpanElement>(null)
  const [needsScroll, setNeedsScroll] = useState(false)
  const [phase, setPhase] = useState<'start' | 'scrolling-right' | 'end' | 'scrolling-left'>('start')
  const offsetRef = useRef(0)
  const rafRef = useRef<number>(0)
  const lastTimeRef = useRef(0)

  // Check if text overflows
  useEffect(() => {
    const check = () => {
      const container = containerRef.current
      const content = contentRef.current
      if (!container || !content) return
      const overflow = content.scrollWidth - container.clientWidth
      setNeedsScroll(overflow > 2)
    }
    check()
    // Re-check on resize
    const ro = new ResizeObserver(check)
    if (containerRef.current) ro.observe(containerRef.current)
    return () => ro.disconnect()
  }, [text])

  // Animation loop
  useEffect(() => {
    if (!needsScroll) {
      setPhase('start')
      offsetRef.current = 0
      return
    }

    const container = containerRef.current
    const content = contentRef.current
    if (!container || !content) return

    const maxOffset = content.scrollWidth - container.clientWidth
    if (maxOffset <= 0) return

    let currentPhase = phase
    let pausedUntil = performance.now() + pauseDuration
    lastTimeRef.current = 0

    const tick = (now: number) => {
      if (lastTimeRef.current === 0) lastTimeRef.current = now
      const dt = now - lastTimeRef.current
      lastTimeRef.current = now

      if (now < pausedUntil) {
        rafRef.current = requestAnimationFrame(tick)
        return
      }

      if (currentPhase === 'start') {
        currentPhase = 'scrolling-right'
        setPhase('scrolling-right')
      } else if (currentPhase === 'scrolling-right') {
        offsetRef.current += (speed * dt) / 1000
        if (offsetRef.current >= maxOffset) {
          offsetRef.current = maxOffset
          currentPhase = 'end'
          setPhase('end')
          pausedUntil = now + pauseDuration
        }
      } else if (currentPhase === 'end') {
        currentPhase = 'scrolling-left'
        setPhase('scrolling-left')
      } else if (currentPhase === 'scrolling-left') {
        offsetRef.current -= (speed * dt) / 1000
        if (offsetRef.current <= 0) {
          offsetRef.current = 0
          currentPhase = 'start'
          setPhase('start')
          pausedUntil = now + pauseDuration
        }
      }

      content.style.transform = `translateX(${-offsetRef.current}px)`
      rafRef.current = requestAnimationFrame(tick)
    }

    rafRef.current = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(rafRef.current)
  }, [needsScroll, speed, pauseDuration])

  return (
    <div
      ref={containerRef}
      className={`overflow-hidden ${className}`}
    >
      <span
        ref={contentRef}
        className="inline-block whitespace-nowrap will-change-transform"
        style={{ transform: needsScroll ? `translateX(${-offsetRef.current}px)` : undefined }}
      >
        {text}
      </span>
    </div>
  )
}
