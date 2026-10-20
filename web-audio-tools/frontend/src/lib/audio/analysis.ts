import { channelData, fromChannels } from './buffer';
import { dbToGain } from './context';

export interface SilenceOptions {
	thresholdDb?: number;
	/** Padding kept around detected sound. */
	keepMs?: number;
	/** Shorten internal pauses longer than this; `null` keeps them. */
	maxGapMs?: number | null;
}

/** Trim leading/trailing silence and optionally shorten long pauses. */
export function trimSilence(
	buf: AudioBuffer,
	{ thresholdDb = -48, keepMs = 120, maxGapMs = null }: SilenceOptions = {}
): AudioBuffer {
	const sr = buf.sampleRate;
	const win = Math.max(1, Math.round(0.02 * sr));
	const chans = channelData(buf);
	const threshold = dbToGain(thresholdDb);
	const windows = Math.ceil(buf.length / win);
	const loud = new Uint8Array(windows);
	for (let w = 0; w < windows; w++) {
		let sum = 0;
		const from = w * win;
		const to = Math.min(buf.length, from + win);
		for (const ch of chans) for (let i = from; i < to; i++) sum += ch[i] * ch[i];
		loud[w] = Math.sqrt(sum / ((to - from) * chans.length)) > threshold ? 1 : 0;
	}
	const first = loud.indexOf(1);
	if (first < 0) return buf;
	const last = loud.lastIndexOf(1);
	const keep = Math.round((keepMs / 1000) * sr);
	const maxGap = maxGapMs === null ? Infinity : Math.round((maxGapMs / 1000) * sr);

	const segments: [number, number][] = [];
	let segStart = Math.max(0, first * win - keep);
	let quietRun = 0;
	for (let w = first; w <= last; w++) {
		if (loud[w]) {
			if (quietRun * win > maxGap) {
				const gapStart = (w - quietRun) * win;
				segments.push([segStart, gapStart + Math.floor(maxGap / 2)]);
				segStart = w * win - Math.floor(maxGap / 2);
			}
			quietRun = 0;
		} else quietRun++;
	}
	segments.push([segStart, Math.min(buf.length, (last + 1) * win + keep)]);

	const length = segments.reduce((n, [a, b]) => n + (b - a), 0);
	return fromChannels(
		chans.map((ch) => {
			const out = new Float32Array(length);
			let o = 0;
			for (const [a, b] of segments) {
				out.set(ch.subarray(a, b), o);
				o += b - a;
			}
			return out;
		}),
		sr
	);
}

function goertzel(x: Float32Array, freq: number, sr: number): number {
	const k = (2 * Math.cos((2 * Math.PI * freq) / sr)) as number;
	let s1 = 0,
		s2 = 0;
	for (let i = 0; i < x.length; i++) {
		const s = x[i] + k * s1 - s2;
		s2 = s1;
		s1 = s;
	}
	return s1 * s1 + s2 * s2 - k * s1 * s2;
}

/** Detect mains hum by comparing harmonic energy at 50/60 Hz against neighbours. */
export function detectHum(buf: AudioBuffer): 50 | 60 | null {
	const sr = buf.sampleRate;
	const len = Math.min(buf.length, sr * 20);
	const mono = new Float32Array(len);
	for (const ch of channelData(buf)) for (let i = 0; i < len; i++) mono[i] += ch[i];

	const score = (base: number) => {
		let tone = 0;
		let around = 0;
		for (const h of [1, 2, 3]) {
			tone += goertzel(mono, base * h, sr);
			around += (goertzel(mono, base * h - 7, sr) + goertzel(mono, base * h + 7, sr)) / 2;
		}
		return tone / (around + 1e-12);
	};
	const s50 = score(50);
	const s60 = score(60);
	const best = s50 > s60 ? 50 : 60;
	return Math.max(s50, s60) > 8 ? best : null;
}
