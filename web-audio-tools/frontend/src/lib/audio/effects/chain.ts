import { dbToGain } from '../context';
import processorsUrl from './processors.js?url';
import { EQ_BANDS, GATE_OFF, type EffectSettings } from './settings';

export interface EffectChain {
	input: AudioNode;
	output: AudioNode;
	update(settings: EffectSettings): void;
	dispose(): void;
}

const loadedContexts = new WeakMap<BaseAudioContext, Promise<void>>();

export function ensureWorklets(ctx: BaseAudioContext): Promise<void> {
	let p = loadedContexts.get(ctx);
	if (!p) {
		p = ctx.audioWorklet.addModule(processorsUrl);
		loadedContexts.set(ctx, p);
	}
	return p;
}

function driveCurve(amount: number): Float32Array<ArrayBuffer> | null {
	if (amount <= 0) return null;
	const k = 1 + amount * 40;
	const n = 2048;
	const curve = new Float32Array(n);
	const norm = Math.tanh(k);
	for (let i = 0; i < n; i++) {
		const x = (i / (n - 1)) * 2 - 1;
		curve[i] = Math.tanh(k * x) / norm;
	}
	return curve;
}

export function generateImpulse(ctx: BaseAudioContext, decay: number): AudioBuffer {
	const sr = ctx.sampleRate;
	const length = Math.max(1, Math.round(sr * Math.max(0.1, decay)));
	const buf = ctx.createBuffer(2, length, sr);
	let seed = 12345;
	const rand = () => {
		seed = (seed * 1664525 + 1013904223) >>> 0;
		return seed / 0xffffffff - 0.5;
	};
	for (let c = 0; c < 2; c++) {
		const ch = buf.getChannelData(c);
		let lp = 0;
		for (let i = 0; i < length; i++) {
			const t = i / length;
			lp += (rand() - lp) * (0.9 - 0.6 * t);
			const early = i < sr * 0.08 && i % Math.round(sr * (0.011 + c * 0.004)) === 0 ? 0.6 : 0;
			ch[i] = (lp * 2 + early) * Math.pow(1 - t, 3.2);
		}
	}
	return buf;
}

/**
 * Build the full effect graph. Works on both `AudioContext` (live preview)
 * and `OfflineAudioContext` (render/export). `live` enables the granular
 * pitch-shifter worklet for microphone input, where the WASM stage cannot run.
 */
