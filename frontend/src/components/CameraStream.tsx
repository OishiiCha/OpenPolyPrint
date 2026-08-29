import { useEffect, useRef, useState } from 'react'

interface CameraStreamProps {
  src: string
  alt?: string
  className?: string
  onError?: () => void
}

/**
 * CameraStream is a hardware-accelerated replacement for <img> when displaying
 * MJPEG camera streams. Instead of letting the browser decode JPEGs in software
 * via an <img> tag, it:
 *
 * 1. Fetches the MJPEG stream as a ReadableStream
 * 2. Parses JPEG frames from the multipart/x-mixed-replace boundaries
 * 3. Decodes each frame using createImageBitmap() — which uses the browser's
 *    hardware-accelerated image decode pipeline (GPU)
 * 4. Renders to a <canvas> — which is GPU-composited
 *
 * This reduces CPU usage on the client and can display frames with lower latency
 * than <img> tags, especially on devices with hardware JPEG decoders.
 *
 * Falls back to a regular <img> tag if the required APIs aren't available.
 */
export function CameraStream({ src, alt = 'camera', className = '', onError }: CameraStreamProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [fallback, setFallback] = useState(false)
  const [hasFrame, setHasFrame] = useState(false)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d', { alpha: false, desynchronized: true })
    if (!ctx) { setFallback(true); return }

    // Check for required APIs — createImageBitmap is the key hardware-accelerated decode path
    if (typeof createImageBitmap !== 'function') {
      setFallback(true)
      return
    }

    let aborted = false
    let rafId = 0
    let pendingBitmap: ImageBitmap | null = null

    // Use a single reusable buffer for accumulating frame data
    let frameBuf: Uint8Array | null = null
    let frameLen = 0

    const renderFrame = () => {
      rafId = 0
      if (pendingBitmap && !aborted) {
        const bm = pendingBitmap
        pendingBitmap = null
        // Resize canvas to match the bitmap if needed
        if (canvas.width !== bm.width || canvas.height !== bm.height) {
          canvas.width = bm.width
          canvas.height = bm.height
        }
        ctx.drawImage(bm, 0, 0)
        bm.close()
        if (!hasFrame) setHasFrame(true)
      }
    }

    const scheduleRender = (bitmap: ImageBitmap) => {
      // If there's a pending bitmap that hasn't been drawn yet, close it (drop frame)
      if (pendingBitmap) pendingBitmap.close()
      pendingBitmap = bitmap
      if (!rafId) {
        rafId = requestAnimationFrame(renderFrame)
      }
    }

    const processChunk = async (chunk: Uint8Array) => {
      // Find JPEG frame boundaries in the chunk
      // JPEG starts with 0xFF 0xD8, ends with 0xFF 0xD9
      // MJPEG multipart uses "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: N\r\n\r\n"
      // But we can just look for JPEG start/end markers directly

      // Append to buffer
      if (!frameBuf) {
        frameBuf = new Uint8Array(Math.max(chunk.length * 2, 256 * 1024))
        frameLen = 0
      }
      // Grow buffer if needed
      if (frameLen + chunk.length > frameBuf.length) {
        const newBuf = new Uint8Array(Math.max(frameBuf.length * 2, frameLen + chunk.length))
        newBuf.set(frameBuf.subarray(0, frameLen))
        frameBuf = newBuf
      }
      frameBuf.set(chunk, frameLen)
      frameLen += chunk.length

      // Scan for complete JPEG frames (0xFF 0xD8 ... 0xFF 0xD9)
      let scanStart = 0
      while (scanStart < frameLen - 4) {
        // Find JPEG start marker
        let jpegStart = -1
        for (let i = scanStart; i < frameLen - 1; i++) {
          if (frameBuf[i] === 0xff && frameBuf[i + 1] === 0xd8) {
            jpegStart = i
            break
          }
        }
        if (jpegStart === -1) break

        // Find JPEG end marker
        let jpegEnd = -1
        for (let i = jpegStart + 2; i < frameLen - 1; i++) {
          if (frameBuf[i] === 0xff && frameBuf[i + 1] === 0xd9) {
            jpegEnd = i + 2 // include the end marker
            break
          }
        }
        if (jpegEnd === -1) {
          // Incomplete frame — keep data from jpegStart onwards
          if (jpegStart > 0) {
            const remaining = frameLen - jpegStart
            frameBuf.copyWithin(0, jpegStart, frameLen)
            frameLen = remaining
          }
          break
        }

        // We have a complete JPEG frame from jpegStart to jpegEnd
        const jpegData = frameBuf.subarray(jpegStart, jpegEnd)

        // Decode using hardware-accelerated createImageBitmap
        try {
          const blob = new Blob([jpegData.slice()], { type: 'image/jpeg' })
          const bitmap = await createImageBitmap(blob, {
            imageOrientation: 'none',
            premultiplyAlpha: 'none',
            colorSpaceConversion: 'none',
          })
          if (!aborted) {
            scheduleRender(bitmap)
          } else {
            bitmap.close()
          }
        } catch {
          // Decode error — skip this frame
        }

        // Move remaining data to start of buffer
        const remaining = frameLen - jpegEnd
        if (remaining > 0) {
          frameBuf.copyWithin(0, jpegEnd, frameLen)
        }
        frameLen = remaining
        scanStart = 0
      }
    }

    const startStream = async () => {
      try {
        const res = await fetch(src, {
          signal: AbortController.prototype.signal
            ? new AbortController().signal
            : undefined,
        })
        if (!res.ok || !res.body) {
          setFallback(true)
          onError?.()
          return
        }

        const reader = res.body.getReader()
        while (!aborted) {
          const { done, value } = await reader.read()
          if (done) break
          if (value) {
            await processChunk(value)
          }
        }
      } catch (e) {
        if (!aborted) {
          // Network error — fall back to img tag which has built-in reconnect
          setFallback(true)
          onError?.()
        }
      }
    }

    startStream()

    return () => {
      aborted = true
      if (rafId) cancelAnimationFrame(rafId)
      if (pendingBitmap) pendingBitmap.close()
    }
  }, [src]) // eslint-disable-line react-hooks/exhaustive-deps

  // Fallback to regular <img> tag if WebCodecs/canvas APIs aren't available
  // or if the stream fails
  if (fallback) {
    return (
      <img
        src={src}
        alt={alt}
        className={className}
        onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; onError?.() }}
      />
    )
  }

  return (
    <canvas
      ref={canvasRef}
      className={className}
      role="img"
      aria-label={alt}
      style={hasFrame ? undefined : { background: '#000' }}
    />
  )
}
