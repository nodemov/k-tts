let shared: AudioContext | null = null;

export function getAudioContext(): AudioContext {
	shared ??= new AudioContext({ latencyHint: 'interactive' });
	return shared;
}

export async function resumeAudio(): Promise<AudioContext> {
	const ctx = getAudioContext();
	if (ctx.state !== 'running') await ctx.resume();
	return ctx;
}

export function createBuffer(channels: number, length: number, sampleRate: number): AudioBuffer {
	return new AudioBuffer({
		numberOfChannels: Math.max(1, channels),
		length: Math.max(1, Math.round(length)),
		sampleRate
	});
}

/** Decode any browser-supported file; falls back to ffmpeg.wasm for exotic formats. */
export async function decodeFile(file: Blob): Promise<AudioBuffer> {
	const data = await file.arrayBuffer();
	try {
		return await getAudioContext().decodeAudioData(data.slice(0));
	} catch {
		const { transcodeToWav } = await import('./ffmpeg');
		const wav = await transcodeToWav(file);
		return getAudioContext().decodeAudioData(await wav.arrayBuffer());
	}
}

export function formatTime(seconds: number, withMs = true): string {
	if (!Number.isFinite(seconds) || seconds < 0) seconds = 0;
	const m = Math.floor(seconds / 60);
	const s = Math.floor(seconds % 60);
	const base = `${m}:${s.toString().padStart(2, '0')}`;
	if (!withMs) return base;
	const ms = Math.floor((seconds % 1) * 1000);
	return `${base}.${ms.toString().padStart(3, '0')}`;
}

export function dbToGain(db: number): number {
	return Math.pow(10, db / 20);
}

export function gainToDb(gain: number): number {
	return gain > 0 ? 20 * Math.log10(gain) : -Infinity;
}
