/**
 * One flat, JSON-serialisable description of the whole processing chain.
 * Every effect has a neutral value so the realtime graph never needs rebuilding.
 */
export interface EffectSettings {
	// Offline stage (Rust/WASM worker)
	denoise: number;
	rate: number;
	pitch: number;
	reverse: boolean;
	// Filters
	highpass: number;
	lowpass: number;
	hum: 0 | 50 | 60;
	eq: number[];
	// Dynamics
	gate: number;
	deesser: number;
	compressor: { enabled: boolean; threshold: number; ratio: number; attack: number; release: number; makeup: number };
	// Colour / fun
	drive: number;
	ringmod: { freq: number; mix: number };
	echo: { time: number; feedback: number; mix: number };
	reverb: { decay: number; mix: number };
	eightD: number;
	// Output
	limiter: number | null;
	gain: number;
	// Post-processing on the rendered file
	loudness: number | null;
	trimSilence: boolean;
}

export const EQ_BANDS = [31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000];

export const GATE_OFF = -100;

export function defaultSettings(): EffectSettings {
	return {
		denoise: 0,
		rate: 1,
		pitch: 0,
		reverse: false,
		highpass: 0,
		lowpass: 0,
		hum: 0,
		eq: EQ_BANDS.map(() => 0),
		gate: GATE_OFF,
		deesser: 0,
		compressor: { enabled: false, threshold: -20, ratio: 3, attack: 0.01, release: 0.2, makeup: 0 },
		drive: 0,
		ringmod: { freq: 60, mix: 0 },
		echo: { time: 0.3, feedback: 0.35, mix: 0 },
		reverb: { decay: 2, mix: 0 },
		eightD: 0,
		limiter: null,
		gain: 0,
		loudness: null,
		trimSilence: false
	};
}

type DeepPartial<T> = { [K in keyof T]?: T[K] extends object ? (T[K] extends unknown[] ? T[K] : DeepPartial<T[K]>) : T[K] };
export type SettingsPatch = DeepPartial<EffectSettings>;

export function withSettings(patch: SettingsPatch, base: EffectSettings = defaultSettings()): EffectSettings {
	const out = structuredClone(base);
	for (const [key, value] of Object.entries(patch) as [keyof EffectSettings, unknown][]) {
		if (value && typeof value === 'object' && !Array.isArray(value)) {
			Object.assign(out[key] as object, value);
		} else if (value !== undefined) {
			(out as unknown as Record<string, unknown>)[key] = Array.isArray(value) ? [...value] : value;
		}
	}
	return out;
}

/** Key of the parameters that require re-running the offline WASM stage. */
export function offlineKey(s: EffectSettings): string {
	return `${s.denoise.toFixed(2)}|${s.rate.toFixed(3)}|${s.pitch.toFixed(2)}|${s.reverse}`;
}

export function hasOfflineStage(s: EffectSettings): boolean {
	return s.denoise > 0 || s.rate !== 1 || s.pitch !== 0 || s.reverse;
}

/** Seconds of tail (reverb/echo) to render after the source ends. */
export function tailSeconds(s: EffectSettings): number {
	let tail = 0;
	if (s.reverb.mix > 0) tail = Math.max(tail, s.reverb.decay + 0.2);
	if (s.echo.mix > 0) {
		const fb = Math.min(0.95, Math.max(0.01, s.echo.feedback));
		tail = Math.max(tail, Math.min(8, s.echo.time * (Math.log(0.001) / Math.log(fb))));
	}
	return tail;
}

/** Diff against defaults keeps shared preset links short. */
export function diffSettings(s: EffectSettings): SettingsPatch {
	const base = defaultSettings();
	const out: Record<string, unknown> = {};
	for (const key of Object.keys(s) as (keyof EffectSettings)[]) {
		if (JSON.stringify(s[key]) !== JSON.stringify(base[key])) out[key] = s[key];
	}
	return out as SettingsPatch;
}
