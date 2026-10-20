<script lang="ts">
	import { onDestroy } from 'svelte';
	import { formatTime } from '#lib/audio/context.ts';
	import { FUN_PRESETS } from '#lib/audio/effects/presets.ts';
	import { defaultSettings } from '#lib/audio/effects/settings.ts';
	import { downloadBlob } from '#lib/audio/export.ts';
	import { LiveMic } from '#lib/audio/mic.svelte.ts';
	import LevelMeter from '#lib/components/LevelMeter.svelte';
	import ProcessorWorkspace from '#lib/components/ProcessorWorkspace.svelte';
	import { setSessionAudio } from '#lib/session.svelte.ts';

	let settings = $state(defaultSettings());
	let source = $state.raw<AudioBuffer | null>(null);
	let sourceName = $state('');
	const mic = new LiveMic();
	let micError = $state('');
	let lastTake = $state.raw<Blob | null>(null);

	$effect(() => {
		const s = $state.snapshot(settings);
		mic.update(s);
	});

	onDestroy(() => mic.stop());

	async function toggleMic() {
		micError = '';
		if (mic.active) return mic.stop();
		try {
			await mic.start($state.snapshot(settings));
		} catch (err) {
			micError = `เปิดไมค์ไม่ได้: ${err instanceof Error ? err.message : err}`;
		}
	}

	async function toggleRecord() {
		if (!mic.recording) return mic.startRecording();
		const take = await mic.stopRecording();
		if (!take) return;
		lastTake = take.blob;
		const name = `voice-fx-${new Date().toISOString().slice(11, 19).replace(/:/g, '')}`;
		source = take.buffer;
		sourceName = name;
		setSessionAudio(take.buffer, name);
	}
</script>

<div class="stagger mx-auto max-w-7xl space-y-5">
	<header>
		<p class="label text-[#ff5cc8]!">03 — Voice FX</p>
		<h1 class="page-title text-4xl sm:text-5xl">เปลี่ยนเสียง<span class="text-[#ff5cc8]">สนุก ๆ</span></h1>
		<p class="mt-2 max-w-2xl text-muted">
			เลือก preset หรือหมุนเองทุกปุ่ม — pitch/speed คุณภาพสูงด้วย WSOLA ใน WebAssembly, ใช้กับไฟล์ หรือพูดใส่ไมค์สด ๆ แล้วอัดเก็บไว้
		</p>
	</header>

	<section class="panel screws flex flex-wrap items-center gap-4 p-5 pt-7">
		<button class="btn" class:btn-signal={mic.active} onclick={toggleMic}>
			<span class="led" class:on={mic.active}></span>
			{mic.active ? 'ปิดไมค์' : '🎙 ไมค์สด (live)'}
		</button>
		{#if mic.active}
			<button class="btn" onclick={toggleRecord} aria-pressed={mic.recording}>
				<span class="led" class:off-red={mic.recording}></span>
				{mic.recording ? `■ หยุดอัด ${formatTime(mic.elapsed, false)}` : '● อัดเสียง'}
			</button>
			<button class="btn btn-sm" aria-pressed={mic.monitor} onclick={() => mic.setMonitor(!mic.monitor)}>
				🎧 Monitor {mic.monitor ? 'on' : 'off'}
			</button>
			<div class="min-w-48 flex-1"><LevelMeter analyser={mic.analyser} /></div>
		{/if}
		{#if lastTake}
			<button class="btn btn-sm btn-ghost" onclick={() => lastTake && downloadBlob(lastTake, `${sourceName}.webm`)}>⤓ ไฟล์อัดล่าสุด</button>
		{/if}
		<p class="w-full text-xs text-muted">
			ใส่หูฟังกันเสียงหอน · โหมดไมค์ใช้ pitch-shifter แบบ low-latency (AudioWorklet) · speed/reverse/ลดนอยส์ ใช้ได้กับไฟล์เท่านั้น
		</p>
		{#if micError}<p class="w-full text-sm text-alarm">{micError}</p>{/if}
	</section>

	<ProcessorWorkspace
		kind="fun"
		path="/fun"
		presets={FUN_PRESETS}
		presetStyle="pads"
		sections={['voice', 'space', 'clean', 'output']}
		bind:settings
		bind:source
		bind:sourceName
	/>
</div>
