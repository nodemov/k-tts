import { createBuffer, dbToGain } from './context';

/** Non-destructive AudioBuffer operations. Every function returns a new buffer. */

export function channelData(buf: AudioBuffer): Float32Array<ArrayBuffer>[] {
	return Array.from({ length: buf.numberOfChannels }, (_, c) => buf.getChannelData(c));
}

export function fromChannels(channels: Float32Array[], sampleRate: number): AudioBuffer {
	const length = channels[0]?.length ?? 0;
	const out = createBuffer(channels.length, length, sampleRate);
	channels.forEach((ch, c) => out.getChannelData(c).set(ch.subarray(0, out.length)));
	return out;
}

export function cloneBuffer(buf: AudioBuffer): AudioBuffer {
	return fromChannels(channelData(buf), buf.sampleRate);
}

function toFrames(buf: AudioBuffer, seconds: number): number {
	return Math.min(buf.length, Math.max(0, Math.round(seconds * buf.sampleRate)));
}

function range(buf: AudioBuffer, start?: number, end?: number): [number, number] {
	const a = start === undefined ? 0 : toFrames(buf, start);
	const b = end === undefined ? buf.length : toFrames(buf, end);
	return a <= b ? [a, b] : [b, a];
}

export function sliceBuffer(buf: AudioBuffer, start: number, end: number): AudioBuffer {
	const [a, b] = range(buf, start, end);
	return fromChannels(
		channelData(buf).map((ch) => ch.slice(a, Math.max(a + 1, b))),
		buf.sampleRate
	);
}

export function removeRange(buf: AudioBuffer, start: number, end: number): AudioBuffer {
	const [a, b] = range(buf, start, end);
	const length = Math.max(1, buf.length - (b - a));
	return fromChannels(
		channelData(buf).map((ch) => {
			const out = new Float32Array(length);
			out.set(ch.subarray(0, a));
			out.set(ch.subarray(b), a);
			return out;
		}),
		buf.sampleRate
	);
}

/** Match channel count by duplicating or averaging channels. */
export function matchChannels(buf: AudioBuffer, channels: number): AudioBuffer {
	if (buf.numberOfChannels === channels) return buf;
	const src = channelData(buf);
	const out = createBuffer(channels, buf.length, buf.sampleRate);
	for (let c = 0; c < channels; c++) {
		const dst = out.getChannelData(c);
		if (src.length === 1) dst.set(src[0]);
		else if (channels === 1) {
			for (let i = 0; i < buf.length; i++) {
				let sum = 0;
				for (const ch of src) sum += ch[i];
				dst[i] = sum / src.length;
			}
		} else dst.set(src[Math.min(c, src.length - 1)]);
	}
	return out;
}

export async function resampleBuffer(buf: AudioBuffer, sampleRate: number): Promise<AudioBuffer> {
	if (buf.sampleRate === sampleRate) return buf;
	const length = Math.ceil(buf.duration * sampleRate);
	const ctx = new OfflineAudioContext(buf.numberOfChannels, length, sampleRate);
	const src = new AudioBufferSourceNode(ctx, { buffer: buf });
	src.connect(ctx.destination);
	src.start();
	return ctx.startRendering();
}

export async function insertBuffer(
	buf: AudioBuffer,
	at: number,
	clip: AudioBuffer,
	replaceUntil?: number
): Promise<AudioBuffer> {
	const fitted = matchChannels(await resampleBuffer(clip, buf.sampleRate), buf.numberOfChannels);
	const [a, b] = range(buf, at, replaceUntil ?? at);
	const length = buf.length - (b - a) + fitted.length;
	const clipData = channelData(fitted);
	return fromChannels(
		channelData(buf).map((ch, c) => {
			const out = new Float32Array(length);
			out.set(ch.subarray(0, a));
			out.set(clipData[c], a);
			out.set(ch.subarray(b), a + fitted.length);
			return out;
		}),
		buf.sampleRate
	);
}

export function insertSilence(buf: AudioBuffer, at: number, seconds: number): Promise<AudioBuffer> {
	const silence = createBuffer(buf.numberOfChannels, seconds * buf.sampleRate, buf.sampleRate);
	return insertBuffer(buf, at, silence);
}

