// OpenPolyPrint Service Worker
// Network-first for everything; cache only used when offline.

const CACHE_NAME = 'openpolyprint-v2';

// Install: skip waiting to activate immediately
self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting());
});

// Activate: clean up old caches and claim clients
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

// Fetch: network-first for everything, fall back to cache only when offline
self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);

  // Never intercept API requests, MJPEG streams, or recordings
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/recordings/')) {
    return;
  }

  // Navigation requests: network-first, fall back to cached index.html
  if (req.mode === 'navigate') {
    event.respondWith(
      fetch(req)
        .then((res) => {
          const copy = res.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put('/index.html', copy));
          return res;
        })
        .catch(() => caches.match('/index.html').then((r) => r || caches.match('/')))
    );
    return;
  }

  // Static assets: network-first, fall back to cache only when offline
  event.respondWith(
    fetch(req)
      .then((res) => {
        if (res && res.status === 200 && res.type === 'basic') {
          const copy = res.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(req, copy));
        }
        return res;
      })
      .catch(() => caches.match(req))
  );
});
