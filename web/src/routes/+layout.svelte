<script lang="ts">
	import '../app.css';
	import { onMount, onDestroy } from 'svelte';
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

	async function openSocket(userId: string) {
		try {
			presence.hydrate(await api.getPresence());
		} catch {
			// The socket refills the store from the next updates.
			presence.hydrate([]);
		}
		// Auth may have changed while the snapshot was in flight.
		if (socketUserId !== userId) return;
		socket = connectPresence(
			() => get(auth).accessToken,
			(entry) => presence.apply(entry),
			(status) => presence.setStatus(status),
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

	let navItems = $derived([
		{ href: '/', label: t('nav.dashboard') },
		{ href: '/players', label: t('nav.players') },
		{ href: '/leaderboards', label: t('nav.leaderboards') },
		{ href: '/games', label: t('nav.games') },
		{ href: '/download', label: t('nav.download') },
		...($auth.user?.role === 'admin' ? [{ href: '/admin', label: t('nav.admin') }] : []),
		{ href: '/account', label: t('nav.account') }
	]);

	function isActive(href: string): boolean {
		return href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
	}
</script>

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
					{#if hasLogo}
						<span class="h-8 w-px bg-[var(--color-border)]"></span>
						<img src="/img/org/logo" alt={orgName} title={orgName} class="h-9 w-auto" />
					{/if}
					<nav class="flex gap-1">
						{#each navItems as item (item.href)}
							<a
								href={item.href}
								class="font-display px-3 py-1.5 text-xs font-bold tracking-wider uppercase transition-colors {isActive(
									item.href
								)
									? 'text-[var(--color-brand-bright)]'
									: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
							>
								{item.label}
								{#if isActive(item.href)}
									<span class="mt-0.5 block h-0.5 w-full bg-[var(--color-brand)]"></span>
								{/if}
							</a>
						{/each}
					</nav>
				</div>
				<div class="flex items-center gap-3">
					<a href="/account" class="flex items-center gap-2">
						<span class="text-sm text-[var(--color-muted)]">{$auth.user.username}</span>
						<Avatar username={$auth.user.username} url={$auth.user.avatar_url} size={32} />
					</a>
				</div>
			</div>
		</header>

		<main class="pd-grid-bg mx-auto w-full max-w-5xl flex-1 px-4 py-6">
			{@render children()}
		</main>
	{/if}
	<Footer />
</div>
