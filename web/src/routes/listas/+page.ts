import { listIngestLists } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	try {
		const lists = await listIngestLists();
		return { lists, error: null };
	} catch (err) {
		return { lists: [], error: err instanceof Error ? err.message : String(err) };
	}
};
