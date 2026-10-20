// @ts-nocheck — runs in AudioWorkletGlobalScope, which TypeScript's DOM lib does not describe.
/* global sampleRate, registerProcessor, AudioWorkletProcessor */
// AudioWorklet processors. Plain JS so the file can be loaded with addModule() as-is.

class NoiseGateProcessor extends AudioWorkletProcessor {
	static get parameterDescriptors() {
		return [
			{ name: 'threshold', defaultValue: -100, minValue: -100, maxValue: 0, automationRate: 'k-rate' },
			{ name: 'range', defaultValue: -60, minValue: -100, maxValue: 0, automationRate: 'k-rate' }
		];
	}

	constructor() {
		super();
		this.env = 0;
		this.gain = 1;
		this.hold = 0;
		this.envAttack = Math.exp(-1 / (0.0005 * sampleRate));
		this.envRelease = Math.exp(-1 / (0.03 * sampleRate));
		this.gainAttack = Math.exp(-1 / (0.002 * sampleRate));
		this.gainRelease = Math.exp(-1 / (0.12 * sampleRate));
		this.holdSamples = Math.round(0.06 * sampleRate);
	}

	process(inputs, outputs, params) {
		const input = inputs[0];
		const output = outputs[0];
		if (!input.length) return true;
		const thresholdDb = params.threshold[0];
		if (thresholdDb <= -99) {
			for (let c = 0; c < output.length; c++) output[c].set(input[c] ?? input[0]);
			return true;
		}
		const open = Math.pow(10, thresholdDb / 20);
		const close = open * 0.6;
		const floor = Math.pow(10, params.range[0] / 20);
		const n = input[0].length;
		for (let i = 0; i < n; i++) {
			let peak = 0;
			for (let c = 0; c < input.length; c++) peak = Math.max(peak, Math.abs(input[c][i]));
			const coef = peak > this.env ? this.envAttack : this.envRelease;
			this.env = peak + (this.env - peak) * coef;

			let target = floor;
			if (this.env > open) {
				this.hold = this.holdSamples;
				target = 1;
			} else if (this.hold > 0 || this.env > close) {
				this.hold = Math.max(0, this.hold - 1);
				target = 1;
			}
			const g = target > this.gain ? this.gainAttack : this.gainRelease;
			this.gain = target + (this.gain - target) * g;
			for (let c = 0; c < output.length; c++) output[c][i] = (input[c] ?? input[0])[i] * this.gain;
		}
		return true;
	}
}

class DeEsserProcessor extends AudioWorkletProcessor {
	static get parameterDescriptors() {
		return [
			{ name: 'amount', defaultValue: 0, minValue: 0, maxValue: 1, automationRate: 'k-rate' },
			{ name: 'frequency', defaultValue: 5500, minValue: 2000, maxValue: 12000, automationRate: 'k-rate' }
		];
	}

	constructor() {
		super();
		this.state = [];
		this.freq = 0;
		this.coef = null;
		this.hfEnv = 0;
		this.fullEnv = 0;
		this.gain = 1;
		this.envCoef = Math.exp(-1 / (0.005 * sampleRate));
		this.attack = Math.exp(-1 / (0.0008 * sampleRate));
		this.release = Math.exp(-1 / (0.05 * sampleRate));
	}

	updateCoef(freq) {
		const w0 = (2 * Math.PI * freq) / sampleRate;
		const alpha = Math.sin(w0) / (2 * 0.707);
		const cos = Math.cos(w0);
		const a0 = 1 + alpha;
		this.coef = [(1 + cos) / 2 / a0, -(1 + cos) / a0, (1 + cos) / 2 / a0, (-2 * cos) / a0, (1 - alpha) / a0];
		this.freq = freq;
	}

