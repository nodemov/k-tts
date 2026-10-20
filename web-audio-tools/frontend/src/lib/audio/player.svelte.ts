import { getAudioContext, resumeAudio } from './context';
import { buildChain, type EffectChain } from './effects/chain';
import type { EffectSettings } from './effects/settings';

/**
 * Realtime preview: plays the WASM-preprocessed buffer through the live
 * effect graph, with an instant A/B bypass against the original.
 */
export class Player {
	playing = $state(false);
	position = $state(0);
	duration = $state(0);
	bypass = $state(false);
	ready = $state(false);

	readonly ctx = getAudioContext();
	readonly analyser: AnalyserNode;
	private readonly master: GainNode;
	private readonly wetGain: GainNode;
	private readonly dryGain: GainNode;
	private chain: EffectChain | null = null;
	private wet: AudioBuffer | null = null;
	private dry: AudioBuffer | null = null;
	private sources: AudioBufferSourceNode[] = [];
	private startedAt = 0;
	private startOffset = 0;
	private raf = 0;

	constructor() {
		this.master = new GainNode(this.ctx);
		this.wetGain = new GainNode(this.ctx);
		this.dryGain = new GainNode(this.ctx, { gain: 0 });
		this.analyser = new AnalyserNode(this.ctx, { fftSize: 2048, smoothingTimeConstant: 0.75 });
		this.wetGain.connect(this.master);
		this.dryGain.connect(this.master);
		this.master.connect(this.analyser).connect(this.ctx.destination);
	}

	async init(settings: EffectSettings) {
		this.chain = await buildChain(this.ctx, settings);
		this.chain.output.connect(this.wetGain);
		this.ready = true;
	}

	update(settings: EffectSettings) {
		this.chain?.update(settings);
	}

	setBuffers(dry: AudioBuffer | null, wet: AudioBuffer | null) {
		const ratio = this.duration > 0 ? this.position / this.duration : 0;
		const wasPlaying = this.playing;
		this.stopSources();
		this.dry = dry;
		this.wet = wet ?? dry;
		this.duration = this.wet?.duration ?? 0;
		this.position = Math.min(this.duration, ratio * this.duration);
		if (wasPlaying) void this.play(this.position);
	}

	setBypass(bypass: boolean) {
		this.bypass = bypass;
		const t = this.ctx.currentTime;
		this.wetGain.gain.setTargetAtTime(bypass ? 0 : 1, t, 0.015);
		this.dryGain.gain.setTargetAtTime(bypass ? 1 : 0, t, 0.015);
	}

	async play(from = this.position) {
		if (!this.wet || !this.chain) return;
		await resumeAudio();
		this.stopSources();
		const offset = from >= this.duration - 0.01 ? 0 : Math.max(0, from);
		const wetSrc = new AudioBufferSourceNode(this.ctx, { buffer: this.wet });
		wetSrc.connect(this.chain.input);
		const start = this.ctx.currentTime + 0.01;
		wetSrc.start(start, offset);
		wetSrc.onended = () => {
			if (this.sources[0] === wetSrc) this.finish();
		};
		this.sources = [wetSrc];
		if (this.dry) {
			const drySrc = new AudioBufferSourceNode(this.ctx, { buffer: this.dry });
			drySrc.connect(this.dryGain);
			drySrc.start(start, Math.min(this.dry.duration, offset * (this.dry.duration / this.duration)));
			this.sources.push(drySrc);
		}
		this.startedAt = start;
		this.startOffset = offset;
		this.playing = true;
		this.tick();
	}

	pause() {
		if (!this.playing) return;
		this.position = this.currentTime();
		this.stopSources();
		this.playing = false;
	}

	toggle() {
		return this.playing ? this.pause() : this.play();
	}

	seek(seconds: number) {
		this.position = Math.max(0, Math.min(this.duration, seconds));
		if (this.playing) void this.play(this.position);
	}

	stop() {
		this.stopSources();
		this.playing = false;
		this.position = 0;
	}

	dispose() {
		this.stop();
		this.chain?.dispose();
		this.master.disconnect();
		this.analyser.disconnect();
	}

	private currentTime() {
		return Math.min(this.duration, this.startOffset + Math.max(0, this.ctx.currentTime - this.startedAt));
	}

	private finish() {
		this.sources = [];
		this.playing = false;
		this.position = 0;
	}

	private tick = () => {
		cancelAnimationFrame(this.raf);
		if (!this.playing) return;
		this.position = this.currentTime();
		this.raf = requestAnimationFrame(this.tick);
	};

	private stopSources() {
		cancelAnimationFrame(this.raf);
		for (const s of this.sources) {
			s.onended = null;
			try {
				s.stop();
			} catch {
				/* not started */
			}
			s.disconnect();
		}
		this.sources = [];
	}
}
