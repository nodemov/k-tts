<script lang="ts">
	import { goto } from '$app/navigation';
	import { decodeFile } from '#lib/audio/context.ts';
	import DropZone from '#lib/components/DropZone.svelte';
	import { NAV } from '#lib/nav.ts';
	import { setSessionAudio } from '#lib/session.svelte.ts';

	let loading = $state(false);
	let error = $state('');

	async function open(files: File[]) {
		loading = true;
		error = '';
		try {
			setSessionAudio(await decodeFile(files[0]), files[0].name);
			await goto('/editor');
		} catch (err) {
			error = `เปิดไฟล์ไม่ได้: ${err instanceof Error ? err.message : err}`;
		} finally {
			loading = false;
		}
	}
</script>

<div class="stagger mx-auto max-w-6xl space-y-10">
	<header class="grid items-end gap-8 lg:grid-cols-[1.4fr_1fr]">
		<div>
			<p class="label mb-4 text-signal!">◉ REC — studio in a browser tab</p>
			<h1 class="page-title text-5xl sm:text-7xl">
				ตัดต่อ ปรับแต่ง<br />
				<span class="text-signal">เล่นกับเสียง</span><br />
				ได้ทันที
			</h1>
		</div>
		<p class="max-w-md text-lg leading-relaxed text-muted">
			ทุกอย่างประมวลผลบนเครื่องคุณด้วย Web Audio + Rust/WebAssembly — ไม่ต้องรออัปโหลด ไฟล์ไม่ออกจากเครื่อง ส่วนงาน AI หนัก ๆ
			ส่งไปที่ server ได้เมื่อต้องการ
		</p>
	</header>

	<DropZone onfiles={open} title={loading ? 'กำลังถอดรหัสเสียง…' : 'ลากไฟล์เสียงมาวางเพื่อเริ่ม'} />
	{#if error}<p class="text-alarm">{error}</p>{/if}

	<section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
		{#each NAV as item (item.href)}
			<a
				href={item.href}
				class="panel group relative overflow-hidden p-5 transition-transform hover:-translate-y-1"
				style="--accent:{item.accent}"
			>
				<span class="absolute inset-x-0 top-0 h-[3px] bg-(--accent) opacity-70 transition-opacity group-hover:opacity-100"></span>
				<span class="font-mono text-xs text-(--accent)">{item.code}</span>
				<h2 class="mt-3 font-display text-2xl font-bold">{item.label}</h2>
				<p class="text-sm text-cream/80">{item.th}</p>
				<p class="mt-3 text-xs leading-relaxed text-muted">{item.blurb}</p>
				<span class="absolute right-4 bottom-4 font-display text-xl text-muted transition-all group-hover:translate-x-1 group-hover:text-(--accent)">→</span>
			</a>
		{/each}
	</section>

	<footer class="flex flex-wrap gap-x-8 gap-y-2 border-t border-line pt-6 text-xs text-muted">
		<span><span class="text-cream">Front-end</span> SvelteKit · TypeScript · Web Audio · AudioWorklet</span>
		<span><span class="text-cream">DSP</span> Rust → WebAssembly (noise reduction, WSOLA)</span>
		<span><span class="text-cream">Codec</span> ffmpeg.wasm (lazy-loaded)</span>
		<span><span class="text-cream">Backend</span> Go + Python AI service</span>
	</footer>
</div>
