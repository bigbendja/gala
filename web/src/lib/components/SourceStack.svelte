<script lang="ts">
	import type { Source } from '$lib/api/types';
	import { friendlyHealthDetail, locatorName, locatorWhere, providerLabel } from '$lib/format';
	import HealthDot from './HealthDot.svelte';

	interface Props {
		sources: Source[];
		/** Etiqueta sobre la lista (por defecto "N fuente(s)"). */
		heading?: string;
	}
	let { sources, heading }: Props = $props();
</script>

<div class="src-stack">
	<div class="src-head">
		<span>{heading ?? `${sources.length} ${sources.length === 1 ? 'fuente' : 'fuentes'}`}</span>
		<span class="hint">se reproduce la mejor disponible</span>
	</div>
	{#each sources as src (src.id)}
		{@const detail = friendlyHealthDetail(src.health_detail)}
		{@const name = locatorName(src.locator)}
		{@const where = locatorWhere(src.locator)}
		<div class="src-row">
			<HealthDot health={src.health} size={8} />
			<span class="tag" class:local={src.provider === 'localfs'}>{providerLabel(src.provider)}</span>
			<span class="what" title={src.locator}>
				<span class="name">{name}</span>
				{#if where}<span class="where">{where}</span>{/if}
			</span>
			{#if detail}<span class="detail" class:bad={src.health === 'caída'}>{detail}</span>{/if}
			{#if src.quality_label}<span class="q">{src.quality_label}</span>{/if}
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
		max-width: 620px;
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
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.src-head .hint {
		font-weight: 400;
		letter-spacing: 0.02em;
		text-transform: none;
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
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.04em;
		color: var(--bg);
		background: var(--muted);
		border-radius: 5px;
		padding: 2px 8px;
		flex-shrink: 0;
	}
	.src-row .tag.local {
		background: var(--accent);
		color: #1a1207;
	}
	/* Nombre del archivo destacado; carpeta o servidor en pequeño debajo. La
	   ruta completa queda en el tooltip: nunca truncada por el medio. */
	.what {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
	}
	.name {
		color: var(--text);
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.where {
		color: var(--muted);
		font-family: var(--mono);
		font-size: 11px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		text-align: left;
	}
	.detail {
		color: var(--muted);
		font-size: 12.5px;
		flex-shrink: 0;
		max-width: 40%;
		text-align: right;
	}
	.detail.bad {
		color: var(--bad);
	}
	.q {
		color: var(--muted);
		font-size: 12px;
		flex-shrink: 0;
	}
	@media (max-width: 600px) {
		.src-row {
			flex-wrap: wrap;
			row-gap: 4px;
		}
		.detail {
			max-width: none;
			text-align: left;
			flex-basis: 100%;
			padding-left: 20px;
		}
		.src-head .hint {
			display: none;
		}
	}
</style>
