<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { createJob, fileUrl, getHealth, uploadFile, watchJob, type Health, type Job, type JobFile, type JobKind } from '#lib/api.ts';
	import { decodeFile } from '#lib/audio/context.ts';
	import { baseName } from '#lib/audio/export.ts';
	import { encodeWav } from '#lib/audio/wav.ts';
	import DropZone from '#lib/components/DropZone.svelte';
	import Fader from '#lib/components/Fader.svelte';
	import { session, setSessionAudio } from '#lib/session.svelte.ts';

	type Tab = 'denoise' | 'separate' | 'transcribe' | 'tts' | 'server';
	const TABS: { id: Tab; label: string; th: string }[] = [
		{ id: 'denoise', label: 'AI Denoise', th: 'ลดเสียงรบกวนด้วย DeepFilterNet' },
		{ id: 'separate', label: 'Stem Split', th: 'แยกเสียงร้อง / ดนตรี / กลอง / เบส (Demucs)' },
		{ id: 'transcribe', label: 'Transcribe', th: 'ถอดเสียงเป็นข้อความ + SRT (Whisper)' },
		{ id: 'tts', label: 'Thai TTS', th: 'ข้อความ → เสียงพูดไทย / หนังสือเสียงทั้งเล่ม' },
		{ id: 'server', label: 'Server ffmpeg', th: 'enhance/convert ไฟล์ใหญ่บน server' }
	];

	let tab = $state<Tab>('denoise');
	let health = $state<Health | null>(null);
	let checked = $state(false);

	let file = $state.raw<Blob | null>(null);
	let fileName = $state('');
	let uploadProgress = $state(0);
	let job = $state<Job | null>(null);
	let running = $state(false);
	let error = $state('');

	let strength = $state(0.8);
	let stems = $state<'vocals' | '4stems'>('vocals');
	let language = $state('th');
	let model = $state('small');
	let ttsText = $state('');
	let chapters = $state<{ title: string; text: string }[]>([]);
	let speed = $state(1.2);
	let engine = $state<'auto' | 'cloud' | 'translate'>('auto');
	let loudness = $state(-16);
	let serverDenoise = $state(true);
	let serverFormat = $state('mp3');

	onMount(async () => {
		health = await getHealth();
		checked = true;
	});

	const needsFile = $derived(tab !== 'tts');

	function useSession() {
		if (!session.buffer) return;
		file = encodeWav(session.buffer, 16);
		fileName = `${baseName(session.name)}.wav`;
	}

	async function pickTextFiles(files: File[]) {
		const loaded = await Promise.all(files.map(async (f) => ({ title: baseName(f.name), text: await f.text() })));
		chapters = [...chapters, ...loaded].sort((a, b) => a.title.localeCompare(b.title, undefined, { numeric: true }));
	}

	function params(): Record<string, unknown> {
		switch (tab) {
			case 'denoise':
				return { strength };
			case 'separate':
				return { mode: stems };
			case 'transcribe':
				return { language: language === 'auto' ? null : language, model };
			case 'tts':
				return {
					chapters: chapters.length ? chapters : [{ title: 'speech', text: ttsText }],
					speed,
					engine
				};
			case 'server':
				return { loudness, denoise: serverDenoise, format: serverFormat };
		}
	}

	async function run() {
		error = '';
		job = null;
		running = true;
		uploadProgress = 0;
		try {
			const kind: JobKind = tab === 'server' ? 'enhance' : tab;
			let uploadId: string | undefined;
			if (needsFile) {
				if (!file) throw new Error('ยังไม่ได้เลือกไฟล์');
				uploadId = await uploadFile(file, fileName, (p) => (uploadProgress = p));
			}
			const created = await createJob(kind, params(), uploadId);
			job = created;
			if (created.status !== 'done' && created.status !== 'error') job = await watchJob(created.id, (j) => (job = j));
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			running = false;
		}
	}

	async function openInEditor(f: JobFile) {
		const blob = await (await fetch(fileUrl(f))).blob();
		setSessionAudio(await decodeFile(blob), f.name);
		await goto('/editor');
	}

	const isAudio = (f: JobFile) => /\.(mp3|wav|flac|ogg|m4a|opus)$/i.test(f.name);
	const transcript = $derived((job?.result?.text as string | undefined) ?? '');
	const modelOk = (name: string) => health?.ai.models[name] ?? false;
