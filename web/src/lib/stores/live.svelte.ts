// Shared, in-memory store for live match states, broadcast over the presence
// WebSocket (see `$lib/ws`). One source of truth for every consumer: the
// players page, the nav indicator, and any page that needs it later.
//
// No persistence, on purpose: this state is broadcast twice a second and
// never stored anywhere. A reloaded page starts empty and refills from the
// next sample within about half a second. Do not add localStorage or a REST
// "catch up" fetch here.

import type { LiveMatch, LiveMatchUpdate } from '../ws';

/** One member's live match, with the game it belongs to. */
export type LiveEntry = {
	game_slug: string;
	match: LiveMatch;
};

let entries = $state<Map<string, LiveEntry>>(new Map());

/** Applies a `live_match` event: sets or replaces the entry, or removes it
 * when the match ended (`match: null`, or a match with no `game_slug`). */
function apply(update: LiveMatchUpdate): void {
	if (update.match && update.game_slug) {
		const next = new Map(entries);
		next.set(update.user_id, { game_slug: update.game_slug, match: update.match });
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
	remove
};
