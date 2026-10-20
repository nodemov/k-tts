<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import WaveSurfer from 'wavesurfer.js';
	import RegionsPlugin from 'wavesurfer.js/dist/plugins/regions.esm.js';
	import SpectrogramPlugin from 'wavesurfer.js/dist/plugins/spectrogram.esm.js';
	import TimelinePlugin from 'wavesurfer.js/dist/plugins/timeline.esm.js';
	import {
		fadeRange,
		gainRange,
		insertBuffer,
		insertSilence,
		normalizePeak,
		removeRange,
		reverseRange,
		silenceRange,
		sliceBuffer
	} from '#lib/audio/buffer.ts';
	import { decodeFile, formatTime } from '#lib/audio/context.ts';
	import { baseName } from '#lib/audio/export.ts';
	import { measureLoudness, type LoudnessReport } from '#lib/audio/loudness.ts';
	import { encodeWav } from '#lib/audio/wav.ts';
	import DropZone from '#lib/components/DropZone.svelte';
	import ExportPanel from '#lib/components/ExportPanel.svelte';
	import { session, setSessionAudio } from '#lib/session.svelte.ts';

	const HISTORY_LIMIT = 40;

	let waveEl: HTMLDivElement;
	let timelineEl: HTMLDivElement;
	let spectroEl: HTMLDivElement;

	let ws: WaveSurfer | null = null;
	let regions: ReturnType<typeof RegionsPlugin.create> | null = null;
	let spectrogram: SpectrogramPlugin | null = null;

	let buffer = $state.raw<AudioBuffer | null>(null);
	let name = $state('untitled');
	let undoStack = $state.raw<AudioBuffer[]>([]);
	let redoStack = $state.raw<AudioBuffer[]>([]);
	let clipboard = $state.raw<AudioBuffer | null>(null);
	let selection = $state<{ start: number; end: number } | null>(null);
	let cursor = $state(0);
	let playing = $state(false);
	let zoom = $state(0);
	let showSpectrogram = $state(false);
	let report = $state<LoudnessReport | null>(null);
	let busy = $state(false);
	let error = $state('');

	onMount(() => {
		regions = RegionsPlugin.create();
		ws = WaveSurfer.create({
			container: waveEl,
			height: 180,
			waveColor: '#6b6252',
			progressColor: '#ff6a1a',
			cursorColor: '#efe6d2',
			cursorWidth: 1,
			barWidth: 2,
			barGap: 1,
			barRadius: 1,
			sampleRate: 22050,
			normalize: false,
			autoScroll: true,
			plugins: [
				regions,
				TimelinePlugin.create({
					container: timelineEl,
					style: { color: '#9a917f', fontSize: '10px', fontFamily: 'JetBrains Mono' }
				})
			]
		});
		regions.enableDragSelection({ color: 'rgba(255,106,26,0.18)' });
		regions.on('region-created', (r) => {
			for (const other of regions!.getRegions()) if (other !== r) other.remove();
			selection = { start: r.start, end: r.end };
		});
		regions.on('region-updated', (r) => (selection = { start: r.start, end: r.end }));
		ws.on('interaction', (t) => {
			regions?.clearRegions();
			selection = null;
			cursor = t;
		});
		ws.on('timeupdate', (t) => (cursor = t));
		ws.on('play', () => (playing = true));
		ws.on('pause', () => (playing = false));
		ws.on('finish', () => (playing = false));

		if (session.buffer) load(session.buffer, session.name, false);
		return () => ws?.destroy();
	});

	async function load(buf: AudioBuffer, fileName: string, resetHistory = true) {
		buffer = buf;
		name = fileName;
		report = null;
		if (resetHistory) {
			undoStack = [];
			redoStack = [];
		}
		const keepTime = Math.min(cursor, buf.duration);
		await ws?.loadBlob(encodeWav(buf, 16));
		if (zoom) ws?.zoom(zoom);
		ws?.setTime(keepTime);
		if (selection) {
			const { start, end } = selection;
			if (end <= buf.duration) regions?.addRegion({ start, end, color: 'rgba(255,106,26,0.18)' });
			else selection = null;
		}
		setSessionAudio(buf, fileName);
	}

	async function openFiles(files: File[]) {
		busy = true;
		error = '';
		try {
			selection = null;
			regions?.clearRegions();
			await load(await decodeFile(files[0]), files[0].name);
		} catch (err) {
			error = `เปิดไฟล์ไม่ได้: ${err instanceof Error ? err.message : err}`;
		} finally {
			busy = false;
		}
	}

	async function edit(fn: (buf: AudioBuffer) => AudioBuffer | Promise<AudioBuffer>, clearSelection = false) {
		if (!buffer || busy) return;
		busy = true;
		try {
			const next = await fn(buffer);
			undoStack = [...undoStack.slice(-HISTORY_LIMIT + 1), buffer];
			redoStack = [];
			if (clearSelection) {
				selection = null;
				regions?.clearRegions();
			}
			await load(next, name, false);
		} finally {
			busy = false;
		}
	}

	async function undo() {
		const prev = undoStack.at(-1);
		if (!prev || !buffer) return;
		redoStack = [...redoStack, buffer];
		undoStack = undoStack.slice(0, -1);
		await load(prev, name, false);
	}

	async function redo() {
		const next = redoStack.at(-1);
		if (!next || !buffer) return;
		undoStack = [...undoStack, buffer];
		redoStack = redoStack.slice(0, -1);
		await load(next, name, false);
	}

	const sel = () => selection ?? { start: 0, end: buffer?.duration ?? 0 };

	const ops = {
		cut: () => {
			if (!selection || !buffer) return;
			const { start, end } = selection;
			clipboard = sliceBuffer(buffer, start, end);
			cursor = start;
			void edit((b) => removeRange(b, start, end), true);
		},
		copy: () => {
			if (selection && buffer) clipboard = sliceBuffer(buffer, selection.start, selection.end);
		},
		paste: () => {
			if (!clipboard) return;
			const clip = clipboard;
			const at = selection?.start ?? cursor;
			const until = selection?.end;
			void edit((b) => insertBuffer(b, at, clip, until), true);
		},
		remove: () => {
			if (!selection) return;
			const { start, end } = selection;
			cursor = start;
			void edit((b) => removeRange(b, start, end), true);
		},
		crop: () => {
			if (!selection) return;
			const { start, end } = selection;
			cursor = 0;
			void edit((b) => sliceBuffer(b, start, end), true);
		},
		silence: () => selection && void edit((b) => silenceRange(b, selection!.start, selection!.end)),
		insertSilence: () => void edit((b) => insertSilence(b, cursor, 1)),
		fadeIn: () => {
			const { start, end } = selection ?? { start: 0, end: Math.min(2, buffer?.duration ?? 0) };
			void edit((b) => fadeRange(b, start, end, 'in'));
		},
		fadeOut: () => {
			const d = buffer?.duration ?? 0;
			const { start, end } = selection ?? { start: Math.max(0, d - 2), end: d };
			void edit((b) => fadeRange(b, start, end, 'out'));
		},
		gain: (db: number) => {
			const { start, end } = sel();
			void edit((b) => gainRange(b, db, start, end));
		},
		normalize: () => {
			const { start, end } = sel();
			void edit((b) => normalizePeak(b, -1, start, end));
		},
		reverse: () => {
			const { start, end } = sel();
			void edit((b) => reverseRange(b, start, end));
		}
	};

	function toggleSpectrogram() {
		showSpectrogram = !showSpectrogram;
		if (!ws) return;
		if (showSpectrogram) {
			spectrogram = ws.registerPlugin(
				SpectrogramPlugin.create({
					container: spectroEl,
					labels: true,
					height: 160,
					colorMap: 'roseus',
					scale: 'mel',
					frequencyMax: 11025,
					labelsBackground: 'rgba(18,17,14,0.6)'
				})
			);
		} else {
			spectrogram?.destroy();
			spectrogram = null;
		}
	}

	function onKey(e: KeyboardEvent) {
		const target = e.target as HTMLElement;
		if (!buffer || ['INPUT', 'SELECT', 'TEXTAREA'].includes(target.tagName)) return;
		const mod = e.metaKey || e.ctrlKey;
		const key = e.key.toLowerCase();
		const run = (fn: () => void) => {
			e.preventDefault();
			fn();
		};
		if (key === ' ') run(() => (selection ? ws?.play(selection.start, selection.end) : ws?.playPause()));
		else if (mod && key === 'z') run(() => (e.shiftKey ? redo() : undo()));
		else if (mod && key === 'y') run(redo);
		else if (mod && key === 'x') run(ops.cut);
		else if (mod && key === 'c') run(ops.copy);
		else if (mod && key === 'v') run(ops.paste);
		else if (key === 'delete' || key === 'backspace') run(ops.remove);
	}

	const tools = $derived([
		{ label: 'Cut', key: '⌘X', run: ops.cut, needsSel: true },
		{ label: 'Copy', key: '⌘C', run: ops.copy, needsSel: true },
		{ label: 'Paste', key: '⌘V', run: ops.paste, disabled: !clipboard },
		{ label: 'Delete', key: '⌫', run: ops.remove, needsSel: true },
		{ label: 'Crop', key: '', run: ops.crop, needsSel: true },
		{ label: 'Silence', key: '', run: ops.silence, needsSel: true },
		{ label: '+1s Silence', key: '', run: ops.insertSilence },
		{ label: 'Fade in', key: '', run: ops.fadeIn },
		{ label: 'Fade out', key: '', run: ops.fadeOut },
		{ label: '+3 dB', key: '', run: () => ops.gain(3) },
		{ label: '−3 dB', key: '', run: () => ops.gain(-3) },
		{ label: 'Normalize', key: '', run: ops.normalize },
		{ label: 'Reverse', key: '', run: ops.reverse }
	]);
