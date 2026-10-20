<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { dbToGain, decodeFile, formatTime, getAudioContext, resumeAudio } from '#lib/audio/context.ts';
	import { downloadBlob } from '#lib/audio/export.ts';
	import { pickRecorderMime } from '#lib/audio/mic.svelte.ts';
	import { BUILTIN_SOUNDS, renderBuiltin } from '#lib/audio/synth.ts';
	import DropZone from '#lib/components/DropZone.svelte';
	import Fader from '#lib/components/Fader.svelte';
	import LevelMeter from '#lib/components/LevelMeter.svelte';
	import Switch from '#lib/components/Switch.svelte';
	import { deletePad, listPads, putPad } from '#lib/idb.ts';
	import { setSessionAudio } from '#lib/session.svelte.ts';

	interface Pad {
		id: string;
		label: string;
		emoji: string;
		color: string;
		user: boolean;
		load: () => Promise<AudioBuffer>;
	}

	const KEYS = '1234567890qwertyuiopasdfghjklzxcvbnm';
	const COLORS = ['#ff6a1a', '#5cc8ff', '#b8ff5c', '#ff5cc8', '#ffd23f', '#9d7bff'];

	const ctx = getAudioContext();
	const master = new GainNode(ctx);
	const analyser = new AnalyserNode(ctx, { fftSize: 1024 });
	const recordDest = new MediaStreamAudioDestinationNode(ctx);
	master.connect(analyser).connect(ctx.destination);
	master.connect(recordDest);

	let userPads = $state<Pad[]>([]);
	let volume = $state(0);
	let randomPitch = $state(false);
	let choke = $state(false);
	let hits = $state<Record<string, number>>({});
	let recording = $state(false);
	let recElapsed = $state(0);
	let recorder: MediaRecorder | null = null;
	let recTimer = 0;
	const buffers = new Map<string, Promise<AudioBuffer>>();
	const playingNodes = new Set<AudioBufferSourceNode>();

	const builtinPads: Pad[] = BUILTIN_SOUNDS.map((s, i) => ({
		id: s.id,
		label: s.label,
		emoji: s.emoji,
		color: COLORS[i % COLORS.length],
		user: false,
		load: () => renderBuiltin(s.id, ctx.sampleRate)
	}));
	const pads = $derived([...builtinPads, ...userPads]);

	$effect(() => {
		master.gain.setTargetAtTime(dbToGain(volume), ctx.currentTime, 0.02);
	});

	onMount(async () => {
		const stored = await listPads().catch(() => []);
		userPads = stored.map((p) => ({
			id: p.id,
			label: p.name,
			emoji: '🎵',
			color: p.color,
			user: true,
			load: () => decodeFile(new Blob([p.data], { type: p.type }))
		}));
	});

	onDestroy(() => {
		for (const n of playingNodes) n.stop();
		master.disconnect();
	});

	async function trigger(pad: Pad) {
		await resumeAudio();
		let p = buffers.get(pad.id);
		if (!p) {
			p = pad.load();
			buffers.set(pad.id, p);
		}
		const buffer = await p;
		if (choke) for (const n of playingNodes) n.stop();
		const src = new AudioBufferSourceNode(ctx, { buffer, playbackRate: randomPitch ? 0.8 + Math.random() * 0.5 : 1 });
		src.connect(master);
		src.onended = () => playingNodes.delete(src);
		playingNodes.add(src);
		src.start();
		hits[pad.id] = (hits[pad.id] ?? 0) + 1;
	}

	async function addFiles(files: File[]) {
		for (const file of files) {
			const id = crypto.randomUUID();
			const color = COLORS[(userPads.length + 3) % COLORS.length];
			const data = await file.arrayBuffer();
			await putPad({ id, name: file.name.replace(/\.[^.]+$/, ''), color, data, type: file.type });
			userPads.push({
				id,
				label: file.name.replace(/\.[^.]+$/, ''),
				emoji: '🎵',
				color,
				user: true,
				load: () => decodeFile(new Blob([data], { type: file.type }))
			});
		}
	}

	async function removePad(pad: Pad) {
		await deletePad(pad.id);
		userPads = userPads.filter((p) => p.id !== pad.id);
		buffers.delete(pad.id);
	}

	function toggleRecord() {
		if (recording && recorder) {
			recorder.stop();
			return;
		}
		const chunks: Blob[] = [];
		const mimeType = pickRecorderMime('audio');
		recorder = new MediaRecorder(recordDest.stream, mimeType ? { mimeType } : undefined);
		recorder.ondataavailable = (e) => e.data.size && chunks.push(e.data);
		recorder.onstop = async () => {
			clearInterval(recTimer);
			recording = false;
			const blob = new Blob(chunks, { type: recorder?.mimeType });
			downloadBlob(blob, `soundboard-${Date.now()}.${blob.type.includes('mp4') ? 'm4a' : 'webm'}`);
			try {
				setSessionAudio(await decodeFile(blob), 'soundboard-take');
			} catch {
				/* decoding is optional */
			}
		};
		recorder.start(250);
		recording = true;
		const began = performance.now();
		recTimer = window.setInterval(() => (recElapsed = (performance.now() - began) / 1000), 100);
	}

	function onKey(e: KeyboardEvent) {
		if (e.repeat || e.metaKey || e.ctrlKey || ['INPUT', 'SELECT', 'TEXTAREA'].includes((e.target as HTMLElement).tagName)) return;
		const idx = KEYS.indexOf(e.key.toLowerCase());
		if (idx >= 0 && pads[idx]) {
			e.preventDefault();
			void trigger(pads[idx]);
		}
	}
