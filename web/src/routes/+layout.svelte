<script lang="ts">
	import '../app.css';
	import { onMount, onDestroy, tick, untrack } from 'svelte';
	import { get } from 'svelte/store';
	import { page } from '$app/state';
	import { auth } from '$lib/stores/auth.svelte';
	import { api, getConfig } from '$lib/api';
	import { connectPresence, type PresenceSocket } from '$lib/ws';
	import { presence } from '$lib/stores/presence.svelte';
	import { liveMatches } from '$lib/stores/live.svelte';
	import Avatar from '$lib/components/Avatar.svelte';
	import Login from '$lib/components/Login.svelte';
	import Footer from '$lib/components/Footer.svelte';
	import { initLocale, t } from '$lib/i18n';

	let { children } = $props();

	let hasLogo = $state(false);
	let orgName = $state('KFIRE');

	onMount(async () => {
		initLocale();
		auth.init();
		try {
			const cfg = await getConfig();
			// Apply the server's dominant accent (orange is the CSS default).
			if (cfg.accent && cfg.accent !== 'orange') {
				document.documentElement.dataset.accent = cfg.accent;
			} else {
				delete document.documentElement.dataset.accent;
			}
			hasLogo = cfg.has_logo;
			orgName = cfg.org_name;
		} catch (e) {
			/* keep defaults when config is unavailable */
		}
	});

	// The layout owns the one and only presence socket, because the nav lives on
	// every page and needs live counts there. Pages read the stores instead of
	// opening their own connection.
	let socket: PresenceSocket | null = null;
	// Which user the socket was opened for. The auth store emits on every token
	// refresh and profile edit, so the effect below reacts to identity changes
	// only; comparing against this guard is what keeps a single socket alive
	// instead of tearing one down and reopening it on each emission.
	let socketUserId: string | null = null;

	// Whether the socket has dropped since the last snapshot. A presence_update
	// is only ever a delta, so everything that changed during an outage was
	// missed: a member who went offline while we were disconnected would stay
	// shown as online until his next action. Reloading the snapshot on the way
	// back is what closes that gap. Missing it used to be hidden by every page
	// reloading the snapshot on mount; with one session-long socket, nothing
	// hides it any more.
	let missedUpdates = false;

	async function loadSnapshot(userId: string) {
		try {
			const snapshot = await api.getPresence();
			presence.hydrate(snapshot);
			// The same snapshot carries the matches in progress, so a page
			// reloaded mid-match draws its cards straight away instead of
			// waiting for the next sample. That wait was imperceptible on
			// Rocket League and lasted a whole turn on Hearthstone.
			liveMatches.hydrate(
				snapshot.flatMap((e) =>
					e.live?.match && e.live.game_slug
						? [{ user_id: e.user_id, game_slug: e.live.game_slug, match: e.live.match }]
						: []
				)
			);
		} catch {
			// The socket refills the store from the next updates.
			presence.hydrate([]);
		}
		// Auth may have changed while the snapshot was in flight.
		return socketUserId === userId;
	}

	function onSocketStatus(userId: string, status: 'connecting' | 'connected' | 'disconnected') {
		presence.setStatus(status);
		if (status === 'disconnected') {
			missedUpdates = true;
			// Live matches have no snapshot and no client-side expiry, so a state
			// held here is only as good as the socket that feeds it. While it is
			// down we cannot know a match ended, and a frozen score would sit on
			// screen forever. Dropping them says "we don't know", which is true;
			// the next sample refills within half a second of reconnecting.
			liveMatches.clear();
		} else if (status === 'connected' && missedUpdates) {
			missedUpdates = false;
			loadSnapshot(userId);
		}
	}

	async function openSocket(userId: string) {
		if (!(await loadSnapshot(userId))) return;
		missedUpdates = false;
		socket = connectPresence(
			() => get(auth).accessToken,
			(entry) => presence.apply(entry),
			(status) => onSocketStatus(userId, status),
			(update) => liveMatches.apply(update)
		);
	}

	$effect(() => {
		const userId = $auth.user?.id ?? null;
		if (userId === socketUserId) return;
		socketUserId = userId;
		socket?.close();
		socket = null;
		if (userId) {
			openSocket(userId);
		} else {
			presence.clear();
			liveMatches.clear();
		}
	});

	onDestroy(() => socket?.close());

	// `live` marks the one entry that reflects the live store. It is decorated
	// only while matches are running: a permanent accent would stop meaning
	// "right now", and nobody should be drawn to a page with nothing on it.
	type NavItem = { href: string; label: string; live?: boolean };

	// The main bar carries what speaks about the guild. Anything about *me* and
	// my machine sits under the avatar, where people look for it anyway.
	let navItems: NavItem[] = $derived([
		{ href: '/', label: t('nav.dashboard') },
		{ href: '/players', label: t('nav.players') },
		{ href: '/leaderboards', label: t('nav.leaderboards') },
		{ href: '/games', label: t('nav.games') },
		{ href: '/live', label: t('nav.live'), live: true },
		{ href: '/recap', label: t('nav.recap') }
	]);

	// Account comes first so the page the avatar used to link to is still the
	// obvious target once the menu is open.
	let userItems: NavItem[] = $derived([
		{ href: '/account', label: t('nav.account') },
		{ href: '/download', label: t('nav.download') },
		...($auth.user?.role === 'admin' ? [{ href: '/admin', label: t('nav.admin') }] : [])
	]);

	function isActive(href: string): boolean {
		return href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
	}

	/*
	 * Both popups follow the W3C disclosure-navigation pattern rather than a
	 * `role="menu"` widget: the contents are plain links, so Tab already walks
	 * them and browsers announce them as links. Arrow keys are added on top for
	 * the people who expect them; Escape always closes and hands focus back, so
	 * the popup can never become a place you get stuck in.
	 */
	let userMenuOpen = $state(false);
	let mobileNavOpen = $state(false);
	let userMenuTrigger = $state<HTMLButtonElement | null>(null);
	let mobileNavTrigger = $state<HTMLButtonElement | null>(null);
	let userMenuRoot = $state<HTMLElement | null>(null);
	let mobileNavRoot = $state<HTMLElement | null>(null);

	function closePopups(restoreFocus: 'user' | 'mobile' | null = null) {
		userMenuOpen = false;
		mobileNavOpen = false;
		if (restoreFocus === 'user') userMenuTrigger?.focus();
		if (restoreFocus === 'mobile') mobileNavTrigger?.focus();
	}

	// Focus the first link of a popup that was opened from the keyboard. The
	// panel is rendered by the same click that calls this, so the query has to
	// wait for the DOM to catch up.
	async function focusFirstLink(root: () => HTMLElement | null) {
		await tick();
		root()?.querySelector<HTMLAnchorElement>('a[href]')?.focus();
	}

	/*
	 * Keyboard handling lives on the window rather than on the panels: hanging a
	 * keydown listener on a <div> or a <nav> would be a static-element handler
	 * that Svelte rightly flags, and the triggers and their panels are two
	 * separate focus targets anyway. Nothing is preventDefault-ed unless we act,
	 * so ordinary arrow-key scrolling is untouched.
	 */
	function onWindowKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			if (userMenuOpen) closePopups('user');
			else if (mobileNavOpen) closePopups('mobile');
			return;
		}
		if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;

		const active = document.activeElement;
		// A closed popup opens downwards from its own trigger.
		if (!userMenuOpen && !mobileNavOpen) {
			if (event.key !== 'ArrowDown') return;
			if (active === userMenuTrigger) {
				event.preventDefault();
				userMenuOpen = true;
				focusFirstLink(() => userMenuRoot);
			} else if (active === mobileNavTrigger) {
				event.preventDefault();
				mobileNavOpen = true;
				focusFirstLink(() => mobileNavRoot);
			}
			return;
		}

		const root = userMenuOpen ? userMenuRoot : mobileNavRoot;
		if (!root || !active || !root.contains(active)) return;
		const links = Array.from(root.querySelectorAll<HTMLAnchorElement>('a[href]'));
		if (links.length === 0) return;
		event.preventDefault();
		// Cycling keeps both ends live instead of turning them into dead keys.
		const at = links.indexOf(active as HTMLAnchorElement);
		const step = event.key === 'ArrowDown' ? 1 : -1;
		links[(at + step + links.length) % links.length].focus();
	}

	// A pointer press anywhere outside both popups dismisses them. Using
	// pointerdown rather than click means a press that starts outside closes the
	// popup even if the pointer is released elsewhere.
	function onWindowPointerDown(event: PointerEvent) {
		if (!userMenuOpen && !mobileNavOpen) return;
		const target = event.target as Node;
		if (userMenuRoot?.contains(target) || mobileNavRoot?.contains(target)) return;
		closePopups();
	}

	// Following a link leaves the popup open over the new page otherwise. The
	// pathname is the only dependency, so this does not fire on unrelated state.
	$effect(() => {
		page.url.pathname;
		untrack(() => closePopups());
	});
