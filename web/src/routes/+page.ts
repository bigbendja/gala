import { getItem, healthSummary, listItems, listVolumes } from '$lib/api/client';
import type { PageLoad } from './$types';

// Brief M10a §5: la galería solo pide lo que necesita para el primer
// pintado; el resto de páginas (Tu galería puede tener miles de items) se
// piden bajo demanda desde +page.svelte según el usuario hace scroll
// (ver Row.svelte, virtualización + paginación perezosa).
export const load: PageLoad = async () => {
	const [galleryRes, wishlistRes, continuingRes, healthRes] = await Promise.allSettled([
		listItems({ confirmed: true, limit: 200, offset: 0 }),
		listItems({ wished: true, limit: 60, offset: 0 }),
		// Brief M10a-fix B3/C: "Continuar viendo" ahora mira TODO el
		// catálogo vía el filtro in_progress, no solo la página de "Tu
		// galería" ya cargada (limitación de M10a, ya resuelta).
		listItems({ in_progress: true, limit: 60, offset: 0 }),
		healthSummary()
	]);

	const gallery = galleryRes.status === 'fulfilled' ? galleryRes.value : null;
	const galleryError = galleryRes.status === 'rejected' ? String(galleryRes.reason) : null;
	const wishlist = wishlistRes.status === 'fulfilled' ? wishlistRes.value : null;
	const wishlistError = wishlistRes.status === 'rejected' ? String(wishlistRes.reason) : null;
	const continuing = continuingRes.status === 'fulfilled' ? continuingRes.value : null;
	const health = healthRes.status === 'fulfilled' ? healthRes.value : null;

	// Hero (Brief M10a §5): el item confirmado más reciente que YA tiene
	// backdrop (si ninguno lo tiene, se omite el hero — nunca se inventa).
	// La lista NO trae `sources` (solo /items/{id} las trae), así que si hay
	// candidato a hero se pide su ficha completa para el stack de fuentes.
	const heroCandidate = (gallery?.items ?? []).find((it) => it.backdrop_path);
	const [hero, volumes] = await Promise.all([
		heroCandidate ? getItem(heroCandidate.id).catch(() => null) : Promise.resolve(null),
		listVolumes().catch(() => [])
	]);

	return { gallery, galleryError, wishlist, wishlistError, continuing, health, hero, volumes };
};
