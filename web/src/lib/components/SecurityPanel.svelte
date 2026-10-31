<script lang="ts">
	// Ajustes › Seguridad (M-Servidor, mockup docs/briefs/GALA_Mockup_Login.html):
	// contraseña de administrador, sesiones abiertas y permiso de administrar
	// desde la red. Todo lo que cambia la exposición (el interruptor) solo se
	// toca desde el propio equipo.
	import { onMount } from 'svelte';
	import {
		adminLogout,
		adminLogoutAll,
		changeAdminPassword,
		getAuthStatus,
		listAuthSessions,
		setCsrfToken,
		setNetworkAdmin
	} from '$lib/api/client';
	import type { AuthSession, AuthStatus } from '$lib/api/types';
	import { errorMessage, formatDate } from '$lib/format';

	let status = $state<AuthStatus | null>(null);
	let sessions = $state<AuthSession[]>([]);
	let loadError = $state('');

	let current = $state('');
	let next = $state('');
	let repeat = $state('');
	let saving = $state(false);
	let saveError = $state('');
	let saveOk = $state('');

	let confirmingLogoutAll = $state(false);
	let busyLogoutAll = $state(false);
	let toggleBusy = $state(false);
	let toggleError = $state('');

	const configured = $derived(status?.configured ?? false);
	const local = $derived(status?.is_localhost ?? true);
	const networkAdmin = $derived(status?.network_admin ?? true);

	const mismatch = $derived(repeat !== '' && next !== repeat);
	const tooShort = $derived(next !== '' && next.length < 8);
	const canSave = $derived(
		next.length >= 8 && next === repeat && (!configured || current !== '') && !saving
	);

	async function load() {
		try {
			status = await getAuthStatus();
			setCsrfToken(status.csrf);
			sessions = status.configured ? await listAuthSessions() : [];
			loadError = '';
		} catch (err) {
			loadError = errorMessage(err, 'No se pudo leer la seguridad.');
		}
	}
	onMount(load);

	async function save(e: Event) {
		e.preventDefault();
		if (!canSave) return;
		saving = true;
		saveError = '';
		saveOk = '';
		try {
			await changeAdminPassword(current, next);
			saveOk = configured ? 'Contraseña cambiada. Las demás sesiones se han cerrado.' : 'Contraseña puesta.';
			current = next = repeat = '';
			await load();
		} catch (err) {
			saveError = errorMessage(err, 'No se pudo guardar la contraseña.');
		} finally {
			saving = false;
		}
	}

	async function logoutAll() {
		busyLogoutAll = true;
		try {
			await adminLogoutAll();
			confirmingLogoutAll = false;
			await load();
		} catch (err) {
			loadError = errorMessage(err, 'No se pudieron cerrar las sesiones.');
		} finally {
			busyLogoutAll = false;
		}
	}

	async function logoutHere() {
		try {
			await adminLogout();
			window.location.assign('/');
		} catch (err) {
			loadError = errorMessage(err, 'No se pudo cerrar la sesión.');
		}
	}

	async function toggleNetwork() {
		if (!local || toggleBusy) return;
		toggleBusy = true;
		toggleError = '';
		try {
			await setNetworkAdmin(!networkAdmin);
			await load();
		} catch (err) {
			toggleError = errorMessage(err, 'No se pudo cambiar el ajuste.');
		} finally {
			toggleBusy = false;
		}
	}

	function sessionLabel(s: AuthSession): string {
		return s.current ? 'Este dispositivo' : s.device;
	}
</script>

