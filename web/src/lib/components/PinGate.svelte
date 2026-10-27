<script lang="ts">
	// Pantalla que ve un móvil/tablet de la red cuando el PC tiene PIN
	// (Brief M7 §3). El PIN se teclea: nunca viaja en el QR ni en la URL.
	import { ApiError, lanAuth } from '$lib/api/client';

	interface Props {
		/** Se llama al acertar el PIN (el layout recarga sus datos). */
		onUnlocked: () => void;
	}
	let { onUnlocked }: Props = $props();

	let pin = $state('');
	let busy = $state(false);
	let error = $state('');

	const complete = $derived(/^\d{4}$/.test(pin));

	function onInput(e: Event) {
		// Solo dígitos, máximo 4 (el teclado numérico del móvil ya ayuda).
		pin = (e.currentTarget as HTMLInputElement).value.replace(/\D/g, '').slice(0, 4);
		(e.currentTarget as HTMLInputElement).value = pin;
		error = '';
	}

	async function submit(e: Event) {
		e.preventDefault();
		if (!complete || busy) return;
		busy = true;
		error = '';
		try {
			await lanAuth(pin);
			pin = '';
			onUnlocked();
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'No se pudo comprobar el PIN. Inténtalo de nuevo.';
			pin = '';
		} finally {
			busy = false;
		}
	}
</script>

<main class="gate">
	<form class="card" onsubmit={submit}>
		<svg width="40" height="40" viewBox="0 0 26 26" fill="none" aria-hidden="true">
			<circle cx="13" cy="13" r="11.5" stroke="#F2A93B" stroke-width="2.4" />
			<path d="M10.5 8.5L18 13l-7.5 4.5v-9z" fill="#F2A93B" />
		</svg>
		<h1>GALA</h1>
		<p class="lead">Esta galería tiene PIN. Escríbelo para entrar.</p>

		<label class="sr-only" for="pin">PIN de 4 dígitos</label>
		<input
			id="pin"
			class="pin"
			type="password"
			inputmode="numeric"
			pattern="[0-9]*"
			autocomplete="off"
			maxlength="4"
			placeholder="••••"
			value={pin}
			oninput={onInput}
			disabled={busy}
		/>

		{#if error}
			<p class="error" role="alert">{error}</p>
		{/if}

		<button class="go" type="submit" disabled={!complete || busy}>
			{busy ? 'Comprobando…' : 'Entrar'}
		</button>
	</form>
</main>

<style>
	.gate {
		min-height: 100vh;
		min-height: 100dvh;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 20px 16px;
	}
	.card {
		width: min(360px, 100%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 14px;
		padding: 32px 24px 28px;
		background: var(--surface);
		border: 1px solid #3d2d1a;
		border-radius: 20px;
		box-shadow: 0 40px 100px rgba(0, 0, 0, 0.6);
		text-align: center;
	}
	h1 {
		font-family: var(--display);
		font-weight: 800;
		font-size: 26px;
		letter-spacing: 0.04em;
		margin: -4px 0 0;
	}
	.lead {
		color: var(--muted);
		font-size: 14.5px;
	}
	.pin {
		width: 100%;
		text-align: center;
		font-family: var(--mono);
		font-size: 28px;
		letter-spacing: 0.5em;
		padding: 14px 8px 14px 18px; /* compensa el espaciado final del último dígito */
		background: var(--bg);
		border: 1.5px solid #4d3a22;
		border-radius: 14px;
		color: var(--text);
		outline: none;
	}
	.pin:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px rgba(242, 169, 59, 0.14);
	}
	.error {
		color: var(--bad);
		font-size: 13.5px;
	}
	.go {
		width: 100%;
		padding: 13px;
		background: var(--accent);
		color: #1a1207;
		font-weight: 600;
		font-size: 15px;
		border-radius: 10px;
		transition: 0.15s;
	}
	.go:hover:not(:disabled) {
		background: #ffbf58;
	}
	.go:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>
