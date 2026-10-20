import { channelData, fromChannels } from '#lib/audio/buffer.ts';
import type { DspRequest, DspResponse } from './protocol';

type Pending = { resolve: (c: Float32Array<ArrayBuffer>[]) => void; reject: (e: Error) => void };
type RequestBody = DspRequest extends infer R ? (R extends DspRequest ? Omit<R, 'id'> : never) : never;

let worker: Worker | null = null;
let nextId = 1;
const pending = new Map<number, Pending>();

function getWorker(): Worker {
	if (worker) return worker;
	worker = new Worker(new URL('./dsp.worker.ts', import.meta.url), { type: 'module' });
	worker.onmessage = (event: MessageEvent<DspResponse>) => {
		const res = event.data;
		const p = pending.get(res.id);
		if (!p) return;
		pending.delete(res.id);
		if (res.error !== undefined) p.reject(new Error(res.error));
		else p.resolve(res.channels);
	};
	return worker;
}

function call(body: RequestBody): Promise<Float32Array<ArrayBuffer>[]> {
	const id = nextId++;
	const req = { ...body, id } as DspRequest;
	return new Promise((resolve, reject) => {
		pending.set(id, { resolve, reject });
		getWorker().postMessage(
			req,
			req.channels.map((c) => c.buffer)
		);
	});
}

/** Copies channels so the transferred originals stay intact on the main thread. */
function copyChannels(buf: AudioBuffer): Float32Array[] {
	return channelData(buf).map((c) => c.slice());
}

/** Spectral noise reduction in Rust/WASM, off the main thread. */
export async function denoiseBuffer(buf: AudioBuffer, strength: number): Promise<AudioBuffer> {
	const channels = await call({ op: 'denoise', channels: copyChannels(buf), sampleRate: buf.sampleRate, strength });
	return fromChannels(channels, buf.sampleRate);
}

/** WSOLA time-stretch (`rate` = speed) and pitch shift in semitones, independently. */
export async function stretchPitchBuffer(buf: AudioBuffer, rate: number, semitones: number): Promise<AudioBuffer> {
	const channels = await call({
		op: 'stretch',
		channels: copyChannels(buf),
		sampleRate: buf.sampleRate,
		rate,
		semitones
	});
	return fromChannels(channels, buf.sampleRate);
}
