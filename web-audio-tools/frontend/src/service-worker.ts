/// <reference lib="webworker" />
import { version } from '$app/env';
import { assets, immutable, prerendered } from '$app/manifest';
import { self } from '$app/service-worker';

const SHELL = `wat-shell-${version}`;
const RUNTIME = 'wat-runtime-v1';

// ffmpeg-core.wasm is ~30 MB, so it is cached on first use instead of on install.
const PRECACHE = [...immutable, ...assets, ...prerendered]
	.map((a) => a.path)
	.filter((p) => !p.includes('ffmpeg-core'));

self.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(SHELL)
			.then((cache) => cache.addAll(['./', ...PRECACHE]))
			.then(() => self.skipWaiting())
	);
});

self.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.filter((k) => k !== SHELL && k !== RUNTIME).map((k) => caches.delete(k))))
			.then(() => self.clients.claim())
	);
});

self.addEventListener('fetch', (event) => {
	const { request } = event;
	const url = new URL(request.url);
	if (request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/')) return;

	if (request.mode === 'navigate') {
		event.respondWith(
			fetch(request).catch(async () => (await caches.match(request)) ?? (await caches.match('./')) ?? Response.error())
		);
		return;
	}

	event.respondWith(
		caches.match(request).then(
			(hit) =>
				hit ??
				fetch(request).then((res) => {
					if (res.ok && res.type === 'basic') {
						const copy = res.clone();
						void caches.open(RUNTIME).then((c) => c.put(request, copy));
					}
					return res;
				})
		)
	);
});
