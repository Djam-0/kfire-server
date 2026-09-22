import type { PubgPlayer, PubgMatch } from './api';

/**
 * A match finished first. PUBG calls it a Chicken Dinner and so does everyone
 * who plays it, which is why the pages say that and not "win".
 */
export function pubgIsChickenDinner(m: Pick<PubgMatch, 'win_place'>): boolean {
	return m.win_place === 1;
}

/** Share of matches finished in the top ten, as a whole percentage. */
export function pubgTopTenRate(p: PubgPlayer): number {
	return p.matches > 0 ? Math.round((p.top_tens * 100) / p.matches) : 0;
}

/**
 * Damage per match, rounded.
 *
 * The server sends the total and the match count rather than an average: an
 * average cannot be summed, so computing it here is what lets the same numbers
 * also feed the guild totals above the table.
 */
export function pubgAvgDamage(p: PubgPlayer): number {
	return p.matches > 0 ? Math.round(p.damage_dealt / p.matches) : 0;
}

/**
 * Members ranked by Chicken Dinners, then top-ten rate, then matches played.
 *
 * On an absolute count and not on a rate, for the same reason as Rocket
 * League: one match played and won reads as 100% and would outrank a season of
 * real play.
 */
export function pubgByWins(players: PubgPlayer[]): PubgPlayer[] {
	return [...players].sort((a, b) => {
		if (a.wins !== b.wins) return b.wins - a.wins;
		const ta = pubgTopTenRate(a);
		const tb = pubgTopTenRate(b);
		if (ta !== tb) return tb - ta;
		return b.matches - a.matches || a.username.localeCompare(b.username);
	});
}

/** Total matches stored for the guild. */
export function pubgTotalMatches(players: PubgPlayer[]): number {
	return players.reduce((n, p) => n + p.matches, 0);
}

/** Total Chicken Dinners across the guild. */
export function pubgTotalWins(players: PubgPlayer[]): number {
	return players.reduce((n, p) => n + p.wins, 0);
}

/**
 * The player-facing name of a PUBG map, by its internal identifier.
 *
 * The identifiers are the same in every language, so this table is not
 * translated: Erangel is Erangel in French. An unknown map falls back to its
 * raw identifier rather than to an empty cell, because a new map ships every
 * few months and a blank would look like missing data.
 *
 * Six of these were seen on real matches on 2026-09-22: Baltic, Tiger, Desert,
 * Neon, DihorOtok and Chimera. The others come from knowledge of the game and
 * are unconfirmed, so a wrong one shows its raw identifier rather than a wrong
 * name. Range_Main, the training range, is deliberately absent: such a session
 * is never stored, so listing it here would suggest otherwise.
 */
const mapNames: Record<string, string> = {
	Baltic_Main: 'Erangel',
	Erangel_Main: 'Erangel',
	Desert_Main: 'Miramar',
	Savage_Main: 'Sanhok',
	DihorOtok_Main: 'Vikendi',
	Summerland_Main: 'Karakin',
	Tiger_Main: 'Taego',
	Kiki_Main: 'Deston',
	Chimera_Main: 'Paramo',
	Heaven_Main: 'Haven',
	Neon_Main: 'Rondo'
};

export function pubgMapLabel(mapName: string): string {
	return mapNames[mapName] ?? mapName;
}

/**
 * The mode label for a match, from PUBG's own identifier.
 *
 * The identifiers read `solo`, `duo`, `squad` and their first-person variants
 * `solo-fpp`, `duo-fpp`, `squad-fpp`. FPP is what players say, so the suffix is
 * kept as is and only the team part is capitalised.
 */
export function pubgModeLabel(gameMode: string): string {
	const [size, ...rest] = gameMode.split('-');
	const head = size.charAt(0).toUpperCase() + size.slice(1);
	return rest.length ? `${head} ${rest.join(' ').toUpperCase()}` : head;
}
