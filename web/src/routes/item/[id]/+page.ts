import { error } from '@sveltejs/kit';
import { ApiError, getItem, listItems } from '$lib/api/client';
import { errorMessage } from '$lib/format';
import type { Item } from '$lib/api/types';
import type { PageLoad } from './$types';

function byNumber<T>(get: (v: T) => number | undefined) {
	return (a: T, b: T) => (get(a) ?? 0) - (get(b) ?? 0);
}

export const load: PageLoad = async ({ params }) => {
	const id = Number(params.id);
	if (!Number.isInteger(id) || id <= 0) {
		error(404, 'Este título no existe.');
	}

	let item: Item;
	try {
		item = await getItem(id);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			error(404, 'Este título ya no está en tu galería.');
		}
		// Fallo de conexión u otro error de servidor: no es un 404 de
		// SvelteKit (el item podría existir), se muestra inline con mensaje
		// en español (Brief M10a §3).
		const emptyEpisodesBySeason: Record<number, Item[]> = {};
		return {
			item: null,
			itemError: errorMessage(err, 'No se pudo cargar esta ficha'),
			seasons: [] as Item[],
			episodesBySeason: emptyEpisodesBySeason
		};
	}

	// Brief M10a-fix B1/C: si es una serie, se listan sus temporadas y,
	// dentro de cada una, sus episodios — todo vía parent_id, sin traerse
	// el catálogo entero. Un fallo aquí no debe tumbar la ficha: se queda
	// sin jerarquía, igual que si la serie no tuviera temporadas todavía.
	let seasons: Item[] = [];
	const episodesBySeason: Record<number, Item[]> = {};
	if (item.kind === 'series') {
		const seasonsRes = await listItems({ parent_id: item.id, limit: 200 }).catch(() => null);
		seasons = (seasonsRes?.items ?? []).sort(byNumber((s) => s.season_number));

		const episodeLists = await Promise.all(
			seasons.map((s) => listItems({ parent_id: s.id, limit: 200 }).catch(() => null))
		);
		episodeLists.forEach((res, i) => {
			episodesBySeason[seasons[i].id] = (res?.items ?? []).sort(byNumber((e) => e.episode_number));
		});
	}

	return { item, itemError: null, seasons, episodesBySeason };
};
