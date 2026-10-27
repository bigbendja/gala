<script lang="ts">
	// QR generado en el navegador con `qrcode-generator` (MIT, sin
	// dependencias, ~50 KB): evita meter una librería de QR en el binario Go.
	// El valor que llega es solo la URL de GALA — nunca el PIN (M7 §2/§5).
	import qrcode from 'qrcode-generator';

	interface Props {
		value: string;
		/** Texto accesible; el QR en sí no lo lee un lector de pantalla. */
		label?: string;
	}
	let { value, label = 'Código QR para abrir GALA en el móvil' }: Props = $props();

	const QUIET = 4; // módulos de margen blanco que exige el estándar

	const qr = $derived.by(() => {
		const code = qrcode(0, 'M');
		code.addData(value);
		code.make();
		const n = code.getModuleCount();
		let d = '';
		for (let y = 0; y < n; y++) {
			for (let x = 0; x < n; x++) {
				if (code.isDark(y, x)) d += `M${x + QUIET} ${y + QUIET}h1v1h-1z`;
			}
		}
		return { size: n + QUIET * 2, d };
	});
</script>

<svg
	class="qr"
	viewBox="0 0 {qr.size} {qr.size}"
	role="img"
	aria-label={label}
	shape-rendering="crispEdges"
>
	<rect width={qr.size} height={qr.size} fill="#fff" />
	<path d={qr.d} fill="#000" />
</svg>

<style>
	.qr {
		display: block;
		width: 100%;
		height: auto;
		border-radius: 12px;
	}
</style>
