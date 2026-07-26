// Tipos que reflejan el JSON REAL de la API (internal/catalog/catalog.go,
// internal/api/health.go) — Brief M10a §3: "mirar las respuestas reales, no
// inventar campos". No añadir campos que el backend no manda todavía.

export type Kind = 'movie' | 'series' | 'season' | 'episode' | 'other';

// catalog.HealthAlive/HealthSlow/HealthDead/HealthDisconnected/HealthUnverified.
export type Health = 'viva' | 'lenta' | 'caída' | 'desconectada' | 'sin_verificar';

export interface Progress {
	position_s: number;
	duration_s?: number;
	watched: boolean;
	updated_at: string;
}

export interface Source {
	id: number;
	item_id: number;
	provider: string;
	locator: string;
	volume_id?: string;
	quality_label?: string;
	health: Health;
	last_checked?: string;
	priority: number;
	added_at: string;
	origin_list_id?: number;
	health_detail?: string;
}

export interface Item {
	id: number;
	kind: Kind;
	parent_id?: number;
	tmdb_id?: number;
	title: string;
	original_title?: string;
	year?: number;
	overview?: string;
	poster_path?: string;
	backdrop_path?: string;
	genres: string[];
	rating_cert?: string;
	confirmed: boolean;
	wished: boolean;
	added_at: string;
	updated_at: string;
	season_number?: number;
	episode_number?: number;
	identify_status?: string;
	identify_note?: string;
	sources?: Source[];
	source_count: number;
	health_summary?: Partial<Record<Health, number>>;
	progress?: Progress;
}

export interface ListResult {
	items: Item[];
	total: number;
	limit: number;
	offset: number;
}

export interface ListItemsParams {
	kind?: string;
	q?: string;
	genre?: string;
	year?: number;
	wished?: boolean;
	confirmed?: boolean;
	/** Hijos directos de este id (temporadas de una serie, episodios de una
	 *  temporada). 'null' = solo items de nivel superior. Brief M10a-fix B1. */
	parent_id?: number | 'null';
	/** Solo items con progreso real para el dispositivo actual. Brief
	 *  M10a-fix B3. */
	in_progress?: boolean;
	limit?: number;
	offset?: number;
}

// catalog.Volume.
export interface Volume {
	id: string;
	label?: string;
	last_seen?: string;
	connected: boolean;
}

// internal/api/health.go: healthSummaryView.
export interface HealthSummary {
	total: number;
	viva: number;
	lenta: number;
	caida: number;
	desconectada: number;
	sin_verificar: number;
	last_run?: string;
	running: boolean;
}
