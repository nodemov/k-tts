<script lang="ts">
	import { EQ_BANDS, GATE_OFF, type EffectSettings } from '#lib/audio/effects/settings.ts';
	import { LOUDNESS_TARGETS } from '#lib/audio/loudness.ts';
	import Fader from './Fader.svelte';
	import Switch from './Switch.svelte';

	export type Section = 'clean' | 'eq' | 'dynamics' | 'voice' | 'space' | 'output';

	interface Props {
		settings: EffectSettings;
		sections: Section[];
		/** Hide controls that only work offline (e.g. for live mic). */
		live?: boolean;
	}

	let { settings = $bindable(), sections, live = false }: Props = $props();

	const has = (s: Section) => sections.includes(s);
	const hz = (v: number) => (v === 0 ? 'off' : v >= 1000 ? `${(v / 1000).toFixed(1)}k` : `${Math.round(v)}`);
	const db = (v: number) => `${v > 0 ? '+' : ''}${v.toFixed(1)} dB`;
	const pct = (v: number) => (v === 0 ? 'off' : `${Math.round(v * 100)}%`);
	const bandLabel = (f: number) => (f >= 1000 ? `${f / 1000}k` : `${f}`);

	const gateOn = $derived(settings.gate > GATE_OFF);
	const limiterOn = $derived(settings.limiter !== null);
	const loudnessOn = $derived(settings.loudness !== null);
</script>

