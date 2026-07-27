import { listItems } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	try {
		const res = await listItems({ kind: 'series', confirmed: true, limit: 60, offset: 0 });
		return { items: res.items, total: res.total, error: null };
	} catch (err) {
		return { items: [], total: 0, error: err instanceof Error ? err.message : String(err) };
	}
};