</script>

<svelte:window onkeydown={onWindowKeydown} onpointerdown={onWindowPointerDown} />

<svelte:head>
	<title>KFIRE</title>
</svelte:head>

<div class="flex min-h-screen flex-col">
	{#if !$auth.ready}
		<div class="grid flex-1 place-items-center text-[var(--color-muted)]">{t('common.loading')}</div>
	{:else if !$auth.user}
		{#if page.url.pathname.startsWith('/reset')}
			{@render children()}
		{:else}
			<Login />
		{/if}
	{:else}
		<header
			class="pd-header-shadow border-b-2 border-[var(--color-brand)]/60 bg-[var(--color-surface)]"
		>
			<div class="mx-auto flex max-w-5xl items-center justify-between px-4 py-3">
				<div class="flex items-center gap-8">
					<a href="/" class="flex items-center gap-3">
						<img src="/kfire-logo-640.png" alt="KFIRE" class="h-10 w-auto" />
						<span
							class="font-display text-2xl font-extrabold tracking-[0.08em] text-[var(--color-text)] italic"
						>
							K<span class="text-[var(--color-brand)]">FIRE</span>
						</span>
					</a>
					<!--
						The org logo is the first thing to go on a phone: it is decoration
						next to the KFIRE mark, and its width is whatever the admin uploaded.
					-->
					{#if hasLogo}
						<span class="hidden h-8 w-px bg-[var(--color-border)] sm:block"></span>
						<img
							src="/img/org/logo"
							alt={orgName}
							title={orgName}
							class="hidden h-9 w-auto sm:block"
						/>
					{/if}
					<!--
						Hidden below `lg`, where the hamburger takes over. The threshold is
						measured in the browser, not guessed. At 12px/700 Exo the six French
						labels (the longer of the two locales) measure 618px once px-3 and
						gap-1 are counted; the logo block adds 123px and its gap-8 another
						32px, and the avatar block on the right is 120px. That is 885px of
						content box, so 917px of viewport once px-4 is counted, and roughly
						936px when /live is decorated (dot + count). `lg` (1024px) is the
						smallest standard breakpoint above that, and the ~90px of slack is
						what a long pseudo or the optional org logo eats into.
					-->
					<nav class="hidden gap-1 lg:flex">
						{#each navItems as item (item.href)}
							{@render navLink(item, '')}
						{/each}
					</nav>
				</div>
				<div class="flex items-center gap-2">
					<div class="relative" bind:this={userMenuRoot}>
						<button
							type="button"
							bind:this={userMenuTrigger}
							onclick={() => {
								mobileNavOpen = false;
								userMenuOpen = !userMenuOpen;
							}}
							aria-haspopup="menu"
							aria-expanded={userMenuOpen}
							aria-controls="user-menu"
							aria-label={t('nav.userMenu')}
							class="flex cursor-pointer items-center gap-2 rounded px-1 py-1 transition-colors hover:bg-[var(--color-surface-2)]"
						>
							<span class="hidden text-sm text-[var(--color-muted)] sm:inline"
								>{$auth.user.username}</span
							>
							<Avatar username={$auth.user.username} url={$auth.user.avatar_url} size={32} />
							{@render chevron(userMenuOpen)}
						</button>
						{#if userMenuOpen}
							<div
								id="user-menu"
								class="pd-header-shadow absolute top-full right-0 z-50 mt-2 flex min-w-44 flex-col border border-[var(--color-border)] bg-[var(--color-surface)] py-1"
							>
								{#each userItems as item (item.href)}
									{@render navLink(item, 'block')}
								{/each}
							</div>
						{/if}
					</div>

					<div class="relative lg:hidden" bind:this={mobileNavRoot}>
						<button
							type="button"
							bind:this={mobileNavTrigger}
							onclick={() => {
								userMenuOpen = false;
								mobileNavOpen = !mobileNavOpen;
							}}
							aria-haspopup="menu"
							aria-expanded={mobileNavOpen}
							aria-controls="mobile-nav"
							aria-label={mobileNavOpen ? t('nav.closeMenu') : t('nav.openMenu')}
							class="flex cursor-pointer items-center rounded p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
						>
							<svg
								class="h-5 w-5"
								viewBox="0 0 20 20"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="square"
								aria-hidden="true"
							>
								{#if mobileNavOpen}
									<path d="M4 4l12 12M16 4L4 16" />
								{:else}
									<path d="M3 5h14M3 10h14M3 15h14" />
								{/if}
							</svg>
						</button>
						{#if mobileNavOpen}
							<nav
								id="mobile-nav"
								class="pd-header-shadow absolute top-full right-0 z-50 mt-2 flex min-w-52 flex-col border border-[var(--color-border)] bg-[var(--color-surface)] py-1"
							>
								{#each navItems as item (item.href)}
									{@render navLink(item, 'block')}
								{/each}
							</nav>
						{/if}
					</div>
				</div>
			</div>
		</header>

		<main class="pd-grid-bg mx-auto w-full max-w-5xl flex-1 px-4 py-6">
			{@render children()}
		</main>
	{/if}
	<Footer />
</div>

<!--
	One renderer for every place a nav entry appears, so the live decoration and
	the active underline cannot drift between the bar and the two popups.
	`extra` only carries layout (the popups stack their entries as blocks).
-->
{#snippet navLink(item: NavItem, extra: string)}
	{@const active = isActive(item.href)}
	{@const isLive = item.live === true && liveMatches.count > 0}
	<a
		href={item.href}
		aria-current={active ? 'page' : undefined}
		class="font-display px-3 py-1.5 text-xs font-bold tracking-wider uppercase transition-colors {extra} {isLive
			? 'text-[var(--color-online)]'
			: active
				? 'text-[var(--color-brand-bright)]'
				: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
	>
		{#if isLive}
			<span
				class="live-dot mr-1 inline-block h-1.5 w-1.5 rounded-full bg-[var(--color-online)] align-middle"
				aria-hidden="true"
			></span>
		{/if}
		{item.label}
		{#if isLive}
			<span class="ml-0.5 tabular-nums">{liveMatches.count}</span>
		{/if}
		{#if active}
			<span class="mt-0.5 block h-0.5 w-full bg-[var(--color-brand)]"></span>
		{/if}
	</a>
{/snippet}

{#snippet chevron(open: boolean)}
	<svg
		class="h-3 w-3 text-[var(--color-muted)] transition-transform {open ? 'rotate-180' : ''}"
		viewBox="0 0 12 12"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		aria-hidden="true"
	>
		<path d="M2 4.5L6 8.5L10 4.5" />
	</svg>
{/snippet}

<style>
	/*
	 * The dot beats to say a match is running right now. Under reduced motion it
	 * keeps its colour and simply stops beating: hiding it would take away the
	 * information, not just the movement.
	 */
	.live-dot {
		animation: live-pulse 1.4s ease-in-out infinite;
	}

	@keyframes live-pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.25;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.live-dot {
			animation: none;
		}
	}
</style>
