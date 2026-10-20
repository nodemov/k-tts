<script lang="ts">
	interface Props {
		label: string;
		value: number;
		min: number;
		max: number;
		step?: number;
		unit?: string;
		format?: (v: number) => string;
		vertical?: boolean;
		disabled?: boolean;
		/** Double-click resets to this value. */
		neutral?: number;
		oninput?: (v: number) => void;
	}

	let {
		label,
		value = $bindable(),
		min,
		max,
		step = 0.01,
		unit = '',
		format,
		vertical = false,
		disabled = false,
		neutral,
		oninput
	}: Props = $props();

	const fill = $derived(((value - min) / (max - min)) * 100);
	const display = $derived(format ? format(value) : `${Number(value.toFixed(2))}${unit}`);

	function reset() {
		if (neutral === undefined) return;
		value = neutral;
		oninput?.(value);
	}
</script>

{#if vertical}
	<label class="flex flex-col items-center gap-1.5 select-none" title={neutral !== undefined ? 'ดับเบิลคลิกเพื่อรีเซ็ต' : undefined}>
		<span class="readout text-[10px]">{display}</span>
		<input
			type="range"
			class="fader vertical"
			{min}
			{max}
			{step}
			{disabled}
			bind:value
			style="--fill:{fill}%"
			oninput={() => oninput?.(value)}
			ondblclick={reset}
			aria-label={label}
		/>
		<span class="label text-[9px]! tracking-widest!">{label}</span>
	</label>
{:else}
	<label class="block select-none" title={neutral !== undefined ? 'ดับเบิลคลิกเพื่อรีเซ็ต' : undefined}>
		<span class="flex items-baseline justify-between gap-2">
			<span class="label">{label}</span>
			<span class="readout text-xs">{display}</span>
		</span>
		<input
			type="range"
			class="fader"
			{min}
			{max}
			{step}
			{disabled}
			bind:value
			style="--fill:{fill}%"
			oninput={() => oninput?.(value)}
			ondblclick={reset}
		/>
	</label>
{/if}
