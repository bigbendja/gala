<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import {
		addFolder,
		getHomeSettings,
		healthSummary,
		identifyStatus,
		listFolders,
		putHomeSettings,
		putTMDBSettings,
		removeFolder,
		scanStatus,
		startHealthCheck,
		startScan
	} from '$lib/api/client';
	import type { FolderInfo, HealthSummary, IdentifyStatus, ScanStatus } from '$lib/api/types';
	import { errorMessage, formatDate, friendlyIdentifyError } from '$lib/format';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// --- Carpetas ---------------------------------------------------------
	let folders = $state<FolderInfo[]>(untrack(() => data.folders ?? []));
	let newFolderPath = $state('');
	let addFolderBusy = $state(false);
	let addFolderError = $state('');
	let confirmingRemove = $state<string | null>(null);
	let removeBusyPaths = $state(new Set<string>());

	async function reloadFolders() {
		try {
			folders = await listFolders();
		} catch {
			// se deja la lista como estaba; el usuario puede reintentar la acción
		}
	}

	async function handleAddFolder() {
		const path = newFolderPath.trim();
		if (!path || addFolderBusy) return;
		addFolderBusy = true;
		addFolderError = '';
		try {
			await addFolder(path);
			newFolderPath = '';
			await reloadFolders();
		} catch (err) {
			addFolderError = errorMessage(err, 'No se pudo añadir la carpeta');
		} finally {
			addFolderBusy = false;
		}
	}

	async function handleRemoveFolder(path: string) {
		confirmingRemove = null;
		removeBusyPaths.add(path);
		removeBusyPaths = new Set(removeBusyPaths);
		try {
			await removeFolder(path);
			await reloadFolders();
		} catch {
			// la carpeta sigue en la lista; reintentar es seguro (idempotente)
		} finally {
			removeBusyPaths.delete(path);
			removeBusyPaths = new Set(removeBusyPaths);
		}
	}

	// --- Inicio (Sistema de Diseño GALA §5: el hero de la Home se puede ocultar) --
	let heroEnabled = $state<boolean | null>(null);
	let heroBusy = $state(false);
	let heroError = $state('');
	onMount(async () => {
		try {
			heroEnabled = (await getHomeSettings()).hero;
		} catch {
			heroError = 'No se pudo leer esta preferencia.';
		}
	});
	async function toggleHero() {
		if (heroEnabled === null || heroBusy) return;
		heroBusy = true;
		heroError = '';
		try {
			heroEnabled = (await putHomeSettings({ hero: !heroEnabled })).hero;
		} catch (err) {
			heroError = errorMessage(err, 'No se pudo guardar');
		} finally {
			heroBusy = false;
		}
	}

	// --- Escaneo ------------------------------------------------------------
	let scan = $state<ScanStatus | null>(untrack(() => data.scan));
	let scanStartBusy = $state(false);
	let scanPollHandle: ReturnType<typeof setInterval> | undefined;

	function stopScanPoll() {
		clearInterval(scanPollHandle);
		scanPollHandle = undefined;
	}

	async function pollScan() {
		try {
			scan = await scanStatus();
			if (!scan.running) {
				stopScanPoll();
				reloadFolders(); // el escaneo pudo traer contenido nuevo/reconectar volúmenes
			}
		} catch {
			stopScanPoll();
		}
	}

	function ensureScanPoll() {
		if (scanPollHandle) return;
		scanPollHandle = setInterval(pollScan, 2000);
	}

	async function handleStartScan() {
		if (scanStartBusy) return;
		scanStartBusy = true;
		try {
			await startScan();
			await pollScan();
			ensureScanPoll();
		} catch {
			// 409 = ya había uno en curso; igualmente empezamos a seguirlo
			ensureScanPoll();
		} finally {
			scanStartBusy = false;
		}
	}

	if (untrack(() => scan?.running)) ensureScanPoll();

	// --- Identificación -------------------------------------------------------
	let identify = $state<IdentifyStatus | null>(untrack(() => data.identify));
	const identifyFriendlyError = $derived(friendlyIdentifyError(identify?.last_error));

	async function reloadIdentify() {
		try {
			identify = await identifyStatus();
		} catch {
			// se deja el último estado conocido
		}
	}

	// --- Clave TMDB -------------------------------------------------------------
	let tmdbConfigured = $state(untrack(() => data.tmdb?.configured ?? false));
	let tmdbKeyInput = $state('');
	let tmdbBusy = $state(false);
	let tmdbMessage = $state<{ kind: 'ok' | 'error'; text: string } | null>(null);

	async function handleSaveTMDBKey() {
		const key = tmdbKeyInput.trim();
		if (!key || tmdbBusy) return;
		tmdbBusy = true;
		tmdbMessage = null;
		try {
			await putTMDBSettings(key);
			tmdbConfigured = true;
			tmdbKeyInput = '';
			tmdbMessage = { kind: 'ok', text: 'Clave guardada y verificada contra TMDB.' };
			await reloadIdentify(); // el last_error de una clave anterior debería desaparecer
		} catch (err) {
			tmdbMessage = { kind: 'error', text: errorMessage(err, 'No se pudo guardar la clave') };
		} finally {
			tmdbBusy = false;
		}
	}

	// --- Salud de fuentes -----------------------------------------------------
	let health = $state<HealthSummary | null>(untrack(() => data.health));
	let healthCheckBusy = $state(false);
	let healthPollHandle: ReturnType<typeof setInterval> | undefined;

	function stopHealthPoll() {
		clearInterval(healthPollHandle);
		healthPollHandle = undefined;
	}

	async function pollHealth() {
		try {
			health = await healthSummary();
			if (!health.running) stopHealthPoll();
		} catch {
			stopHealthPoll();
		}
	}

	function ensureHealthPoll() {
		if (healthPollHandle) return;
		healthPollHandle = setInterval(pollHealth, 2000);
	}

	async function handleStartHealthCheck() {
		if (healthCheckBusy) return;
		healthCheckBusy = true;
		try {
			await startHealthCheck();
			await pollHealth();
			ensureHealthPoll();
		} catch {
			ensureHealthPoll();
		} finally {
			healthCheckBusy = false;
		}
	}

	if (untrack(() => health?.running)) ensureHealthPoll();

	// --- Sistema ------------------------------------------------------------
	const sys = untrack(() => data.sys);

	$effect(() => {
		return () => {
			stopScanPoll();
			stopHealthPoll();
		};
	});
