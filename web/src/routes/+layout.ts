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

export interface Chrome {
	pendingCount: number;
	lanEnabled: boolean;
}

// Brief M10b §3/§4: la píldora "N por confirmar" de la topbar necesita el
// recuento en CADA página (no solo en /pendientes), y debe refrescarse tras
// una ingesta (invalidateAll() vuelve a correr este load). Solo cuentan los
// "review" — pending/error son transitorios (la cola ya los reintenta
// sola) y junk no se ofrece para confirmar (Brief M10b §3).
//
// Modo LAN (M7): un dispositivo de la red (session.local === false) no puede
// leer "Por confirmar" ni el estado del Modo LAN (solo del PC), así que ni
// los pide; y si le falta el PIN no se pide nada más hasta que lo teclee.
//
// Sistema de Diseño GALA §2.3: este load SOLO espera a la sesión (una
// petición mínima, necesaria para saber si pedir el PIN). El recuento de
// pendientes y el estado del LAN viajan como promesa: la topbar y la
// estructura se pintan al instante y esos datos entran cuando llegan.
export const load: LayoutLoad = async () => {
	const session = await getLanSession().catch(() => LOCAL_SESSION);
	if (!session.local) {
		const chrome: Promise<Chrome> = Promise.resolve({ pendingCount: 0, lanEnabled: false });
		return { session, chrome };
	}
	const chrome: Promise<Chrome> = Promise.all([
		listPending().catch(() => []),
		getLanStatus().catch(() => null)
	]).then(([pending, lan]) => ({
		pendingCount: pending.filter((it) => it.identify_status === 'review').length,
		lanEnabled: lan?.enabled ?? false
	}));
	return { session, chrome };
};