function mapRange(
	buf: AudioBuffer,
	start: number | undefined,
	end: number | undefined,
	fn: (segment: Float32Array, channel: number) => void
): AudioBuffer {
	const out = cloneBuffer(buf);
	const [a, b] = range(buf, start, end);
	channelData(out).forEach((ch, c) => fn(ch.subarray(a, b), c));
	return out;
}

export function silenceRange(buf: AudioBuffer, start: number, end: number): AudioBuffer {
	return mapRange(buf, start, end, (seg) => seg.fill(0));
}

export type FadeCurve = 'linear' | 'exponential' | 'equal-power';

function fadeShape(t: number, curve: FadeCurve): number {
	if (curve === 'equal-power') return Math.sin((t * Math.PI) / 2);
	if (curve === 'exponential') return t * t;
	return t;
}

export function fadeRange(
	buf: AudioBuffer,
	start: number,
	end: number,
	direction: 'in' | 'out',
	curve: FadeCurve = 'equal-power'
): AudioBuffer {
	return mapRange(buf, start, end, (seg) => {
		const n = seg.length;
		for (let i = 0; i < n; i++) {
			const t = n > 1 ? i / (n - 1) : 1;
			seg[i] *= fadeShape(direction === 'in' ? t : 1 - t, curve);
		}
	});
}

export function gainRange(buf: AudioBuffer, db: number, start?: number, end?: number): AudioBuffer {
	const g = dbToGain(db);
	return mapRange(buf, start, end, (seg) => {
		for (let i = 0; i < seg.length; i++) seg[i] *= g;
	});
}

export function reverseRange(buf: AudioBuffer, start?: number, end?: number): AudioBuffer {
	return mapRange(buf, start, end, (seg) => seg.reverse());
}

export function peakLevel(buf: AudioBuffer): number {
	let peak = 0;
	for (const ch of channelData(buf)) {
		for (let i = 0; i < ch.length; i++) {
			const v = Math.abs(ch[i]);
			if (v > peak) peak = v;
		}
	}
	return peak;
}

export function normalizePeak(buf: AudioBuffer, targetDb = -1, start?: number, end?: number): AudioBuffer {
	const region = start === undefined ? buf : sliceBuffer(buf, start, end ?? buf.duration);
	const peak = peakLevel(region);
	if (peak === 0) return cloneBuffer(buf);
	const gain = dbToGain(targetDb) / peak;
	return mapRange(buf, start, end, (seg) => {
		for (let i = 0; i < seg.length; i++) seg[i] *= gain;
	});
}

export function concatBuffers(buffers: AudioBuffer[]): AudioBuffer {
	const sampleRate = buffers[0].sampleRate;
	const channels = Math.max(...buffers.map((b) => b.numberOfChannels));
	const fitted = buffers.map((b) => matchChannels(b, channels));
	const length = fitted.reduce((n, b) => n + b.length, 0);
	const out = createBuffer(channels, length, sampleRate);
	let offset = 0;
	for (const b of fitted) {
		for (let c = 0; c < channels; c++) out.getChannelData(c).set(b.getChannelData(c), offset);
		offset += b.length;
	}
	return out;
}

/** Downsampled min/max peaks for drawing waveforms quickly. */
export function computePeaks(buf: AudioBuffer, buckets: number): Float32Array {
	const out = new Float32Array(buckets * 2);
	const chans = channelData(buf);
	const size = buf.length / buckets;
	for (let b = 0; b < buckets; b++) {
		const from = Math.floor(b * size);
		const to = Math.max(from + 1, Math.floor((b + 1) * size));
		const stride = Math.max(1, Math.floor((to - from) / 256));
		let min = 0;
		let max = 0;
		for (const ch of chans) {
			for (let i = from; i < to && i < ch.length; i += stride) {
				const v = ch[i];
				if (v < min) min = v;
				if (v > max) max = v;
			}
		}
		out[b * 2] = min;
		out[b * 2 + 1] = max;
	}
	return out;
}
