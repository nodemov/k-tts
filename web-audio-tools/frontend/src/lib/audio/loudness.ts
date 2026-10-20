import { channelData, cloneBuffer, peakLevel } from './buffer';
import { dbToGain, gainToDb } from './context';
import { limitPeaks } from './limiter';

/** ITU-R BS.1770-4 integrated loudness (LUFS) with K-weighting and gating. */

type Biquad = [b0: number, b1: number, b2: number, a1: number, a2: number];

function kWeighting(fs: number): [Biquad, Biquad] {
	let f0 = 1681.974450955533;
	const G = 3.999843853973347;
	let Q = 0.7071752369554196;
	let K = Math.tan((Math.PI * f0) / fs);
	const Vh = Math.pow(10, G / 20);
	const Vb = Math.pow(Vh, 0.4996667741545416);
	let a0 = 1 + K / Q + K * K;
	const shelf: Biquad = [
		(Vh + (Vb * K) / Q + K * K) / a0,
		(2 * (K * K - Vh)) / a0,
		(Vh - (Vb * K) / Q + K * K) / a0,
		(2 * (K * K - 1)) / a0,
		(1 - K / Q + K * K) / a0
	];
	f0 = 38.13547087602444;
	Q = 0.5003270373238773;
	K = Math.tan((Math.PI * f0) / fs);
	a0 = 1 + K / Q + K * K;
	const highpass: Biquad = [1, -2, 1, (2 * (K * K - 1)) / a0, (1 - K / Q + K * K) / a0];
	return [shelf, highpass];
}

function filter(x: Float32Array, [b0, b1, b2, a1, a2]: Biquad): Float32Array {
	const y = new Float32Array(x.length);
	let x1 = 0,
		x2 = 0,
		y1 = 0,
		y2 = 0;
	for (let i = 0; i < x.length; i++) {
		const v = b0 * x[i] + b1 * x1 + b2 * x2 - a1 * y1 - a2 * y2;
		x2 = x1;
		x1 = x[i];
		y2 = y1;
		y1 = v;
		y[i] = v;
	}
	return y;
}

export interface LoudnessReport {
	integrated: number;
	peakDb: number;
	shortTermMax: number;
}

export function measureLoudness(buf: AudioBuffer): LoudnessReport {
	const fs = buf.sampleRate;
	const [shelf, hp] = kWeighting(fs);
	const weighted = channelData(buf)
		.slice(0, 2)
		.map((ch) => filter(filter(ch, shelf), hp));

	const block = Math.round(0.4 * fs);
	const step = Math.round(0.1 * fs);
	const powers: number[] = [];
	for (let start = 0; start + block <= buf.length; start += step) {
		let sum = 0;
		for (const ch of weighted) {
			let s = 0;
			for (let i = start; i < start + block; i++) s += ch[i] * ch[i];
			sum += s / block;
		}
		powers.push(sum);
	}
	if (powers.length === 0) {
		let sum = 0;
		for (const ch of weighted) sum += ch.reduce((a, v) => a + v * v, 0) / Math.max(1, ch.length);
		powers.push(sum);
	}

	const toLufs = (p: number) => -0.691 + 10 * Math.log10(p);
	const mean = (arr: number[]) => arr.reduce((a, b) => a + b, 0) / Math.max(1, arr.length);
	const absGated = powers.filter((p) => toLufs(p) > -70);
	const relThreshold = toLufs(mean(absGated)) - 10;
	const gated = absGated.filter((p) => toLufs(p) > relThreshold);

	const shortTerm: number[] = [];
	for (let i = 0; i + 30 <= powers.length; i += 10) shortTerm.push(toLufs(mean(powers.slice(i, i + 30))));

	return {
		integrated: gated.length ? toLufs(mean(gated)) : -Infinity,
		peakDb: gainToDb(peakLevel(buf)),
		shortTermMax: shortTerm.length ? Math.max(...shortTerm) : toLufs(mean(powers))
	};
}

/** Apply gain to hit `targetLufs`, then a look-ahead limiter keeps peaks under `ceilingDb`. */
export function normalizeLoudness(buf: AudioBuffer, targetLufs: number, ceilingDb = -1): AudioBuffer {
	const { integrated } = measureLoudness(buf);
	if (!Number.isFinite(integrated)) return cloneBuffer(buf);
	const gain = dbToGain(targetLufs - integrated);
	const out = cloneBuffer(buf);
	for (const ch of channelData(out)) for (let i = 0; i < ch.length; i++) ch[i] *= gain;
	return peakLevel(out) > dbToGain(ceilingDb) ? limitPeaks(out, ceilingDb) : out;
}

export const LOUDNESS_TARGETS = [
	{ value: -14, label: 'Streaming (Spotify/YouTube) −14' },
	{ value: -16, label: 'Podcast / Apple −16' },
	{ value: -19, label: 'Podcast mono −19' },
	{ value: -23, label: 'Broadcast EBU R128 −23' },
	{ value: -9, label: 'Loud / Club −9' }
];
