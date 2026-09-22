// Shared, in-memory store for live match states, broadcast over the presence
// WebSocket (see `$lib/ws`). One source of truth for every consumer: the
// players page, the nav indicator, and any page that needs it later.
//
// No persistence, on purpose: this state is never stored anywhere. Do not add
// localStorage, and do not add a REST "catch up" fetch of your own.
//
// A reloaded page is seeded from the presence snapshot it already requests
// (see `hydrate`). That used to be unnecessary, back when the only live game
// sampled twice a second and a reload refilled before anyone noticed.
// Hearthstone broke it: it emits on turn changes, so a reload mid-turn left
// the card missing for as long as the turn lasted.

import type { LiveMatch, LiveMatchUpdate } from '../ws';

/** One member's live match, with the game it belongs to. `user_id` repeats the
 * map key, so a consumer walking `list` still knows whose match it is (the
 * live page needs it to put a name on a card). */
export type LiveEntry = {
	user_id: string;
	game_slug: string;
	match: LiveMatch;
};

let entries = $state<Map<string, LiveEntry>>(new Map());

/** Applies a `live_match` event: sets or replaces the entry, or removes it
 * when the match ended (`match: null`, or a match with no `game_slug`). */
function apply(update: LiveMatchUpdate): void {
	if (update.match && update.game_slug) {
		const next = new Map(entries);
		next.set(update.user_id, {
			user_id: update.user_id,
			game_slug: update.game_slug,
			match: update.match
		});
		entries = next;
	} else {
		remove(update.user_id);
	}
}

/** Removes a member's live match, if it has one. */
function remove(userId: string): void {
	if (!entries.has(userId)) return;
	const next = new Map(entries);
	next.delete(userId);
	entries = next;
}

/** Drops every match. Used on sign-out, so the next account never inherits the
 * previous one's matches while waiting for the first sample. */
function clear(): void {
	entries = new Map();
}

/** Seeds the store from what the presence snapshot carried, replacing
 *  whatever it held.
 *
 *  Replacing rather than merging is the point: the snapshot is the server's
 *  full truth at that instant, so a match it does not mention is a match that
 *  has ended, and keeping it would freeze a dead score on the page. */
function hydrate(snapshot: LiveEntry[]): void {
	const next = new Map<string, LiveEntry>();
	for (const e of snapshot) next.set(e.user_id, e);
	entries = next;
}

export const liveMatches = {
	/** The current live match for a member, if any. */
	get(userId: string): LiveEntry | undefined {
		return entries.get(userId);
	},
	/** Whether a member currently has a live match. */
	has(userId: string): boolean {
		return entries.has(userId);
	},
	/** The list of matches currently in progress. */
	get list(): LiveEntry[] {
		return [...entries.values()];
	},
	/** How many matches are currently in progress. */
	get count(): number {
		return entries.size;
	},
	apply,
	remove,
	clear,
	hydrate
};
