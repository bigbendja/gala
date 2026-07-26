<script lang="ts">
	import { imgUrl } from '$lib/api/client';
	import type { Health, Item } from '$lib/api/types';
	import { displayTitle, isUnidentified } from '$lib/format';
	import HealthDot from './HealthDot.svelte';

	interface Props {
		item: Item;
		/** "wide" = tarjeta de "Continuar viendo" (16:10, más ancha). */
		variant?: 'default' | 'wide' | 'wish';
	}
	let { item, variant = 'default' }: Props = $props();

	const poster = $derived(imgUrl(item.poster_path));
	// Paleta de respaldo cuando aún no hay carátula (p. ej. recién añadido,
	// TMDB todavía no ha resuelto la imagen): igual criterio visual que el
	// mockup (p1..p8), elegida de forma determinista por id para que un
	// mismo item no cambie de color entre renders.
	const paletteIndex = $derived((item.id % 8) + 1);

	// Un dot por fuente reconstruido desde el recuento de health_summary
	// (Brief M10a §5: "varios HealthDot"), acotado para no desbordar la
	// tarjeta si un item tiene muchísimas fuentes.
	const MAX_DOTS = 4;
	const healthDots = $derived.by(() => {
		const summary = item.health_summary ?? {};
		const all: Health[] = [];
		for (const [health, count] of Object.entries(summary)) {
			for (let i = 0; i < (count ?? 0); i++) all.push(health as Health);
		}
		return { shown: all.slice(0, MAX_DOTS), extra: Math.max(0, all.length - MAX_DOTS) };
	});

	// Brief M10a §5: badge "NUEVO" si se puede derivar de los datos reales
	// (added_at reciente); "LISTA·M3U" se omite — el listado no expone el
	// origen de ingesta por item (ver informe de la entrega).
	// Brief M10a-fix A1: con 24h, prácticamente todo lo recién escaneado
	// salía marcado NUEVO a la vez (si todo es nuevo, nada lo es); 48h dejó
	// la mayoría de tarjetas sin badge en la prueba real.
	const NEW_BADGE_WINDOW_HOURS = 48;
	const isNew = $derived(
		Date.now() - new Date(item.added_at).getTime() < NEW_BADGE_WINDOW_HOURS * 3600 * 1000
	);

	// Brief M10a-fix A4: título de presentación limpio para items sin
	// identificar, sin tocar la lógica de M3 (solo esta vista).
	const title = $derived(displayTitle(item));
	const unidentified = $derived(variant === 'default' && isUnidentified(item));

	const progressPct = $derived.by(() => {
		const p = item.progress;
		if (!p || !p.duration_s || p.duration_s <= 0) return null;
		return Math.min(100, Math.round((p.position_s / p.duration_s) * 100));
	});
</script>

<a
	class="card"
	class:wide-card={variant === 'wide'}
	class:wish={variant === 'wish'}
	href="/item/{item.id}"
