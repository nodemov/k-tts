<script lang="ts">
	import { computePeaks } from '#lib/audio/buffer.ts';
	import { formatTime } from '#lib/audio/context.ts';

	interface Props {
		buffer: AudioBuffer | null;
		position?: number;
		duration?: number;
		height?: number;
		color?: string;
		progressColor?: string;
		onseek?: (seconds: number) => void;
	}

	let {
		buffer,
		position = 0,
		duration,
		height = 96,
		color = '#5a5244',
		progressColor = '#ff6a1a',
		onseek
	}: Props = $props();

	let canvas: HTMLCanvasElement;
	let width = $state(0);
	let hover = $state<number | null>(null);
	const total = $derived(duration ?? buffer?.duration ?? 0);
	const dpr = typeof devicePixelRatio === 'number' ? devicePixelRatio : 1;

	const peaks = $derived(buffer && width > 0 ? computePeaks(buffer, Math.floor(width / 2)) : null);

	$effect(() => {
		if (!canvas || !width) return;
		const ctx = canvas.getContext('2d')!;
		canvas.width = width * dpr;
		canvas.height = height * dpr;
		ctx.scale(dpr, dpr);
		ctx.clearRect(0, 0, width, height);

		const mid = height / 2;
		ctx.fillStyle = '#ffffff08';
		ctx.fillRect(0, mid, width, 1);
		if (!peaks) return;

		const n = peaks.length / 2;
		const progressX = total > 0 ? (position / total) * width : 0;
		for (let i = 0; i < n; i++) {
			const x = i * 2;
			const min = peaks[i * 2];
			const max = peaks[i * 2 + 1];
			const top = mid - max * mid * 0.95;
			const h = Math.max(1, (max - min) * mid * 0.95);
			ctx.fillStyle = x < progressX ? progressColor : color;
			ctx.fillRect(x, top, 1.4, h);
		}
		ctx.fillStyle = '#efe6d2';
		ctx.fillRect(progressX, 0, 1, height);
	});

	function timeAt(e: PointerEvent): number {
		const rect = canvas.getBoundingClientRect();
		return Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width)) * total;
	}
</script>

<div class="relative w-full" bind:clientWidth={width} style="height:{height}px">
	<canvas
		bind:this={canvas}
		class="block h-full w-full rounded-sm"
		class:cursor-pointer={!!onseek}
		style="width:100%;height:{height}px"
		onpointerdown={(e) => onseek?.(timeAt(e))}
		onpointermove={(e) => (hover = timeAt(e))}
		onpointerleave={() => (hover = null)}
	></canvas>
	{#if hover !== null && onseek && total > 0}
		<span
			class="pointer-events-none absolute top-1 -translate-x-1/2 rounded bg-ink/90 px-1 font-mono text-[10px] text-cream"
			style="left:{(hover / total) * 100}%">{formatTime(hover)}</span
		>
	{/if}
</div>
