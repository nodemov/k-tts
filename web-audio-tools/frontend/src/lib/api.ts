/** Client for the Go backend (TTS, server-side ffmpeg, AI jobs). */

const BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? '';
const CHUNK = 4 * 1024 * 1024;

export type JobKind = 'tts' | 'enhance' | 'convert' | 'denoise' | 'separate' | 'transcribe';
export type JobStatus = 'queued' | 'running' | 'done' | 'error';

export interface JobFile {
	name: string;
	size: number;
	url: string;
}

export interface Job {
	id: string;
	kind: JobKind;
	status: JobStatus;
	progress: number;
	message: string;
	error?: string;
	files: JobFile[];
	result?: Record<string, unknown>;
	cached: boolean;
	createdAt: string;
}

export interface Health {
	ok: boolean;
	ffmpeg: boolean;
	cloudTTS: boolean;
	ai: { reachable: boolean; models: Record<string, boolean> };
}

async function json<T>(res: Response): Promise<T> {
	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error((body as { error?: string }).error ?? res.statusText);
	}
	return res.json() as Promise<T>;
}

export async function getHealth(): Promise<Health | null> {
	try {
		return await json<Health>(await fetch(`${BASE}/api/health`, { signal: AbortSignal.timeout(3000) }));
	} catch {
		return null;
	}
}

/** Resumable chunked upload: retries a failed chunk from the server-reported offset. */
export async function uploadFile(file: Blob, name: string, onProgress?: (ratio: number) => void): Promise<string> {
	const { id } = await json<{ id: string }>(
		await fetch(`${BASE}/api/uploads`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name, size: file.size })
		})
	);
	let offset = 0;
	let failures = 0;
	while (offset < file.size) {
		try {
			const res = await fetch(`${BASE}/api/uploads/${id}`, {
				method: 'PATCH',
				headers: { 'Upload-Offset': String(offset), 'Content-Type': 'application/octet-stream' },
				body: file.slice(offset, offset + CHUNK)
			});
			offset = (await json<{ offset: number }>(res)).offset;
			failures = 0;
			onProgress?.(offset / file.size);
		} catch (err) {
			if (++failures > 5) throw err;
			await new Promise((r) => setTimeout(r, 500 * failures));
			const head = await fetch(`${BASE}/api/uploads/${id}`, { method: 'HEAD' });
			offset = Number(head.headers.get('Upload-Offset') ?? offset);
		}
	}
	await json(await fetch(`${BASE}/api/uploads/${id}/complete`, { method: 'POST' }));
	return id;
}

export async function createJob(kind: JobKind, params: Record<string, unknown>, uploadId?: string): Promise<Job> {
	return json<Job>(
		await fetch(`${BASE}/api/jobs`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ kind, uploadId, params })
		})
	);
}

/** Server-Sent Events stream of job updates; resolves when the job finishes. */
export function watchJob(id: string, onUpdate: (job: Job) => void): Promise<Job> {
	return new Promise((resolve, reject) => {
		const es = new EventSource(`${BASE}/api/jobs/${id}/events`);
		es.onmessage = (e) => {
			const job = JSON.parse(e.data) as Job;
			onUpdate(job);
			if (job.status === 'done' || job.status === 'error') {
				es.close();
				if (job.status === 'done') resolve(job);
				else reject(new Error(job.error ?? 'job failed'));
			}
		};
		es.onerror = () => {
			es.close();
			reject(new Error('lost connection to server'));
		};
	});
}

export function fileUrl(file: JobFile): string {
	return `${BASE}${file.url}`;
}
