import { error } from '@sveltejs/kit';
import { ApiError, getItem, getPlay, getPlayback } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	const id = Number(params.id);
	if (!Number.isInteger(id) || id <= 0) {
		error(404, 'Item no válido');
	}

	try {
		const item = await getItem(id);
		const [play, progress] = await Promise.all([
			getPlay(id),
			// El progreso guardado es best-effort: si falla, se reproduce
			// igual desde el principio (Brief M10b §1: "reanudar" es una
			// mejora, no un requisito para poder ver algo).
			getPlayback(id).catch(() => null)
		]);
		return { item, play, progress, loadError: null };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			error(404, 'No encontrado en el catálogo');
		}
		return {
			item: null,
			play: null,
			progress: null,
			loadError: err instanceof Error ? err.message : String(err)
		};
	}
};