</script>

<svelte:head>
	<title>Ajustes — GALA</title>
</svelte:head>

<div class="page">
	<header>
		<h1>Ajustes</h1>
		<p class="muted">Todo lo que antes había que hacer a mano, desde aquí.</p>
	</header>

	<section class="card" id="carpetas">
		<h2>Carpetas</h2>
		{#if data.foldersError}
			<p class="error">No se pudieron cargar las carpetas: {data.foldersError}</p>
		{:else if folders.length === 0}
			<p class="muted">No hay ninguna carpeta registrada todavía.</p>
		{:else}
			<ul class="folders">
				{#each folders as folder (folder.path)}
					<li>
						<span class="dot-status" class:on={folder.connected} title={folder.connected ? 'Conectada' : 'Desconectada'}
						></span>
						<span class="path">{folder.path}</span>
						{#if confirmingRemove === folder.path}
							<span class="confirm">
								¿Quitar?
								<button class="btn-ghost small" onclick={() => handleRemoveFolder(folder.path)}>Sí</button>
								<button class="btn-ghost small" onclick={() => (confirmingRemove = null)}>No</button>
							</span>
						{:else}
							<button
								class="btn-ghost small"
								disabled={removeBusyPaths.has(folder.path)}
								onclick={() => (confirmingRemove = folder.path)}
							>
								{removeBusyPaths.has(folder.path) ? 'Quitando…' : 'Quitar'}
							</button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}

		<div class="add-folder">
			<input
				type="text"
				placeholder="Ruta de la carpeta, p. ej. E:\Peliculas"
				bind:value={newFolderPath}
				onkeydown={(e) => e.key === 'Enter' && handleAddFolder()}
			/>
			<button class="btn-add" disabled={addFolderBusy || !newFolderPath.trim()} onclick={handleAddFolder}>
				{addFolderBusy ? 'Añadiendo…' : 'Añadir carpeta'}
			</button>
		</div>
		{#if addFolderError}<p class="error">{addFolderError}</p>{/if}
		<p class="hint">Al quitar una carpeta, los items se conservan: solo se quitan sus fuentes locales.</p>
	</section>

	<section class="card" id="inicio">
		<h2>Inicio</h2>
		<div class="switch-row">
			<div>
				<p class="switch-title">Bloque destacado</p>
				<p class="muted">
					El carrusel grande de títulos que abre la pantalla de inicio. Apágalo si prefieres ir directo a las
					filas.
				</p>
			</div>
			<button
				class="switch"
				class:on={heroEnabled === true}
				role="switch"
				aria-checked={heroEnabled === true}
				aria-label="Mostrar el bloque destacado en Inicio"
				disabled={heroEnabled === null || heroBusy}
				onclick={toggleHero}
			>
				<span class="knob"></span>
			</button>
		</div>
		{#if heroError}<p class="error">{heroError}</p>{/if}
	</section>

	<section class="card">
		<h2>Escaneo</h2>
		{#if data.scanError && !scan}
			<p class="error">No se pudo cargar el estado del escaneo: {data.scanError}</p>
		{:else if scan}
			{#if scan.running}
				<p class="status running">
					Escaneando… {scan.files_seen} archivos vistos, {scan.items_new} items nuevos
				</p>
			{:else if scan.last_finished}
				<p class="muted">Último escaneo: {formatDate(scan.last_finished)}</p>
			{:else}
				<p class="muted">Todavía no se ha escaneado nada.</p>
			{/if}
			<button class="btn-add sec" disabled={scan.running || scanStartBusy} onclick={handleStartScan}>
				{scan.running ? 'Escaneando…' : 'Escanear ahora'}
			</button>
		{/if}
	</section>

	<section class="card">
		<h2>Identificación</h2>
		{#if data.identifyError && !identify}
			<p class="error">No se pudo cargar el estado: {data.identifyError}</p>
		{:else if identify}
			<div class="id-stats">
				<span><b>{identify.pending}</b> pendientes</span>
				<span><b>{identify.processed}</b> procesados</span>
				<span class:on={identify.online} class="conn-dot">{identify.online ? 'Con conexión' : 'Sin conexión'}</span>
			</div>
			{#if identifyFriendlyError}
				<p class="error">{identifyFriendlyError}</p>
			{/if}
		{/if}
	</section>

	<section class="card">
		<h2>Clave TMDB</h2>
		<p class="muted">
			{#if data.tmdbError}
				No se pudo comprobar el estado: {data.tmdbError}
			{:else if tmdbConfigured}
				Configurada. Puedes cambiarla si ha dejado de funcionar.
			{:else}
				No configurada todavía: GALA usa una clave de desarrollo por defecto que puede dejar de
				servir en cualquier momento.
			{/if}
		</p>
		<div class="add-folder">
			<input
				type="password"
				autocomplete="off"
				placeholder="Nueva clave de TMDB"
				bind:value={tmdbKeyInput}
				onkeydown={(e) => e.key === 'Enter' && handleSaveTMDBKey()}
			/>
			<button class="btn-add sec" disabled={tmdbBusy || !tmdbKeyInput.trim()} onclick={handleSaveTMDBKey}>
				{tmdbBusy ? 'Comprobando…' : 'Guardar clave'}
			</button>
		</div>
		{#if tmdbMessage}
			<p class={tmdbMessage.kind === 'ok' ? 'ok' : 'error'}>{tmdbMessage.text}</p>
		{/if}
		<p class="hint">Nunca se muestra la clave guardada, solo si hay una configurada.</p>
	</section>

	<section class="card">
		<h2>Salud de fuentes</h2>
		{#if data.healthError && !health}
			<p class="error">No se pudo cargar el resumen: {data.healthError}</p>
		{:else if health}
			<div class="health-stats">
				<span>{health.total} total</span>
				<span class="ok-text">{health.viva} vivas</span>
				<span class="warn-text">{health.lenta} lentas</span>
				<span class="bad-text">{health.caida} caídas</span>
				<span>{health.desconectada} desconectadas</span>
				<span>{health.sin_verificar} sin verificar</span>
			</div>
			<button class="btn-add sec" disabled={health.running || healthCheckBusy} onclick={handleStartHealthCheck}>
				{health.running ? 'Verificando…' : 'Verificar ahora'}
			</button>
		{/if}
	</section>

	<section class="card">
		<h2>Sistema</h2>
		{#if data.sysError || !sys}
			<p class="error">No se pudo conectar con GALA para leer la versión.</p>
		{:else}
			<div class="sys-stats">
				<span>Versión {sys.version}</span>
				<span class:ok-text={sys.ffmpeg_available} class:bad-text={!sys.ffmpeg_available}>
					ffmpeg {sys.ffmpeg_available ? 'disponible' : 'no disponible (solo direct play)'}
				</span>
			</div>
			<p class="muted">
				<a class="repo-link" href="https://github.com/bigbendja/gala" target="_blank" rel="noreferrer">
					Código fuente en GitHub ↗
				</a>
			</p>
		{/if}
	</section>
</div>

<style>
	.page {
		max-width: 760px;
		margin: 0 auto;
		padding: 34px 28px 60px;
	}
	header h1 {
		font-family: var(--display);
		font-size: 28px;
		font-weight: 800;
		margin-bottom: 8px;
	}
	.muted {
		color: var(--muted);
		font-size: 14px;
	}
	.error {
		color: var(--bad);
		font-size: 13.5px;
	}
	.ok {
		color: var(--ok);
		font-size: 13.5px;
	}
	.hint {
		color: var(--muted);
		font-size: 12.5px;
		margin-top: 10px;
	}
	.card {
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		padding: 20px 22px;
		margin-top: 22px;
	}
	.card h2 {
		font-family: var(--display);
		font-size: 17px;
		font-weight: 600;
		margin-bottom: 14px;
	}
	.folders {
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin-bottom: 16px;
	}
	.folders li {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 0;
		border-bottom: 1px solid rgba(51, 38, 25, 0.5);
	}
	.folders li:last-child {
		border-bottom: none;
	}
	.dot-status {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--bad);
		flex-shrink: 0;
	}
	.dot-status.on {
		background: var(--ok);
		box-shadow: 0 0 7px rgba(111, 207, 142, 0.5);
	}
	.path {
		flex: 1;
		font-family: var(--mono);
		font-size: 12.5px;
		overflow-wrap: anywhere;
	}
	.confirm {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 13px;
		color: var(--warn);
		white-space: nowrap;
	}
	.add-folder {
		display: flex;
		gap: 10px;
		flex-wrap: wrap;
	}
	.add-folder input {
		flex: 1;
		min-width: 200px;
		background: var(--bg);
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 9px 12px;
		color: var(--text);
		font-size: 13.5px;
	}
	.add-folder input:focus {
		outline: none;
		border-color: var(--accent);
	}
	.btn-add {
		display: inline-flex;
		align-items: center;
		background: var(--accent);
		color: #1a1207;
		font-weight: 600;
		border-radius: 8px;
		padding: 9px 18px;
		font-size: 13.5px;
		transition: 0.15s;
		white-space: nowrap;
	}
	.btn-add:hover:not(:disabled) {
		background: #ffbf58;
	}
	.btn-add:disabled {
		opacity: 0.5;
		cursor: default;
	}
	/* Un solo primario por pantalla ("Añadir carpeta"): el resto, secundarios. */
	.btn-add.sec {
		background: rgba(255, 255, 255, 0.03);
		color: var(--text);
		border: 1px solid var(--line);
		font-weight: 500;
	}
	.btn-add.sec:hover:not(:disabled) {
		background: var(--surface-2);
	}
	.btn-add {
		min-height: 40px;
	}
	.switch-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 20px;
	}
	.switch-title {
		font-weight: 600;
		margin-bottom: 2px;
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
		opacity: 0.5;
		cursor: default;
	}
	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: var(--text);
		transition: 0.2s;
	}
	.switch.on .knob {
		left: 23px;
		background: #1a1207;
	}
	.btn-ghost {
		padding: 9px 16px;
		border-radius: 8px;
		border: 1px solid var(--line);
		color: var(--text);
		background: rgba(255, 255, 255, 0.03);
		font-weight: 500;
		font-size: 13.5px;
		white-space: nowrap;
	}
	.btn-ghost.small {
		padding: 8px 12px;
		min-height: 36px;
		font-size: 12.5px;
	}
	.btn-ghost:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.status.running {
		color: var(--accent);
		margin-bottom: 12px;
	}
	.id-stats,
	.health-stats,
	.sys-stats {
		display: flex;
		gap: 16px;
		flex-wrap: wrap;
		font-size: 13.5px;
		margin-bottom: 12px;
	}
	.id-stats b {
		color: var(--text);
	}
	.conn-dot::before {
		content: '● ';
		color: var(--bad);
	}
	.conn-dot.on::before {
		color: var(--ok);
	}
	.ok-text {
		color: var(--ok);
	}
	.warn-text {
		color: var(--warn);
	}
	.bad-text {
		color: var(--bad);
	}
	.repo-link {
		color: var(--accent);
		text-decoration: none;
		font-size: 13.5px;
	}
	.repo-link:hover {
		text-decoration: underline;
	}
	[type='password'] {
		font-family: var(--mono);
	}

	@media (max-width: 600px) {
		.page {
			padding: 20px 16px 60px;
		}
		.add-folder {
			flex-direction: column;
		}
		.add-folder .btn-add {
			justify-content: center;
		}
	}
</style>
