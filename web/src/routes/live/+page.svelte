<script lang="ts">
	// Every match being played right now, one card each. The page only reads the
	// shared live store: there is no snapshot to fetch and nothing to persist,
	// so a page opened mid-match fills up with the next sample, half a second
	// later. That is the design, not a gap to paper over.
	import { liveMatches } from '$lib/stores/live.svelte';
	import { presence } from '$lib/stores/presence.svelte';
	import { rlLiveMatch, type RlLiveMatch } from '$lib/rocketleague';
	import RocketLeagueCard from '$lib/components/live/RocketLeagueCard.svelte';
	import { t } from '$lib/i18n';

	let names = $derived(new Map(presence.list.map((m) => [m.user_id, m.username])));

	type RlCard = { userId: string; username?: string; match: RlLiveMatch };

	// Rocket League is the only game with a card so far. A slug we cannot render
	// yields nothing at all, and that is the normal case, not an error: a client
	// newer than this page, or Hearthstone reporting before its card exists,
	// must never produce an empty or half-drawn card.
	let rlCards = $derived(
		liveMatches.list
			.flatMap((entry): RlCard[] => {
				const match = rlLiveMatch(entry);
				return match ? [{ userId: entry.user_id, username: names.get(entry.user_id), match }] : [];
			})
			// Sorted on a stable key rather than on the store's iteration order, so
			// cards keep their place across the twice-a-second refresh.
			.sort(
				(a, b) =>
					(a.username ?? '').localeCompare(b.username ?? '') || a.userId.localeCompare(b.userId)
			)
	);
</script>

<h1 class="pd-heading mb-5 text-xl">{t('live.heading')}</h1>

{#if rlCards.length === 0}
	<p class="text-[var(--color-muted)]">{t('live.empty')}</p>
{:else}
	<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each rlCards as card (card.userId)}
			<RocketLeagueCard username={card.username} match={card.match} />
		{/each}
	</div>
{/if}
