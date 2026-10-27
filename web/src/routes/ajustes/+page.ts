import {
	getTMDBSettings,
	healthSummary,
	identifyStatus,
	listFolders,
	ping,
	scanStatus
} from '$lib/api/client';
import { errorMessage } from '$lib/format';
import type { PageLoad } from './$types';

// Brief M10c §2: todo lo que hoy se hace por curl, desde la UI. Cada
// sección es independiente (Promise.allSettled): si una falla (p. ej. el
// backend arrancó sin worker de salud en algún test), el resto de Ajustes
// sigue siendo usable.
export const load: PageLoad = async () => {
	const [folders, scan, identify, tmdb, health, sys] = await Promise.allSettled([
		listFolders(),
		scanStatus(),
		identifyStatus(),
		getTMDBSettings(),
		healthSummary(),
		ping()
	]);

	const val = <T>(r: PromiseSettledResult<T>) => (r.status === 'fulfilled' ? r.value : null);
	const err = <T>(r: PromiseSettledResult<T>) => (r.status === 'rejected' ? errorMessage(r.reason, 'No se pudo cargar') : null);

	return {
		folders: val(folders),
		foldersError: err(folders),
		scan: val(scan),
		scanError: err(scan),
		identify: val(identify),
		identifyError: err(identify),
		tmdb: val(tmdb),
		tmdbError: err(tmdb),
		health: val(health),
		healthError: err(health),
		sys: val(sys),
		sysError: err(sys)
	};
};
