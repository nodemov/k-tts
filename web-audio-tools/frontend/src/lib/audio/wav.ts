export type WavBitDepth = 16 | 24 | 32;

export function encodeWav(buf: AudioBuffer, bitDepth: WavBitDepth = 16): Blob {
	const channels = buf.numberOfChannels;
	const frames = buf.length;
	const bytesPerSample = bitDepth / 8;
	const blockAlign = channels * bytesPerSample;
	const dataSize = frames * blockAlign;
	const out = new ArrayBuffer(44 + dataSize);
	const view = new DataView(out);
	const isFloat = bitDepth === 32;

	const writeStr = (offset: number, s: string) => {
		for (let i = 0; i < s.length; i++) view.setUint8(offset + i, s.charCodeAt(i));
	};
	writeStr(0, 'RIFF');
	view.setUint32(4, 36 + dataSize, true);
	writeStr(8, 'WAVE');
	writeStr(12, 'fmt ');
	view.setUint32(16, 16, true);
	view.setUint16(20, isFloat ? 3 : 1, true);
	view.setUint16(22, channels, true);
	view.setUint32(24, buf.sampleRate, true);
	view.setUint32(28, buf.sampleRate * blockAlign, true);
	view.setUint16(32, blockAlign, true);
	view.setUint16(34, bitDepth, true);
	writeStr(36, 'data');
	view.setUint32(40, dataSize, true);

	const data = Array.from({ length: channels }, (_, c) => buf.getChannelData(c));
	let offset = 44;
	for (let i = 0; i < frames; i++) {
		for (let c = 0; c < channels; c++) {
			const s = Math.max(-1, Math.min(1, data[c][i]));
			if (isFloat) view.setFloat32(offset, s, true);
			else if (bitDepth === 16) view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true);
			else {
				const v = Math.round(s < 0 ? s * 0x800000 : s * 0x7fffff);
				view.setUint8(offset, v & 0xff);
				view.setUint8(offset + 1, (v >> 8) & 0xff);
				view.setUint8(offset + 2, (v >> 16) & 0xff);
			}
			offset += bytesPerSample;
		}
	}
	return new Blob([out], { type: 'audio/wav' });
}
