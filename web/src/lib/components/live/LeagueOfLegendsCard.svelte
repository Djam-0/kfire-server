<script lang="ts">
	// One League game in progress. Deliberately poorer than the Rocket League
	// card, and that is not a gap to fill: the source is Riot's Spectator API,
	// polled by the server, which knows the champion and the start but never
	// the score or the KDA. Nothing here may be invented from the rest.
	import type { LolLiveMatch } from '$lib/lol';
	import { formatDuration } from '$lib/format';
	import { t } from '$lib/i18n';

	// `username` is optional for the same reason as on the Rocket League card:
	// the live store is keyed by user id and presence is what puts a name on
	// it, so a member missing from presence still gets his card.
	let { username, match }: { username?: string; match: LolLiveMatch } = $props();

	// Ticks once a minute so the elapsed time stays honest without a reload,
	// the same mechanism the game page already uses for its live list.
	let now = $state(Date.now());
	$effect(() => {
		const id = setInterval(() => (now = Date.now()), 60_000);
		return () => clearInterval(id);
	});
	let elapsed = $derived(
		Math.max(0, Math.floor((now - new Date(match.started_at).getTime()) / 1000))
	);
</script>

<article class="pd-card flex flex-col gap-2 p-3">
	<header class="flex items-center gap-2">
		<span class="font-display min-w-0 flex-1 truncate font-semibold text-[var(--color-text)]">
			{username ?? t('live.unknownMember')}
		</span>
		<span
			class="pd-cut-sm font-display shrink-0 bg-[var(--color-online)]/15 px-2 py-0.5 text-xs font-bold text-[var(--color-online)] uppercase italic"
		>
			{t('live.badge')}
		</span>
	</header>

	<div class="flex items-center justify-center gap-2">
		{#if match.champion_icon}
			<img
				src={match.champion_icon}
				alt=""
				width="40"
				height="40"
				loading="lazy"
				class="shrink-0 rounded-sm"
			/>
		{/if}
		<span class="font-display min-w-0 truncate text-lg font-bold text-[var(--color-text)]">
			{match.champion_name || t('live.unknownChampion')}
		</span>
	</div>

	<dl class="grid grid-cols-2 gap-1 border-t border-[var(--color-border)] pt-2 text-center">
		<div class="min-w-0">
			<dt class="truncate text-[10px] tracking-wide text-[var(--color-muted)] uppercase">
				{t('live.mode')}
			</dt>
			<dd class="font-display truncate text-sm font-bold text-[var(--color-text)] uppercase">
				{match.mode}
			</dd>
		</div>
		<div class="min-w-0">
			<dt class="truncate text-[10px] tracking-wide text-[var(--color-muted)] uppercase">
				{t('live.elapsed')}
			</dt>
			<dd class="font-display text-sm font-bold tabular-nums text-[var(--color-text)]">
				{formatDuration(elapsed)}
			</dd>
		</div>
	</dl>
</article>
