<script lang="ts">
	import { session } from '#lib/session.svelte.ts';

	interface Props {
		onfiles: (files: File[]) => void;
		onsession?: () => void;
		accept?: string;
		multiple?: boolean;
		compact?: boolean;
		title?: string;
		hint?: string;
	}

	let {
		onfiles,
		onsession,
		accept = 'audio/*,video/*,.mp3,.wav,.flac,.ogg,.m4a,.aac,.opus,.webm',
		multiple = false,
		compact = false,
		title = 'ลากไฟล์เสียงมาวางที่นี่',
		hint = 'MP3 · WAV · FLAC · OGG · M4A · วิดีโอ — ประมวลผลในเครื่องคุณ ไม่อัปโหลด'
	}: Props = $props();

	let over = $state(false);
	let input: HTMLInputElement;

	function take(list: FileList | null | undefined) {
		const files = Array.from(list ?? []);
		if (files.length) onfiles(multiple ? files : files.slice(0, 1));
	}
</script>

<div
	role="button"
	tabindex="0"
	class="panel group flex cursor-pointer flex-col items-center justify-center gap-2 border-dashed text-center transition-colors {compact
		? 'px-4 py-5'
		: 'px-6 py-14'}"
	class:border-signal={over}
	style={over ? 'background: rgb(255 106 26 / 0.06)' : ''}
	ondragover={(e) => {
		e.preventDefault();
		over = true;
	}}
	ondragleave={() => (over = false)}
	ondrop={(e) => {
		e.preventDefault();
		over = false;
		take(e.dataTransfer?.files);
	}}
	onclick={() => input.click()}
	onkeydown={(e) => {
		if (e.key !== 'Enter' && e.key !== ' ') return;
		e.preventDefault();
		e.stopPropagation();
		input.click();
	}}
>
	<svg viewBox="0 0 64 24" class="h-6 w-16 text-signal transition-transform group-hover:scale-110" aria-hidden="true">
		<path
			d="M0 12h6l3-8 4 16 4-14 3 10 3-6 3 4 3-9 4 14 3-11 3 7 3-3 3 2 3-5 3 6h8"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linejoin="round"
		/>
	</svg>
	<p class="font-display text-lg font-bold">{title}</p>
	{#if !compact}<p class="text-sm text-muted">{hint}</p>{/if}
	<span class="btn btn-sm mt-1">หรือเลือกไฟล์</span>
	{#if onsession && session.buffer}
		<button
			type="button"
			class="btn btn-sm btn-ghost mt-1 text-phosphor!"
			onclick={(e) => {
				e.stopPropagation();
				onsession();
			}}
		>
			↳ ใช้คลิปปัจจุบัน: {session.name}
		</button>
	{/if}
	<input
		bind:this={input}
		type="file"
		class="hidden"
		{accept}
		{multiple}
		onclick={(e) => e.stopPropagation()}
		onchange={(e) => {
			take(e.currentTarget.files);
			e.currentTarget.value = '';
		}}
	/>
</div>
