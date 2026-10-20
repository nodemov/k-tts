<script lang="ts">
	interface Props {
		analyser: AnalyserNode | null;
		segments?: number;
	}

	let { analyser, segments = 32 }: Props = $props();

	let level = $state(-60);
	let peak = $state(-60);

	$effect(() => {
		if (!analyser) return;
		const data = new Float32Array(analyser.fftSize);
		let raf = 0;
		let lv = -60;
		let pkHeld = -60;
		let heldAt = 0;
		const loop = () => {
			analyser.getFloatTimeDomainData(data);
			let sum = 0;
			let max = 0;
			for (const v of data) {
				sum += v * v;
				max = Math.max(max, Math.abs(v));
			}
			const rms = 20 * Math.log10(Math.sqrt(sum / data.length) + 1e-9);
			const pk = 20 * Math.log10(max + 1e-9);
			lv = Math.max(rms, lv - 1.2);
			if (pk > pkHeld || performance.now() - heldAt > 1200) {
				pkHeld = pk;
				heldAt = performance.now();
			}
			level = lv;
			peak = pkHeld;
			raf = requestAnimationFrame(loop);
		};
		raf = requestAnimationFrame(loop);
		return () => cancelAnimationFrame(raf);
	});

	const toRatio = (db: number) => Math.max(0, Math.min(1, (db + 60) / 60));
	const lit = $derived(Math.round(toRatio(level) * segments));
	const peakSeg = $derived(Math.min(segments - 1, Math.round(toRatio(peak) * segments)));
</script>

<div class="flex items-center gap-3">
	<div class="flex h-3 flex-1 gap-[2px]" aria-label="level meter">
		{#each { length: segments } as _, i (i)}
			{@const zone = i / segments}
			<span
				class="h-full flex-1 rounded-[1px] transition-opacity duration-75"
				style="background:{zone > 0.9 ? 'var(--color-alarm)' : zone > 0.75 ? 'var(--color-signal)' : 'var(--color-phosphor)'};opacity:{i <
					lit || i === peakSeg
					? 1
					: 0.12}"
			></span>
		{/each}
	</div>
	<span class="readout w-16 text-right text-xs">{peak > -59 ? peak.toFixed(1) : '−∞'} dB</span>
</div>
