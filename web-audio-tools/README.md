# Web Audio Tools

เครื่องมือจัดการเสียงบนเว็บ — ตัดต่อ, ปรับเสียงให้ดีขึ้น, เอฟเฟกต์สนุกๆ, มิกเซอร์,
ซาวด์บอร์ด, visualizer, AI (แยกเสียงร้อง / ลดเสียงรบกวน / ถอดเสียงเป็นข้อความ),
TTS ภาษาไทย และ batch processing

งานส่วนใหญ่ทำในเบราว์เซอร์ทั้งหมด (ไม่ต้องอัปโหลด) จึงเร็วและเป็นส่วนตัว
server ใช้เฉพาะงานหนัก: TTS, ffmpeg ฝั่ง server และโมเดล AI

| ส่วน | ภาษา / เทคโนโลยี | หน้าที่ |
| --- | --- | --- |
| `frontend/` | TypeScript, SvelteKit 3 (Svelte 5), Tailwind v4, wavesurfer.js, ffmpeg.wasm | UI, Web Audio effect chain, AudioWorklet, export, PWA |
| `dsp-wasm/` | Rust → WebAssembly | ลดเสียงรบกวน (spectral gating), time-stretch / pitch-shift (WSOLA) |
| `backend/` | Go (stdlib `net/http`) | API, resumable upload, job queue + SSE progress, cache, TTS, ffmpeg two-pass loudnorm, เสิร์ฟ frontend |
| `ai-service/` | Python, FastAPI | DeepFilterNet, Demucs, faster-whisper (มี ffmpeg fallback) |

## เริ่มใช้งาน

ต้องมี Node 22+, Go 1.25+, ffmpeg, Python 3.12 + [uv](https://docs.astral.sh/uv/)
(Rust จำเป็นเฉพาะเมื่อแก้ `dsp-wasm/` — ไฟล์ `.wasm` ที่ build แล้วอยู่ใน repo)

```sh
cd web-audio-tools
make install        # npm + venv ของ ai-service
make frontend       # build SPA ไปที่ frontend/build
make ai             # terminal 1: AI service ที่ :8001
make backend        # terminal 2: API + เว็บที่ http://localhost:8080
```

ระหว่างพัฒนา frontend ใช้ `make dev-frontend` (http://localhost:5173, proxy `/api` ไปที่ :8080)

โมเดล AI เป็น optional: `make install-ml` (หลาย GB) — ถ้าไม่ติดตั้ง
ลดเสียงรบกวนจะใช้ ffmpeg `afftdn`, karaoke ใช้ phase cancellation, ส่วน 4 stems
และถอดเสียงจะแจ้งว่ายังไม่พร้อม หน้า **AI → Server** แสดงสถานะทั้งหมด

ทดสอบทั้งหมด: `make test`

## ตั้งค่า backend

ใช้ environment variable หรือ flag (`go run ./web-audio-tools/backend -h`)

| ตัวแปร | ค่าเริ่มต้น | |
| --- | --- | --- |
| `ADDR` | `:8080` | |
| `DATA_DIR` | `web-audio-tools/data` | ต้องเป็นโฟลเดอร์เดียวกับของ ai-service |
| `STATIC_DIR` | `web-audio-tools/frontend/build` | ว่าง = เสิร์ฟเฉพาะ API |
| `AI_SERVICE_URL` | `http://127.0.0.1:8001` | |
| `WORKERS` | `4` | จำนวน job ที่รันพร้อมกัน |
| `MAX_UPLOAD_MB` | `2048` | |
| `CORS_ORIGIN` | `*` | |
| `GOOGLE_APPLICATION_CREDENTIALS` | — | เปิดใช้ Google Cloud TTS (ไม่มีจะใช้ Translate TTS) |

ไฟล์อัปโหลดและผลลัพธ์ของ job ถูกลบอัตโนมัติหลัง 24 ชั่วโมง

## API

| Method | Path | |
| --- | --- | --- |
| `GET` | `/api/health` | สถานะ ffmpeg, Cloud TTS, AI service |
| `POST` | `/api/uploads` | `{name, size}` → `{id}` |
| `PATCH` / `HEAD` | `/api/uploads/{id}` | ส่ง chunk พร้อม `Upload-Offset` / ถาม offset ปัจจุบันเพื่อ resume |
| `POST` | `/api/uploads/{id}/complete` | ตรวจขนาด + คำนวณ sha256 (ใช้เป็น cache key) |
| `POST` | `/api/jobs` | `{kind, uploadId?, params}` — kind: `tts`, `enhance`, `convert`, `denoise`, `separate`, `transcribe` |
| `GET` | `/api/jobs/{id}/events` | Server-Sent Events ของสถานะ job |
| `GET` | `/api/jobs/{id}/files/{name}` | ดาวน์โหลดผลลัพธ์ (รองรับ Range) |
