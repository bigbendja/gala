import { error } from '@sveltejs/kit';
import { ApiError, getItem } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	const id = Number(params.id);
	if (!Number.isInteger(id) || id <= 0) {
		error(404, 'Item no válido');
	}

	try {
		const item = await getItem(id);
		return { item, itemError: null };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			error(404, 'No encontrado en el catálogo');
		}
		// Fallo de conexión u otro error de servidor: no es un 404 de
		// SvelteKit (el item podría existir), se muestra inline con mensaje
		// en español (Brief M10a §3).
		return { item: null, itemError: err instanceof Error ? err.message : String(err) };
	}
};