<section class="card" id="seguridad">
	<h2>Seguridad</h2>
	<p class="desc">
		Contraseña de administración para acceder desde la red. El propio equipo donde corre GALA siempre tiene
		acceso.
	</p>

	{#if loadError}<p class="error" role="alert">{loadError}</p>{/if}

	{#if status && !configured && !local}
		<p class="muted">
			La administración desde la red no está configurada. Pon una contraseña desde el equipo o por SSH.
		</p>
	{:else if status}
		<form class="form" onsubmit={save}>
			<div class="row">
				{#if configured}
					<input
						class="input"
						type="password"
						autocomplete="current-password"
						placeholder="Contraseña actual"
						aria-label="Contraseña actual"
						bind:value={current}
					/>
				{/if}
				<input
					class="input"
					type="password"
					autocomplete="new-password"
					placeholder={configured ? 'Nueva contraseña' : 'Contraseña de administrador'}
					aria-label="Nueva contraseña"
					bind:value={next}
				/>
			</div>
			<div class="row">
				<input
					class="input"
					type="password"
					autocomplete="new-password"
					placeholder="Repetir contraseña"
					aria-label="Repetir contraseña"
					bind:value={repeat}
				/>
				<button class="btn btn-secondary" type="submit" disabled={!canSave}>
					{saving ? 'Guardando…' : configured ? 'Cambiar contraseña' : 'Poner contraseña'}
				</button>
			</div>
			{#if tooShort}<p class="hint">Mínimo 8 caracteres.</p>{/if}
			{#if mismatch}<p class="error">Las dos contraseñas no coinciden.</p>{/if}
			{#if saveError}<p class="error" role="alert">{saveError}</p>{/if}
			{#if saveOk}<p class="ok" role="status">{saveOk}</p>{/if}
		</form>

		{#if configured}
			<div class="sessions">
				{#each sessions as s (s.id)}
					<div class="sess">
						<div>
							<div>{sessionLabel(s)}</div>
							<div class="mono">{s.ip} · {s.device}</div>
						</div>
						<div class="sess-right">
							<span class="meta">{s.current ? 'ahora' : formatDate(s.last_seen)}</span>
							{#if s.current}<span class="pill this">activa</span>{/if}
						</div>
					</div>
				{:else}
					<p class="muted">No hay ninguna sesión de red abierta.</p>
				{/each}
			</div>

			<div class="actions">
				{#if confirmingLogoutAll}
					<span class="confirm">¿Cerrar todas las sesiones?</span>
					<button class="btn btn-quiet-danger" onclick={logoutAll} disabled={busyLogoutAll}>Sí, cerrar</button>
					<button class="btn btn-secondary" onclick={() => (confirmingLogoutAll = false)}>No</button>
				{:else}
					<button class="btn btn-secondary" onclick={() => (confirmingLogoutAll = true)} disabled={sessions.length === 0}>
						Cerrar todas las sesiones
					</button>
				{/if}
				{#if !local}
					<button class="btn btn-secondary" onclick={logoutHere}>Cerrar sesión en este dispositivo</button>
				{/if}
			</div>

			<div class="toggle">
				<div class="t">
					Permitir administrar desde la red
					<small>
						Con contraseña. Si lo desactivas, GALA solo se podrá administrar desde el propio equipo donde
						corre.
					</small>
					{#if !local}<small>Este ajuste solo se cambia desde el equipo donde corre GALA.</small>{/if}
				</div>
				<button
					class="switch"
					class:on={networkAdmin}
					role="switch"
					aria-checked={networkAdmin}
					aria-label="Permitir administrar desde la red"
					title={local ? '' : 'Solo se cambia desde el equipo donde corre GALA'}
					disabled={!local || toggleBusy}
					onclick={toggleNetwork}
				>
					<span class="knob"></span>
				</button>
			</div>
			{#if toggleError}<p class="error" role="alert">{toggleError}</p>{/if}
		{/if}

		<p class="hint">
			La contraseña viaja sin cifrar por tu red local: úsala solo en redes de confianza y no expongas GALA a
			Internet.
		</p>
	{:else if !loadError}
		<p class="muted">Cargando…</p>
	{/if}
</section>

<style>
	.card {
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		padding: 20px 22px;
		margin-top: 22px;
	}
	h2 {
		font-family: var(--display);
		font-size: 17px;
		font-weight: 600;
		margin: 0 0 2px;
	}
	.desc {
		color: var(--muted);
		font-size: 12.5px;
		margin: 0 0 16px;
	}
	.muted,
	.hint {
		color: var(--muted);
		font-size: 13px;
	}
	.hint {
		margin-top: 12px;
		font-size: 12px;
	}
	.error {
		color: var(--bad);
		font-size: 13.5px;
		margin-top: 8px;
	}
	.ok {
		color: var(--ok);
		font-size: 13.5px;
		margin-top: 8px;
	}
	.form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.row {
		display: flex;
		gap: 10px;
		flex-wrap: wrap;
	}
	.input {
		flex: 1;
		min-width: 170px;
		background: var(--bg);
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 9px 12px;
		color: var(--text);
		font-size: 13.5px;
	}
	.input:focus {
		outline: none;
		border-color: var(--accent);
	}
	.sessions {
		border-top: 1px solid var(--line);
		margin-top: 16px;
		padding-top: 6px;
	}
	.sess {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
		padding: 9px 0;
		border-bottom: 1px solid var(--line);
		font-size: 13px;
	}
	.sess:last-child {
		border-bottom: 0;
	}
	.sess-right {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.meta {
		color: var(--muted);
		font-size: 11.5px;
	}
	.mono {
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--muted);
		overflow-wrap: anywhere;
	}
	.pill {
		font-size: 10.5px;
		font-family: var(--mono);
		text-transform: uppercase;
		letter-spacing: 0.1em;
		padding: 3px 8px;
		border-radius: 999px;
		border: 1px solid var(--line);
		color: var(--muted);
	}
	.pill.this {
		color: var(--ok);
		border-color: rgba(111, 207, 142, 0.4);
	}
	.actions {
		display: flex;
		gap: 10px;
		flex-wrap: wrap;
		align-items: center;
		margin-top: 14px;
	}
	.confirm {
		font-size: 13px;
		color: var(--warn);
	}
	.toggle {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 14px;
		border-top: 1px solid var(--line);
		margin-top: 16px;
		padding-top: 14px;
	}
	.t {
		font-size: 13px;
	}
	.t small {
		display: block;
		color: var(--muted);
		font-size: 11.5px;
		margin-top: 2px;
	}
	.switch {
		position: relative;
		flex-shrink: 0;
		width: 48px;
		height: 28px;
		border-radius: 99px;
		background: var(--off);
		transition: 0.2s;
	}
	.switch.on {
		background: var(--accent);
	}
	.switch:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: #1a1205;
		transition: 0.2s;
	}
	.switch.on .knob {
		transform: translateX(20px);
	}
	@media (prefers-reduced-motion: reduce) {
		.switch,
		.knob {
			transition: none;
		}
	}
</style>
