<script lang="ts">
	// One League game in progress, from either of the two sources the server
	// publishes under this slug: Riot's Spectator API polled by the server,
	// which knows the champion, the mode and the start, or Riot's local API on
	// the member's own machine pushed by the client, which knows the level, the
	// KDA, the creeps and the gold. The card renders what it has and never
	// invents the rest.
	//
	// The rich fields are refreshed several times a second, so no figure may
	// decide the width of what holds it: the gold going from 9999 to 10000 must
	// leave every other line exactly where it was. Hence equal-fraction cells
	// and `tabular-nums` everywhere a number is shown.
	import type { LolLiveMatch } from '$lib/lol';
	import { formatClock, formatDuration } from '$lib/format';
	import { t } from '$lib/i18n';
	import GameIcon from '$lib/components/GameIcon.svelte';

	// `username` is optional for the same reason as on the Rocket League card:
	// the live store is keyed by user id and presence is what puts a name on
	// it, so a member missing from presence still gets his card.
	// `icon` is the game's icon, handed down by the page from presence. It is
	// optional for the same reason `username` is: a card must never wait on
	// presence to exist.
	let { username, match, icon }: { username?: string; match: LolLiveMatch; icon?: string } =
		$props();

	// The client pushes the in-game clock outright. Spectator only gives the
	// start, so the elapsed time has to be counted here, and it ticks once a
	// minute so it stays honest without a reload. The interval is only armed
	// for that source: the client's own figure is already fresher than anything
	// a timer could produce.
	let now = $state(Date.now());
	$effect(() => {
		if (match.game_time_seconds !== undefined || !match.started_at) return;
		const id = setInterval(() => (now = Date.now()), 60_000);
		return () => clearInterval(id);
	});

	let clock = $derived.by(() => {
		if (match.game_time_seconds !== undefined) return formatClock(match.game_time_seconds);
		if (!match.started_at) return '';
		return formatDuration(Math.max(0, Math.floor((now - new Date(match.started_at).getTime()) / 1000)));
	});

	let championLabel = $derived(match.champion || match.champion_name || t('live.unknownChampion'));

	// Two cells per row whichever source is talking, so the card keeps the same
	// footprint: a member whose client goes quiet mid-game falls back to
	// Spectator without the page reflowing around them.
	let stats = $derived.by(() => {
		const rows: { label: string; value: string }[] = [];
		if (match.kills !== undefined && match.deaths !== undefined && match.assists !== undefined) {
			rows.push({ label: t('live.lolKda'), value: `${match.kills} / ${match.deaths} / ${match.assists}` });
		}
		if (match.creep_score !== undefined) {
			rows.push({ label: t('live.lolCreeps'), value: String(match.creep_score) });
		}
		if (match.gold !== undefined) {
			rows.push({ label: t('live.lolGold'), value: String(match.gold) });
		}
		if (match.mode) {
			rows.push({ label: t('live.mode'), value: match.mode });
		}
		if (clock) {
			rows.push({ label: t('live.elapsed'), value: clock });
		}
		return rows;
	});
</script>

<article class="pd-card flex flex-col gap-2 p-3">
	<header class="flex items-center gap-2">
		<GameIcon url={icon} size={20} />
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
			{championLabel}
		</span>
		{#if match.level !== undefined}
			<!--
				A fixed box: the level goes from 9 to 10 in the middle of a game
				and the champion name next to it must not slide.
			-->
			<span
				class="pd-cut-sm font-display inline-block w-7 shrink-0 bg-[var(--color-border)] py-0.5 text-center text-xs font-bold tabular-nums text-[var(--color-text)]"
				title={t('live.lolLevel')}
			>
				{match.level}
			</span>
		{/if}
	</div>

	{#if stats.length > 0}
		<dl class="grid grid-cols-2 gap-1 border-t border-[var(--color-border)] pt-2 text-center">
			{#each stats as s (s.label)}
				<div class="min-w-0">
					<dt class="truncate text-[10px] tracking-wide text-[var(--color-muted)] uppercase">
						{s.label}
					</dt>
					<dd class="font-display truncate text-sm font-bold tabular-nums text-[var(--color-text)]">
						{s.value}
					</dd>
				</div>
			{/each}
		</dl>
	{/if}
</article>
