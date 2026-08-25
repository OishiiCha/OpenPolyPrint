import { useEffect, useState } from 'react'

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

export function usePWAInstall() {
  const [installEvent, setInstallEvent] = useState<BeforeInstallPromptEvent | null>(null)
  const [installed, setInstalled] = useState(false)
  const [isHttps, setIsHttps] = useState(true)

  useEffect(() => {
    // Check if already installed (running in standalone mode)
    if (window.matchMedia('(display-mode: standalone)').matches || (navigator as any).standalone) {
      setInstalled(true)
      return
    }

    // Check if served over HTTPS or localhost — Chrome only fires
    // beforeinstallprompt on secure origins. On HTTP LAN IPs (e.g. Pi Docker),
    // the event never fires so we need to show manual instructions instead.
    const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1'
    setIsHttps(window.location.protocol === 'https:' || isLocalhost)

    const handler = (e: Event) => {
      e.preventDefault()
      setInstallEvent(e as BeforeInstallPromptEvent)
    }
    const installedHandler = () => {
      setInstalled(true)
      setInstallEvent(null)
    }

    window.addEventListener('beforeinstallprompt', handler)
    window.addEventListener('appinstalled', installedHandler)
    return () => {
      window.removeEventListener('beforeinstallprompt', handler)
      window.removeEventListener('appinstalled', installedHandler)
    }
  }, [])

  const promptInstall = async () => {
    if (!installEvent) return
    await installEvent.prompt()
    const choice = await installEvent.userChoice
    if (choice.outcome === 'accepted') {
      setInstalled(true)
    }
    setInstallEvent(null)
  }

  // canInstall: Chrome fired beforeinstallprompt (HTTPS/localhost only)
  // canInstallManual: not installed, not in standalone, and no prompt event
  //   — show a button with manual install instructions for HTTP
  return {
    canInstall: !!installEvent && !installed,
    canInstallManual: !installed && !installEvent,
    isHttps,
    installed,
    promptInstall,
  }
}
