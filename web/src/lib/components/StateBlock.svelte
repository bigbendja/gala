<script lang="ts">
	// Estado vacío o de error de una lista/sección (Sistema de Diseño GALA
	// §2.1/§2.2): título breve + una línea + (si aplica) la acción que lo
	// resuelve. Nunca un hueco triste ni un mensaje técnico.
	interface Props {
		title: string;
		text?: string;
		/** Botón de acción (primario por defecto; "quiet" para reintentar). */
		actionLabel?: string;
		actionHref?: string;
		onAction?: () => void;
		tone?: 'empty' | 'error';
		/** Más compacto, para dentro de una sección (no a pantalla completa). */
		compact?: boolean;
	}
	let { title, text, actionLabel, actionHref, onAction, tone = 'empty', compact = false }: Props = $props();
</script>

<div class="state" class:compact class:error={tone === 'error'} role={tone === 'error' ? 'alert' : undefined}>
	<p class="t">{title}</p>
	{#if text}<p class="x">{text}</p>{/if}
	{#if actionLabel}
		{#if actionHref}
			<a class="btn btn-secondary" class:btn-primary={tone === 'empty'} href={actionHref}>{actionLabel}</a>
		{:else}
			<button class="btn btn-secondary" class:btn-primary={tone === 'empty'} onclick={onAction}>{actionLabel}</button>
		{/if}
	{/if}
</div>

<style>
	.state {
		max-width: 520px;
		margin: 14vh auto 0;
		padding: 0 28px;
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
	}
	.state.compact {
		margin: 0;
		padding: 28px 16px;
		max-width: none;
		border: 1px dashed var(--line);
		border-radius: var(--r);
	}
	.t {
		font-family: var(--display);
		font-size: 22px;
		font-weight: 600;
	}
	.compact .t {
		font-size: 17px;
	}
	.x {
		color: var(--muted);
		font-size: 14.5px;
		max-width: 44ch;
	}
	.error .t {
		color: var(--text);
	}
	.state :global(.btn) {
		margin-top: 10px;
	}
</style>
