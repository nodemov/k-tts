<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { decodeFile } from '#lib/audio/context.ts';
	import type { Preset } from '#lib/audio/effects/presets.ts';
	import { offlineKey, withSettings, type EffectSettings } from '#lib/audio/effects/settings.ts';
	import { baseName } from '#lib/audio/export.ts';
	import { measureLoudness } from '#lib/audio/loudness.ts';
	import { Player } from '#lib/audio/player.svelte.ts';
	import { processAudio, runOfflineStage, type Stage } from '#lib/audio/render.ts';
	import type { PresetKind } from '#lib/presets-store.ts';
	import { readSharedPreset } from '#lib/presets-store.ts';
	import { session, setSessionAudio } from '#lib/session.svelte.ts';
	import DropZone from './DropZone.svelte';
	import EffectControls, { type Section } from './EffectControls.svelte';
	import ExportPanel from './ExportPanel.svelte';
	import LevelMeter from './LevelMeter.svelte';
	import PresetBar from './PresetBar.svelte';
	import Transport from './Transport.svelte';
	import WavePreview from './WavePreview.svelte';

	interface Props {
		kind: PresetKind;
		path: string;
		presets: Preset[];
		sections: Section[];
		settings: EffectSettings;
		presetStyle?: 'cards' | 'pads';
		/** Rendered above the player, e.g. one-click buttons or a mic panel. */
		header?: Snippet<[{ source: AudioBuffer | null; apply: (s: EffectSettings, name?: string) => void }]>;
		/** Lets the page replace the source (e.g. after recording). */
		source?: AudioBuffer | null;
		sourceName?: string;
	}

	let {
		kind,
		path,
		presets,
		sections,
		settings = $bindable(),
		presetStyle = 'cards',
		header,
		source = $bindable(null),
		sourceName = $bindable('')
	}: Props = $props();

	let player = $state<Player | null>(null);
	let processed = $state.raw<AudioBuffer | null>(null);
	let processedKey = '';
	let dspBusy = $state(false);
	let activePreset = $state('');
	let error = $state('');
	let renderStage = $state<Stage | ''>('');
	let sourceLufs = $state<number | null>(null);
	let token = 0;

	onMount(() => {
		const p = new Player();
		void p.init($state.snapshot(settings)).then(() => (player = p));
		const shared = readSharedPreset();
		if (shared) {
			settings = shared.settings;
			activePreset = shared.name ?? 'shared';
		}
		if (!source && session.buffer) {
			source = session.buffer;
			sourceName = session.name;
		}
		return () => p.dispose();
	});

	$effect(() => {
		const s = $state.snapshot(settings);
		player?.update(s);
	});

	$effect(() => {
		const src = source;
		const key = offlineKey(settings);
		if (!src) return;
		sourceLufs = null;
		const timer = setTimeout(async () => {
			const mine = ++token;
			dspBusy = true;
			try {
				const out = await runOfflineStage(src, $state.snapshot(settings));
				if (mine !== token) return;
				processed = out;
				processedKey = key;
				player?.setBuffers(src, out);
			} catch (err) {
				error = `DSP error: ${err instanceof Error ? err.message : err}`;
			} finally {
				if (mine === token) dspBusy = false;
			}
		}, 200);
		return () => clearTimeout(timer);
	});

	$effect(() => {
		if (player && source && !processed) player.setBuffers(source, source);
	});

	$effect(() => {
		const src = source;
		if (!src) return;
		const id = setTimeout(() => (sourceLufs = measureLoudness(src).integrated), 50);
		return () => clearTimeout(id);
	});

	async function open(files: File[]) {
		error = '';
		try {
			const buf = await decodeFile(files[0]);
			processed = null;
			source = buf;
			sourceName = files[0].name;
			setSessionAudio(buf, files[0].name);
		} catch (err) {
			error = `เปิดไฟล์ไม่ได้: ${err instanceof Error ? err.message : err}`;
		}
	}

	function apply(s: EffectSettings, name = '') {
		settings = s;
		activePreset = name;
	}

	async function render() {
		const s = $state.snapshot(settings);
		const pre = processed && processedKey === offlineKey(s) ? processed : undefined;
		try {
			return await processAudio(source!, s, (st) => (renderStage = st), pre);
		} finally {
			renderStage = '';
		}
	}

	function onKey(e: KeyboardEvent) {
		const tag = (e.target as HTMLElement).tagName;
		if (!player || ['INPUT', 'SELECT', 'TEXTAREA'].includes(tag)) return;
		if (e.key === ' ') {
			e.preventDefault();
			void player.toggle();
		} else if (e.key.toLowerCase() === 'b') player.setBypass(!player.bypass);
	}