</script>

<svelte:window onkeydown={onKey} />

<div class="stagger mx-auto max-w-7xl space-y-5">
	<header class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<p class="label text-signal!">01 — Editor</p>
			<h1 class="page-title text-4xl sm:text-5xl">ตัดต่อเสียง</h1>
		</div>
		{#if buffer}
			<div class="flex flex-wrap gap-2">
				<button class="btn" onclick={() => goto('/enhance')}>→ Enhance</button>
				<button class="btn" onclick={() => goto('/fun')}>→ Voice FX</button>
				<button class="btn" onclick={() => goto('/visualizer')}>→ Visualizer</button>
			</div>
		{/if}
	</header>

	{#if !buffer}
		<DropZone onfiles={openFiles} title={busy ? 'กำลังถอดรหัส…' : 'ลากไฟล์เสียงมาวางเพื่อเริ่มตัดต่อ'} />
		{#if error}<p class="text-alarm">{error}</p>{/if}
	{/if}

	<section class="panel screws overflow-hidden" class:hidden={!buffer}>
		<div class="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-line px-5 pt-6 pb-3">
			<input class="field w-auto! min-w-48 flex-1 font-display text-lg font-bold" bind:value={name} aria-label="ชื่อไฟล์" />
			{#if buffer}
				<span class="readout text-sm">{formatTime(cursor)} / {formatTime(buffer.duration)}</span>
				<span class="font-mono text-xs text-muted">
					{buffer.sampleRate} Hz · {buffer.numberOfChannels === 1 ? 'mono' : `${buffer.numberOfChannels}ch`}
				</span>
				{#if selection}
					<span class="font-mono text-xs text-signal">
						SEL {formatTime(selection.start)} → {formatTime(selection.end)} ({(selection.end - selection.start).toFixed(2)}s)
					</span>
				{/if}
			{/if}
		</div>

		<div class="bg-[#0d0c0a] px-2 pt-2">
			<div bind:this={waveEl}></div>
			<div bind:this={timelineEl}></div>
			<div bind:this={spectroEl} class:hidden={!showSpectrogram}></div>
		</div>

		<div class="flex flex-wrap items-center gap-2 border-t border-line px-5 py-3">
			<button
				class="btn btn-signal w-28"
				onclick={() => (selection && !playing ? ws?.play(selection.start, selection.end) : ws?.playPause())}
				title="Space"
			>
				{playing ? '❚❚ Pause' : selection ? '▶ Play sel' : '▶ Play'}
			</button>
			<button class="btn" onclick={() => ws?.stop()} aria-label="Stop">■</button>
			<button class="btn" onclick={undo} disabled={!undoStack.length || busy} title="⌘Z">↶ Undo</button>
			<button class="btn" onclick={redo} disabled={!redoStack.length || busy} title="⇧⌘Z">↷ Redo</button>
			<label class="ml-auto flex items-center gap-2">
				<span class="label">Zoom</span>
				<input
					type="range"
					class="fader w-36!"
					min="0"
					max="500"
					step="5"
					bind:value={zoom}
					style="--fill:{zoom / 5}%"
					oninput={() => ws?.zoom(zoom)}
				/>
			</label>
			<button class="btn" aria-pressed={showSpectrogram} onclick={toggleSpectrogram}>Spectrogram</button>
		</div>

		<div class="flex flex-wrap gap-1.5 border-t border-line bg-ink/40 px-5 py-3">
			{#each tools as tool (tool.label)}
				<button
					class="btn btn-sm"
					onclick={tool.run}
					disabled={busy || tool.disabled || (tool.needsSel && !selection)}
					title={tool.key || undefined}
				>
					{tool.label}
				</button>
			{/each}
		</div>
	</section>

	{#if buffer}
		<div class="grid gap-4 lg:grid-cols-[1fr_auto]">
			<section class="panel space-y-3 p-5">
				<h2 class="label text-cream!">Export</h2>
				<ExportPanel render={() => buffer!} filename={baseName(name)} />
			</section>
			<section class="panel min-w-64 space-y-2 p-5">
				<div class="flex items-center justify-between gap-3">
					<h2 class="label text-cream!">Loudness</h2>
					<button class="btn btn-sm" onclick={() => buffer && (report = measureLoudness(buffer))}>วัด</button>
				</div>
				{#if report}
					<p class="readout text-3xl">{Number.isFinite(report.integrated) ? report.integrated.toFixed(1) : '−∞'} <span class="text-sm">LUFS</span></p>
					<p class="font-mono text-xs text-muted">peak {report.peakDb.toFixed(1)} dBFS · short-term max {report.shortTermMax.toFixed(1)}</p>
				{:else}
					<p class="text-xs text-muted">วัดความดังตามมาตรฐาน ITU-R BS.1770</p>
				{/if}
			</section>
		</div>

		<details class="text-xs text-muted">
			<summary class="cursor-pointer">คีย์ลัด</summary>
			<p class="mt-2 font-mono">
				ลากบน waveform = เลือกช่วง · Space เล่น/หยุด · ⌘Z / ⇧⌘Z undo/redo · ⌘X ⌘C ⌘V · Delete ลบช่วงที่เลือก
			</p>
		</details>
		<DropZone compact onfiles={openFiles} title="เปิดไฟล์อื่น" />
	{/if}
</div>
