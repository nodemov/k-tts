import { channelData, fromChannels, reverseRange } from './buffer';
import { trimSilence } from './analysis';
import { buildChain } from './effects/chain';
import { hasOfflineStage, tailSeconds, type EffectSettings } from './effects/settings';
import { normalizeLoudness } from './loudness';
import { denoiseBuffer, stretchPitchBuffer } from '#lib/dsp/client.ts';

export type Stage = 'dsp' | 'chain' | 'post';

/** Offline WASM stage: reverse, noise reduction, time-stretch/pitch-shift. */
export async function runOfflineStage(src: AudioBuffer, s: EffectSettings): Promise<AudioBuffer> {
	if (!hasOfflineStage(s)) return src;
	let buf = src;
	if (s.reverse) buf = reverseRange(buf);
	if (s.denoise > 0) buf = await denoiseBuffer(buf, s.denoise);
	if (s.rate !== 1 || s.pitch !== 0) buf = await stretchPitchBuffer(buf, s.rate, s.pitch);
	return buf;
}

function needsStereo(src: AudioBuffer, s: EffectSettings): boolean {
	return src.numberOfChannels > 1 || s.eightD > 0 || s.reverb.mix > 0;
}

function trimTail(buf: AudioBuffer, minLength: number): AudioBuffer {
	const chans = channelData(buf);
	let end = buf.length;
	const floor = 1e-4;
	while (end > minLength && chans.every((ch) => Math.abs(ch[end - 1]) < floor)) end--;
	if (end === buf.length) return buf;
	return fromChannels(
		chans.map((ch) => ch.slice(0, end)),
		buf.sampleRate
	);
}

/** Realtime-graph effects rendered faster than realtime with OfflineAudioContext. */
export async function renderChain(src: AudioBuffer, s: EffectSettings): Promise<AudioBuffer> {
	const sr = src.sampleRate;
	const length = src.length + Math.ceil(tailSeconds(s) * sr);
	const ctx = new OfflineAudioContext(needsStereo(src, s) ? 2 : 1, length, sr);
	const chain = await buildChain(ctx, s);
	const node = new AudioBufferSourceNode(ctx, { buffer: src });
	node.connect(chain.input);
	chain.output.connect(ctx.destination);
	node.start();
	const rendered = await ctx.startRendering();
	return trimTail(rendered, src.length);
}

export async function processAudio(
	src: AudioBuffer,
	s: EffectSettings,
	onStage?: (stage: Stage) => void,
	preprocessed?: AudioBuffer
): Promise<AudioBuffer> {
	onStage?.('dsp');
	const pre = preprocessed ?? (await runOfflineStage(src, s));
	onStage?.('chain');
	let out = await renderChain(pre, s);
	onStage?.('post');
	if (s.trimSilence) out = trimSilence(out, { maxGapMs: 900 });
	if (s.loudness !== null) out = normalizeLoudness(out, s.loudness, s.limiter ?? -1);
	return out;
}
