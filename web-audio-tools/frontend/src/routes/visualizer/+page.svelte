<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { decodeFile, formatTime, getAudioContext, resumeAudio } from '#lib/audio/context.ts';
	import { baseName, downloadBlob } from '#lib/audio/export.ts';
	import { pickRecorderMime } from '#lib/audio/mic.svelte.ts';
	import DropZone from '#lib/components/DropZone.svelte';
	import { session } from '#lib/session.svelte.ts';

	type Style = 'bars' | 'radial' | 'scope' | 'particles';
	const STYLES: { id: Style; label: string }[] = [
		{ id: 'bars', label: 'Spectrum bars' },
		{ id: 'radial', label: 'Radial pulse' },
		{ id: 'scope', label: 'Oscilloscope' },
		{ id: 'particles', label: 'Particles' }
	];
	const PALETTES: Record<string, [string, string, string]> = {
		ember: ['#12110e', '#ff6a1a', '#ffd23f'],
		neon: ['#07060f', '#5cc8ff', '#ff5cc8'],
		acid: ['#060b05', '#b8ff5c', '#efe6d2'],
		ink: ['#efe6d2', '#12110e', '#ff6a1a']
	};
	const SIZES = { '16:9': [1280, 720], '1:1': [1080, 1080], '9:16': [720, 1280] } as const;

	let canvas: HTMLCanvasElement;
	let style = $state<Style>('radial');
	let palette = $state('ember');
	let size = $state<keyof typeof SIZES>('16:9');
	let title = $state('');
	let source = $state.raw<AudioBuffer | null>(null);
	let sourceName = $state('');
	let mode = $state<'file' | 'mic'>('file');
	let playing = $state(false);
	let recording = $state(false);
	let position = $state(0);
	let error = $state('');

	const ctx = getAudioContext();
	const analyser = new AnalyserNode(ctx, { fftSize: 2048, smoothingTimeConstant: 0.8 });
	const out = new GainNode(ctx);
	const recordDest = new MediaStreamAudioDestinationNode(ctx);
	analyser.connect(out).connect(ctx.destination);
	analyser.connect(recordDest);

	let fileNode: AudioBufferSourceNode | null = null;
	let micStream: MediaStream | null = null;
	let micNode: MediaStreamAudioSourceNode | null = null;
	let recorder: MediaRecorder | null = null;
	let startedAt = 0;
	let raf = 0;

	const freq = new Uint8Array(analyser.frequencyBinCount);
	const wave = new Uint8Array(analyser.fftSize);
	let particles: { x: number; y: number; vx: number; vy: number; life: number; size: number }[] = [];
	let rotation = 0;

	onMount(() => {
		if (session.buffer) {
			source = session.buffer;
			sourceName = session.name;
			title = baseName(session.name);
		}
		draw();
	});

	onDestroy(() => {
		cancelAnimationFrame(raf);
		stopAll();
		analyser.disconnect();
		out.disconnect();
	});

	async function open(files: File[]) {
		try {
			stopAll();
			source = await decodeFile(files[0]);
			sourceName = files[0].name;
			title ||= baseName(files[0].name);
		} catch (err) {
			error = `เปิดไฟล์ไม่ได้: ${err instanceof Error ? err.message : err}`;
		}
	}

	async function playFile() {
		if (!source) return;
		await resumeAudio();
		stopAll();
		fileNode = new AudioBufferSourceNode(ctx, { buffer: source });
		fileNode.connect(analyser);
		fileNode.onended = () => {
			playing = false;
			if (recording) recorder?.stop();
		};
		fileNode.start();
		startedAt = ctx.currentTime;
		playing = true;
	}

	async function startMic() {
		await resumeAudio();
		stopAll();
		try {
			micStream = await navigator.mediaDevices.getUserMedia({ audio: true });
			micNode = new MediaStreamAudioSourceNode(ctx, { mediaStream: micStream });
			micNode.connect(analyser);
			out.gain.value = 0;
			playing = true;
		} catch (err) {
			error = `เปิดไมค์ไม่ได้: ${err instanceof Error ? err.message : err}`;
		}
	}

	function stopAll() {
		if (fileNode) {
			fileNode.onended = null;
			try {
				fileNode.stop();
			} catch {
				/* not started */
			}
			fileNode.disconnect();
			fileNode = null;
		}
		micStream?.getTracks().forEach((t) => t.stop());
		micNode?.disconnect();
		micStream = null;
		micNode = null;
		out.gain.value = 1;
		playing = false;
	}

	async function record() {
		if (recording) {
			recorder?.stop();
			return;
		}
		const mimeType = pickRecorderMime('video');
		const stream = new MediaStream([...canvas.captureStream(60).getVideoTracks(), ...recordDest.stream.getAudioTracks()]);
		const chunks: Blob[] = [];
		recorder = new MediaRecorder(stream, { mimeType: mimeType || undefined, videoBitsPerSecond: 8_000_000 });
		recorder.ondataavailable = (e) => e.data.size && chunks.push(e.data);
		recorder.onstop = () => {
			recording = false;
			const blob = new Blob(chunks, { type: recorder?.mimeType || 'video/webm' });
			downloadBlob(blob, `${baseName(title || 'visualizer')}.${blob.type.includes('mp4') ? 'mp4' : 'webm'}`);
			if (mode === 'file') stopAll();
		};
		recorder.start(500);
		recording = true;
		if (mode === 'file') await playFile();
		else if (!playing) await startMic();
	}

	function bandEnergy(from: number, to: number) {
		let sum = 0;
		for (let i = from; i < to; i++) sum += freq[i];
		return sum / ((to - from) * 255);
	}

	function draw() {
		raf = requestAnimationFrame(draw);
		const [w, h] = SIZES[size];
		if (canvas.width !== w || canvas.height !== h) {
			canvas.width = w;
			canvas.height = h;
		}
		const g = canvas.getContext('2d')!;
		const [bg, c1, c2] = PALETTES[palette];
		analyser.getByteFrequencyData(freq);
		analyser.getByteTimeDomainData(wave);
		if (playing && mode === 'file') position = ctx.currentTime - startedAt;
		const bass = bandEnergy(1, 12);
		const grad = g.createLinearGradient(0, h, w, 0);
		grad.addColorStop(0, c1);
		grad.addColorStop(1, c2);

		g.globalAlpha = style === 'scope' || style === 'particles' ? 0.28 : 1;
		g.fillStyle = bg;
		g.fillRect(0, 0, w, h);
		g.globalAlpha = 1;
		g.shadowBlur = 24;
		g.shadowColor = c1;

		const bins = 96;
		const logBin = (i: number) => Math.min(freq.length - 1, Math.floor(Math.pow(freq.length * 0.7, i / bins)));

		if (style === 'bars') {
			const bw = w / bins;
			g.fillStyle = grad;
			for (let i = 0; i < bins; i++) {
				const v = freq[logBin(i)] / 255;
				const bh = Math.pow(v, 1.4) * h * 0.42;
				g.fillRect(i * bw + 2, h / 2 - bh, bw - 4, bh * 2);
			}
		} else if (style === 'radial') {
			rotation += 0.002 + bass * 0.01;
			const cx = w / 2;
			const cy = h / 2;
			const r = Math.min(w, h) * (0.18 + bass * 0.06);
			g.strokeStyle = grad;
			g.lineWidth = Math.max(2, Math.min(w, h) / 220);
			g.lineCap = 'round';
			for (let i = 0; i < bins * 2; i++) {
				const v = freq[logBin(i % bins)] / 255;
				const a = rotation + (i / (bins * 2)) * Math.PI * 2;
				const len = r * 0.15 + Math.pow(v, 1.5) * r * 1.3;
				g.beginPath();
				g.moveTo(cx + Math.cos(a) * r, cy + Math.sin(a) * r);
				g.lineTo(cx + Math.cos(a) * (r + len), cy + Math.sin(a) * (r + len));
				g.stroke();
			}
			g.beginPath();
			g.arc(cx, cy, r * 0.9, 0, Math.PI * 2);
			g.fillStyle = c1;
			g.globalAlpha = 0.15 + bass * 0.4;
			g.fill();
			g.globalAlpha = 1;
		} else if (style === 'scope') {
			g.strokeStyle = grad;
			g.lineWidth = 3;
			g.beginPath();
			for (let i = 0; i < wave.length; i++) {
				const x = (i / (wave.length - 1)) * w;
				const y = h / 2 + ((wave[i] - 128) / 128) * h * 0.38;
				if (i === 0) g.moveTo(x, y);
				else g.lineTo(x, y);
			}
			g.stroke();
		} else {
			const cx = w / 2;
			const cy = h / 2;
			const spawn = Math.floor(bass * bass * 30);
			for (let i = 0; i < spawn; i++) {
				const a = Math.random() * Math.PI * 2;
				const speed = 2 + Math.random() * 8 * (0.5 + bass);
				particles.push({ x: cx, y: cy, vx: Math.cos(a) * speed, vy: Math.sin(a) * speed, life: 1, size: 1 + Math.random() * 4 });
			}
			particles = particles.filter((p) => p.life > 0 && p.x > -20 && p.x < w + 20 && p.y > -20 && p.y < h + 20);
			for (const p of particles) {
				p.x += p.vx;
				p.y += p.vy;
				p.vx *= 1.01;
				p.vy *= 1.01;
				p.life -= 0.008;
				g.fillStyle = p.life > 0.5 ? c2 : c1;
				g.globalAlpha = Math.max(0, p.life);
				g.beginPath();
				g.arc(p.x, p.y, p.size * (1 + bass), 0, Math.PI * 2);
				g.fill();
			}
			g.globalAlpha = 1;
		}

		g.shadowBlur = 0;
		if (title) {
			g.fillStyle = palette === 'ink' ? '#12110e' : '#efe6d2';
			g.font = `700 ${Math.round(Math.min(w, h) / 16)}px 'Chakra Petch', 'Anuphan', sans-serif`;
			g.textAlign = 'center';
			g.fillText(title, w / 2, h - Math.min(w, h) / 12);
		}
		if (recording) {
			g.fillStyle = '#ff3b52';
			g.beginPath();
			g.arc(36, 36, 10, 0, Math.PI * 2);
			g.fill();
		}
	}
