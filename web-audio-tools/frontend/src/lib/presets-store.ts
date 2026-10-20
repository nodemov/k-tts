import { diffSettings, withSettings, type EffectSettings, type SettingsPatch } from '#lib/audio/effects/settings.ts';

export type PresetKind = 'enhance' | 'fun';

export interface SavedPreset {
	id: string;
	name: string;
	kind: PresetKind;
	patch: SettingsPatch;
	createdAt: number;
}

const KEY = 'wat.presets.v1';

export function loadSavedPresets(kind?: PresetKind): SavedPreset[] {
	try {
		const all = JSON.parse(localStorage.getItem(KEY) ?? '[]') as SavedPreset[];
		return kind ? all.filter((p) => p.kind === kind) : all;
	} catch {
		return [];
	}
}

export function savePreset(name: string, kind: PresetKind, settings: EffectSettings): SavedPreset {
	const preset: SavedPreset = { id: crypto.randomUUID(), name, kind, patch: diffSettings(settings), createdAt: Date.now() };
	localStorage.setItem(KEY, JSON.stringify([preset, ...loadSavedPresets()]));
	return preset;
}

export function deletePreset(id: string) {
	localStorage.setItem(KEY, JSON.stringify(loadSavedPresets().filter((p) => p.id !== id)));
}

function toBase64Url(text: string): string {
	const bytes = new TextEncoder().encode(text);
	let bin = '';
	for (const b of bytes) bin += String.fromCharCode(b);
	return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function fromBase64Url(text: string): string {
	const bin = atob(text.replace(/-/g, '+').replace(/_/g, '/'));
	return new TextDecoder().decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
}

export function shareLink(path: string, settings: EffectSettings, name?: string): string {
	const payload = toBase64Url(JSON.stringify({ n: name, p: diffSettings(settings) }));
	return `${location.origin}${path}#preset=${payload}`;
}

export function readSharedPreset(hash = location.hash): { name?: string; settings: EffectSettings } | null {
	const match = /preset=([\w-]+)/.exec(hash);
	if (!match) return null;
	try {
		const data = JSON.parse(fromBase64Url(match[1])) as { n?: string; p: SettingsPatch };
		return { name: data.n, settings: withSettings(data.p) };
	} catch {
		return null;
	}
}
