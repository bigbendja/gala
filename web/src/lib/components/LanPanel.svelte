<script lang="ts">
	// Panel del Modo LAN (Brief M7 §4): activar/desactivar, QR + URL para
	// conectar un móvil por wifi, y PIN opcional. Solo se abre en el PC.
	import { ApiError, getLanStatus, setLanEnabled, setLanPin } from '$lib/api/client';
	import type { LanStatus } from '$lib/api/types';
	import QrCode from './QrCode.svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
		/** Avisa al layout para que la píldora de la topbar refleje el estado. */
		onChanged: (enabled: boolean) => void;
	}
	let { open, onClose, onChanged }: Props = $props();

	let status = $state<LanStatus | null>(null);
	let loadError = $state('');
	let busy = $state(false);
	let actionError = $state('');
	let confirmingEnable = $state(false);
	let editingPin = $state(false);
	let pinInput = $state('');
	let chosenIp = $state('');
	let copied = $state(false);

	const pinValid = $derived(/^\d{4}$/.test(pinInput));
	const url = $derived(
		status && chosenIp ? `http://${chosenIp}:${status.port}` : ''
	);

	$effect(() => {
		if (open) void load();
	});

	async function load() {
		loadError = '';
		try {
			setStatus(await getLanStatus());
		} catch (err) {
			status = null;
			loadError =
				err instanceof ApiError
					? err.message
					: 'No se pudo consultar el Modo LAN. Inténtalo de nuevo.';
		}
	}

	function setStatus(s: LanStatus) {
		status = s;
		// Conserva la IP elegida a mano mientras siga siendo candidata.
		if (!s.candidates.some((c) => c.ip === chosenIp)) chosenIp = s.lan_ip;
	}

	async function run(action: () => Promise<LanStatus>) {
		if (busy) return;
		busy = true;
		actionError = '';
		try {
			setStatus(await action());
			onChanged(status?.enabled ?? false);
		} catch (err) {
			actionError =
				err instanceof ApiError ? err.message : 'No se pudo completar la acción. Inténtalo de nuevo.';
		} finally {
			busy = false;
		}
	}

	async function enable() {
		confirmingEnable = false;
		await run(() => setLanEnabled(true));
	}

	async function disable() {
		await run(() => setLanEnabled(false));
	}

	async function savePin() {
		if (!pinValid) return;
		await run(() => setLanPin(pinInput));
		if (!actionError) {
			editingPin = false;
			pinInput = '';
		}
	}

	async function removePin() {
		await run(() => setLanPin(''));
		editingPin = false;
		pinInput = '';
	}

	async function copyUrl() {
		try {
			await navigator.clipboard.writeText(url);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			// sin portapapeles (contexto no seguro): la URL sigue visible para teclearla
		}
	}

	function close() {
		confirmingEnable = false;
		editingPin = false;
		pinInput = '';
		actionError = '';
		onClose();
	}

	function onPinInput(e: Event) {
		const el = e.currentTarget as HTMLInputElement;
		pinInput = el.value.replace(/\D/g, '').slice(0, 4);
		el.value = pinInput;
	}

	function handleOverlayClick(e: MouseEvent) {
		if (e.target === e.currentTarget) close();
	}
	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') close();
	}
</script>

<svelte:window onkeydown={open ? handleKeydown : undefined} />

