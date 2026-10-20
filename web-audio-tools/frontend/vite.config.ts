import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

const apiTarget = process.env.API_URL ?? 'http://localhost:8080';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter({ fallback: '200.html', precompress: true })
		})
	],
	// ffmpeg.wasm spawns its own module worker and must not be pre-bundled.
	optimizeDeps: { exclude: ['@ffmpeg/ffmpeg', '@ffmpeg/util'] },
	worker: { format: 'es' },
	server: {
		proxy: { '/api': { target: apiTarget, changeOrigin: true } }
	}
});
