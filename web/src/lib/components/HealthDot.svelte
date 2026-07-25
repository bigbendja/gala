<script lang="ts">
	import { healthColor, healthLabel } from '$lib/health';
	import type { Health } from '$lib/api/types';

	interface Props {
		health: Health | string;
		/** Tamaño en px del punto; el mockup usa 6-8px según el contexto. */
		size?: number;
	}
	let { health, size = 8 }: Props = $props();
</script>

<span
	class="dot"
	class:glow={health === 'viva'}
	style:width="{size}px"
	style:height="{size}px"
	style:background={healthColor(health)}
	title={healthLabel(health)}
	aria-label={healthLabel(health)}
	role="img"
></span>

<style>
	.dot {
		display: inline-block;
		border-radius: 50%;
		flex-shrink: 0;
	}
	/* Brillo sutil en "viva", igual que .h-ok del mockup. */
	.dot.glow {
		box-shadow: 0 0 7px rgba(111, 207, 142, 0.5);
	}
</style>