{#if open}
	<div
		class="overlay"
		role="dialog"
		aria-modal="true"
		aria-labelledby="lan-t"
		tabindex="-1"
		onclick={handleOverlayClick}
		onkeydown={handleKeydown}
	>
		<div class="modal">
			<div class="modal-head">
				<h3 id="lan-t">Modo LAN</h3>
				<p>Ve tu galería en el móvil o la tele de casa, por wifi, sin instalar nada.</p>
			</div>

			<div class="modal-body">
				{#if loadError}
					<p class="error" role="alert">{loadError}</p>
				{:else if !status}
					<p class="muted">Cargando…</p>
				{:else if !status.enabled}
					<div class="off">
						<span class="state off-dot">●</span> Desactivado: solo este PC puede ver GALA.
					</div>

					{#if confirmingEnable}
						<div class="warn" role="alert">
							<strong>Antes de activarlo</strong>
							<p>
								Los demás dispositivos conectados a tu wifi podrán ver tu galería y reproducir tus
								vídeos. No podrán cambiar ajustes, carpetas ni borrar nada: eso solo se puede desde
								este PC. Puedes poner un PIN para que pidan clave al entrar.
							</p>
						</div>
						<div class="actions">
							<button class="btn-add" disabled={busy} onclick={enable}>
								{busy ? 'Activando…' : 'Entendido, activar'}
							</button>
							<button class="btn-ghost" disabled={busy} onclick={() => (confirmingEnable = false)}>
								Cancelar
							</button>
						</div>
					{:else}
						<div class="actions">
							<button class="btn-add" onclick={() => (confirmingEnable = true)}>
								Activar Modo LAN
							</button>
							<button class="btn-ghost" onclick={close}>Cerrar</button>
						</div>
					{/if}
				{:else}
					<div class="on"><span class="state on-dot">●</span> Activo en tu red wifi</div>

					{#if status.lan_ip}
						<div class="qr-wrap">
							<div class="qr-box"><QrCode value={url} /></div>
							<div class="qr-side">
								<p class="step">En el móvil, abre la cámara y apunta al código.</p>
								<p class="or">O escribe esta dirección en el navegador:</p>
								<div class="url-row">
									<code class="url">{url}</code>
									<button class="link-btn" onclick={copyUrl}>{copied ? 'Copiada' : 'Copiar'}</button>
								</div>
								<p class="hint">El móvil tiene que estar en la misma wifi que este PC.</p>
							</div>
						</div>

						{#if status.candidates.length > 1}
							<div class="cands">
								<span class="muted">¿No conecta? Prueba con otra red de este PC:</span>
								{#each status.candidates as c (c.ip)}
									<button
										class="chip"
										class:sel={c.ip === chosenIp}
										onclick={() => (chosenIp = c.ip)}
										title={c.interface}
									>
										{c.ip}{c.virtual ? ' (virtual)' : ''}
									</button>
								{/each}
							</div>
						{/if}
					{:else}
						<div class="warn" role="alert">
							<strong>No encuentro tu red</strong>
							<p>
								Conecta este PC a la wifi (o por cable al router) y vuelve a abrir este panel para
								ver el código QR.
							</p>
						</div>
					{/if}

					<section class="pin-sec">
						<h4>PIN de acceso</h4>
						{#if editingPin}
							<form
								class="pin-form"
								onsubmit={(e) => {
									e.preventDefault();
									void savePin();
								}}
							>
								<label class="sr-only" for="lan-pin">PIN de 4 dígitos</label>
								<input
									id="lan-pin"
									class="pin"
									type="password"
									inputmode="numeric"
									autocomplete="off"
									maxlength="4"
									placeholder="4 dígitos"
									value={pinInput}
									oninput={onPinInput}
								/>
								<button class="btn-add small" type="submit" disabled={!pinValid || busy}>Guardar</button>
								<button
									class="btn-ghost small"
									type="button"
									onclick={() => {
										editingPin = false;
										pinInput = '';
									}}>Cancelar</button
								>
							</form>
						{:else if status.pin_set}
							<p class="muted">Activado: los demás dispositivos tienen que teclearlo. Este PC no lo necesita.</p>
							<div class="actions row">
								<button class="btn-ghost small" disabled={busy} onclick={() => (editingPin = true)}>Cambiar</button>
								<button class="btn-ghost small" disabled={busy} onclick={removePin}>Quitar PIN</button>
							</div>
						{:else}
							<p class="muted">Sin PIN: cualquiera en tu wifi puede entrar.</p>
							<div class="actions row">
								<button class="btn-ghost small" onclick={() => (editingPin = true)}>Poner PIN</button>
							</div>
						{/if}
					</section>

					<p class="hint">
						Desde otros dispositivos solo se puede ver la galería, reproducir y guardar el progreso.
						Ajustes, carpetas y claves se manejan solo desde este PC. La conexión por wifi va sin
						cifrar (HTTP): úsalo en una red de confianza.
					</p>

					<div class="actions">
						<button class="btn-ghost" disabled={busy} onclick={disable}>
							{busy ? 'Desactivando…' : 'Desactivar Modo LAN'}
						</button>
						<button class="btn-ghost" onclick={close}>Cerrar</button>
					</div>
				{/if}

				{#if actionError}
					<p class="error" role="alert">{actionError}</p>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(8, 6, 4, 0.78);
		backdrop-filter: blur(6px);
		z-index: 100;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 20px;
	}
	.modal {
		width: min(560px, 100%);
		max-height: 90vh;
		overflow-y: auto;
		background: var(--surface);
		border: 1px solid #3d2d1a;
		border-radius: 20px;
		box-shadow: 0 40px 100px rgba(0, 0, 0, 0.7);
	}
	.modal-head {
		padding: 24px 28px 0;
	}
	.modal-head h3 {
		font-family: var(--display);
		font-size: 24px;
		font-weight: 800;
		margin-bottom: 6px;
	}
	.modal-head p,
	.muted {
		color: var(--muted);
		font-size: 14px;
	}
	.modal-body {
		padding: 18px 28px 26px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.on,
	.off {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 14px;
		font-weight: 500;
	}
	.on {
		color: var(--ok);
	}
	.off {
		color: var(--muted);
	}
	.state {
		font-size: 11px;
	}
	.on-dot {
		color: var(--ok);
		text-shadow: 0 0 8px var(--ok);
	}
	.off-dot {
		color: var(--off);
	}
	.warn {
		border: 1px solid rgba(242, 169, 59, 0.4);
		background: rgba(242, 169, 59, 0.08);
		border-radius: 12px;
		padding: 12px 14px;
		font-size: 13.5px;
	}
	.warn strong {
		color: var(--accent);
	}
	.warn p {
		margin-top: 4px;
		color: var(--text);
	}
	.qr-wrap {
		display: flex;
		gap: 20px;
		align-items: center;
	}
	.qr-box {
		flex: 0 0 200px;
		width: 200px;
		background: #fff;
		border-radius: 12px;
		padding: 4px;
	}
	.qr-side {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.step {
		font-weight: 500;
		font-size: 14.5px;
	}
	.or {
		color: var(--muted);
		font-size: 13px;
	}
	.url-row {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
	}
	.url {
		font-family: var(--mono);
		font-size: 14px;
		color: var(--accent);
		word-break: break-all;
	}
	.cands {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		font-size: 12.5px;
	}
	.chip {
		font-family: var(--mono);
		font-size: 12px;
		border: 1px solid var(--line);
		border-radius: 99px;
		padding: 5px 11px;
		color: var(--muted);
	}
	.chip.sel {
		color: var(--text);
		border-color: var(--accent);
		background: rgba(242, 169, 59, 0.1);
	}
	.pin-sec {
		border-top: 1px solid var(--line);
		padding-top: 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.pin-sec h4 {
		font-family: var(--display);
		font-size: 16px;
		font-weight: 700;
	}
	.pin-form {
		display: flex;
		gap: 8px;
		align-items: center;
		flex-wrap: wrap;
	}
	.pin {
		width: 130px;
		font-family: var(--mono);
		font-size: 16px;
		letter-spacing: 0.3em;
		padding: 10px 12px;
		background: var(--bg);
		border: 1.5px solid #4d3a22;
		border-radius: 10px;
		color: var(--text);
		outline: none;
	}
	.pin:focus {
		border-color: var(--accent);
	}
	.hint {
		color: var(--muted);
		font-size: 12.5px;
	}
	.error {
		color: var(--bad);
		font-size: 13.5px;
	}
	.actions {
		display: flex;
		gap: 12px;
	}
	.actions.row {
		gap: 8px;
	}
	.actions .btn-add {
		flex: 1;
		justify-content: center;
		padding: 13px;
	}
	.actions .btn-ghost {
		padding: 13px 22px;
	}
	.small {
		padding: 9px 16px !important;
		font-size: 13.5px;
		flex: none !important;
	}
	.btn-add {
		display: flex;
		align-items: center;
		background: var(--accent);
		color: #1a1207;
		font-weight: 600;
		border-radius: 10px;
		font-size: 14px;
		transition: 0.15s;
	}
	.btn-add:hover:not(:disabled) {
		background: #ffbf58;
	}
	.btn-add:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.btn-ghost {
		border-radius: 10px;
		border: 1px solid var(--line);
		color: var(--text);
		background: rgba(255, 255, 255, 0.03);
		font-weight: 500;
	}
	.btn-ghost:hover:not(:disabled) {
		background: var(--surface-2);
	}
	.btn-ghost:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.link-btn {
		color: var(--accent);
		font-size: 13px;
		text-decoration: underline;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}

	@media (max-width: 520px) {
		.modal-head {
			padding: 20px 18px 0;
		}
		.modal-body {
			padding: 16px 18px 22px;
		}
		.qr-wrap {
			flex-direction: column;
			align-items: stretch;
		}
		.qr-box {
			flex: none;
			width: min(240px, 100%);
			align-self: center;
		}
		.actions {
			flex-direction: column;
		}
	}
</style>
