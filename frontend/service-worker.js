// No response is stored. Every navigation and Connect request goes to the
// current authenticated host, including after installation and refresh.
self.addEventListener("install", () => { self.skipWaiting(); });
self.addEventListener("activate", event => { event.waitUntil(self.clients.claim()); });
self.addEventListener("fetch", event => {
  const url = new URL(event.request.url);
  if (event.request.method === "GET" && url.origin === self.location.origin &&
      (url.pathname.startsWith("/assets/") || url.pathname.startsWith("/icons/"))) {
    event.respondWith(fetch(event.request));
  }
});
