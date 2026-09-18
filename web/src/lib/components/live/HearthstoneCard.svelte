<script lang="ts">
	// One Hearthstone game in progress. Poorer than the Rocket League card on
	// purpose: the game's log names the opponent and lists every card played,
	// and none of that is broadcast, so there is nothing else to show. Nothing
	// here may be inferred from the rest.
	import { hsModeLabel, type HsLiveMatch } from '$lib/hearthstone';
	import { t } from '$lib/i18n';

	// `username` is optional for the same reason as on the other live cards: the
	// live store is keyed by user id and presence is what puts a name on it, so
	// a member missing from presence still gets his card.
	let { username, match }: { username?: string; match: HsLiveMatch } = $props();

	// A place exists only in Battlegrounds. A constructed game sends no such
	// key, so the column simply is not drawn rather than showing a dash.
	let hasPlacement = $derived(typeof match.placement === 'number');
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

	<div class="flex items-center justify-center">
		<span class="font-display min-w-0 truncate text-lg font-bold text-[var(--color-text)]">
			{hsModeLabel(match.mode)}
		</span>
	</div>

	<dl
		class="grid gap-1 border-t border-[var(--color-border)] pt-2 text-center"
		class:grid-cols-1={!hasPlacement}
		class:grid-cols-2={hasPlacement}
	>
		<div class="min-w-0">
			<dt class="truncate text-[10px] tracking-wide text-[var(--color-muted)] uppercase">
				{t('live.hsTurn')}
			</dt>
			<dd class="font-display text-sm font-bold tabular-nums text-[var(--color-text)]">
				{match.turn}
			</dd>
		</div>
		{#if hasPlacement}
			<div class="min-w-0">
				<dt class="truncate text-[10px] tracking-wide text-[var(--color-muted)] uppercase">
					{t('live.hsPlacement')}
				</dt>
				<dd class="font-display text-sm font-bold tabular-nums text-[var(--color-text)]">
					{match.placement}
				</dd>
			</div>
		{/if}
	</dl>
</article>
