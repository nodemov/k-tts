import { decodeFile, resumeAudio } from './context';
import { buildChain, type EffectChain } from './effects/chain';
import type { EffectSettings } from './effects/settings';

export function pickRecorderMime(kind: 'audio' | 'video'): string {
	const candidates =
		kind === 'audio'
			? ['audio/webm;codecs=opus', 'audio/mp4', 'audio/ogg;codecs=opus', 'audio/webm']
			: ['video/webm;codecs=vp9,opus', 'video/webm;codecs=vp8,opus', 'video/mp4;codecs=avc1,mp4a', 'video/mp4', 'video/webm'];
	return candidates.find((m) => MediaRecorder.isTypeSupported(m)) ?? '';
}

/** Microphone → live effect chain → speakers + recorder. */
export class LiveMic {
	active = $state(false);
	recording = $state(false);
	monitor = $state(true);
	elapsed = $state(0);

	analyser: AnalyserNode | null = null;
	private ctx: AudioContext | null = null;
	private stream: MediaStream | null = null;
	private chain: EffectChain | null = null;
	private monitorGain: GainNode | null = null;
	private dest: MediaStreamAudioDestinationNode | null = null;
	private recorder: MediaRecorder | null = null;
	private chunks: Blob[] = [];
	private timer = 0;

	async start(settings: EffectSettings) {
		if (this.active) return;
		const ctx = await resumeAudio();
		this.ctx = ctx;
		this.stream = await navigator.mediaDevices.getUserMedia({
			audio: { echoCancellation: false, noiseSuppression: false, autoGainControl: false }
		});
		const source = new MediaStreamAudioSourceNode(ctx, { mediaStream: this.stream });
		this.chain = await buildChain(ctx, settings, { live: true });
		this.monitorGain = new GainNode(ctx, { gain: this.monitor ? 1 : 0 });
		this.dest = new MediaStreamAudioDestinationNode(ctx);
		this.analyser = new AnalyserNode(ctx, { fftSize: 2048 });
		source.connect(this.chain.input);
		this.chain.output.connect(this.analyser);
		this.chain.output.connect(this.dest);
		this.chain.output.connect(this.monitorGain).connect(ctx.destination);
		this.active = true;
	}

	update(settings: EffectSettings) {
		this.chain?.update(settings);
	}

	setMonitor(on: boolean) {
		this.monitor = on;
		if (this.monitorGain && this.ctx) this.monitorGain.gain.setTargetAtTime(on ? 1 : 0, this.ctx.currentTime, 0.02);
	}

	startRecording() {
		if (!this.dest || this.recording) return;
		this.chunks = [];
		const mimeType = pickRecorderMime('audio');
		this.recorder = new MediaRecorder(this.dest.stream, mimeType ? { mimeType } : undefined);
		this.recorder.ondataavailable = (e) => e.data.size && this.chunks.push(e.data);
		this.recorder.start(250);
		this.recording = true;
		const began = performance.now();
		this.timer = window.setInterval(() => (this.elapsed = (performance.now() - began) / 1000), 100);
	}

	stopRecording(): Promise<{ blob: Blob; buffer: AudioBuffer } | null> {
		const recorder = this.recorder;
		clearInterval(this.timer);
		this.recording = false;
		if (!recorder) return Promise.resolve(null);
		return new Promise((resolve) => {
			recorder.onstop = async () => {
				const blob = new Blob(this.chunks, { type: recorder.mimeType });
				this.recorder = null;
				try {
					resolve({ blob, buffer: await decodeFile(blob) });
				} catch {
					resolve(null);
				}
			};
			recorder.stop();
		});
	}

	stop() {
		if (this.recording) void this.stopRecording();
		this.stream?.getTracks().forEach((t) => t.stop());
		this.chain?.dispose();
		this.monitorGain?.disconnect();
		this.analyser?.disconnect();
		this.stream = null;
		this.chain = null;
		this.analyser = null;
		this.active = false;
	}
}
