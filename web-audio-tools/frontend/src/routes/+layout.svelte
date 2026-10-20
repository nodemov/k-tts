<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { formatTime } from '#lib/audio/context.ts';
	import { NAV } from '#lib/nav.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const active = (href: string) => page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
</script>

<svelte:head>
	<link rel="icon" href="/icon.svg" type="image/svg+xml" />
	<title>Web Audio Tools</title>
</svelte:head>

<div class="flex min-h-dvh flex-col lg:flex-row">
	<aside
		class="sticky top-0 z-40 flex shrink-0 flex-col border-b border-line bg-ink/90 backdrop-blur lg:h-dvh lg:w-64 lg:border-r lg:border-b-0"
	>
		<a href="/" class="flex items-center gap-3 px-5 py-4 lg:py-6">
			<span class="flex h-8 items-end gap-[3px]" aria-hidden="true">
				{#each [0.5, 0.9, 0.35, 0.75, 0.6] as h, i (i)}
					<span class="vu-bar w-[5px] rounded-[1px] bg-signal" style="height:{h * 100}%;animation-delay:{i * 0.13}s"></span>
				{/each}
			</span>
			<span class="leading-none">
				<span class="block font-display text-xl font-bold tracking-tight">WEB·AUDIO</span>
				<span class="label text-[9px]!">tools — ทำงานในเบราว์เซอร์</span>
			</span>
		</a>

		<nav class="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-col lg:overflow-visible lg:pb-0">
			{#each NAV as item (item.href)}
				<a
					href={item.href}
					class="group flex shrink-0 items-center gap-3 rounded border border-transparent px-2 py-2 transition-colors hover:border-line hover:bg-panel"
					class:nav-active={active(item.href)}
					style="--accent:{item.accent}"
				>
					<span class="font-mono text-[10px] text-muted group-[.nav-active]:text-(--accent)">{item.code}</span>
					<span class="flex flex-col leading-tight">
						<span class="font-display text-sm font-bold">{item.label}</span>
						<span class="hidden text-[11px] text-muted lg:block">{item.th}</span>
					</span>
					<span class="led ml-auto hidden lg:inline-block" class:on={active(item.href)}></span>
				</a>
			{/each}
		</nav>

		<div class="mt-auto hidden border-t border-line p-4 lg:block">
			<p class="label mb-1">คลิปปัจจุบัน</p>
			{#if session.buffer}
				<p class="truncate text-sm" title={session.name}>{session.name}</p>
				<p class="readout text-xs">
					{formatTime(session.buffer.duration, false)} · {session.buffer.sampleRate / 1000}k · {session.buffer.numberOfChannels === 1
						? 'mono'
						: 'stereo'}
				</p>
			{:else}
				<p class="text-xs text-muted">ยังไม่ได้โหลดเสียง — เปิดไฟล์จากเครื่องมือไหนก็ได้ แล้วส่งต่อข้ามหน้าได้</p>
			{/if}
		</div>
	</aside>

	<main class="min-w-0 flex-1 px-4 py-6 sm:px-8 lg:py-10">
		{@render children()}
	</main>
</div>

<style>
	.nav-active {
		border-color: var(--color-line);
		background: linear-gradient(90deg, color-mix(in srgb, var(--accent) 14%, transparent), transparent 70%);
	}
	.vu-bar {
		animation: vu 1.1s ease-in-out infinite alternate;
		transform-origin: bottom;
	}
	@keyframes vu {
		0% {
			transform: scaleY(0.35);
		}
		60% {
			transform: scaleY(1);
		}
		100% {
			transform: scaleY(0.55);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.vu-bar {
			animation: none;
		}
	}
</style>