</script>

<div class="stagger mx-auto max-w-6xl space-y-5">
	<header class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<p class="label text-cobalt!">07 — AI Lab</p>
			<h1 class="page-title text-4xl sm:text-5xl">AI <span class="text-cobalt">แยกเสียง · ถอดความ · TTS</span></h1>
			<p class="mt-2 max-w-2xl text-muted">งานที่ต้องใช้โมเดลใหญ่จะส่งไปประมวลผลที่ Go backend + Python AI service · ผลลัพธ์ถูก cache ด้วย hash ของไฟล์</p>
		</div>
		<div class="panel flex flex-wrap gap-x-4 gap-y-1 px-4 py-3 font-mono text-[11px]">
			{#if !checked}
				<span class="text-muted">checking server…</span>
			{:else if !health}
				<span class="flex items-center gap-1.5"><span class="led off-red"></span>backend offline</span>
			{:else}
				<span class="flex items-center gap-1.5"><span class="led on"></span>go api</span>
				<span class="flex items-center gap-1.5"><span class="led" class:on={health.ffmpeg}></span>ffmpeg</span>
				<span class="flex items-center gap-1.5"><span class="led" class:on={health.cloudTTS} class:warn={!health.cloudTTS}></span>cloud tts</span>
				<span class="flex items-center gap-1.5"><span class="led" class:on={health.ai.reachable}></span>ai service</span>
				{#each Object.entries(health.ai.models) as [name, ok] (name)}
					<span class="flex items-center gap-1.5"><span class="led" class:on={ok}></span>{name}</span>
				{/each}
			{/if}
		</div>
	</header>

	{#if checked && !health}
		<div class="panel border-signal/60 p-5 text-sm">
			<p class="font-display font-bold text-signal">ยังไม่ได้เปิด backend</p>
			<p class="mt-1 text-muted">รัน <code class="font-mono text-cream">go run ./web-audio-tools/backend</code> จากโฟลเดอร์ k-tts (และ AI service ถ้าต้องการ) แล้วรีเฟรชหน้านี้ — เครื่องมืออื่นทุกหน้าใช้งานได้โดยไม่ต้องมี server</p>
		</div>
	{/if}

	<nav class="grid gap-2 sm:grid-cols-5">
		{#each TABS as t (t.id)}
			<button class="panel p-3 text-left transition-colors" class:border-cobalt={tab === t.id} onclick={() => ((tab = t.id), (job = null), (error = ''))}>
				<span class="font-display text-sm font-bold" class:text-cobalt={tab === t.id}>{t.label}</span>
				<span class="mt-0.5 block text-[11px] leading-snug text-muted">{t.th}</span>
			</button>
		{/each}
	</nav>

	<section class="panel screws space-y-5 p-5 pt-7">
		{#if needsFile}
			{#if file}
				<div class="flex items-center justify-between gap-3">
					<p class="truncate font-display font-bold">{fileName}</p>
					<button class="btn btn-sm btn-ghost" onclick={() => (file = null)}>เปลี่ยนไฟล์</button>
				</div>
			{:else}
				<DropZone
					compact
					onfiles={(fs) => ((file = fs[0]), (fileName = fs[0].name))}
					onsession={useSession}
					title="เลือกไฟล์เสียงที่จะส่งไปประมวลผล"
				/>
			{/if}
		{/if}

		{#if tab === 'denoise'}
			<div class="max-w-sm"><Fader label="Strength" bind:value={strength} min={0.1} max={1} step={0.05} format={(v) => `${Math.round(v * 100)}%`} /></div>
			{#if health && !modelOk('deepfilternet')}<p class="text-xs text-signal">DeepFilterNet ยังไม่ได้ติดตั้ง — server จะใช้ ffmpeg afftdn แทน</p>{/if}
		{:else if tab === 'separate'}
			<div class="flex gap-2">
				<button class="btn" aria-pressed={stems === 'vocals'} onclick={() => (stems = 'vocals')}>เสียงร้อง + ดนตรี (karaoke)</button>
				<button class="btn" aria-pressed={stems === '4stems'} onclick={() => (stems = '4stems')}>4 stems: vocals/drums/bass/other</button>
			</div>
			{#if health && !modelOk('demucs')}<p class="text-xs text-signal">Demucs ยังไม่ได้ติดตั้ง — โหมด karaoke จะใช้ phase cancellation (คุณภาพต่ำกว่า), 4 stems ใช้ไม่ได้</p>{/if}
		{:else if tab === 'transcribe'}
			<div class="flex flex-wrap gap-4">
				<label class="space-y-1">
					<span class="label">Language</span>
					<select class="field w-40!" bind:value={language}>
						<option value="th">ไทย</option>
						<option value="en">English</option>
						<option value="auto">Auto-detect</option>
					</select>
				</label>
				<label class="space-y-1">
					<span class="label">Model</span>
					<select class="field w-40!" bind:value={model}>
						{#each ['tiny', 'base', 'small', 'medium', 'large-v3'] as m (m)}<option value={m}>{m}</option>{/each}
					</select>
				</label>
			</div>
			{#if health && !modelOk('whisper')}<p class="text-xs text-alarm">ยังไม่ได้ติดตั้ง faster-whisper บน AI service</p>{/if}
		{:else if tab === 'tts'}
			<div class="grid gap-4 lg:grid-cols-[1fr_280px]">
				<div class="space-y-3">
					{#if chapters.length}
						<div class="max-h-64 space-y-1 overflow-y-auto">
							{#each chapters as ch, i (ch.title + i)}
								<div class="flex items-center gap-2 rounded border border-line px-3 py-1.5 text-sm">
									<span class="font-mono text-xs text-muted">{String(i + 1).padStart(3, '0')}</span>
									<span class="flex-1 truncate">{ch.title}</span>
									<span class="font-mono text-xs text-muted">{ch.text.length.toLocaleString()} ตัวอักษร</span>
									<button class="text-muted hover:text-alarm" aria-label="ลบ" onclick={() => chapters.splice(i, 1)}>×</button>
								</div>
							{/each}
						</div>
						<button class="btn btn-sm btn-ghost" onclick={() => (chapters = [])}>ล้างรายการ</button>
					{:else}
						<textarea class="field min-h-48 font-sans leading-relaxed" bind:value={ttsText} placeholder="พิมพ์หรือวางข้อความภาษาไทยที่นี่…"></textarea>
					{/if}
					<DropZone
						compact
						multiple
						accept=".txt,text/plain"
						onfiles={pickTextFiles}
						title="หรือวางไฟล์ .txt หลายบท → หนังสือเสียง (1 ไฟล์ต่อบท)"
					/>
				</div>
				<div class="space-y-4">
					<Fader label="Speed" bind:value={speed} min={0.5} max={2.5} step={0.05} neutral={1} format={(v) => `${v.toFixed(2)}×`} />
					<label class="block space-y-1">
						<span class="label">Engine</span>
						<select class="field" bind:value={engine}>
							<option value="auto">Auto (Cloud → Translate fallback)</option>
							<option value="cloud">Google Cloud TTS (Neural2)</option>
							<option value="translate">Google Translate TTS</option>
						</select>
					</label>
					<p class="text-xs text-muted">ใช้ pipeline เดียวกับ k-tts: ทำความสะอาดข้อความ แบ่งประโยคภาษาไทย รวมไฟล์ ปรับความเร็ว และ enhance ด้วย ffmpeg</p>
				</div>
			</div>
		{:else if tab === 'server'}
			<div class="flex flex-wrap items-end gap-4">
				<label class="space-y-1">
					<span class="label">Loudness</span>
					<select class="field w-44!" bind:value={loudness}>
						{#each [-14, -16, -19, -23] as l (l)}<option value={l}>{l} LUFS</option>{/each}
					</select>
				</label>
				<label class="space-y-1">
					<span class="label">Output</span>
					<select class="field w-32!" bind:value={serverFormat}>
						{#each ['mp3', 'wav', 'flac', 'm4a', 'ogg'] as f (f)}<option value={f}>{f}</option>{/each}
					</select>
				</label>
				<button class="btn" aria-pressed={serverDenoise} onclick={() => (serverDenoise = !serverDenoise)}>
					<span class="led" class:on={serverDenoise}></span>ffmpeg denoise
				</button>
			</div>
			<p class="text-xs text-muted">ใช้ ffmpeg บน server (highpass → afftdn → compressor → loudnorm 2-pass) เหมาะกับไฟล์ยาวหลายชั่วโมงที่เปิดในเบราว์เซอร์ไม่ไหว</p>
		{/if}

		<div class="flex flex-wrap items-center gap-4 border-t border-line pt-4">
			<button
				class="btn btn-signal"
				onclick={run}
				disabled={running || !health || (needsFile && !file) || (tab === 'tts' && !chapters.length && !ttsText.trim())}
			>
				{running ? 'กำลังทำงาน…' : '▶ เริ่มประมวลผล'}
			</button>
			{#if running && needsFile && uploadProgress < 1}
				<span class="font-mono text-xs text-muted">upload {Math.round(uploadProgress * 100)}%</span>
			{/if}
			{#if job}
				<div class="min-w-56 flex-1 space-y-1">
					<div class="flex justify-between font-mono text-xs">
						<span class:text-alarm={job.status === 'error'} class:text-phosphor={job.status === 'done'}>
							{job.status}{job.cached ? ' · cached' : ''} — {job.message}
						</span>
						<span>{Math.round(job.progress * 100)}%</span>
					</div>
					<div class="h-1 overflow-hidden rounded bg-ink">
						<div class="h-full bg-cobalt transition-all" style="width:{job.progress * 100}%"></div>
					</div>
				</div>
			{/if}
		</div>
		{#if error}<p class="text-sm text-alarm">{error}</p>{/if}
	</section>

	{#if job?.status === 'done'}
		<section class="panel space-y-3 p-5">
			<h2 class="label text-cream!">Results</h2>
			{#if transcript}
				<pre class="max-h-72 overflow-y-auto rounded bg-ink p-4 font-sans text-sm leading-relaxed whitespace-pre-wrap">{transcript}</pre>
			{/if}
			<div class="grid gap-2 md:grid-cols-2">
				{#each job.files as f (f.url)}
					<div class="flex flex-col gap-2 rounded border border-line p-3">
						<div class="flex items-center justify-between gap-2">
							<span class="truncate font-mono text-sm">{f.name}</span>
							<span class="font-mono text-xs text-muted">{(f.size / 1024 / 1024).toFixed(2)} MB</span>
						</div>
						{#if isAudio(f)}<audio controls preload="none" src={fileUrl(f)} class="w-full"></audio>{/if}
						<div class="flex gap-2">
							<a class="btn btn-sm" href={fileUrl(f)} download={f.name}>⤓ Download</a>
							{#if isAudio(f)}<button class="btn btn-sm btn-ghost" onclick={() => openInEditor(f)}>→ Editor</button>{/if}
						</div>
					</div>
				{/each}
			</div>
		</section>
	{/if}
</div>
