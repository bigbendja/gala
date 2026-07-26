import { listPending } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	const pendingRes = await listPending()
		.then((items) => ({ items, error: null }))
		.catch((err) => ({ items: [], error: err instanceof Error ? err.message : String(err) }));
	return pendingRes;
};
