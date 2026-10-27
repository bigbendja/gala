<script lang="ts">
	import { ingest } from '$lib/api/client';
	import { errorMessage } from '$lib/format';
	import type { ResumenIngesta } from '$lib/api/types';

	interface Props {
		open: boolean;
		onClose: () => void;
		/** Se llama tras un ingest con éxito (Brief M10b §2: "refrescar la
		 *  galería para que aparezca lo nuevo"). */
		onIngested: () => void;
		/** "Elegir carpeta": lleva al flujo real de añadir carpeta (Ajustes ›
		 *  Carpetas). */
		onOpenSettings?: () => void;
	}
	let { open, onClose, onIngested, onOpenSettings }: Props = $props();

	let text = $state('');
	let droppedFile = $state<File | null>(null);
	let dragOver = $state(false);
	let status = $state<'idle' | 'sending' | 'done' | 'error'>('idle');
	let resumen = $state<ResumenIngesta | null>(null);
	let errorMsg = $state('');

	function reset() {
		text = '';
		droppedFile = null;
		dragOver = false;
		status = 'idle';
		resumen = null;
		errorMsg = '';
	}

	function close() {
		onClose();
		// El reset se retrasa a después del cierre para no verse "en vivo"
		// mientras la modal aún se desvanece.
		setTimeout(reset, 200);
	}

	function handleOverlayClick(e: MouseEvent) {
		if (e.target === e.currentTarget) close();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') close();
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		const file = e.dataTransfer?.files?.[0];
		if (file) {
			droppedFile = file;
			text = '';
		}
	}

	function clearDroppedFile() {
		droppedFile = null;
	}

	async function submit() {
		if (status === 'sending') return;
		const input = droppedFile ?? text;
		if (typeof input === 'string' && !input.trim()) return;
		status = 'sending';
		errorMsg = '';
		try {
			resumen = await ingest(input);
			status = 'done';
			onIngested();
		} catch (err) {
			errorMsg = errorMessage(err, 'No se pudo añadir');
			status = 'error';
		}
	}

	const canSubmit = $derived(status !== 'sending' && (droppedFile !== null || text.trim() !== ''));
</script>

<svelte:window onkeydown={open ? handleKeydown : undefined} />

