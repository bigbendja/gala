<script lang="ts">
	import { page } from '$app/state';
</script>

<svelte:head>
	<title>GALA — {page.status}</title>
</svelte:head>

<div class="state-page">
	<p class="code">{page.status}</p>
	<p>{page.error?.message ?? 'Algo ha ido mal.'}</p>
	<a class="link" href="/">← Volver a la galería</a>
</div>

<style>
	.state-page {
		max-width: 560px;
		margin: 15vh auto;
		text-align: center;
		padding: 0 28px;
	}
	.code {
		font-family: var(--display);
		font-size: 56px;
		font-weight: 800;
		color: var(--accent);
	}
	.link {
		display: inline-block;
		margin-top: 18px;
		color: var(--accent);
		text-decoration: none;
		font-weight: 500;
	}
</style>
