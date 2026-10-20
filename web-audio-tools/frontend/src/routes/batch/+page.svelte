<script lang="ts">
	import { onMount } from 'svelte';
	import { zipSync } from 'fflate';
	import { decodeFile } from '#lib/audio/context.ts';
	import { ENHANCE_PRESETS, FUN_PRESETS } from '#lib/audio/effects/presets.ts';
	import { withSettings, type EffectSettings } from '#lib/audio/effects/settings.ts';
	import { DEFAULT_EXPORT, EXPORT_FORMATS, baseName, downloadBlob, exportAudio, type ExportOptions } from '#lib/audio/export.ts';
	import { measureLoudness } from '#lib/audio/loudness.ts';
	import { processAudio } from '#lib/audio/render.ts';
	import DropZone from '#lib/components/DropZone.svelte';
	import { loadSavedPresets, readSharedPreset } from '#lib/presets-store.ts';

	interface Item {
		id: string;
		file: File;
		status: 'pending' | 'working' | 'done' | 'error';
		stage: string;
		result?: Blob;
		lufs?: number;
		error?: string;
	}

	interface Choice {
		id: string;
		label: string;
		settings: () => EffectSettings;
	}

	let items = $state<Item[]>([]);
	let choices = $state<Choice[]>([]);
	let choiceId = $state('enhance:voice');
	let opts = $state<ExportOptions>({ ...DEFAULT_EXPORT });
	let running = $state(false);

	onMount(() => {
		const shared = readSharedPreset();
		choices = [
			...(shared ? [{ id: 'shared', label: `🔗 ${shared.name ?? 'Shared preset'}`, settings: () => shared.settings }] : []),
			...ENHANCE_PRESETS.map((p) => ({ id: `enhance:${p.id}`, label: `Enhance · ${p.name}`, settings: () => withSettings(p.patch) })),
			...FUN_PRESETS.map((p) => ({ id: `fun:${p.id}`, label: `Voice FX · ${p.emoji} ${p.name}`, settings: () => withSettings(p.patch) })),
			...loadSavedPresets().map((p) => ({ id: `saved:${p.id}`, label: `★ ${p.name}`, settings: () => withSettings(p.patch) })),
			{ id: 'none', label: 'ไม่ใส่เอฟเฟกต์ (แปลงไฟล์อย่างเดียว)', settings: () => withSettings({}) }
		];
		if (shared) choiceId = 'shared';
	});

	const done = $derived(items.filter((i) => i.status === 'done'));
	const lossy = $derived(EXPORT_FORMATS.find((f) => f.id === opts.format)?.lossy ?? false);

	function add(files: File[]) {
		items.push(...files.map((file) => ({ id: crypto.randomUUID(), file, status: 'pending' as const, stage: '' })));
	}

	async function runAll() {
		running = true;
		const settings = choices.find((c) => c.id === choiceId)!.settings();
		const exportOpts = $state.snapshot(opts);
		for (const item of items) {
			if (item.status === 'done') continue;
			item.status = 'working';
			item.error = undefined;
			try {
				item.stage = 'decode';
				const buf = await decodeFile(item.file);
				const out = await processAudio(buf, settings, (s) => (item.stage = s));
				item.stage = 'encode';
				item.result = await exportAudio(out, exportOpts);
				item.lufs = measureLoudness(out).integrated;
				item.status = 'done';
			} catch (err) {
				item.status = 'error';
				item.error = err instanceof Error ? err.message : String(err);
			}
			item.stage = '';
		}
		running = false;
	}

	const outName = (item: Item) => `${baseName(item.file.name)}.${opts.format}`;

	async function downloadZip() {
		const entries: Record<string, [Uint8Array, { level: 0 }]> = {};
		for (const item of done) entries[outName(item)] = [new Uint8Array(await item.result!.arrayBuffer()), { level: 0 }];
		const zip = zipSync(entries);
		downloadBlob(new Blob([zip.slice()], { type: 'application/zip' }), `batch-${choiceId.replace(/[:]/g, '-')}.zip`);
	}
</script>

<div class="stagger mx-auto max-w-6xl space-y-5">
	<header>
		<p class="label">08 — Batch</p>
		<h1 class="page-title text-4xl sm:text-5xl">ประมวลผลหลายไฟล์</h1>
		<p class="mt-2 text-muted">ใช้ preset เดียวกันกับทุกไฟล์ — ทำงานในเครื่องทั้งหมด แล้วดาวน์โหลดเป็น ZIP</p>
	</header>

	<section class="panel screws flex flex-wrap items-end gap-4 p-5 pt-7">
		<label class="min-w-64 flex-1 space-y-1">
			<span class="label">Preset</span>
			<select class="field" bind:value={choiceId}>
				{#each choices as c (c.id)}<option value={c.id}>{c.label}</option>{/each}
			</select>
		</label>
		<label class="space-y-1">
			<span class="label">Format</span>
			<select class="field w-36!" bind:value={opts.format}>
				{#each EXPORT_FORMATS as f (f.id)}<option value={f.id}>{f.label}</option>{/each}
			</select>
		</label>
		{#if lossy}
			<label class="space-y-1">
				<span class="label">Bitrate</span>
				<select class="field w-28!" bind:value={opts.bitrate}>
					{#each [128, 192, 256, 320] as b (b)}<option value={b}>{b} kbps</option>{/each}
				</select>
			</label>
		{/if}
		<button class="btn btn-signal" onclick={runAll} disabled={running || !items.length}>
			{running ? 'กำลังทำงาน…' : `▶ Process ${items.filter((i) => i.status !== 'done').length} ไฟล์`}
		</button>
		<button class="btn" onclick={downloadZip} disabled={!done.length || running}>⤓ ZIP ({done.length})</button>
	</section>

	{#if items.length}
		<section class="panel divide-y divide-line">
			{#each items as item, i (item.id)}
				<div class="flex flex-wrap items-center gap-3 px-4 py-3">
					<span
						class="led"
						class:on={item.status === 'done'}
						class:warn={item.status === 'working'}
						class:off-red={item.status === 'error'}
					></span>
					<span class="min-w-0 flex-1 truncate text-sm">{item.file.name}</span>
					<span class="font-mono text-xs text-muted">
						{#if item.status === 'working'}{item.stage}…{:else if item.status === 'done' && item.lufs !== undefined}{item.lufs.toFixed(1)} LUFS · {(item.result!.size / 1024 / 1024).toFixed(2)} MB{:else if item.status === 'error'}<span class="text-alarm">{item.error}</span>{:else}{(item.file.size / 1024 / 1024).toFixed(2)} MB{/if}
					</span>
					{#if item.status === 'done'}
						<button class="btn btn-sm" onclick={() => downloadBlob(item.result!, outName(item))}>⤓</button>
					{/if}
					<button class="btn btn-sm btn-ghost" disabled={running} aria-label="ลบ" onclick={() => items.splice(i, 1)}>×</button>
				</div>
			{/each}
		</section>
	{/if}

	<DropZone multiple compact={items.length > 0} onfiles={add} title="ลากไฟล์เสียงหลายไฟล์มาวาง" />
</div>
