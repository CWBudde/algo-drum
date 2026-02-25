const CACHE_VERSION = "algo-drum-v1";
const CACHE_NAME = `${CACHE_VERSION}-runtime`;

const PRECACHE_PATHS = [
  "",
  "index.html",
  "site.webmanifest",
  "favicon-32x32.png",
  "favicon-64x64.png",
  "apple-touch-icon.png",
  "apple-touch-icon-180.png",
  "pwa-192x192.png",
  "pwa-512x512.png",
  "wasm_exec.js",
  "algo_drum.wasm",
];

function toScopedUrl(path) {
  return new URL(path, self.registration.scope).toString();
}

async function precacheAssets() {
  const cache = await caches.open(CACHE_NAME);

  await Promise.all(
    PRECACHE_PATHS.map(async (path) => {
      const url = toScopedUrl(path);
      try {
        const response = await fetch(url, { cache: "no-cache" });
        if (response.ok) {
          await cache.put(url, response);
        }
      } catch {
        // Ignore a failed precache entry to keep SW install resilient.
      }
    }),
  );
}

self.addEventListener("install", (event) => {
  event.waitUntil(precacheAssets().then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys
          .filter((key) => key.startsWith("algo-drum-") && key !== CACHE_NAME)
          .map((key) => caches.delete(key)),
      ),
    ).then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") {
    return;
  }

  const requestUrl = new URL(request.url);
  if (requestUrl.origin !== self.location.origin) {
    return;
  }

  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request).catch(() =>
        caches.match(toScopedUrl("index.html")),
      ),
    );
    return;
  }

  event.respondWith(
    caches.match(request).then((cached) => {
      const networkFetch = fetch(request).then((response) => {
        if (response && response.ok) {
          void caches.open(CACHE_NAME).then((cache) => cache.put(request, response.clone()));
        }
        return response;
      });

      return cached ?? networkFetch;
    }),
  );
});
