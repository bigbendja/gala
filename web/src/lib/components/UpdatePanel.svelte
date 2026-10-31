<script lang="ts">
	// Ajustes › Actualización (M-Servidor, mockup docs/briefs/GALA_Mockup_Login.html).
	// Comprueba a demanda, enseña las notas, actualiza con barra de progreso y
	// maneja el hueco del reinicio (reconecta sola). Nada de avisos por su cuenta.
	import { onDestroy, onMount } from 'svelte';
	import {
		ApiError,
		applyUpdate,
		checkUpdate,
		dismissUpdateResult,
		getUpdateStatus,
		isUp,
		runningVersion
	} from '$lib/api/client';
	import type { UpdateInfo, UpdateStatus } from '$lib/api/types';
	import { errorMessage, formatDate } from '$lib/format';

	let current = $state('');
	let available = $state<UpdateInfo | null>(null);
	let canApply = $state(true);
	let reason = $state('');
	let result = $state<UpdateStatus['result']>(undefined);
	let phase = $state<UpdateStatus['status']['phase']>('idle');
	let done = $state(0);
	let total = $state(0);
	let applyError = $state('');

	let checking = $state(false);
	let checkError = $state('');
	let checkedOk = $state(false); // se comprobó y no hay nada nuevo
	let loadError = $state('');

	let askForce = $state(0); // >0 = hay reproducciones en curso: pedir confirmación
	let reconnectTimeout = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;
	let destroyed = false;

	const busy = $derived(phase === 'downloading' || phase === 'verifying' || phase === 'installing' || phase === 'restarting');
	const percent = $derived(total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0);

	const sleep = (ms: number) => new Promise<void>((r) => (timer = setTimeout(r, ms)));

	onDestroy(() => {
		destroyed = true;
		if (timer) clearTimeout(timer);
	});

	function apply(s: UpdateStatus) {
		current = s.current;
		available = s.available ?? null;
		canApply = s.can_apply;
		reason = s.reason ?? '';
		result = s.result;
		phase = s.status.phase;
		done = s.status.done;
		total = s.status.total;
		applyError = s.status.phase === 'error' ? (s.status.error ?? 'La actualización falló.') : '';
	}

	async function refresh() {
		try {
			apply(await getUpdateStatus());
			loadError = '';
		} catch (err) {
			loadError = errorMessage(err, 'No se pudo leer el estado de la actualización.');
		}
	}

	onMount(async () => {
		await refresh();
		if (busy) void follow();
	});

	async function search() {
		checking = true;
		checkError = '';
		checkedOk = false;
		try {
			const r = await checkUpdate(true);
			current = r.current;
			canApply = r.can_apply;
			reason = r.reason ?? '';
			if (!r.checked) {
				checkError = r.error ?? 'No se pudo comprobar las actualizaciones.';
			}
			if (r.info?.update_available) available = r.info;
			else {
				available = null;
				checkedOk = r.checked;
			}
		} catch (err) {
			checkError = errorMessage(err, 'No se pudo comprobar las actualizaciones.');
		} finally {
			checking = false;
		}
	}

	async function start(force = false) {
		applyError = '';
		askForce = 0;
		try {
			await applyUpdate(force);
			phase = 'downloading';
			done = 0;
			total = 0;
			void follow();
		} catch (err) {
			if (err instanceof ApiError && err.code === 'playback_active') {
				askForce = 1;
			} else {
				applyError = errorMessage(err, 'No se pudo iniciar la actualización.');
			}
		}
	}

	// Sigue el progreso; al llegar a «Reiniciando…» espera a que GALA vuelva.
	async function follow() {
		while (!destroyed) {
			await sleep(1000);
			let s: UpdateStatus;
			try {
				s = await getUpdateStatus();
			} catch {
				// El servidor ya salió a reiniciarse.
				if (phase === 'restarting') break;
				continue;
			}
			apply(s);
			if (s.status.phase === 'error' || s.status.phase === 'idle') return;
			if (s.status.phase === 'restarting') break;
		}
		if (!destroyed) await reconnect();
	}

	async function reconnect() {
		phase = 'restarting';
		reconnectTimeout = false;
		const before = current;
		const start = Date.now();
		let wasDown = false;
		while (!destroyed && Date.now() - start < 180_000) {
			await sleep(1500);
			if (!(await isUp())) {
				wasDown = true;
				continue;
			}
			const v = await runningVersion();
			if (v && (wasDown || v !== before)) {
				window.location.reload();
				return;
			}
		}
		if (!destroyed) reconnectTimeout = true;
	}

	async function dismiss() {
		try {
			await dismissUpdateResult();
			result = undefined;
		} catch (err) {
			loadError = errorMessage(err, 'No se pudo descartar el aviso.');
		}
	}

	/** Las notas de una release vienen en Markdown: se enseñan como líneas sueltas. */
	function noteLines(notes: string): string[] {
		return notes
			.split('\n')
			.map((l) => l.replace(/^[\s>*#-]+/, '').replace(/\*\*/g, '').trim())
			.filter(Boolean)
			.slice(0, 8);
	}

	function mb(n: number): string {
		return `${(n / (1024 * 1024)).toFixed(1).replace('.', ',')} MB`;
	}

	const phaseLabel = $derived(
		phase === 'downloading'
			? 'Descargando…'
			: phase === 'verifying'
				? 'Verificando la descarga…'
				: phase === 'installing'
					? 'Instalando…'
					: phase === 'restarting'
						? 'Reiniciando…'
						: ''
	);
	// Una sola cadena: Svelte recorta los espacios entre texto y bloques {#if}.
	const phaseText = $derived(
		phase === 'downloading' && total > 0
			? `${phaseLabel} ${percent}% · ${mb(done)} de ${mb(total)}`
			: phaseLabel
	);
</script>

<section class="card" id="actualizacion">
	<h2>Actualización</h2>
	<p class="desc">
		GALA comprueba si hay una versión nueva publicada cuando se lo pides. La actualización se aplica y reinicia
		sola.
	</p>

	{#if loadError}<p class="error" role="alert">{loadError}</p>{/if}

	{#if result}
		<div class="result" class:bad={result.status === 'rolled_back'} role="status">
			{#if result.status === 'rolled_back'}
				<p>
					<strong>La actualización a {result.to} falló y se revirtió a {result.from}.</strong>
					GALA sigue funcionando con la versión anterior.
				</p>
				{#if result.reason}<p class="small">{result.reason}</p>{/if}
			{:else}
				<p>
					<strong>Actualizado a {result.to}.</strong>
					{#if result.at}<span class="small">{formatDate(result.at)}</span>{/if}
				</p>
				<p class="small">
					La versión anterior se guarda junto al programa como <span class="mono">gala.prev</span>. Si algo
					queda mal aunque GALA haya arrancado bien, puedes volver a ella (mira la guía de servidor).
				</p>
			{/if}
			<button class="btn btn-secondary" onclick={dismiss}>Entendido</button>
		</div>
	{/if}

	<div class="verline">
		Versión actual <span class="cur">{current || '…'}</span>
		<button class="btn btn-secondary btn-sm" onclick={search} disabled={checking || busy}>
			{checking ? 'Buscando…' : 'Buscar actualizaciones'}
		</button>
	</div>

	{#if checkError}<p class="error" role="alert">{checkError}</p>{/if}
	{#if checkedOk && !available && !checkError}
		<p class="ok" role="status">Estás al día: no hay ninguna versión nueva.</p>
	{/if}

	{#if available}
		<div class="update">
			<p class="ver">Disponible <span class="tag">{available.latest}</span></p>
			{#if noteLines(available.notes).length}
				<ul>
					{#each noteLines(available.notes) as line (line)}
						<li>{line}</li>
					{/each}
				</ul>
			{/if}

			{#if busy}
				<div class="progress" role="status" aria-live="polite">
					<p class="phase">{phaseText}</p>
					<div
						class="bar"
						role="progressbar"
						aria-label="Progreso de la actualización"
						aria-valuemin={0}
						aria-valuemax={100}
						aria-valuenow={phase === 'downloading' ? percent : phase === 'restarting' ? undefined : 100}
					>
						<span
							class:indeterminate={phase !== 'downloading' || total === 0}
							style:width={phase === 'downloading' && total > 0 ? `${percent}%` : undefined}
						></span>
					</div>
					{#if phase === 'restarting'}
						<p class="small">GALA se está reiniciando. Esta página se recargará sola cuando vuelva.</p>
					{/if}
					{#if reconnectTimeout}
						<p class="error" role="alert">
							GALA tarda más de lo normal en volver. Si no responde pronto, revisa el servicio en el servidor
							(<span class="mono">systemctl status gala</span>).
						</p>
					{/if}
				</div>
			{:else if askForce}
				<div class="ask" role="alert">
					<p>Hay una reproducción en curso. Si actualizas ahora, se cortará.</p>
					<div class="actions">
						<button class="btn btn-secondary" onclick={() => start(true)}>Actualizar igualmente</button>
						<button class="btn btn-secondary" onclick={() => (askForce = 0)}>Esperar</button>
					</div>
				</div>
			{:else if canApply && available.asset_url}
				<button class="btn btn-primary" onclick={() => start(false)}>Actualizar y reiniciar</button>
				<p class="okline">
					<span class="dot-ok"></span> Se guardará la versión anterior por si hay que revertir.
				</p>
			{:else}
				<p class="muted">
					{reason ||
						'Esta versión no incluye un binario verificable para este equipo.'}
				</p>
				{#if available.page_url}
					<a class="btn btn-secondary" href={available.page_url} target="_blank" rel="noreferrer">
						Ver la versión ↗
					</a>
				{/if}
			{/if}
			{#if applyError}<p class="error" role="alert">{applyError}</p>{/if}
		</div>
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
	.small {
		color: var(--muted);
		font-size: 12.5px;
	}
	.error {
		color: var(--bad);
		font-size: 13.5px;
		margin-top: 8px;
	}
	.ok {
		color: var(--ok);
		font-size: 13.5px;
		margin-top: 10px;
	}
	.mono {
		font-family: var(--mono);
	}
	.verline {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 10px;
		font-size: 13px;
		color: var(--muted);
		margin: 0 0 14px;
	}
	.cur {
		font-family: var(--mono);
		color: var(--text);
	}
	.update {
		background: var(--surface-2);
		border: 1px solid rgba(242, 169, 59, 0.35);
		border-radius: 10px;
		padding: 14px;
		margin-top: 12px;
	}
	.ver {
		font-family: var(--display);
		font-weight: 600;
		font-size: 15px;
		margin: 0 0 6px;
	}
	.tag {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--accent);
		margin-left: 6px;
	}
	ul {
		margin: 0 0 14px;
		padding-left: 18px;
		color: var(--muted);
		font-size: 12.5px;
	}
	li {
		margin: 2px 0;
	}
	.okline {
		display: flex;
		align-items: center;
		gap: 8px;
		color: var(--ok);
		font-size: 12.5px;
		margin: 10px 0 0;
	}
	.dot-ok {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--ok);
		display: inline-block;
	}
	.result {
		border: 1px solid rgba(111, 207, 142, 0.4);
		background: rgba(111, 207, 142, 0.07);
		border-radius: 10px;
		padding: 12px 14px;
		margin-bottom: 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		align-items: flex-start;
		font-size: 13.5px;
	}
	.result.bad {
		border-color: rgba(225, 96, 79, 0.45);
		background: rgba(225, 96, 79, 0.08);
	}
	.result p {
		margin: 0;
	}
	.progress {
		margin-top: 4px;
	}
	.phase {
		font-size: 13.5px;
		margin: 0 0 8px;
	}
	.bar {
		height: 8px;
		border-radius: 99px;
		background: var(--line);
		overflow: hidden;
	}
	.bar span {
		display: block;
		height: 100%;
		background: var(--accent);
		border-radius: 99px;
		transition: width 0.3s ease;
	}
	.bar span.indeterminate {
		width: 35%;
		animation: slide 1.4s ease-in-out infinite;
	}
	@keyframes slide {
		0% {
			transform: translateX(-100%);
		}
		100% {
			transform: translateX(300%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.bar span {
			transition: none;
		}
		.bar span.indeterminate {
			animation: none;
			width: 100%;
			opacity: 0.6;
		}
	}
	.ask {
		font-size: 13.5px;
	}
	.ask p {
		margin: 0 0 10px;
		color: var(--warn);
	}
	.actions {
		display: flex;
		gap: 10px;
		flex-wrap: wrap;
	}
</style>
