# Web Audio Tools — frontend

SvelteKit (Svelte 5, static SPA) + Tailwind v4. Audio processing runs in the
browser: Web Audio graph + AudioWorklets, Rust/WASM DSP in a worker
(`src/lib/dsp`), and ffmpeg.wasm for decoding/encoding.

```sh
npm install
npm run dev          # http://localhost:5173, proxies /api to $API_URL (default :8080)
npm run check
npm run build        # -> build/, served by the Go backend
npm run build:wasm   # rebuild src/lib/dsp/dsp.wasm from ../dsp-wasm (needs Rust + wasm32 target)
```

See `../README.md` for the whole stack.
