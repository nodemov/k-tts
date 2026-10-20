<script lang="ts">
	import { detectHum } from '#lib/audio/analysis.ts';
	import { ENHANCE_PRESETS } from '#lib/audio/effects/presets.ts';
	import { defaultSettings, withSettings } from '#lib/audio/effects/settings.ts';
	import ProcessorWorkspace from '#lib/components/ProcessorWorkspace.svelte';

	let settings = $state(defaultSettings());
	let source = $state.raw<AudioBuffer | null>(null);
	let sourceName = $state('');
	let detected = $state<string>('');
</script>

<div class="stagger mx-auto max-w-7xl space-y-5">
	<header>
		<p class="label text-phosphor!">02 — Enhance</p>
		<h1 class="page-title text-4xl sm:text-5xl">ปรับเสียงให้ดีขึ้น</h1>
		<p class="mt-2 max-w-2xl text-muted">
			ลดเสียงรบกวนด้วย Rust/WASM, ตัดเสียงฮัม, EQ, de-esser, compressor และปรับความดังตามมาตรฐาน LUFS — ฟังผลแบบ realtime แล้วเทียบ A/B ได้ทันที
		</p>
	</header>

	<ProcessorWorkspace
		kind="enhance"
		path="/enhance"
		presets={ENHANCE_PRESETS}
		sections={['clean', 'eq', 'dynamics', 'output']}
		bind:settings
		bind:source
		bind:sourceName
	>
		{#snippet header({ source, apply })}
			{#if source}
				<section class="panel relative flex flex-wrap items-center gap-5 overflow-hidden p-5">
					<div
						class="pointer-events-none absolute -top-24 -right-10 h-56 w-56 rounded-full bg-phosphor/10 blur-3xl"
						aria-hidden="true"
					></div>
					<button
						class="btn btn-signal px-6! py-4! text-base!"
						onclick={() => {
							const hum = detectHum(source);
							detected = hum ? `พบเสียงฮัม ${hum} Hz — ตัดให้แล้ว` : 'ไม่พบเสียงฮัม';
							apply(withSettings({ ...ENHANCE_PRESETS[0].patch, hum: hum ?? 0 }), ENHANCE_PRESETS[0].name);
						}}
					>
						✦ One-click Enhance
					</button>
					<div class="text-sm">
						<p class="font-display font-bold">วิเคราะห์ไฟล์ แล้วเลือกการตั้งค่าที่เหมาะให้อัตโนมัติ</p>
						<p class="text-muted">{detected || 'ลดนอยส์ · ตรวจเสียงฮัม · EQ เสียงพูด · compressor · −16 LUFS · ตัดช่วงเงียบ'}</p>
					</div>
				</section>
			{/if}
		{/snippet}
	</ProcessorWorkspace>
</div>