>
	{#if isNew && variant === 'default'}
		<div class="badge">NUEVO</div>
	{/if}
	{#if variant === 'wish'}
		<span class="wish-tag">Sin fuente</span>
	{/if}

	<div class="poster p{paletteIndex}">
		{#if poster}
			<img src={poster} alt="" loading="lazy" />
		{:else}
			<b>{title}</b>
		{/if}
		{#if progressPct !== null}
			<div class="progress"><i style:width="{progressPct}%"></i></div>
		{/if}
	</div>

	<div class="info">
		<div class="t">{title}</div>
		<div class="m">
			{#if variant === 'wish'}
				<span>{item.year ?? ''} · buscando…</span>
			{:else if unidentified}
				<span class="unidentified">Sin identificar</span>
				{#if healthDots.shown.length > 0}
					<div class="dots">
						{#each healthDots.shown as h, i (i)}
							<HealthDot health={h} size={6} />
						{/each}
					</div>
				{/if}
			{:else}
				<span>{item.year ?? ''}</span>
				{#if healthDots.shown.length > 0}
					<div class="dots">
						{#each healthDots.shown as h, i (i)}
							<HealthDot health={h} size={6} />
						{/each}
						{#if healthDots.extra > 0}
							<span class="extra">+{healthDots.extra}</span>
						{/if}
					</div>
				{/if}
			{/if}
		</div>
	</div>
</a>

<style>
	.card {
		position: relative;
		display: block;
		border-radius: 12px;
		overflow: hidden;
		background: var(--surface);
		border: 1px solid var(--line);
		transition: 0.18s;
		cursor: pointer;
		text-decoration: none;
		color: inherit;
	}
	.card:hover {
		transform: translateY(-4px);
		border-color: #4d3a22;
		box-shadow: 0 14px 34px rgba(0, 0, 0, 0.5);
	}
	.card .poster {
		aspect-ratio: 2 / 3;
		position: relative;
		display: flex;
		align-items: flex-end;
		padding: 14px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 17px;
		line-height: 1.1;
	}
	.card .poster img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.card.wide-card .poster {
		aspect-ratio: 16 / 10;
		font-size: 18px;
	}
	.card .poster::after {
		content: '';
		position: absolute;
		inset: 0;
		background: linear-gradient(180deg, transparent 40%, rgba(10, 7, 4, 0.85));
		pointer-events: none;
	}
	.card .poster b {
		position: relative;
		z-index: 1;
	}
	.card .info {
		padding: 10px 12px 12px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.card .t {
		font-weight: 600;
		font-size: 13.5px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.card .m {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 12px;
		color: var(--muted);
	}
	.dots {
		display: flex;
		align-items: center;
		gap: 4px;
		margin-left: auto;
	}
	/* Brief M10a-fix A4: marca sobria, sin inventar título. */
	.unidentified {
		font-family: var(--mono);
		font-size: 10.5px;
		letter-spacing: 0.03em;
		color: var(--muted);
		font-style: italic;
	}
	.extra {
		font-family: var(--mono);
		font-size: 10px;
	}
	.progress {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 3px;
		background: rgba(0, 0, 0, 0.5);
	}
	.progress i {
		display: block;
		height: 100%;
		background: var(--accent);
	}
	/* Brief M10a-fix A2: menor peso visual que antes (sin borde, más
	   pequeño) y un halo/sombra difuso propio en vez de una chapa opaca, para
	   que se lea sobre cualquier carátula sin tapar caras/logos del póster. */
	.badge {
		position: absolute;
		top: 8px;
		left: 8px;
		z-index: 1;
		font-family: var(--mono);
		font-size: 9.5px;
		font-weight: 500;
		letter-spacing: 0.07em;
		background: rgba(16, 12, 9, 0.5);
		backdrop-filter: blur(3px);
		border-radius: 5px;
		padding: 2px 7px;
		color: var(--text);
		text-shadow: 0 1px 3px rgba(0, 0, 0, 0.8);
		box-shadow: 0 1px 5px rgba(0, 0, 0, 0.45);
	}

	/* Paleta de respaldo sin carátula (Brief M10a §5), calcada del mockup. */
	.p1 {
		background: linear-gradient(150deg, #59331a, #241206 60%, #0e0804);
	}
	.p2 {
		background: linear-gradient(150deg, #173a33, #0a1a17 60%, #060d0b);
	}
	.p3 {
		background: linear-gradient(150deg, #3a1f3f, #170b1c 60%, #0a0510);
	}
	.p4 {
		background: linear-gradient(150deg, #1d3050, #0c1424 60%, #05080f);
	}
	.p5 {
		background: linear-gradient(150deg, #5c1f1f, #230a0a 60%, #0f0404);
	}
	.p6 {
		background: linear-gradient(150deg, #4d4416, #1e1a06 60%, #0d0b03);
	}
	.p7 {
		background: linear-gradient(150deg, #20423b, #0d1d1a 60%, #060f0d);
	}
	.p8 {
		background: linear-gradient(150deg, #513a5e, #1f1426 60%, #0e0813);
	}

	/* Deseos (Brief M10a §5). */
	.card.wish {
		border-style: dashed;
		border-color: #42301b;
		background: rgba(27, 21, 16, 0.5);
	}
	.card.wish .poster {
		opacity: 0.55;
	}
	.wish-tag {
		position: absolute;
		top: 10px;
		right: 10px;
		z-index: 2;
		font-size: 11px;
		background: rgba(242, 169, 59, 0.15);
		border: 1px solid rgba(242, 169, 59, 0.4);
		color: var(--accent);
		border-radius: 6px;
		padding: 3px 8px;
		font-weight: 500;
	}
</style>
