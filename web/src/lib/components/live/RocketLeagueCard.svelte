<script lang="ts">
	// One Rocket League match in progress. Everything it shows is refreshed
	// twice a second, so no figure may decide the width of what holds it: a
	// score going from 9 to 10, or a clock from 1:09 to 0:59, must leave every
	// other line exactly where it was. Hence fixed boxes and `tabular-nums`.
	import type { RlLiveMatch } from '$lib/rocketleague';
	import { formatClock } from '$lib/format';
	import { t } from '$lib/i18n';
	import GameIcon from '$lib/components/GameIcon.svelte';

	// `username` is optional: the live store is keyed by user id and presence is
	// what puts a name on it, so a member missing from presence still gets his
	// card rather than disappearing from the page. A neutral label, not a blank
	// space, because an empty name reads as a broken card, not as a missing one.
	// `icon` is the game's icon, handed down by the page from presence. It is
	// optional for the same reason `username` is: a card must never wait on
	// presence to exist.
	let { username, match, icon }: { username?: string; match: RlLiveMatch; icon?: string } =
		$props();

	let stats = $derived([
		{ label: t('game.rlGoals'), value: match.goals },
		{ label: t('game.rlAssists'), value: match.assists },
		{ label: t('game.rlSaves'), value: match.saves },
		{ label: t('game.rlShots'), value: match.shots },
		{ label: t('game.rlScore'), value: match.score },
		{ label: t('game.rlDemos'), value: match.demos }
	]);
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
			{t('players.rlLive')}
		</span>
	</header>

	<!--
		Blue and orange, with the clock between them. The live payload does not
		say which side the member is on, so neither does this card.
	-->
	<div class="font-display flex items-center justify-center gap-3 tabular-nums">
		<span class="flex items-center gap-1.5">
			<span
				class="inline-block h-2.5 w-2.5 shrink-0 rounded-full bg-[var(--color-blue)]"
				title={t('game.rlBlue')}
			></span>
			<span class="inline-block w-8 text-right text-2xl font-bold text-[var(--color-text)]"
				>{match.team_blue_score}</span
			>
		</span>
		<span class="inline-block w-14 text-center text-sm text-[var(--color-muted)]"
			>{formatClock(match.seconds_remaining)}</span
		>
		<span class="flex items-center gap-1.5">
			<span class="inline-block w-8 text-left text-2xl font-bold text-[var(--color-text)]"
				>{match.team_orange_score}</span
			>
			<span
				class="inline-block h-2.5 w-2.5 shrink-0 rounded-full bg-[var(--color-brand-bright)]"
				title={t('game.rlOrange')}
			></span>
		</span>
	</div>

	<!--
		The overtime line always occupies its row, empty or not: a match tipping
		into overtime must not make the card grow under the reader's eyes.
	-->
	<p
		class="font-display h-4 text-center text-[10px] font-bold tracking-wide text-[var(--color-magenta)] uppercase"
	>
		{#if match.overtime}{t('players.rlOvertime')}{/if}
	</p>

	<dl class="grid grid-cols-3 gap-1 border-t border-[var(--color-border)] pt-2 text-center">
		{#each stats as s (s.label)}
			<div class="min-w-0">
				<dt class="truncate text-[10px] tracking-wide text-[var(--color-muted)] uppercase">
					{s.label}
				</dt>
				<dd class="font-display text-sm font-bold tabular-nums text-[var(--color-text)]">
					{s.value}
				</dd>
			</div>
		{/each}
	</dl>
</article>
