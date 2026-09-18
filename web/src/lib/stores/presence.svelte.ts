// Shared, in-memory store for member presence, fed by the single presence
// WebSocket the root layout owns (see `$lib/ws`). One source of truth for every
// consumer: the dashboard, the players page, and the nav indicator.
//
// Unlike live matches, presence does have a REST snapshot (`api.getPresence`),
// because it is durable state: the layout loads it once at sign-in and the
// socket keeps it current from there. Nothing is written to storage.

import type { PresenceEntry } from '../api';

type Status = 'connecting' | 'connected' | 'disconnected';

let entries = $state<Map<string, PresenceEntry>>(new Map());
let status = $state<Status>('connecting');
let loaded = $state(false);

/** Seeds the store from the REST snapshot, replacing whatever it held. */
function hydrate(snapshot: PresenceEntry[]): void {
	const next = new Map<string, PresenceEntry>();
	for (const e of snapshot) next.set(e.user_id, e);
	entries = next;
	loaded = true;
}

/** Applies a `presence_update` event: one member's new state. */
function apply(entry: PresenceEntry): void {
	const next = new Map(entries);
	next.set(entry.user_id, entry);
	entries = next;
}

function setStatus(next: Status): void {
	status = next;
}

/** Drops every entry and returns to the pre-snapshot state, so a next sign-in
 * never shows the previous account's members. */
function clear(): void {
	entries = new Map();
	loaded = false;
	status = 'connecting';
}

export const presence = {
	/** Every known member, in no particular order; consumers sort. */
	get list(): PresenceEntry[] {
		return [...entries.values()];
	},
	/** Whether the initial REST snapshot has been loaded. */
	get loaded(): boolean {
		return loaded;
	},
	/** The presence socket's connection state. */
	get status(): Status {
		return status;
	},
	hydrate,
	apply,
	setStatus,
	clear
};
