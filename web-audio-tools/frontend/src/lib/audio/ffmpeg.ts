import type { FFmpeg } from '@ffmpeg/ffmpeg';

let instance: Promise<FFmpeg> | null = null;
let queue: Promise<unknown> = Promise.resolve();
const progressListeners = new Set<(ratio: number) => void>();

/** Lazily loads ffmpeg.wasm (~30 MB) on first use only. */
export function loadFFmpeg(): Promise<FFmpeg> {
	instance ??= (async () => {
		const [{ FFmpeg }, core, wasm] = await Promise.all([
			import('@ffmpeg/ffmpeg'),
			import('@ffmpeg/core?url'),
			import('@ffmpeg/core/wasm?url')
		]);
		const ffmpeg = new FFmpeg();
		ffmpeg.on('progress', ({ progress }) => {
			for (const fn of progressListeners) fn(Math.max(0, Math.min(1, progress)));
		});
		await ffmpeg.load({ coreURL: core.default, wasmURL: wasm.default });
		return ffmpeg;
	})().catch((err) => {
		instance = null;
		throw err;
	});
	return instance;
}

export function isFFmpegLoaded(): boolean {
	return instance !== null;
}

/** ffmpeg.wasm can only run one command at a time, so calls are serialised. */
export function runFFmpeg(
	input: Blob,
	inputName: string,
	outputName: string,
	args: string[],
	onProgress?: (ratio: number) => void
): Promise<Blob> {
	const task = queue.then(async () => {
		const ffmpeg = await loadFFmpeg();
		if (onProgress) progressListeners.add(onProgress);
		try {
			await ffmpeg.writeFile(inputName, new Uint8Array(await input.arrayBuffer()));
			const code = await ffmpeg.exec(['-i', inputName, ...args, outputName]);
			if (code !== 0) throw new Error(`ffmpeg exited with code ${code}`);
			const data = await ffmpeg.readFile(outputName);
			const bytes = typeof data === 'string' ? new TextEncoder().encode(data) : data;
			return new Blob([bytes.slice()], { type: mimeFor(outputName) });
		} finally {
			if (onProgress) progressListeners.delete(onProgress);
			await ffmpeg.deleteFile(inputName).catch(() => {});
			await ffmpeg.deleteFile(outputName).catch(() => {});
		}
	});
	queue = task.catch(() => {});
	return task;
}

export function transcodeToWav(file: Blob): Promise<Blob> {
	return runFFmpeg(file, 'source.bin', 'decoded.wav', ['-vn', '-c:a', 'pcm_f32le']);
}

function mimeFor(name: string): string {
	const ext = name.split('.').pop() ?? '';
	return (
		{
			mp3: 'audio/mpeg',
			ogg: 'audio/ogg',
			flac: 'audio/flac',
			m4a: 'audio/mp4',
			wav: 'audio/wav',
			opus: 'audio/ogg'
		}[ext] ?? 'application/octet-stream'
	);
}
