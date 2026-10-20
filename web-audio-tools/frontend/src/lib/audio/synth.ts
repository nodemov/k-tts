/** Built-in soundboard sounds, synthesised offline so no samples ship with the app. */

type Builder = (ctx: OfflineAudioContext, out: AudioNode) => void;

function noise(ctx: BaseAudioContext, seconds: number): AudioBuffer {
	const buf = ctx.createBuffer(1, Math.ceil(seconds * ctx.sampleRate), ctx.sampleRate);
	const ch = buf.getChannelData(0);
	for (let i = 0; i < ch.length; i++) ch[i] = Math.random() * 2 - 1;
	return buf;
}

function env(ctx: BaseAudioContext, at: number, attack: number, decay: number, peak = 1): GainNode {
	const g = new GainNode(ctx, { gain: 0 });
	g.gain.setValueAtTime(0, at);
	g.gain.linearRampToValueAtTime(peak, at + attack);
	g.gain.exponentialRampToValueAtTime(0.0001, at + attack + decay);
	return g;
}

function kick(ctx: BaseAudioContext, out: AudioNode, at: number, level = 1) {
	const osc = new OscillatorNode(ctx, { frequency: 150 });
	osc.frequency.exponentialRampToValueAtTime(40, at + 0.12);
	osc.connect(env(ctx, at, 0.002, 0.35, level)).connect(out);
	osc.start(at);
	osc.stop(at + 0.5);
}

function snare(ctx: BaseAudioContext, out: AudioNode, at: number, level = 0.8) {
	const src = new AudioBufferSourceNode(ctx, { buffer: noise(ctx, 0.3) });
	src.connect(new BiquadFilterNode(ctx, { type: 'highpass', frequency: 1500 })).connect(env(ctx, at, 0.001, 0.18, level)).connect(out);
	src.start(at);
	const body = new OscillatorNode(ctx, { type: 'triangle', frequency: 190 });
	body.connect(env(ctx, at, 0.001, 0.1, level * 0.6)).connect(out);
	body.start(at);
	body.stop(at + 0.2);
}

function cymbal(ctx: BaseAudioContext, out: AudioNode, at: number, decay = 1.2) {
	const src = new AudioBufferSourceNode(ctx, { buffer: noise(ctx, decay + 0.1) });
	src.connect(new BiquadFilterNode(ctx, { type: 'highpass', frequency: 6500 })).connect(env(ctx, at, 0.002, decay, 0.5)).connect(out);
	src.start(at);
}

function tone(ctx: BaseAudioContext, out: AudioNode, type: OscillatorType, freq: number, at: number, dur: number, level = 0.4) {
	const osc = new OscillatorNode(ctx, { type, frequency: freq });
	osc.connect(env(ctx, at, 0.01, dur, level)).connect(out);
	osc.start(at);
	osc.stop(at + dur + 0.05);
	return osc;
}

