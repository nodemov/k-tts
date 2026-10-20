<script lang="ts">
	import { formatTime } from '#lib/audio/context.ts';
	import type { Player } from '#lib/audio/player.svelte.ts';

	interface Props {
		player: Player;
		showBypass?: boolean;
		busy?: boolean;
	}

	let { player, showBypass = true, busy = false }: Props = $props();
</script>

<div class="flex flex-wrap items-center gap-2">
	<button class="btn btn-signal w-24" onclick={() => player.toggle()} disabled={!player.duration} title="Space">
		{player.playing ? '❚❚ Pause' : '▶ Play'}
	</button>
	<button class="btn" onclick={() => player.stop()} disabled={!player.duration} aria-label="Stop">■</button>
	{#if showBypass}
		<button
			class="btn"
			aria-pressed={player.bypass}
			onclick={() => player.setBypass(!player.bypass)}
			disabled={!player.duration}
			title="กด B เพื่อสลับฟังเสียงต้นฉบับ"
		>
			{player.bypass ? 'A · ต้นฉบับ' : 'B · ปรับแล้ว'}
		</button>
	{/if}
	<span class="readout ml-auto text-sm">
		{formatTime(player.position)} <span class="text-muted">/ {formatTime(player.duration)}</span>
	</span>
	{#if busy}<span class="flex items-center gap-1.5 text-xs text-signal"><span class="led warn animate-pulse"></span>DSP</span>{/if}
</div>
