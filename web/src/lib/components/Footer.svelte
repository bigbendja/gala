<script lang="ts">
	import { page } from '$app/state';
</script>

<footer>
	<span>GALA v0.1 · código abierto</span><span class="f-dot"></span>
	<span>Metadatos: TMDB (offline listo)</span><span class="f-dot"></span>
	<span>Tu contenido nunca sale de tus dispositivos</span>
	<span class="right">{page.url.host}</span>
</footer>

<style>
	footer {
		border-top: 1px solid var(--line);
		padding: 22px 28px;
		display: flex;
		gap: 16px;
		align-items: center;
		color: var(--muted);
		font-size: 12.5px;
		max-width: 1236px;
		margin: 0 auto;
		flex-wrap: wrap;
	}
	footer .f-dot {
		width: 4px;
		height: 4px;
		border-radius: 50%;
		background: var(--line);
	}
	footer .right {
		margin-left: auto;
		font-family: var(--mono);
		font-size: 11.5px;
	}
</style>