<div class="grid gap-4 md:grid-cols-2">
	{#if has('clean')}
		<section class="panel screws space-y-3 p-4 pt-6">
			<h3 class="label text-cream!">Clean-up</h3>
			{#if !live}
				<Fader label="Noise reduction (WASM)" bind:value={settings.denoise} min={0} max={1} step={0.05} format={pct} neutral={0} />
			{/if}
			<Fader label="High-pass" bind:value={settings.highpass} min={0} max={400} step={5} format={hz} neutral={0} />
			<Fader label="Low-pass" bind:value={settings.lowpass} min={0} max={18000} step={100} format={hz} neutral={0} />
			<div class="flex flex-wrap items-center gap-2">
				<span class="label mr-1">Hum</span>
				{#each [0, 50, 60] as const as h (h)}
					<button class="btn btn-sm" aria-pressed={settings.hum === h} onclick={() => (settings.hum = h)}>
						{h === 0 ? 'off' : `${h} Hz`}
					</button>
				{/each}
			</div>
			<div class="flex items-end gap-3">
				<Switch
					label="Noise gate"
					checked={gateOn}
					onchange={(v) => (settings.gate = v ? -50 : GATE_OFF)}
				/>
				{#if gateOn}
					<div class="flex-1">
						<Fader label="Threshold" bind:value={settings.gate} min={-80} max={-20} step={1} unit=" dB" />
					</div>
				{/if}
			</div>
			<Fader label="De-esser" bind:value={settings.deesser} min={0} max={1} step={0.05} format={pct} neutral={0} />
		</section>
	{/if}

	{#if has('eq')}
		<section class="panel screws p-4 pt-6">
			<div class="mb-2 flex items-center justify-between">
				<h3 class="label text-cream!">10-band EQ</h3>
				<button class="btn btn-sm btn-ghost" onclick={() => (settings.eq = EQ_BANDS.map(() => 0))}>flat</button>
			</div>
			<div class="flex justify-between gap-1 overflow-x-auto pb-1">
				{#each EQ_BANDS as f, i (f)}
					<Fader vertical label={bandLabel(f)} bind:value={settings.eq[i]} min={-12} max={12} step={0.5} neutral={0} format={(v) => v.toFixed(1)} />
				{/each}
			</div>
		</section>
	{/if}

	{#if has('dynamics')}
		<section class="panel screws space-y-3 p-4 pt-6">
			<div class="flex items-center justify-between">
				<h3 class="label text-cream!">Compressor</h3>
				<Switch label={settings.compressor.enabled ? 'on' : 'off'} bind:checked={settings.compressor.enabled} />
			</div>
			<div class="grid grid-cols-2 gap-3" class:opacity-40={!settings.compressor.enabled}>
				<Fader label="Threshold" bind:value={settings.compressor.threshold} min={-50} max={0} step={1} unit=" dB" />
				<Fader label="Ratio" bind:value={settings.compressor.ratio} min={1} max={20} step={0.5} format={(v) => `${v}:1`} />
				<Fader label="Attack" bind:value={settings.compressor.attack} min={0.001} max={0.1} step={0.001} format={(v) => `${(v * 1000).toFixed(0)} ms`} />
				<Fader label="Release" bind:value={settings.compressor.release} min={0.02} max={1} step={0.01} format={(v) => `${(v * 1000).toFixed(0)} ms`} />
				<Fader label="Make-up" bind:value={settings.compressor.makeup} min={0} max={18} step={0.5} format={db} />
			</div>
		</section>
	{/if}

	{#if has('voice')}
		<section class="panel screws space-y-3 p-4 pt-6">
			<h3 class="label text-cream!">Voice</h3>
			<Fader label="Pitch" bind:value={settings.pitch} min={-12} max={12} step={0.1} neutral={0} format={(v) => `${v > 0 ? '+' : ''}${v.toFixed(1)} st`} />
			{#if !live}
				<Fader label="Speed (คงคีย์เดิม)" bind:value={settings.rate} min={0.5} max={2} step={0.05} neutral={1} format={(v) => `${v.toFixed(2)}×`} />
				<Switch label="ย้อนกลับ (reverse)" bind:checked={settings.reverse} />
			{/if}
			<Fader label="Drive / distortion" bind:value={settings.drive} min={0} max={1} step={0.05} format={pct} neutral={0} />
			<div class="grid grid-cols-2 gap-3">
				<Fader label="Ring mod" bind:value={settings.ringmod.mix} min={0} max={1} step={0.05} format={pct} neutral={0} />
				<Fader label="Ring freq" bind:value={settings.ringmod.freq} min={1} max={400} step={1} unit=" Hz" />
			</div>
		</section>
	{/if}

	{#if has('space')}
		<section class="panel screws space-y-3 p-4 pt-6">
			<h3 class="label text-cream!">Space</h3>
			<div class="grid grid-cols-3 gap-3">
				<Fader label="Echo" bind:value={settings.echo.mix} min={0} max={1} step={0.05} format={pct} neutral={0} />
				<Fader label="Time" bind:value={settings.echo.time} min={0.01} max={1.5} step={0.01} format={(v) => `${(v * 1000).toFixed(0)} ms`} />
				<Fader label="Feedback" bind:value={settings.echo.feedback} min={0} max={0.9} step={0.05} format={pct} />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<Fader label="Reverb" bind:value={settings.reverb.mix} min={0} max={1} step={0.05} format={pct} neutral={0} />
				<Fader label="Decay" bind:value={settings.reverb.decay} min={0.2} max={8} step={0.1} format={(v) => `${v.toFixed(1)} s`} />
			</div>
			<Fader label="8D rotation" bind:value={settings.eightD} min={0} max={0.5} step={0.01} neutral={0} format={(v) => (v === 0 ? 'off' : `${v.toFixed(2)} Hz`)} />
		</section>
	{/if}

	{#if has('output')}
		<section class="panel screws space-y-3 p-4 pt-6">
			<h3 class="label text-cream!">Output</h3>
			<Fader label="Gain" bind:value={settings.gain} min={-24} max={12} step={0.5} format={db} neutral={0} />
			<div class="flex items-end gap-3">
				<Switch label="Limiter" checked={limiterOn} onchange={(v) => (settings.limiter = v ? -1 : null)} />
				{#if settings.limiter !== null}
					<div class="flex-1">
						<Fader
							label="Ceiling"
							value={settings.limiter}
							oninput={(v) => (settings.limiter = v)}
							min={-6}
							max={0}
							step={0.1}
							unit=" dB"
						/>
					</div>
				{/if}
			</div>
			{#if !live}
				<div class="flex flex-wrap items-end gap-3">
					<Switch label="Loudness (LUFS)" checked={loudnessOn} onchange={(v) => (settings.loudness = v ? -16 : null)} />
					{#if settings.loudness !== null}
						<select class="field w-auto! flex-1" bind:value={settings.loudness}>
							{#each LOUDNESS_TARGETS as t (t.value)}
								<option value={t.value}>{t.label}</option>
							{/each}
						</select>
					{/if}
				</div>
				<Switch label="ตัดช่วงเงียบอัตโนมัติ" bind:checked={settings.trimSilence} />
				<p class="text-xs text-muted">Loudness และการตัดช่วงเงียบ ทำตอน export (ฟังพรีวิวยังไม่ถูกนำไปใช้)</p>
			{/if}
		</section>
	{/if}
</div>
