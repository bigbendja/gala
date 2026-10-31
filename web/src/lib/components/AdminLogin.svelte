<script lang="ts">
	// Acceso de administrador desde otro dispositivo de la red (M-Servidor,
	// mockup docs/briefs/GALA_Mockup_Login.html). En el propio equipo no se
	// pide: esta pantalla solo la ve un cliente de red.
	import { ApiError, adminLogin } from '$lib/api/client';
	import type { AuthStatus } from '$lib/api/types';

	interface Props {
		status: AuthStatus | null;
		/** Se llama al iniciar sesión (el layout recarga sus datos). */
		onDone: () => void;
	}
	let { status, onDone }: Props = $props();

	let password = $state('');
	let busy = $state(false);
	let error = $state('');

	// Qué mostrar según el estado real del servidor: sin contraseña puesta o
	// con la administración de red apagada no hay nada que teclear.
	const blocked = $derived(
		status && !status.configured
			? 'La administración desde la red no está configurada. Pon una contraseña desde el equipo o por SSH.'
			: status && !status.network_admin
				? 'La administración desde la red está desactivada. Actívala desde el equipo donde corre GALA.'
				: ''
	);

	async function submit(e: Event) {
		e.preventDefault();
		if (!password || busy) return;
		busy = true;
		error = '';
		try {
			await adminLogin(password);
			password = '';
			onDone();
		} catch (err) {
			error =
				err instanceof ApiError
					? err.message
					: 'No se pudo comprobar la contraseña. Inténtalo de nuevo.';
			password = '';
		} finally {
			busy = false;
		}
	}
</script>

<main class="stage">
	<form class="card" onsubmit={submit}>
		<p class="brand">GALA<span class="dot">.</span></p>
		<p class="sub">Administración</p>

		{#if blocked}
			<p class="blocked" role="status">{blocked}</p>
		{:else}
			{#if error}
				<p class="err" role="alert">{error}</p>
			{/if}
			<label class="fld" for="admin-pw">Contraseña</label>
			<!-- svelte-ignore a11y_autofocus -->
			<input
				id="admin-pw"
				class="input"
				type="password"
				autocomplete="current-password"
				autofocus
				bind:value={password}
				disabled={busy}
			/>
			<button class="btn btn-primary go" type="submit" disabled={!password || busy}>
				{busy ? 'Comprobando…' : 'Entrar'}
			</button>
		{/if}

		<p class="hint">
			Solo para administrar desde otro dispositivo de la red. En el propio servidor (localhost) no se pide
			contraseña. ¿La olvidaste? Resetéala por SSH con
			<span class="mono">gala admin set-password</span>.
		</p>
	</form>
</main>

<style>
	.stage {
		min-height: 70vh;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 32px 16px;
		background: radial-gradient(1200px 400px at 50% -10%, #241a11 0%, transparent 60%);
	}
	.card {
		width: 100%;
		max-width: 320px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: var(--r);
		padding: 28px 24px;
		box-shadow: 0 20px 60px rgba(0, 0, 0, 0.45);
	}
	.brand {
		font-family: var(--display);
		font-weight: 800;
		font-size: 28px;
		letter-spacing: 0.02em;
		margin: 0;
	}
	.dot {
		color: var(--accent);
	}
	.sub {
		color: var(--muted);
		font-size: 13px;
		margin: 2px 0 22px;
	}
	.fld {
		display: block;
		font-size: 12px;
		color: var(--muted);
		margin: 0 0 6px;
	}
	.input {
		width: 100%;
		background: var(--surface-2);
		border: 1px solid var(--line);
		color: var(--text);
		border-radius: 10px;
		padding: 11px 12px;
		font-size: 14px;
		outline: none;
	}
	.input:focus {
		border-color: var(--accent-deep);
		box-shadow: 0 0 0 3px rgba(242, 169, 59, 0.15);
	}
	.go {
		width: 100%;
		margin-top: 16px;
	}
	.err {
		background: rgba(225, 96, 79, 0.12);
		border: 1px solid rgba(225, 96, 79, 0.4);
		color: #f0a99d;
		font-size: 12.5px;
		border-radius: 8px;
		padding: 8px 10px;
		margin: 0 0 14px;
	}
	.blocked {
		font-size: 13.5px;
		margin: 0;
		line-height: 1.5;
	}
	.hint {
		font-size: 11.5px;
		color: var(--muted);
		margin: 16px 0 0;
		line-height: 1.45;
	}
	.mono {
		font-family: var(--mono);
	}
</style>
