<script lang="ts">
	// Every match being played right now, one card each. The page only reads the
	// shared live store: there is no snapshot to fetch and nothing to persist,
	// so a page opened mid-match fills up with the next sample, half a second
	// later. That is the design, not a gap to paper over.
	import { liveMatches } from '$lib/stores/live.svelte';
	import { presence } from '$lib/stores/presence.svelte';
	import { rlLiveMatch, type RlLiveMatch } from '$lib/rocketleague';
	import { lolLiveMatch, type LolLiveMatch } from '$lib/lol';
	import { hsLiveMatch, type HsLiveMatch } from '$lib/hearthstone';
	import RocketLeagueCard from '$lib/components/live/RocketLeagueCard.svelte';
	import LeagueOfLegendsCard from '$lib/components/live/LeagueOfLegendsCard.svelte';
	import HearthstoneCard from '$lib/components/live/HearthstoneCard.svelte';
	import { t } from '$lib/i18n';

	let names = $derived(new Map(presence.list.map((m) => [m.user_id, m.username])));

	type RlCard = { userId: string; username?: string; match: RlLiveMatch };
	type LolCard = { userId: string; username?: string; match: LolLiveMatch };
	type HsCard = { userId: string; username?: string; match: HsLiveMatch };

	// Sorted on a stable key rather than on the store's iteration order, so
	// cards keep their place across the twice-a-second refresh.
	function byName<T extends { userId: string; username?: string }>(a: T, b: T): number {
		return (a.username ?? '').localeCompare(b.username ?? '') || a.userId.localeCompare(b.userId);
	}

	// Only the games with a card of their own get one. A slug we cannot render
	// yields nothing at all, and that is the normal case, not an error: a client
	// newer than this page must never produce an empty or half-drawn card.
	let rlCards = $derived(
		liveMatches.list
			.flatMap((entry): RlCard[] => {
				const match = rlLiveMatch(entry);
				return match ? [{ userId: entry.user_id, username: names.get(entry.user_id), match }] : [];
			})
			.sort(byName)
	);

	let lolCards = $derived(
		liveMatches.list
			.flatMap((entry): LolCard[] => {
				const match = lolLiveMatch(entry);
				return match ? [{ userId: entry.user_id, username: names.get(entry.user_id), match }] : [];
			})
			.sort(byName)
	);

	let hsCards = $derived(
		liveMatches.list
			.flatMap((entry): HsCard[] => {
				const match = hsLiveMatch(entry);
				return match ? [{ userId: entry.user_id, username: names.get(entry.user_id), match }] : [];
			})
			.sort(byName)
	);
</script>

<h1 class="pd-heading mb-5 text-xl">{t('live.heading')}</h1>

{#if rlCards.length === 0 && lolCards.length === 0 && hsCards.length === 0}
	<p class="text-[var(--color-muted)]">{t('live.empty')}</p>
{:else}
	<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each rlCards as card (card.userId)}
			<RocketLeagueCard username={card.username} match={card.match} />
		{/each}
		{#each lolCards as card (card.userId)}
			<LeagueOfLegendsCard username={card.username} match={card.match} />
		{/each}
		{#each hsCards as card (card.userId)}
			<HearthstoneCard username={card.username} match={card.match} />
		{/each}
	</div>
{/if}