{#if open}
	<div
		class="overlay"
		role="dialog"
		aria-modal="true"
		aria-labelledby="ing-t"
		tabindex="-1"
		onclick={handleOverlayClick}
		onkeydown={handleKeydown}
	>
		<div class="modal">
			<div class="modal-head">
				<h3 id="ing-t">Pega lo que sea</h3>
				<p>
					Un enlace, veinte enlaces, una lista M3U, o un texto de WhatsApp con enlaces por medio.
					GALA detecta, cataloga y descarta el ruido.
				</p>
			</div>
			<div class="modal-body">
				{#if status === 'done' && resumen}
					<div class="detect">
						<div class="detect-head">Resultado</div>
						<div class="detect-row">
							<span class="ic">◆</span> Detectados
							<span class="n">{resumen.detectados}</span>
						</div>
						<div class="detect-row">
							<span class="ic">✓</span> Añadidos a tu galería
							<span class="n">{resumen.anadidos}</span>
						</div>
						{#if resumen.duplicados > 0}
							<div class="detect-row">
								<span class="ic">⧉</span> Ya los tenías (duplicados)
								<span class="n">{resumen.duplicados}</span>
							</div>
						{/if}
						{#if resumen.pendientes_conector > 0}
							<div class="detect-row">
								<span class="ic">◌</span> Pendientes de conector (Drive/Mega)
								<span class="n">{resumen.pendientes_conector}</span>
							</div>
						{/if}
						{#if resumen.omitidos_no_soportados > 0}
							<div class="detect-row warn-row">
								<span class="ic">⊘</span> Omitidos (host no soportado)
								<span class="n">{resumen.omitidos_no_soportados}</span>
							</div>
						{/if}
						{#if resumen.sin_clasificar > 0}
							<div class="detect-row warn-row">
								<span class="ic">?</span> Sin clasificar
								<span class="n">{resumen.sin_clasificar}</span>
							</div>
						{/if}
						{#if resumen.basura > 0}
							<div class="detect-row">
								<span class="ic">✕</span> Basura descartada
								<span class="n">{resumen.basura}</span>
							</div>
						{/if}
						{#if resumen.detalles && resumen.detalles.length > 0}
							<div class="detalles">
								{#each resumen.detalles as d, i (i)}
									<p>{d}</p>
								{/each}
							</div>
						{/if}
					</div>
					<p class="hint">
						Lo añadido se enriquece solo en segundo plano (título, carátula, año). Si algo quedó
						"sin clasificar", puede necesitar tu confirmación.
					</p>
					<div class="modal-actions">
						<button class="btn-add" onclick={reset}>Pegar más</button>
						<button class="btn-ghost" onclick={close}>Cerrar</button>
					</div>
				{:else}
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="drop-zone"
						class:drag-over={dragOver}
						ondragover={(e) => {
							e.preventDefault();
							dragOver = true;
						}}
						ondragleave={() => (dragOver = false)}
						ondrop={handleDrop}
					>
						{#if droppedFile}
							<div class="dropped-file">
								<span>{droppedFile.name}</span>
								<button class="link-btn" onclick={clearDroppedFile}>Quitar</button>
							</div>
						{:else}
							<textarea
								class="paste"
								spellcheck="false"
								placeholder={'Pega aquí… o arrastra un archivo\n\nEj.:  https://drive.google.com/file/d/…\n      http://lista.ejemplo.com/canales.m3u\n      E:\\Peliculas\\'}
								bind:value={text}
							></textarea>
						{/if}
					</div>

					{#if status === 'error'}
						<p class="error">{errorMsg}</p>
					{/if}

					<!-- Un control o funciona, o se ve deshabilitado con su motivo. -->
					<div class="alt-src">
						<button
							class="chip"
							onclick={() => {
								close();
								onOpenSettings?.();
							}}>Elegir carpeta</button
						>
						<button class="chip" disabled title="Próximamente">Escanear disco USB</button>
						<button class="chip" disabled title="Próximamente">Importar .gala</button>
					</div>

					<div class="modal-actions">
						<button class="btn-add" disabled={!canSubmit} onclick={submit}>
							{status === 'sending' ? 'Añadiendo…' : 'Añadir a mi galería'}
						</button>
						<button class="btn-ghost" onclick={close}>Cancelar</button>
					</div>
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
		width: min(640px, 100%);
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
	.modal-head p {
		color: var(--muted);
		font-size: 14px;
	}
	.modal-body {
		padding: 20px 28px 28px;
	}
	.drop-zone {
		border-radius: 14px;
		transition: 0.15s;
	}
	.drop-zone.drag-over {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.paste {
		width: 100%;
		min-height: 150px;
		background: var(--bg);
		border: 1.5px dashed #4d3a22;
		border-radius: 14px;
		color: var(--text);
		font-family: var(--mono);
		font-size: 13px;
		padding: 16px;
		resize: vertical;
		outline: none;
		transition: 0.15s;
	}
	.paste:focus {
		border-color: var(--accent);
		border-style: solid;
		box-shadow: 0 0 0 3px rgba(242, 169, 59, 0.14);
	}
	.paste::placeholder {
		color: #5f5347;
	}
	.dropped-file {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 150px;
		background: var(--bg);
		border: 1.5px solid #4d3a22;
		border-radius: 14px;
		padding: 16px;
		font-size: 13.5px;
	}
	.link-btn {
		color: var(--accent);
		font-size: 13px;
		text-decoration: underline;
	}
	.error {
		margin-top: 12px;
		color: var(--bad);
		font-size: 13.5px;
	}
	.detect {
		border: 1px solid var(--line);
		border-radius: 12px;
		overflow: hidden;
	}
	.detect-head {
		font-size: 11.5px;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
		padding: 10px 16px;
		border-bottom: 1px solid var(--line);
		background: rgba(0, 0, 0, 0.2);
	}
	.detect-row {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 16px;
		font-size: 13.5px;
		border-bottom: 1px solid rgba(51, 38, 25, 0.5);
	}
	.detect-row:last-child {
		border-bottom: none;
	}
	.detect-row .ic {
		width: 22px;
		text-align: center;
	}
	.detect-row .n {
		margin-left: auto;
		font-family: var(--mono);
		font-size: 12px;
		color: var(--muted);
	}
	.detect-row.warn-row {
		color: var(--warn);
	}
	.detalles {
		padding: 10px 16px;
		font-size: 12.5px;
		color: var(--muted);
		background: rgba(0, 0, 0, 0.15);
	}
	.detalles p {
		margin: 4px 0;
	}
	.hint {
		margin-top: 14px;
		color: var(--muted);
		font-size: 12.5px;
	}
	.modal-actions {
		display: flex;
		gap: 12px;
		margin-top: 20px;
	}
	.modal-actions .btn-add {
		flex: 1;
		justify-content: center;
		padding: 13px;
	}
	.modal-actions .btn-ghost {
		padding: 13px 22px;
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
	.btn-ghost:hover {
		background: var(--surface-2);
	}
	.alt-src {
		display: flex;
		gap: 10px;
		margin-top: 16px;
		flex-wrap: wrap;
	}
	.chip {
		min-height: 40px;
		font-size: 13px;
		font-weight: 500;
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 99px;
		padding: 7px 16px;
		background: rgba(255, 255, 255, 0.03);
		transition: 0.15s;
	}
	.chip:hover:not(:disabled) {
		background: var(--surface-2);
	}
	/* Aún no existe: se ve apagado y con borde discontinuo (el motivo va en el
	   tooltip), nunca como un botón que invita a pulsar y no hace nada. */
	.chip:disabled {
		opacity: 0.4;
		border-style: dashed;
		color: var(--muted);
		cursor: default;
	}
</style>
