import { channelData, cloneBuffer } from './buffer';
import { dbToGain } from './context';

/**
 * Offline look-ahead brickwall limiter. The gain curve is a sliding minimum
 * of the required reduction, smoothed by a moving average of the same length
 * so attacks are click-free and never overshoot the ceiling.
 */
export function limitPeaks(buf: AudioBuffer, ceilingDb = -1, lookaheadMs = 5, releaseMs = 80): AudioBuffer {
	const out = cloneBuffer(buf);
	const chans = channelData(out);
	const n = out.length;
	const ceiling = dbToGain(ceilingDb);
	const L = Math.max(1, Math.round((lookaheadMs / 1000) * out.sampleRate));

	const required = new Float32Array(n);
	for (let i = 0; i < n; i++) {
		let peak = 0;
		for (const ch of chans) peak = Math.max(peak, Math.abs(ch[i]));
		required[i] = peak > ceiling ? ceiling / peak : 1;
	}

	// Sliding minimum over [i, i + L] using a monotonic deque.
	const minAhead = new Float32Array(n);
	const deque = new Int32Array(n + L + 1);
	let head = 0;
	let tail = 0;
	let next = 0;
	for (let i = 0; i < n; i++) {
		const limit = Math.min(n - 1, i + L);
		while (next <= limit) {
			while (tail > head && required[deque[tail - 1]] >= required[next]) tail--;
			deque[tail++] = next++;
		}
		while (deque[head] < i) head++;
		minAhead[i] = required[deque[head]];
	}

	// Moving average over the trailing window keeps gain <= required at every peak.
	const smoothed = new Float32Array(n);
	let acc = 0;
	for (let i = 0; i < n; i++) {
		acc += minAhead[i];
		if (i >= L) acc -= minAhead[i - L];
		smoothed[i] = acc / Math.min(i + 1, L);
	}

	const release = Math.exp(-1 / ((releaseMs / 1000) * out.sampleRate));
	let g = 1;
	for (let i = 0; i < n; i++) {
		const target = smoothed[i];
		g = target < g ? target : target + (g - target) * release;
		for (const ch of chans) ch[i] *= g;
	}
	return out;
}