const BUILDERS: Record<string, { seconds: number; build: Builder }> = {
	airhorn: {
		seconds: 1.6,
		build: (ctx, out) => {
			for (const [t, d] of [
				[0, 0.25],
				[0.3, 0.25],
				[0.6, 0.9]
			]) {
				for (const f of [466, 470, 932]) {
					const o = new OscillatorNode(ctx, { type: 'sawtooth', frequency: f });
					const g = new GainNode(ctx, { gain: 0 });
					g.gain.setValueAtTime(0, t);
					g.gain.linearRampToValueAtTime(0.18, t + 0.02);
					g.gain.setValueAtTime(0.18, t + d - 0.03);
					g.gain.linearRampToValueAtTime(0, t + d);
					o.connect(g).connect(out);
					o.start(t);
					o.stop(t + d);
				}
			}
		}
	},
	rimshot: {
		seconds: 1.8,
		build: (ctx, out) => {
			snare(ctx, out, 0.05, 0.6);
			kick(ctx, out, 0.3, 0.9);
			snare(ctx, out, 0.3, 0.4);
			cymbal(ctx, out, 0.6, 1.1);
		}
	},
	drumroll: {
		seconds: 2.6,
		build: (ctx, out) => {
			for (let i = 0; i < 40; i++) snare(ctx, out, i * 0.05, 0.25 + (i / 40) * 0.5);
			cymbal(ctx, out, 2.0, 0.6);
			kick(ctx, out, 2.0);
		}
	},
	laser: {
		seconds: 0.6,
		build: (ctx, out) => {
			const o = tone(ctx, out, 'square', 1800, 0, 0.45, 0.25);
			o.frequency.exponentialRampToValueAtTime(120, 0.45);
		}
	},
	coin: {
		seconds: 0.6,
		build: (ctx, out) => {
			tone(ctx, out, 'square', 988, 0, 0.08, 0.25);
			tone(ctx, out, 'square', 1319, 0.08, 0.4, 0.25);
		}
	},
	boing: {
		seconds: 0.9,
		build: (ctx, out) => {
			const o = new OscillatorNode(ctx, { type: 'sine', frequency: 120 });
			const lfo = new OscillatorNode(ctx, { frequency: 14 });
			const depth = new GainNode(ctx, { gain: 70 });
			depth.gain.exponentialRampToValueAtTime(1, 0.8);
			lfo.connect(depth).connect(o.frequency);
			o.frequency.linearRampToValueAtTime(320, 0.8);
			o.connect(env(ctx, 0, 0.005, 0.8, 0.6)).connect(out);
			o.start();
			lfo.start();
			o.stop(0.85);
			lfo.stop(0.85);
		}
	},
	sad: {
		seconds: 2.4,
		build: (ctx, out) => {
			[392, 370, 349].forEach((f, i) => tone(ctx, out, 'sawtooth', f, i * 0.45, 0.4, 0.18));
			const last = tone(ctx, out, 'sawtooth', 330, 1.35, 0.95, 0.2);
			const vib = new OscillatorNode(ctx, { frequency: 6 });
			vib.connect(new GainNode(ctx, { gain: 8 })).connect(last.frequency);
			vib.start(1.35);
			vib.stop(2.3);
		}
	},
	applause: {
		seconds: 3,
		build: (ctx, out) => {
			for (let i = 0; i < 90; i++) {
				const at = Math.random() * 2.4;
				const src = new AudioBufferSourceNode(ctx, { buffer: noise(ctx, 0.05) });
				src.connect(new BiquadFilterNode(ctx, { type: 'bandpass', frequency: 1200 + Math.random() * 1500, Q: 1.5 }))
					.connect(env(ctx, at, 0.001, 0.04, 0.3 + Math.random() * 0.3))
					.connect(out);
				src.start(at);
			}
		}
	},
	siren: {
		seconds: 2.2,
		build: (ctx, out) => {
			const o = new OscillatorNode(ctx, { type: 'triangle', frequency: 700 });
			const lfo = new OscillatorNode(ctx, { frequency: 1.4 });
			lfo.connect(new GainNode(ctx, { gain: 260 })).connect(o.frequency);
			const g = new GainNode(ctx, { gain: 0.3 });
			g.gain.setValueAtTime(0.3, 1.9);
			g.gain.linearRampToValueAtTime(0, 2.1);
			o.connect(g).connect(out);
			o.start();
			lfo.start();
			o.stop(2.15);
			lfo.stop(2.15);
		}
	},
	bell: {
		seconds: 2.5,
		build: (ctx, out) => {
			[1, 2.76, 5.4, 8.93].forEach((m, i) => tone(ctx, out, 'sine', 523 * m, 0, 2.2 / (i + 1), 0.25 / (i + 1)));
		}
	},
	beat: {
		seconds: 2.2,
		build: (ctx, out) => {
			for (let bar = 0; bar < 4; bar++) {
				const t = bar * 0.5;
				kick(ctx, out, t);
				snare(ctx, out, t + 0.25, 0.5);
				cymbal(ctx, out, t + 0.125, 0.05);
				cymbal(ctx, out, t + 0.375, 0.05);
			}
		}
	},
	whoosh: {
		seconds: 1.2,
		build: (ctx, out) => {
			const src = new AudioBufferSourceNode(ctx, { buffer: noise(ctx, 1.2) });
			const f = new BiquadFilterNode(ctx, { type: 'bandpass', frequency: 300, Q: 3 });
			f.frequency.exponentialRampToValueAtTime(5000, 0.6);
			f.frequency.exponentialRampToValueAtTime(400, 1.1);
			const g = new GainNode(ctx, { gain: 0 });
			g.gain.linearRampToValueAtTime(0.9, 0.55);
			g.gain.linearRampToValueAtTime(0, 1.15);
			src.connect(f).connect(g).connect(out);
			src.start();
		}
	}
};

export const BUILTIN_SOUNDS = [
	{ id: 'airhorn', label: 'Air Horn', emoji: '📯' },
	{ id: 'rimshot', label: 'Ba Dum Tss', emoji: '🥁' },
	{ id: 'drumroll', label: 'Drum Roll', emoji: '🪘' },
	{ id: 'applause', label: 'ปรบมือ', emoji: '👏' },
	{ id: 'sad', label: 'Sad Trombone', emoji: '🎺' },
	{ id: 'laser', label: 'Laser', emoji: '🔫' },
	{ id: 'coin', label: 'Coin', emoji: '🪙' },
	{ id: 'boing', label: 'Boing', emoji: '🌀' },
	{ id: 'siren', label: 'Siren', emoji: '🚨' },
	{ id: 'bell', label: 'Bell', emoji: '🔔' },
	{ id: 'beat', label: 'Beat', emoji: '🎛️' },
	{ id: 'whoosh', label: 'Whoosh', emoji: '💨' }
];

const cache = new Map<string, Promise<AudioBuffer>>();

export function renderBuiltin(id: string, sampleRate: number): Promise<AudioBuffer> {
	const key = `${id}@${sampleRate}`;
	let p = cache.get(key);
	if (!p) {
		const def = BUILDERS[id];
		const ctx = new OfflineAudioContext(1, Math.ceil(def.seconds * sampleRate), sampleRate);
		const bus = new DynamicsCompressorNode(ctx, { threshold: -10, ratio: 4 });
		bus.connect(ctx.destination);
		def.build(ctx, bus);
		p = ctx.startRendering();
		cache.set(key, p);
	}
	return p;
}
