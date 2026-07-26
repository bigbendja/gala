import { listPending } from '$lib/api/client';
import type { LayoutLoad } from './$types';

// SPA pura: sin renderizado en servidor (CLAUDE.md M10a — "Cliente, sin
// SSR"). GALA es un binario que sirve esta build estática; no hay Node en
// producción, solo el propio backend Go respondiendo a /api/v1/*.
export const ssr = false;

// Brief M10b §3/§4: la píldora "N por confirmar" de la topbar necesita el
// recuento en CADA página (no solo en /pendientes), y debe refrescarse tras
// una ingesta (invalidateAll() vuelve a correr este load). Solo cuentan los
// "review" — pending/error son transitorios (la cola ya los reintenta
// sola) y junk no se ofrece para confirmar (Brief M10b §3).
export const load: LayoutLoad = async () => {
	const pending = await listPending().catch(() => []);
	const pendingCount = pending.filter((it) => it.identify_status === 'review').length;
	return { pendingCount };
};
