<script lang="ts">
	// One finished Hearthstone match, read as the mode itself reads it. The rule
	// lives in `hsResultInfo`; this only draws it, so every page that shows a
	// match result shows the same words, the same accent and the same crown.
	import { hsResultInfo } from '$lib/hearthstone';

	// `placement` is null on a constructed game and may also be missing from a
	// Battlegrounds log, which the rule already handles; both spellings are
	// accepted because the recap sends null and a profile sends nothing.
	let {
		mode,
		result,
		placement = null,
		class: extraClass = ''
	}: {
		mode: string;
		result: string;
		placement?: number | null;
		class?: string;
	} = $props();

	let info = $derived(hsResultInfo(mode, result, placement));
</script>

<span class="font-display inline-flex items-center gap-1 font-bold {info.colorClass} {extraClass}">
	{#if info.crown}
		<!-- Drawn rather than typed: an emoji crown is at the mercy of the
		     reader's font, and this one is always legible and takes the accent
		     colour with it. -->
		<svg
			width="12"
			height="12"
			viewBox="0 0 24 24"
			fill="currentColor"
			aria-hidden="true"
			class="shrink-0"
		>
			<path d="M3 7l4.5 3.5L12 4l4.5 6.5L21 7l-1.8 11H4.8L3 7z" />
			<rect x="4.8" y="19" width="14.4" height="2" />
		</svg>
	{/if}
	{info.label}
</span>