	process(inputs, outputs, params) {
		const input = inputs[0];
		const output = outputs[0];
		if (!input.length) return true;
		const amount = params.amount[0];
		if (amount <= 0) {
			for (let c = 0; c < output.length; c++) output[c].set(input[c] ?? input[0]);
			return true;
		}
		const freq = params.frequency[0];
		if (freq !== this.freq) this.updateCoef(freq);
		const [b0, b1, b2, a1, a2] = this.coef;
		while (this.state.length < input.length) this.state.push([0, 0, 0, 0]);

		const threshold = 0.45 - 0.3 * amount;
		const minGain = Math.pow(10, (-14 * amount) / 20);
		const n = input[0].length;
		const hp = input.map(() => new Float32Array(n));

		for (let c = 0; c < input.length; c++) {
			const s = this.state[c];
			const x = input[c];
			const y = hp[c];
			for (let i = 0; i < n; i++) {
				const v = b0 * x[i] + b1 * s[0] + b2 * s[1] - a1 * s[2] - a2 * s[3];
				s[1] = s[0];
				s[0] = x[i];
				s[3] = s[2];
				s[2] = v;
				y[i] = v;
			}
		}

		for (let i = 0; i < n; i++) {
			let h = 0;
			let f = 0;
			for (let c = 0; c < input.length; c++) {
				h = Math.max(h, Math.abs(hp[c][i]));
				f = Math.max(f, Math.abs(input[c][i]));
			}
			this.hfEnv = h + (this.hfEnv - h) * this.envCoef;
			this.fullEnv = f + (this.fullEnv - f) * this.envCoef;
			const ratio = this.hfEnv / (this.fullEnv + 1e-9);
			let target = 1;
			if (this.hfEnv > 0.01 && ratio > threshold) target = Math.max(minGain, Math.pow(threshold / ratio, 1 + amount));
			const coef = target < this.gain ? this.attack : this.release;
			this.gain = target + (this.gain - target) * coef;
			for (let c = 0; c < output.length; c++) {
				const src = input[c] ?? input[0];
				const high = (hp[c] ?? hp[0])[i];
				output[c][i] = src[i] - high + high * this.gain;
			}
		}
		return true;
	}
}

/** Low-latency granular pitch shifter for live microphone input. */
class PitchShifterProcessor extends AudioWorkletProcessor {
	static get parameterDescriptors() {
		return [{ name: 'ratio', defaultValue: 1, minValue: 0.25, maxValue: 4, automationRate: 'k-rate' }];
	}

	constructor() {
		super();
		this.size = 8192;
		this.buffers = [];
		this.write = 0;
		this.phase = 0;
		this.window = Math.round(0.045 * sampleRate);
	}

	read(buf, delay) {
		let pos = this.write - delay;
		while (pos < 0) pos += this.size;
		const i = Math.floor(pos);
		const frac = pos - i;
		const a = buf[i % this.size];
		const b = buf[(i + 1) % this.size];
		return a + (b - a) * frac;
	}

	process(inputs, outputs, params) {
		const input = inputs[0];
		const output = outputs[0];
		if (!input.length) return true;
		const ratio = params.ratio[0];
		while (this.buffers.length < input.length) this.buffers.push(new Float32Array(this.size));
		const n = input[0].length;
		if (Math.abs(ratio - 1) < 1e-3) {
			for (let c = 0; c < output.length; c++) output[c].set(input[c] ?? input[0]);
			return true;
		}
		const step = (1 - ratio) / this.window;
		for (let i = 0; i < n; i++) {
			for (let c = 0; c < input.length; c++) this.buffers[c][this.write] = input[c][i];
			this.phase = (this.phase + step + 1) % 1;
			const p2 = (this.phase + 0.5) % 1;
			const g1 = Math.sin(Math.PI * this.phase);
			const g2 = Math.sin(Math.PI * p2);
			const d1 = this.phase * this.window + 1;
			const d2 = p2 * this.window + 1;
			for (let c = 0; c < output.length; c++) {
				const buf = this.buffers[c] ?? this.buffers[0];
				output[c][i] = this.read(buf, d1) * g1 + this.read(buf, d2) * g2;
			}
			this.write = (this.write + 1) % this.size;
		}
		return true;
	}
}

registerProcessor('noise-gate', NoiseGateProcessor);
registerProcessor('de-esser', DeEsserProcessor);
registerProcessor('pitch-shifter', PitchShifterProcessor);
