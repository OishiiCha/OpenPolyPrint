import { useEffect, useState } from 'react'

export function usePushNotifications() {
  const [supported, setSupported] = useState(false)
  const [permission, setPermission] = useState<NotificationPermission>('default')
  const [subscribed, setSubscribed] = useState(false)
  const [vapidKey, setVapidKey] = useState<string | null>(null)

  useEffect(() => {
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
      return
    }
    setSupported(true)
    setPermission(Notification.permission)

    // Fetch VAPID public key
    fetch('/api/push/vapid-key')
      .then((r) => r.json())
      .then((data) => {
        if (data.key) {
          setVapidKey(data.key)
          // Check if already subscribed
          navigator.serviceWorker.ready
            .then((reg) => reg.pushManager.getSubscription())
            .then((sub) => {
              if (sub) setSubscribed(true)
            })
            .catch(() => {})
        }
      })
      .catch(() => {})
  }, [])

  const subscribe = async () => {
    if (!vapidKey) return false

    const perm = await Notification.requestPermission()
    setPermission(perm)
    if (perm !== 'granted') return false

    const reg = await navigator.serviceWorker.ready
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: vapidKey,
    })

    await fetch('/api/push/subscribe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(sub),
    })

    setSubscribed(true)
    return true
  }

  const unsubscribe = async () => {
    const reg = await navigator.serviceWorker.ready
    const sub = await reg.pushManager.getSubscription()
    if (sub) {
      await sub.unsubscribe()
      await fetch('/api/push/unsubscribe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ endpoint: sub.endpoint }),
      })
    }
    setSubscribed(false)
  }

  return { supported, permission, subscribed, subscribe, unsubscribe }
}
