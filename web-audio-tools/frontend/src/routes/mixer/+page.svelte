<script lang="ts">
	import { onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { peakLevel } from '#lib/audio/buffer.ts';
	import { dbToGain, decodeFile, formatTime, getAudioContext, resumeAudio } from '#lib/audio/context.ts';
	import { baseName } from '#lib/audio/export.ts';
	import { limitPeaks } from '#lib/audio/limiter.ts';
	import DropZone from '#lib/components/DropZone.svelte';
	import ExportPanel from '#lib/components/ExportPanel.svelte';
	import Fader from '#lib/components/Fader.svelte';
	import LevelMeter from '#lib/components/LevelMeter.svelte';
	import WavePreview from '#lib/components/WavePreview.svelte';
	import { session, setSessionAudio } from '#lib/session.svelte.ts';

	interface Track {
		id: string;
		name: string;
		buffer: AudioBuffer;
		gain: number;
		pan: number;
		offset: number;
		mute: boolean;
		solo: boolean;
		color: string;
	}

	const COLORS = ['#ff6a1a', '#5cc8ff', '#b8ff5c', '#ff5cc8', '#ffd23f', '#9d7bff'];

	let tracks = $state<Track[]>([]);
	let pxPerSec = $state(40);
	let playhead = $state(0);
	let playing = $state(false);
	let loading = $state(false);
	let master = $state(0);

	const ctx = getAudioContext();
	const masterGain = new GainNode(ctx);
	const analyser = new AnalyserNode(ctx, { fftSize: 2048 });
	masterGain.connect(analyser).connect(ctx.destination);
	let live = new Map<string, { src: AudioBufferSourceNode; gain: GainNode; pan: StereoPannerNode }>();
	let startedAt = 0;
	let startFrom = 0;
	let raf = 0;

	const total = $derived(Math.max(1, ...tracks.map((t) => t.offset + t.buffer.duration)));
	const anySolo = $derived(tracks.some((t) => t.solo));
	const audible = (t: Track) => !t.mute && (!anySolo || t.solo);

	$effect(() => {
		masterGain.gain.setTargetAtTime(dbToGain(master), ctx.currentTime, 0.02);
		for (const t of tracks) {
			const n = live.get(t.id);
			if (!n) continue;
			n.gain.gain.setTargetAtTime(audible(t) ? dbToGain(t.gain) : 0, ctx.currentTime, 0.02);
			n.pan.pan.setTargetAtTime(t.pan, ctx.currentTime, 0.02);
		}
	});

	onDestroy(() => {
		stop();
		masterGain.disconnect();
	});

	function addBuffer(buffer: AudioBuffer, name: string) {
		tracks.push({
			id: crypto.randomUUID(),
			name,
			buffer,
			gain: 0,
			pan: 0,
			offset: 0,
			mute: false,
			solo: false,
			color: COLORS[tracks.length % COLORS.length]
		});
	}

	async function addFiles(files: File[]) {
		loading = true;
		try {
			for (const f of files) addBuffer(await decodeFile(f), f.name);
		} finally {
			loading = false;
		}
	}

	function scheduleGraph(target: BaseAudioContext, dest: AudioNode, from: number, when: number) {
		const nodes = new Map<string, { src: AudioBufferSourceNode; gain: GainNode; pan: StereoPannerNode }>();
		for (const t of tracks) {
			const inBuffer = Math.max(0, from - t.offset);
			if (inBuffer >= t.buffer.duration) continue;
			const src = new AudioBufferSourceNode(target, { buffer: t.buffer });
			const gain = new GainNode(target, { gain: audible(t) ? dbToGain(t.gain) : 0 });
			const pan = new StereoPannerNode(target, { pan: t.pan });
			src.connect(gain).connect(pan).connect(dest);
			src.start(when + Math.max(0, t.offset - from), inBuffer);
			nodes.set(t.id, { src, gain, pan });
		}
		return nodes;
	}

	async function play() {
		await resumeAudio();
		stopSources();
		const from = playhead >= total - 0.01 ? 0 : playhead;
		const when = ctx.currentTime + 0.03;
		live = scheduleGraph(ctx, masterGain, from, when);
		startedAt = when;
		startFrom = from;
		playing = true;
		const loop = () => {
			playhead = startFrom + Math.max(0, ctx.currentTime - startedAt);
			if (playhead >= total) {
				stop();
				playhead = 0;
				return;
			}
			raf = requestAnimationFrame(loop);
		};
		loop();
	}

	function stopSources() {
		cancelAnimationFrame(raf);
		for (const n of live.values()) {
			try {
				n.src.stop();
			} catch {
				/* not started */
			}
			n.src.disconnect();
			n.pan.disconnect();
		}
		live = new Map();
	}

	function stop() {
		stopSources();
		playing = false;
	}

	function seek(t: number) {
		playhead = Math.max(0, Math.min(total, t));
		if (playing) void play();
	}

	async function mixdown(): Promise<AudioBuffer> {
		const sr = Math.max(...tracks.map((t) => t.buffer.sampleRate));
		const offline = new OfflineAudioContext(2, Math.ceil(total * sr), sr);
		const out = new GainNode(offline, { gain: dbToGain(master) });
		out.connect(offline.destination);
		scheduleGraph(offline, out, 0, 0);
		const rendered = await offline.startRendering();
		return peakLevel(rendered) > 0.99 ? limitPeaks(rendered, -1) : rendered;
	}

	function startDrag(e: PointerEvent, track: Track) {
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		const startX = e.clientX;
		const startOffset = track.offset;
		const move = (ev: PointerEvent) => {
			track.offset = Math.max(0, startOffset + (ev.clientX - startX) / pxPerSec);
		};
		const up = () => {
			el.removeEventListener('pointermove', move);
			el.removeEventListener('pointerup', up);
			if (playing) void play();
		};
		el.addEventListener('pointermove', move);
		el.addEventListener('pointerup', up);
	}

	function onKey(e: KeyboardEvent) {
		if (['INPUT', 'SELECT', 'TEXTAREA'].includes((e.target as HTMLElement).tagName)) return;
		if (e.key === ' ' && tracks.length) {
			e.preventDefault();
			if (playing) stop();
			else void play();
		}
	}

	const rulerMarks = $derived(Array.from({ length: Math.ceil(total) + 1 }, (_, i) => i).filter((s) => s % (pxPerSec < 25 ? 10 : pxPerSec < 60 ? 5 : 1) === 0));
</script>

<svelte:window onkeydown={onKey} />

<div class="stagger mx-auto max-w-7xl space-y-5">
	<header class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<p class="label text-cobalt!">04 — Mixer</p>
			<h1 class="page-title text-4xl sm:text-5xl">มิกซ์หลายแทร็ก</h1>
			<p class="mt-2 text-muted">ลากคลิปเลื่อนตำแหน่งบน timeline · ปรับ gain/pan · mute/solo · mixdown เป็นไฟล์เดียว</p>
		</div>
		{#if session.buffer}
			<button class="btn" onclick={() => session.buffer && addBuffer(session.buffer, session.name)}>+ เพิ่มคลิปปัจจุบัน</button>
		{/if}
	</header>

	{#if tracks.length}
		<section class="panel screws overflow-hidden pt-5">
			<div class="flex flex-wrap items-center gap-3 border-b border-line px-5 pb-3">
				<button class="btn btn-signal w-24" onclick={() => (playing ? stop() : play())}>{playing ? '❚❚ Stop' : '▶ Play'}</button>
				<button class="btn" onclick={() => seek(0)}>⏮</button>
				<span class="readout text-sm">{formatTime(playhead)} / {formatTime(total)}</span>
				<div class="min-w-40 flex-1"><LevelMeter {analyser} /></div>
				<label class="flex items-center gap-2">
					<span class="label">Zoom</span>
					<input type="range" class="fader w-28!" min="10" max="200" bind:value={pxPerSec} style="--fill:{((pxPerSec - 10) / 190) * 100}%" />
				</label>
				<div class="w-40"><Fader label="Master" bind:value={master} min={-24} max={6} step={0.5} neutral={0} format={(v) => `${v.toFixed(1)} dB`} /></div>
			</div>

			<div class="flex">
				<div class="w-60 shrink-0 border-r border-line">
					<div class="h-6 border-b border-line"></div>
					{#each tracks as t, i (t.id)}
						<div class="flex h-24 flex-col justify-center gap-1.5 border-b border-line px-3" style="box-shadow: inset 3px 0 0 {t.color}">
							<div class="flex items-center gap-1">
								<span class="flex-1 truncate text-sm font-bold" title={t.name}>{t.name}</span>
								<button class="btn btn-sm px-1.5!" aria-pressed={t.mute} onclick={() => (t.mute = !t.mute)}>M</button>
								<button class="btn btn-sm px-1.5!" aria-pressed={t.solo} onclick={() => (t.solo = !t.solo)}>S</button>
								<button class="btn btn-sm btn-ghost px-1.5!" aria-label="ลบแทร็ก" onclick={() => tracks.splice(i, 1)}>×</button>
							</div>
							<div class="grid grid-cols-2 gap-2">
								<Fader label="Gain" bind:value={t.gain} min={-30} max={12} step={0.5} neutral={0} format={(v) => v.toFixed(1)} />
								<Fader label="Pan" bind:value={t.pan} min={-1} max={1} step={0.05} neutral={0} format={(v) => (v === 0 ? 'C' : v < 0 ? `L${Math.round(-v * 100)}` : `R${Math.round(v * 100)}`)} />
							</div>
						</div>
					{/each}
				</div>

				<div class="relative flex-1 overflow-x-auto bg-[#0d0c0a]">
					<div class="relative" style="width:{total * pxPerSec + 200}px">
						<button
							class="relative block h-6 w-full cursor-pointer border-b border-line"
							aria-label="seek"
							onpointerdown={(e) => seek((e.clientX - e.currentTarget.getBoundingClientRect().left) / pxPerSec)}
						>
							{#each rulerMarks as s (s)}
								<span class="absolute top-1 font-mono text-[10px] text-muted" style="left:{s * pxPerSec + 3}px">{s}s</span>
								<span class="absolute bottom-0 h-2 w-px bg-line" style="left:{s * pxPerSec}px"></span>
							{/each}
						</button>
						{#each tracks as t (t.id)}
							<div class="relative h-24 border-b border-line">
								<div
									role="slider"
									tabindex="0"
									aria-label="ตำแหน่ง {t.name}"
									aria-valuenow={t.offset}
									class="absolute top-2 bottom-2 cursor-grab overflow-hidden rounded border active:cursor-grabbing"
									class:opacity-40={!audible(t)}
									style="left:{t.offset * pxPerSec}px;width:{t.buffer.duration * pxPerSec}px;border-color:{t.color};background:color-mix(in srgb, {t.color} 10%, transparent)"
									onpointerdown={(e) => startDrag(e, t)}
									onkeydown={(e) => {
										if (e.key === 'ArrowLeft') t.offset = Math.max(0, t.offset - 0.1);
										if (e.key === 'ArrowRight') t.offset += 0.1;
									}}
								>
									<WavePreview buffer={t.buffer} height={78} color={t.color} progressColor={t.color} />
								</div>
							</div>
						{/each}
						<div class="pointer-events-none absolute top-0 bottom-0 w-px bg-cream shadow-[0_0_8px_#efe6d2]" style="left:{playhead * pxPerSec}px"></div>
					</div>
				</div>
			</div>
		</section>

		<section class="panel space-y-3 p-5">
			<h2 class="label text-cream!">Mixdown</h2>
			<ExportPanel render={mixdown} filename={`mix-${baseName(tracks[0].name)}`} label="Export mix" />
			<button
				class="btn btn-sm btn-ghost"
				onclick={async () => {
					setSessionAudio(await mixdown(), 'mixdown');
					await goto('/editor');
				}}>→ เปิด mix ใน Editor</button
			>
		</section>
	{/if}

	<DropZone compact={tracks.length > 0} multiple onfiles={addFiles} title={loading ? 'กำลังโหลด…' : 'ลากหลายไฟล์มาวางเพื่อเพิ่มแทร็ก'} />
</div>
