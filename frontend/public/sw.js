// Infinite Minesweeper Service Worker
// Provides offline support and performance optimization for the game

const CACHE_VERSION = 'infinite-minesweeper-v1';
const STATIC_CACHE = `${CACHE_VERSION}-static`;
const DYNAMIC_CACHE = `${CACHE_VERSION}-dynamic`;
const CHUNK_CACHE = `${CACHE_VERSION}-chunks`;

// Static assets to cache on install
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/manifest.json',
  '/favicon.ico',
  '/pwa-icon-192.png',
  '/pwa-icon-512.png',
  // Main JS and CSS will be added dynamically based on Vite's build output
];

// Game-specific routes that should be cached
const GAME_ROUTES = [
  '/leaderboard',
  '/hotspot'
];

// Assets that should never be cached (real-time data)
const NEVER_CACHE = [
  '/ws',
  '/spectate'
];

// Install event - cache static assets
self.addEventListener('install', (event) => {
  console.log('[SW] Installing...');
  
  event.waitUntil(
    caches.open(STATIC_CACHE)
      .then((cache) => {
        console.log('[SW] Caching static assets');
        return cache.addAll(STATIC_ASSETS);
      })
      .then(() => {
        // Skip waiting to activate immediately
        return self.skipWaiting();
      })
      .catch((error) => {
        console.error('[SW] Failed to cache static assets:', error);
      })
  );
});

// Activate event - clean up old caches
self.addEventListener('activate', (event) => {
  console.log('[SW] Activating...');
  
  event.waitUntil(
    caches.keys()
      .then((cacheNames) => {
        return Promise.all(
          cacheNames.map((cacheName) => {
            // Delete old cache versions
            if (cacheName !== STATIC_CACHE && 
                cacheName !== DYNAMIC_CACHE && 
                cacheName !== CHUNK_CACHE &&
                cacheName.startsWith('infinite-minesweeper-')) {
              console.log('[SW] Deleting old cache:', cacheName);
              return caches.delete(cacheName);
            }
          })
        );
      })
      .then(() => {
        // Claim all clients immediately
        return self.clients.claim();
      })
  );
});

// Fetch event - implement caching strategies
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);
  
  // Skip caching for WebSocket connections and never-cache routes
  if (NEVER_CACHE.some(route => url.pathname.startsWith(route)) ||
      request.headers.get('upgrade') === 'websocket') {
    return;
  }

  // Handle different types of requests with appropriate caching strategies
  if (isStaticAsset(url)) {
    // Static assets: Cache First strategy
    event.respondWith(cacheFirst(request, STATIC_CACHE));
  } else if (isGameRoute(url)) {
    // Game data: Network First with fallback
    event.respondWith(networkFirst(request, DYNAMIC_CACHE));
  } else if (isChunkData(url)) {
    // Chunk data: Stale While Revalidate for performance
    event.respondWith(staleWhileRevalidate(request, CHUNK_CACHE));
  } else {
    // Default: Network First for other requests
    event.respondWith(networkFirst(request, DYNAMIC_CACHE));
  }
});

// Cache First strategy - good for static assets
async function cacheFirst(request, cacheName) {
  try {
    const cache = await caches.open(cacheName);
    const cachedResponse = await cache.match(request);
    
    if (cachedResponse) {
      return cachedResponse;
    }
    
    const networkResponse = await fetch(request);
    if (networkResponse.status === 200) {
      cache.put(request, networkResponse.clone());
    }
    
    return networkResponse;
  } catch (error) {
    console.error('[SW] Cache First failed:', error);
    return new Response('Offline', { 
      status: 503, 
      statusText: 'Service Unavailable' 
    });
  }
}

// Network First strategy - good for dynamic content
async function networkFirst(request, cacheName) {
  try {
    const networkResponse = await fetch(request);
    
    if (networkResponse.status === 200) {
      const cache = await caches.open(cacheName);
      cache.put(request, networkResponse.clone());
    }
    
    return networkResponse;
  } catch (error) {
    console.log('[SW] Network failed, trying cache:', error);
    
    const cache = await caches.open(cacheName);
    const cachedResponse = await cache.match(request);
    
    if (cachedResponse) {
      return cachedResponse;
    }
    
    // Return offline page for navigation requests
    if (request.mode === 'navigate') {
      const offlineResponse = await cache.match('/');
      if (offlineResponse) {
        return offlineResponse;
      }
    }
    
    return new Response('Offline', { 
      status: 503, 
      statusText: 'Service Unavailable' 
    });
  }
}

// Stale While Revalidate - good for chunk data that changes frequently
async function staleWhileRevalidate(request, cacheName) {
  const cache = await caches.open(cacheName);
  const cachedResponse = await cache.match(request);
  
  const fetchPromise = fetch(request)
    .then((networkResponse) => {
      if (networkResponse.status === 200) {
        cache.put(request, networkResponse.clone());
      }
      return networkResponse;
    })
    .catch(() => null);
  
  // Return cached version immediately if available
  if (cachedResponse) {
    // Update cache in background
    fetchPromise.catch(() => {}); // Ignore errors in background update
    return cachedResponse;
  }
  
  // Otherwise wait for network
  const networkResponse = await fetchPromise;
  if (networkResponse) {
    return networkResponse;
  }
  
  return new Response('Offline', { 
    status: 503, 
    statusText: 'Service Unavailable' 
  });
}

// Helper functions to categorize requests
function isStaticAsset(url) {
  return (
    url.pathname.startsWith('/assets/') ||
    url.pathname.endsWith('.js') ||
    url.pathname.endsWith('.css') ||
    url.pathname.endsWith('.png') ||
    url.pathname.endsWith('.jpg') ||
    url.pathname.endsWith('.ico') ||
    url.pathname.endsWith('.json') ||
    url.pathname === '/' ||
    url.pathname === '/index.html' ||
    url.pathname === '/manifest.json'
  );
}

function isGameRoute(url) {
  return GAME_ROUTES.some(route => url.pathname.startsWith(route));
}

function isChunkData(url) {
  // This might need adjustment based on actual chunk API endpoints
  return (
    url.pathname.startsWith('/api/chunk') ||
    url.pathname.startsWith('/chunk') ||
    url.pathname.includes('minimap')
  );
}

// Handle background sync for offline actions (future enhancement)
self.addEventListener('sync', (event) => {
  if (event.tag === 'background-sync') {
    console.log('[SW] Background sync triggered');
    // Could be used for offline game moves synchronization
  }
});

// Handle push notifications (future enhancement)
self.addEventListener('push', (event) => {
  console.log('[SW] Push message received');
  
  const options = {
    body: 'New activity in Infinite Minesweeper!',
    icon: '/pwa-icon-192.png',
    badge: '/favicon.ico',
    vibrate: [100, 50, 100],
    data: {
      dateOfArrival: Date.now(),
      primaryKey: '1'
    }
  };
  
  event.waitUntil(
    self.registration.showNotification('Infinite Minesweeper', options)
  );
});

// Handle notification clicks
self.addEventListener('notificationclick', (event) => {
  console.log('[SW] Notification click received');
  
  event.notification.close();
  
  event.waitUntil(
    clients.openWindow('/')
  );
});

console.log('[SW] Service Worker loaded');