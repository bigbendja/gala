import { getLanSession, getLanStatus, listPending } from '$lib/api/client';
import type { LanSession } from '$lib/api/types';
import type { LayoutLoad } from './$types';

// SPA pura: sin renderizado en servidor (CLAUDE.md M10a — "Cliente, sin
// SSR"). GALA es un binario que sirve esta build estática; no hay Node en
// producción, solo el propio backend Go respondiendo a /api/v1/*.
export const ssr = false;

// Si el backend no responde a /lan/session (no debería pasar) se asume el
// PC: es el caso de siempre y no muestra nada que el backend no permita igual.
const LOCAL_SESSION: LanSession = { local: true, pin_required: false, authenticated: true };

// Brief M10b §3/§4: la píldora "N por confirmar" de la topbar necesita el
// recuento en CADA página (no solo en /pendientes), y debe refrescarse tras
// una ingesta (invalidateAll() vuelve a correr este load). Solo cuentan los
// "review" — pending/error son transitorios (la cola ya los reintenta
// sola) y junk no se ofrece para confirmar (Brief M10b §3).
//
// Modo LAN (M7): un dispositivo de la red (session.local === false) no puede
// leer "Por confirmar" ni el estado del Modo LAN (solo del PC), así que ni
// los pide; y si le falta el PIN no se pide nada más hasta que lo teclee.
export const load: LayoutLoad = async () => {
	const session = await getLanSession().catch(() => LOCAL_SESSION);
	if (!session.local) {
		return { pendingCount: 0, session, lanEnabled: false };
	}
	const [pending, lan] = await Promise.all([
		listPending().catch(() => []),
		getLanStatus().catch(() => null)
	]);
	const pendingCount = pending.filter((it) => it.identify_status === 'review').length;
	return { pendingCount, session, lanEnabled: lan?.enabled ?? false };
};
