<script lang="ts">
	import { DEFAULT_EXPORT, EXPORT_FORMATS, downloadBlob, exportAudio, type ExportOptions } from '#lib/audio/export.ts';
	import { isFFmpegLoaded } from '#lib/audio/ffmpeg.ts';

	interface Props {
		render: () => Promise<AudioBuffer> | AudioBuffer;
		filename: string;
		disabled?: boolean;
		label?: string;
	}

	let { render, filename, disabled = false, label = 'Export' }: Props = $props();

	let opts = $state<ExportOptions>({ ...DEFAULT_EXPORT });
	let busy = $state(false);
	let stage = $state('');
	let progress = $state(0);
	let error = $state('');

	const lossy = $derived(EXPORT_FORMATS.find((f) => f.id === opts.format)?.lossy ?? false);

	async function run() {
		busy = true;
		error = '';
		progress = 0;
		try {
			stage = 'กำลังประมวลผล…';
			const buf = await render();
			if (opts.format !== 'wav' && !isFFmpegLoaded()) stage = 'โหลด encoder (ครั้งแรกเท่านั้น)…';
			else stage = 'กำลังเข้ารหัส…';
			const blob = await exportAudio(buf, $state.snapshot(opts), (p) => {
				stage = 'กำลังเข้ารหัส…';
				progress = p;
			});
			downloadBlob(blob, `${filename}.${opts.format}`);
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
			stage = '';
		}
	}
</script>

<div class="flex flex-wrap items-end gap-3">
	<label class="flex flex-col gap-1">
		<span class="label">Format</span>
		<select class="field w-36!" bind:value={opts.format}>
			{#each EXPORT_FORMATS as f (f.id)}
				<option value={f.id}>{f.label}</option>
			{/each}
		</select>
	</label>
	{#if lossy}
		<label class="flex flex-col gap-1">
			<span class="label">Bitrate</span>
			<select class="field w-28!" bind:value={opts.bitrate}>
				{#each [96, 128, 160, 192, 256, 320] as b (b)}
					<option value={b}>{b} kbps</option>
				{/each}
			</select>
		</label>
	{:else}
		<label class="flex flex-col gap-1">
			<span class="label">Bit depth</span>
			<select class="field w-28!" bind:value={opts.bitDepth}>
				<option value={16}>16-bit</option>
				<option value={24}>24-bit</option>
				{#if opts.format === 'wav'}<option value={32}>32-bit float</option>{/if}
			</select>
		</label>
	{/if}
	<button class="btn btn-signal" onclick={run} disabled={disabled || busy}>
		{busy ? 'กำลังทำงาน…' : `⤓ ${label}`}
	</button>
	{#if busy}
		<div class="flex min-w-48 flex-1 flex-col gap-1">
			<span class="text-xs text-muted">{stage}</span>
			<div class="h-1 overflow-hidden rounded bg-ink">
				<div class="h-full bg-signal transition-all" style="width:{Math.max(4, progress * 100)}%"></div>
			</div>
		</div>
	{/if}
	{#if error}<p class="w-full text-sm text-alarm">{error}</p>{/if}
</div>
