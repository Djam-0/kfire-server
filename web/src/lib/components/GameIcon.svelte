<script lang="ts">
	// A game's icon, beside its name.
	//
	// The icon is decoration: the name is always there and always readable, so
	// this component renders nothing at all rather than a placeholder when the
	// catalog has no image. A grey square in every row would be noise, and a
	// broken image would look like a bug in the page.
	//
	// `alt` is empty on purpose. The game's name sits right next to the icon in
	// every caller, so naming it again would make a screen reader say it twice.
	let { url, size = 20 }: { url?: string; size?: number } = $props();

	// The server only sends a URL for a game it holds an image for, so a 404 is
	// not expected here. It stays possible (a cache miss whose source has since
	// disappeared from the CDN), and hiding the element is the one degradation
	// that leaves the row exactly as it would be with no icon at all.
	let broken = $state(false);
</script>

{#if url && !broken}
	<img
		src={url}
		alt=""
		width={size}
		height={size}
		loading="lazy"
		onerror={() => (broken = true)}
		class="pd-cut-sm shrink-0 object-cover"
		style="width:{size}px;height:{size}px"
	/>
{/if}