</script>

<svelte:window onkeydown={onKey} />

<div class="stagger mx-auto max-w-7xl space-y-5">
	<header>
		<p class="label text-[#ffd23f]!">05 — Soundboard</p>
		<h1 class="page-title text-4xl sm:text-5xl">ปุ่มเสียง<span class="text-[#ffd23f]">เอฟเฟกต์</span></h1>
		<p class="mt-2 text-muted">กดปุ่มหรือคีย์บอร์ด (1–0, Q–P …) · เพิ่มเสียงของคุณเองได้ เก็บไว้ในเบราว์เซอร์ · อัดทั้ง session เป็นไฟล์</p>
	</header>

	<section class="panel screws flex flex-wrap items-center gap-4 p-5 pt-7">
		<div class="w-44"><Fader label="Volume" bind:value={volume} min={-30} max={6} step={0.5} neutral={0} format={(v) => `${v.toFixed(1)} dB`} /></div>
		<Switch label="Random pitch" bind:checked={randomPitch} />
		<Switch label="Choke (ตัดเสียงก่อนหน้า)" bind:checked={choke} />
		<button class="btn" onclick={toggleRecord} aria-pressed={recording}>
			<span class="led" class:off-red={recording}></span>
			{recording ? `■ หยุด ${formatTime(recElapsed, false)}` : '● อัด session'}
		</button>
		<div class="min-w-48 flex-1"><LevelMeter {analyser} /></div>
	</section>

	<section class="grid grid-cols-3 gap-3 sm:grid-cols-4 lg:grid-cols-6">
		{#each pads as pad, i (pad.id)}
			<div class="group relative">
				<button
					class="pad panel flex aspect-square w-full flex-col items-center justify-center gap-2 p-3 select-none"
					style="--pad:{pad.color}"
					onpointerdown={() => trigger(pad)}
				>
					{#key hits[pad.id]}
						<span class="pad-flash" class:go={!!hits[pad.id]}></span>
					{/key}
					<span class="text-4xl drop-shadow">{pad.emoji}</span>
					<span class="line-clamp-2 text-center font-display text-sm font-bold">{pad.label}</span>
					{#if KEYS[i]}<kbd class="absolute top-2 right-2 font-mono text-[10px] text-muted uppercase">{KEYS[i]}</kbd>{/if}
				</button>
				{#if pad.user}
					<button
						class="absolute top-1 left-1 hidden rounded bg-ink/80 px-1.5 text-xs text-muted group-hover:block hover:text-alarm"
						aria-label="ลบ {pad.label}"
						onclick={() => removePad(pad)}>×</button
					>
				{/if}
			</div>
		{/each}
	</section>

	<DropZone compact multiple onfiles={addFiles} title="+ เพิ่มเสียงของคุณ (ลากหลายไฟล์ได้)" />
</div>

<style>
	.pad {
		position: relative;
		overflow: hidden;
		border-bottom: 4px solid color-mix(in srgb, var(--pad) 55%, #000);
		transition: transform 0.05s;
	}
	.pad:active {
		transform: translateY(3px);
		border-bottom-width: 1px;
	}
	.pad::after {
		content: '';
		position: absolute;
		inset: auto 0 0 0;
		height: 3px;
		background: var(--pad);
		opacity: 0.7;
	}
	.pad-flash {
		position: absolute;
		inset: 0;
		background: radial-gradient(circle, color-mix(in srgb, var(--pad) 55%, transparent), transparent 70%);
		opacity: 0;
		pointer-events: none;
	}
	.pad-flash.go {
		animation: flash 0.45s ease-out;
	}
	@keyframes flash {
		from {
			opacity: 1;
			transform: scale(0.6);
		}
		to {
			opacity: 0;
			transform: scale(1.4);
		}
	}
</style>
