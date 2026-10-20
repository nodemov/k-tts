<script lang="ts">
	import { onMount } from 'svelte';
	import { withSettings, type EffectSettings } from '#lib/audio/effects/settings.ts';
	import { deletePreset, loadSavedPresets, savePreset, shareLink, type PresetKind, type SavedPreset } from '#lib/presets-store.ts';

	interface Props {
		kind: PresetKind;
		path: string;
		settings: EffectSettings;
		onload: (settings: EffectSettings, name: string) => void;
	}

	let { kind, path, settings, onload }: Props = $props();

	let saved = $state<SavedPreset[]>([]);
	let toast = $state('');

	onMount(() => (saved = loadSavedPresets(kind)));

	function flash(msg: string) {
		toast = msg;
		setTimeout(() => (toast = ''), 2200);
	}

	function save() {
		const name = prompt('ตั้งชื่อ preset');
		if (!name) return;
		savePreset(name, kind, $state.snapshot(settings));
		saved = loadSavedPresets(kind);
		flash('บันทึกแล้ว');
	}

	async function share() {
		const link = shareLink(path, $state.snapshot(settings));
		try {
			await navigator.clipboard.writeText(link);
			flash('คัดลอกลิงก์แล้ว — ส่งให้เพื่อนได้เลย');
		} catch {
			prompt('คัดลอกลิงก์นี้', link);
		}
	}
</script>

<div class="flex flex-wrap items-center gap-2">
	<span class="label mr-1">My presets</span>
	{#each saved as p (p.id)}
		<span class="inline-flex overflow-hidden rounded border border-line">
			<button class="px-2 py-1 text-xs hover:bg-panel-2" onclick={() => onload(withSettings(p.patch), p.name)}>{p.name}</button>
			<button
				class="border-l border-line px-1.5 text-xs text-muted hover:text-alarm"
				aria-label="ลบ {p.name}"
				onclick={() => {
					deletePreset(p.id);
					saved = loadSavedPresets(kind);
				}}>×</button
			>
		</span>
	{/each}
	<button class="btn btn-sm" onclick={save}>+ Save</button>
	<button class="btn btn-sm" onclick={share}>🔗 Share link</button>
	{#if toast}<span class="text-xs text-phosphor">{toast}</span>{/if}
</div>
