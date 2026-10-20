import { runFFmpeg } from './ffmpeg';
import { encodeWav, type WavBitDepth } from './wav';

export type ExportFormat = 'wav' | 'mp3' | 'ogg' | 'opus' | 'flac' | 'm4a';

export interface ExportOptions {
	format: ExportFormat;
	bitrate: number;
	bitDepth: WavBitDepth;
}

export const EXPORT_FORMATS: { id: ExportFormat; label: string; lossy: boolean }[] = [
	{ id: 'mp3', label: 'MP3', lossy: true },
	{ id: 'wav', label: 'WAV', lossy: false },
	{ id: 'flac', label: 'FLAC', lossy: false },
	{ id: 'ogg', label: 'OGG Vorbis', lossy: true },
	{ id: 'opus', label: 'Opus', lossy: true },
	{ id: 'm4a', label: 'M4A (AAC)', lossy: true }
];

export const DEFAULT_EXPORT: ExportOptions = { format: 'mp3', bitrate: 192, bitDepth: 16 };

const CODEC_ARGS: Record<Exclude<ExportFormat, 'wav'>, (bitrate: number) => string[]> = {
	mp3: (b) => ['-c:a', 'libmp3lame', '-b:a', `${b}k`],
	ogg: (b) => ['-c:a', 'libvorbis', '-b:a', `${b}k`],
	opus: (b) => ['-c:a', 'libopus', '-b:a', `${Math.min(b, 256)}k`, '-ar', '48000'],
	flac: () => ['-c:a', 'flac', '-compression_level', '8'],
	m4a: (b) => ['-c:a', 'aac', '-b:a', `${b}k`]
};

export async function exportAudio(
	buf: AudioBuffer,
	opts: ExportOptions,
	onProgress?: (ratio: number) => void
): Promise<Blob> {
	if (opts.format === 'wav') return encodeWav(buf, opts.bitDepth);
	const wav = encodeWav(buf, opts.format === 'flac' && opts.bitDepth === 24 ? 24 : 16);
	return runFFmpeg(wav, 'export.wav', `export.${opts.format}`, CODEC_ARGS[opts.format](opts.bitrate), onProgress);
}

export function baseName(name: string): string {
	return name.replace(/\.[^.]+$/, '') || 'audio';
}

export function downloadBlob(blob: Blob, filename: string): void {
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
	setTimeout(() => URL.revokeObjectURL(url), 10_000);
}
