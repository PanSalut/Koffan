// Koffan Service Worker - Offline Support
const CACHE_VERSION = 'koffan-v1';
const STATIC_CACHE = CACHE_VERSION + '-static';
const DYNAMIC_CACHE = CACHE_VERSION + '-dynamic';

// Static assets to cache on install
const STATIC_ASSETS = [
    '/',
    '/static/app.js',
    '/static/offline-storage.js',
    '/static/manifest.json',
    '/static/koffan-logo.webp',
    '/static/icon-192.png',
    '/static/icon-512.png',
    '/static/favicon.ico',
    '/static/favicon-96.png',
    '/static/apple-touch-icon.png'
];

// CDN assets to cache
const CDN_ASSETS = [
    'https://cdn.tailwindcss.com',
    'https://unpkg.com/htmx.org@1.9.10',
    'https://unpkg.com/htmx.org@1.9.10/dist/ext/ws.js',
    'https://unpkg.com/@alpinejs/collapse@3.13.5/dist/cdn.min.js',
    'https://unpkg.com/alpinejs@3.13.5/dist/cdn.min.js'
];

// Install event - cache static assets
self.addEventListener('install', (event) => {
    console.log('[SW] Installing service worker...');
    event.waitUntil(
        caches.open(STATIC_CACHE)
            .then(cache => {
                console.log('[SW] Caching static assets');
                // Cache local assets
                const localPromise = cache.addAll(STATIC_ASSETS).catch(err => {
                    console.warn('[SW] Some static assets failed to cache:', err);
                });
                // Cache CDN assets (may fail due to CORS)
                const cdnPromise = Promise.all(
                    CDN_ASSETS.map(url =>
                        fetch(url, { mode: 'cors' })
                            .then(response => {
                                if (response.ok) {
                                    return cache.put(url, response);
                                }
                            })
                            .catch(() => console.warn('[SW] Failed to cache CDN:', url))
                    )
                );
                return Promise.all([localPromise, cdnPromise]);
            })
            .then(() => self.skipWaiting())
    );
});

// Activate event - cleanup old caches
self.addEventListener('activate', (event) => {
    console.log('[SW] Activating service worker...');
    event.waitUntil(
        caches.keys()
            .then(keys => {
                return Promise.all(
                    keys.filter(key => {
                        return key.startsWith('koffan-') &&
                               key !== STATIC_CACHE &&
                               key !== DYNAMIC_CACHE;
                    }).map(key => {
                        console.log('[SW] Deleting old cache:', key);
                        return caches.delete(key);
                    })
                );
            })
            .then(() => self.clients.claim())
    );
});

// Fetch event - handle requests
self.addEventListener('fetch', (event) => {
    const url = new URL(event.request.url);

    // Skip WebSocket connections
    if (url.pathname === '/ws') {
        return;
    }

    // Skip non-GET requests (let them go through, app.js handles offline queueing)
    if (event.request.method !== 'GET') {
        return;
    }

    // Skip API data endpoint - always fetch fresh when online
    if (url.pathname === '/api/data') {
        event.respondWith(networkFirst(event.request));
        return;
    }

    // Static assets - Cache First
    if (url.pathname.startsWith('/static/')) {
        event.respondWith(cacheFirst(event.request));
        return;
    }

    // CDN assets - Cache First
    if (CDN_ASSETS.some(cdn => event.request.url.startsWith(cdn.split('/').slice(0, 3).join('/')))) {
        event.respondWith(cacheFirst(event.request));
        return;
    }

    // HTML pages (/, /login) - Network First with cache fallback
    if (event.request.headers.get('accept')?.includes('text/html')) {
        event.respondWith(networkFirst(event.request));
        return;
    }

    // Stats and other API - Network First
    if (url.pathname === '/stats' || url.pathname.startsWith('/sections/') || url.pathname.startsWith('/items/')) {
        event.respondWith(networkFirst(event.request));
        return;
    }

    // Default - Network First
    event.respondWith(networkFirst(event.request));
});

// Cache First strategy - for static assets
async function cacheFirst(request) {
    const cached = await caches.match(request);
    if (cached) {
        return cached;
    }

    try {
        const response = await fetch(request);
        if (response.ok) {
            const cache = await caches.open(STATIC_CACHE);
            cache.put(request, response.clone());
        }
        return response;
    } catch (error) {
        console.warn('[SW] Cache first failed:', request.url);
        // Return a simple offline page for HTML requests
        if (request.headers.get('accept')?.includes('text/html')) {
            return new Response('<html><body style="font-family:system-ui;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#fafaf9"><div style="text-align:center"><h1 style="color:#78716c">Koffan Offline</h1><p style="color:#a8a29e">Check your connection</p></div></body></html>', {
                headers: { 'Content-Type': 'text/html' }
            });
        }
        throw error;
    }
}

// Network First strategy - for dynamic content
async function networkFirst(request) {
    try {
        const response = await fetch(request);
        if (response.ok) {
            const cache = await caches.open(DYNAMIC_CACHE);
            cache.put(request, response.clone());
        }
        return response;
    } catch (error) {
        console.log('[SW] Network first fallback to cache:', request.url);
        const cached = await caches.match(request);
        if (cached) {
            return cached;
        }

        // Return offline fallback for HTML
        if (request.headers.get('accept')?.includes('text/html')) {
            // Try to return cached main page
            const mainPage = await caches.match('/');
            if (mainPage) {
                return mainPage;
            }
            return new Response('<html><body style="font-family:system-ui;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#fafaf9"><div style="text-align:center"><h1 style="color:#78716c">Koffan Offline</h1><p style="color:#a8a29e">Check your connection</p></div></body></html>', {
                headers: { 'Content-Type': 'text/html' }
            });
        }

        throw error;
    }
}

// Listen for messages from the app
self.addEventListener('message', (event) => {
    if (event.data && event.data.type === 'SKIP_WAITING') {
        self.skipWaiting();
    }

    if (event.data && event.data.type === 'CLEAR_CACHE') {
        event.waitUntil(
            caches.keys().then(keys => {
                return Promise.all(keys.map(key => caches.delete(key)));
            })
        );
    }
});