</script>

<div class="stagger mx-auto max-w-7xl space-y-5">
	<header>
		<p class="label text-[#9d7bff]!">06 — Visualizer</p>
		<h1 class="page-title text-4xl sm:text-5xl">ภาพเคลื่อนไหว<span class="text-[#9d7bff]">จากเสียง</span></h1>
		<p class="mt-2 text-muted">เลือกสไตล์ ใส่ชื่อเพลง แล้วกดอัด — ได้วิดีโอพร้อมเสียง ลง Reels/TikTok/YouTube ได้ทันที</p>
	</header>

	<div class="grid gap-5 lg:grid-cols-[1fr_300px]">
		<div class="panel overflow-hidden p-2">
			<canvas bind:this={canvas} class="mx-auto block max-h-[70vh] w-full rounded object-contain" style="aspect-ratio:{SIZES[size][0]}/{SIZES[size][1]}"></canvas>
		</div>

		<aside class="panel screws space-y-4 p-5 pt-7">
			<div class="flex gap-1">
				<button class="btn btn-sm flex-1" aria-pressed={mode === 'file'} onclick={() => ((mode = 'file'), stopAll())}>ไฟล์</button>
				<button class="btn btn-sm flex-1" aria-pressed={mode === 'mic'} onclick={() => ((mode = 'mic'), stopAll())}>ไมค์</button>
			</div>

			{#if mode === 'file'}
				{#if source}
					<p class="truncate text-sm" title={sourceName}>{sourceName}</p>
					<p class="readout text-xs">{formatTime(Math.min(position, source.duration))} / {formatTime(source.duration)}</p>
					<button class="btn btn-signal w-full" onclick={() => (playing ? stopAll() : playFile())} disabled={recording}>
						{playing ? '■ Stop' : '▶ Play'}
					</button>
				{/if}
				<DropZone compact onfiles={open} title={source ? 'เปลี่ยนไฟล์' : 'เลือกไฟล์เสียง'} />
			{:else}
				<button class="btn btn-signal w-full" onclick={() => (playing ? stopAll() : startMic())}>
					{playing ? '■ ปิดไมค์' : '🎙 เปิดไมค์'}
				</button>
			{/if}

			<label class="block space-y-1">
				<span class="label">Style</span>
				<select class="field" bind:value={style}>
					{#each STYLES as s (s.id)}<option value={s.id}>{s.label}</option>{/each}
				</select>
			</label>
			<div class="space-y-1">
				<span class="label">Palette</span>
				<div class="flex gap-2">
					{#each Object.entries(PALETTES) as [id, [bg, a, b]] (id)}
						<button
							class="h-8 flex-1 rounded border-2"
							style="background:linear-gradient(135deg,{bg} 30%,{a} 30% 65%,{b} 65%);border-color:{palette === id ? '#efe6d2' : 'transparent'}"
							aria-label={id}
							onclick={() => (palette = id)}
						></button>
					{/each}
				</div>
			</div>
			<div class="space-y-1">
				<span class="label">Format</span>
				<div class="flex gap-1">
					{#each Object.keys(SIZES) as s (s)}
						<button class="btn btn-sm flex-1" aria-pressed={size === s} onclick={() => (size = s as keyof typeof SIZES)}>{s}</button>
					{/each}
				</div>
			</div>
			<label class="block space-y-1">
				<span class="label">Title</span>
				<input class="field" bind:value={title} placeholder="ชื่อเพลง / ข้อความ" />
			</label>
			<button class="btn w-full" onclick={record} disabled={mode === 'file' && !source} aria-pressed={recording}>
				<span class="led" class:off-red={recording}></span>
				{recording ? '■ หยุดอัดวิดีโอ' : '● อัดเป็นวิดีโอ'}
			</button>
			<p class="text-xs text-muted">โหมดไฟล์: อัดตั้งแต่ต้นจนจบเพลงอัตโนมัติ</p>
			{#if error}<p class="text-sm text-alarm">{error}</p>{/if}
		</aside>
	</div>
</div>