</script>

<svelte:window onkeydown={onKey} />

{@render header?.({ source, apply })}

{#if !source}
	<DropZone onfiles={open} onsession={() => session.buffer && ((source = session.buffer), (sourceName = session.name))} />
{:else}
	<section class="panel screws space-y-4 p-5 pt-7">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<p class="truncate font-display text-lg font-bold">{sourceName}</p>
			<div class="flex items-center gap-4 font-mono text-xs text-muted">
				{#if sourceLufs !== null && Number.isFinite(sourceLufs)}<span>ต้นฉบับ {sourceLufs.toFixed(1)} LUFS</span>{/if}
				<button class="btn btn-sm btn-ghost" onclick={() => ((source = null), (processed = null), player?.stop())}>เปลี่ยนไฟล์</button>
			</div>
		</div>
		<WavePreview
			buffer={player?.bypass ? source : (processed ?? source)}
			position={player?.position ?? 0}
			duration={player?.duration}
			onseek={(t) => player?.seek(t)}
			progressColor={player?.bypass ? '#9a917f' : '#ff6a1a'}
		/>
		{#if player}
			<Transport {player} busy={dspBusy} />
			<LevelMeter analyser={player.analyser} />
		{/if}
		<p class="text-xs text-muted">Space = เล่น/หยุด · B = เทียบเสียงต้นฉบับ (A/B) · ดับเบิลคลิก fader เพื่อรีเซ็ต</p>
	</section>
{/if}
{#if error}<p class="text-sm text-alarm">{error}</p>{/if}

<section class="space-y-3">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<h2 class="label text-cream!">Presets</h2>
		<button class="btn btn-sm btn-ghost" onclick={() => apply(withSettings({}), '')}>Reset ทั้งหมด</button>
	</div>
	{#if presetStyle === 'pads'}
		<div class="grid grid-cols-3 gap-2 sm:grid-cols-5 xl:grid-cols-8">
			{#each presets as p (p.id)}
				<button
					class="panel flex aspect-square flex-col items-center justify-center gap-1 p-2 transition-transform hover:-translate-y-0.5 active:translate-y-0.5"
					class:outline-2={activePreset === p.name}
					class:outline-signal={activePreset === p.name}
					onclick={() => apply(withSettings(p.patch), p.name)}
					title={p.description}
				>
					<span class="text-3xl">{p.emoji}</span>
					<span class="text-center font-display text-xs font-bold">{p.name}</span>
				</button>
			{/each}
		</div>
	{:else}
		<div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
			{#each presets as p (p.id)}
				<button
					class="panel p-4 text-left transition-colors hover:border-signal"
					class:border-signal={activePreset === p.name}
					onclick={() => apply(withSettings(p.patch), p.name)}
				>
					<span class="font-display font-bold">{p.name}</span>
					<span class="mt-1 block text-xs leading-relaxed text-muted">{p.description}</span>
				</button>
			{/each}
		</div>
	{/if}
	<PresetBar {kind} {path} {settings} onload={(s, n) => apply(s, n)} />
</section>

<EffectControls bind:settings {sections} />

{#if source}
	<section class="panel screws space-y-3 p-5 pt-7">
		<div class="flex items-center justify-between">
			<h2 class="label text-cream!">Render & Export</h2>
			{#if renderStage}
				<span class="font-mono text-xs text-signal">
					{renderStage === 'dsp' ? 'WASM DSP…' : renderStage === 'chain' ? 'Rendering effects…' : 'Loudness / trim…'}
				</span>
			{/if}
		</div>
		<ExportPanel {render} filename={`${baseName(sourceName)}-${kind}`} />
		<button
			class="btn btn-sm btn-ghost"
			onclick={async () => {
				const out = await render();
				setSessionAudio(out, `${baseName(sourceName)}-${kind}.wav`);
				source = out;
				sourceName = `${baseName(sourceName)}-${kind}`;
				processed = null;
				apply(withSettings({}), '');
			}}>✓ Apply — ใช้ผลลัพธ์เป็นคลิปปัจจุบัน (ต่อยอดได้)</button
		>
	</section>
{/if}
