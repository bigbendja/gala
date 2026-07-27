<script lang="ts">
	import type { Source } from '$lib/api/types';
	import { cleanHealthDetail, providerLabel, shortLocator } from '$lib/format';
	import HealthDot from './HealthDot.svelte';

	interface Props {
		sources: Source[];
	}
	let { sources }: Props = $props();
</script>

<div class="src-stack">
	<div class="src-head">
		<span>{sources.length} {sources.length === 1 ? 'fuente' : 'fuentes'}</span>
		<span>se reproduce la mejor disponible</span>
	</div>
	{#each sources as src (src.id)}
		{@const detail = cleanHealthDetail(src.health_detail)}
		<div class="src-row">
			<HealthDot health={src.health} size={8} />
			<span class="tag" class:local={src.provider === 'localfs'}>{providerLabel(src.provider)}</span>
			<span class="path" title={src.locator}>
				{shortLocator(src.locator)}{#if detail} · {detail}{/if}
			</span>
			<span class="q">{src.quality_label ?? '—'}</span>
		</div>
	{/each}
</div>

<style>
	.src-stack {
		margin-top: 26px;
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.25);
		overflow: hidden;
		max-width: 560px;
		width: 100%;
		box-sizing: border-box;
	}
	.src-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 10px;
		padding: 11px 16px;
		border-bottom: 1px solid var(--line);
		font-family: var(--mono);
		font-size: 12px;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.src-head span:last-child {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.src-row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 11px 16px;
		border-bottom: 1px solid rgba(51, 38, 25, 0.5);
		font-size: 13.5px;
		min-width: 0;
	}
	.src-row:last-child {
		border-bottom: none;
	}
	.src-row .tag {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--bg);
		background: var(--muted);
		border-radius: 5px;
		padding: 2px 8px;
		font-weight: 500;
		flex-shrink: 0;
	}
	.src-row .tag.local {
		background: var(--accent);
		color: #1a1207;
	}
	.src-row .path {
		color: var(--muted);
		font-family: var(--mono);
		font-size: 12px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
		min-width: 0;
	}
	.src-row .q {
		color: var(--muted);
		font-size: 12px;
		flex-shrink: 0;
	}
</style>
