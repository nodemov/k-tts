/// <reference lib="webworker" />
import wasmUrl from './dsp.wasm?url';
import type { DspRequest, DspResponse } from './protocol';

interface DspExports {
	memory: WebAssembly.Memory;
	alloc_f32(len: number): number;
	free_f32(ptr: number, len: number): void;
	denoise(ptr: number, frames: number, channels: number, sampleRate: number, strength: number): void;
	stretch_out_frames(frames: number, rate: number): number;
	stretch_pitch(
		input: number,
		frames: number,
		channels: number,
		sampleRate: number,
		rate: number,
		semitones: number,
		output: number
	): number;
}

let wasm: Promise<DspExports> | null = null;

function load(): Promise<DspExports> {
	wasm ??= WebAssembly.instantiateStreaming(fetch(wasmUrl), {}).then(
		(r) => r.instance.exports as unknown as DspExports
	);
	return wasm;
}

function writePlanar(dsp: DspExports, channels: Float32Array[]): { ptr: number; len: number } {
	const frames = channels[0].length;
	const len = frames * channels.length;
	const ptr = dsp.alloc_f32(len);
	const view = new Float32Array(dsp.memory.buffer, ptr, len);
	channels.forEach((ch, c) => view.set(ch, c * frames));
	return { ptr, len };
}

function readPlanar(dsp: DspExports, ptr: number, frames: number, count: number): Float32Array<ArrayBuffer>[] {
	const view = new Float32Array(dsp.memory.buffer, ptr, frames * count);
	return Array.from({ length: count }, (_, c) => view.slice(c * frames, (c + 1) * frames));
}

async function handle(req: DspRequest): Promise<Float32Array<ArrayBuffer>[]> {
	const dsp = await load();
	const frames = req.channels[0].length;
	const count = req.channels.length;
	const input = writePlanar(dsp, req.channels);
	try {
		if (req.op === 'denoise') {
			dsp.denoise(input.ptr, frames, count, req.sampleRate, req.strength);
			return readPlanar(dsp, input.ptr, frames, count);
		}
		const outFrames = dsp.stretch_out_frames(frames, req.rate);
		const outLen = outFrames * count;
		const outPtr = dsp.alloc_f32(outLen);
		try {
			const written = dsp.stretch_pitch(input.ptr, frames, count, req.sampleRate, req.rate, req.semitones, outPtr);
			return readPlanar(dsp, outPtr, written, count);
		} finally {
			dsp.free_f32(outPtr, outLen);
		}
	} finally {
		dsp.free_f32(input.ptr, input.len);
	}
}

self.onmessage = async (event: MessageEvent<DspRequest>) => {
	const req = event.data;
	try {
		const channels = await handle(req);
		const res: DspResponse = { id: req.id, channels };
		self.postMessage(res, channels.map((c) => c.buffer));
	} catch (err) {
		const res: DspResponse = { id: req.id, error: String(err) };
		self.postMessage(res);
	}
};
