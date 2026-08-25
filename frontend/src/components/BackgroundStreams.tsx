import { useEffect, useRef } from 'react'
import { useCameras } from '../hooks/useCameras'

/**
 * BackgroundStreams keeps hidden <img> elements mounted for all enabled
 * USB/MiP cameras. This maintains the MJPEG HTTP connection to the backend
 * even when the user navigates away from the camera page, so the stream
 * stays warm and resumes instantly when returning.
 *
 * The hidden images are rendered off-screen (1x1, opacity 0) so they don't
 * affect layout. The browser keeps the HTTP connection alive and continues
 * receiving frames, which means the backend streamer always has at least
 * one subscriber and never stops producing frames.
 */
export function BackgroundStreams() {
  const { cameras } = useCameras()
  const refs = useRef<Map<string, HTMLImageElement>>(new Map())

  // Clean up img elements for removed cameras
  useEffect(() => {
    const currentIds = new Set(cameras.filter(c => c.enabled && c.url).map(c => c.id))
    for (const [id, img] of refs.current) {
      if (!currentIds.has(id)) {
        img.src = ''
        refs.current.delete(id)
      }
    }
  }, [cameras])

  return (
    <div aria-hidden="true" style={{ position: 'fixed', top: -1, left: -1, width: 1, height: 1, overflow: 'hidden', opacity: 0, pointerEvents: 'none' }}>
      {cameras
        .filter((c) => c.enabled && c.url && (c.type === 'usb' || c.type === 'mipi'))
        .map((cam) => (
          <img
            key={cam.id}
            ref={(el) => {
              if (el) refs.current.set(cam.id, el)
            }}
            src={cam.url}
            alt=""
            style={{ width: 1, height: 1, display: 'block' }}
            onError={() => {
              // Retry after a short delay if the stream drops
              const img = refs.current.get(cam.id)
              if (img) {
                const src = img.src
                img.src = ''
                setTimeout(() => { img.src = src }, 2000)
              }
            }}
          />
        ))}
    </div>
  )
}