export async function buildChain(
	ctx: BaseAudioContext,
	initial: EffectSettings,
	{ live = false }: { live?: boolean } = {}
): Promise<EffectChain> {
	await ensureWorklets(ctx);
	const nyquist = ctx.sampleRate / 2;
	const nodes: AudioNode[] = [];
	const sources: AudioScheduledSourceNode[] = [];
	const track = <T extends AudioNode>(n: T): T => (nodes.push(n), n);

	const input = track(new GainNode(ctx));
	const highpass = track(new BiquadFilterNode(ctx, { type: 'highpass', Q: 0.707 }));
	const lowpass = track(new BiquadFilterNode(ctx, { type: 'lowpass', Q: 0.707 }));
	const hum = [1, 2, 3].map(() => track(new BiquadFilterNode(ctx, { type: 'peaking', Q: 25, gain: 0 })));
	const eq = EQ_BANDS.map((frequency, i) =>
		track(
			new BiquadFilterNode(ctx, {
				type: i === 0 ? 'lowshelf' : i === EQ_BANDS.length - 1 ? 'highshelf' : 'peaking',
				frequency,
				Q: 1.2,
				gain: 0
			})
		)
	);
	const gate = track(new AudioWorkletNode(ctx, 'noise-gate', { outputChannelCount: [2] }));
	const deesser = track(new AudioWorkletNode(ctx, 'de-esser', { outputChannelCount: [2] }));
	const shaper = track(new WaveShaperNode(ctx, { oversample: '4x' }));

	const ringOut = track(new GainNode(ctx));
	const ringDry = track(new GainNode(ctx));
	const ringWet = track(new GainNode(ctx, { gain: 0 }));
	const ringMix = track(new GainNode(ctx, { gain: 0 }));
	const ringOsc = new OscillatorNode(ctx, { type: 'sine', frequency: 60 });
	sources.push(ringOsc);

	const pitch = live ? track(new AudioWorkletNode(ctx, 'pitch-shifter', { outputChannelCount: [2] })) : null;

	const echoOut = track(new GainNode(ctx));
	const echoDelay = track(new DelayNode(ctx, { maxDelayTime: 2 }));
	const echoFeedback = track(new GainNode(ctx));
	const echoWet = track(new GainNode(ctx, { gain: 0 }));
	const echoTone = track(new BiquadFilterNode(ctx, { type: 'lowpass', frequency: 6000 }));

	const reverbOut = track(new GainNode(ctx));
	const reverbDry = track(new GainNode(ctx));
	const convolver = track(new ConvolverNode(ctx, { disableNormalization: false }));
	const reverbWet = track(new GainNode(ctx, { gain: 0 }));
	let reverbDecay = -1;

	const panner = track(new StereoPannerNode(ctx));
	const panDepth = track(new GainNode(ctx, { gain: 0 }));
	const panLfo = new OscillatorNode(ctx, { type: 'sine', frequency: 0.1 });
	sources.push(panLfo);

	const compressor = track(new DynamicsCompressorNode(ctx));
	const makeup = track(new GainNode(ctx));
	const limiter = track(new DynamicsCompressorNode(ctx, { knee: 0, ratio: 20, attack: 0.001, release: 0.05 }));
	const output = track(new GainNode(ctx));

	const serial: AudioNode[] = [input, highpass, lowpass, ...hum, ...eq, gate, deesser, shaper];
	for (let i = 0; i < serial.length - 1; i++) serial[i].connect(serial[i + 1]);

	shaper.connect(ringDry).connect(ringOut);
	shaper.connect(ringWet).connect(ringMix).connect(ringOut);
	ringOsc.connect(ringWet.gain);

	let afterRing: AudioNode = ringOut;
	if (pitch) afterRing = afterRing.connect(pitch);

	afterRing.connect(echoOut);
	afterRing.connect(echoDelay);
	echoDelay.connect(echoTone).connect(echoFeedback).connect(echoDelay);
	echoTone.connect(echoWet).connect(echoOut);

	echoOut.connect(reverbDry).connect(reverbOut);
	echoOut.connect(convolver).connect(reverbWet).connect(reverbOut);

	reverbOut.connect(panner);
	panLfo.connect(panDepth).connect(panner.pan);
	panner.connect(compressor).connect(makeup).connect(limiter).connect(output);

	ringOsc.start();
	panLfo.start();

	const isLive = ctx instanceof AudioContext;
	const set = (param: AudioParam, value: number) => {
		if (isLive) param.setTargetAtTime(value, (ctx as AudioContext).currentTime, 0.02);
		else param.value = value;
	};
	const param = (node: AudioWorkletNode, name: string) => node.parameters.get(name)!;

	function update(s: EffectSettings) {
		set(highpass.frequency, s.highpass > 0 ? Math.min(s.highpass, nyquist - 100) : 10);
		set(lowpass.frequency, s.lowpass > 0 ? Math.min(s.lowpass, nyquist - 100) : nyquist - 100);
		hum.forEach((f, i) => {
			const base = s.hum || 50;
			set(f.frequency, Math.min(base * (i + 1), nyquist - 100));
			set(f.gain, s.hum ? -30 : 0);
		});
		eq.forEach((band, i) => set(band.gain, s.eq[i] ?? 0));

		set(param(gate, 'threshold'), s.gate > GATE_OFF ? s.gate : GATE_OFF);
		set(param(deesser, 'amount'), s.deesser);
		shaper.curve = driveCurve(s.drive);

		set(ringDry.gain, 1 - s.ringmod.mix);
		set(ringMix.gain, s.ringmod.mix);
		set(ringOsc.frequency, s.ringmod.freq);

		if (pitch) set(param(pitch, 'ratio'), Math.pow(2, s.pitch / 12));

		set(echoDelay.delayTime, Math.min(2, Math.max(0.005, s.echo.time)));
		set(echoFeedback.gain, Math.min(0.95, s.echo.feedback));
		set(echoWet.gain, s.echo.mix);

		if (s.reverb.mix > 0 && Math.abs(s.reverb.decay - reverbDecay) > 0.05) {
			convolver.buffer = generateImpulse(ctx, s.reverb.decay);
			reverbDecay = s.reverb.decay;
		}
		set(reverbDry.gain, 1 - s.reverb.mix * 0.5);
		set(reverbWet.gain, s.reverb.mix);

		set(panLfo.frequency, Math.max(0.01, s.eightD));
		set(panDepth.gain, s.eightD > 0 ? 0.95 : 0);

		const c = s.compressor;
		set(compressor.threshold, c.enabled ? c.threshold : 0);
		set(compressor.ratio, c.enabled ? c.ratio : 1);
		set(compressor.attack, c.attack);
		set(compressor.release, c.release);
		set(compressor.knee, c.enabled ? 6 : 0);
		set(makeup.gain, dbToGain(c.enabled ? c.makeup : 0));

		set(limiter.threshold, s.limiter ?? 0);
		set(limiter.ratio, s.limiter === null ? 1 : 20);
		set(output.gain, dbToGain(s.gain));
	}

	update(initial);

	return {
		input,
		output,
		update,
		dispose() {
			for (const src of sources) {
				try {
					src.stop();
				} catch {
					/* already stopped */
				}
				src.disconnect();
			}
			for (const n of nodes) n.disconnect();
		}
	};
}
