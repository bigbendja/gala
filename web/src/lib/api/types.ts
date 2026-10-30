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
	/** Nº de temporadas (solo series, solo en listados). */
	season_count?: number;
	/** Nota de TMDB (solo en listados, si ya se consultó el detalle). */
	vote_average?: number;
	health_summary?: Partial<Record<Health, number>>;
	progress?: Progress;
	/** Serie a la que pertenece un episodio o una temporada. */
	series_id?: number;
	series_title?: string;
}

/** GET /api/v1/genres — género con contenido y cuántos títulos lo tienen. */
export interface GenreCount {
	name: string;
	count: number;
}

/** GET /api/v1/facets?kind= — con qué se puede filtrar Películas o Series. */
export interface Facets {
	genres: GenreCount[];
	years: number[];
}

export type SortOrder = 'recent' | 'year' | 'title' | 'rating';

/** GET/PUT /api/v1/settings/home — preferencias de la Home. */
export interface HomeSettings {
	hero: boolean;
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
	/** Orden del listado (por defecto, lo añadido más reciente primero). */
	sort?: SortOrder;
	/** Solo lo que se puede abrir: series y lo que tenga fuente. */
	playable?: boolean;
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

// --- Reproducción (Brief M10b §1; internal/api/stream.go) ------------------

// media.PlayMode.
export type PlayMode = 'direct' | 'remux' | 'reject' | 'analyzing';

export interface PlayMediaView {
	container?: string;
	video_codec?: string;
	audio_codec?: string;
	duration_s?: number;
}

export interface PlaySourceView {
	source_id: number;
	provider: string;
	quality_label?: string;
	health: Health;
	mode: PlayMode;
	url: string;
	reason?: string;
	media?: PlayMediaView;
	/** Solo si la fuente es localfs y la petición viene de localhost. */
	path?: string;
}

export interface PlayResponse {
	item_id: number;
	sources: PlaySourceView[];
	chosen_source_id?: number;
	ffmpeg_available: boolean;
}

// --- Ingesta (Brief M10b §2; internal/ingest/pipeline.go) -------------------

// ingest.ResumenIngesta.
export interface ResumenIngesta {
	detectados: number;
	total: number;
	anadidos: number;
	duplicados: number;
	pendientes_conector: number;
	omitidos_no_soportados: number;
	omitidos_no_directos: number;
	sin_clasificar: number;
	basura: number;
	por_tipo: Record<string, number>;
	detalles: string[] | null;
	list_id: number;
}

// --- Por confirmar (Brief M10b §3; internal/catalog/repo_identify.go) ------

// catalog.Candidate.
export interface Candidate {
	tmdb_id: number;
	kind: 'movie' | 'tv';
	title: string;
	original_title?: string;
	year?: number;
	poster_path?: string;
	score: number;
}

// catalog.PendingItem (Item + candidates, si los hay).
export interface PendingItem extends Item {
	candidates?: Candidate[];
}

export interface ConfirmRequest {
	tmdb_id?: number;
	kind?: 'movie' | 'tv';
	title?: string;
}

// --- Ajustes (Brief M10c §2; internal/api/localfs.go, scan.go, queue.go) ---

// internal/api/localfs.go: folderInfo.
export interface FolderInfo {
	path: string;
	connected: boolean;
}

// scan.Status.
export interface ScanStatus {
	running: boolean;
	last_started?: string;
	last_finished?: string;
	files_seen: number;
	items_new: number;
}

// identify.Status.
export interface IdentifyStatus {
	pending: number;
	processing: boolean;
	online: boolean;
	last_error?: string;
	processed: number;
}

// internal/api/settings.go: tmdbSettingsView.
export interface TMDBSettings {
	configured: boolean;
}

// catalog.IngestList.
export interface IngestList {
	id: number;
	name: string;
	type: string;
	source_url?: string;
	last_sync?: string;
	item_count: number;
}

// --- Modo LAN (M7) ---

// internal/lan.Candidate: una IPv4 que un móvil podría usar para llegar a GALA.
export interface LanCandidate {
	ip: string;
	interface: string;
	virtual: boolean;
}

// internal/api/lan.go: lanStatusView. Nunca lleva el PIN, solo pin_set.
export interface LanStatus {
	enabled: boolean;
	bind: string;
	port: number;
	lan_ip: string;
	url: string;
	pin_set: boolean;
	qr_payload: string;
	candidates: LanCandidate[];
}

// internal/lan.sessionView: quién es este cliente y si le falta el PIN.
export interface LanSession {
	local: boolean;
	pin_required: boolean;
	authenticated: boolean;
}

/** GET /api/v1/items/{id}/details — lo que TMDB da además de lo básico. */
export interface CastMember {
	id: number;
	name: string;
	character?: string;
	/** Fichero para /img/{archivo}; vacío = se enseñan las iniciales. */
	photo?: string;
}

export interface CrewGroup {
	role: string;
	names: string[];
}

export interface SimilarTitle {
	tmdb_id: number;
	kind: 'movie' | 'tv';
	title: string;
	year?: number;
	poster?: string;
	/** Item de la biblioteca (se puede abrir su ficha); sin él, no está en tu galería. */
	item_id?: number;
}

export interface MediaFile {
	source_id: number;
	container?: string;
	video_codec?: string;
	audio_codec?: string;
	width?: number;
	height?: number;
	bitrate?: number;
	duration_s?: number;
	size_bytes?: number;
}

export interface ItemDetails {
	has_details: boolean;
	runtime_min?: number;
	vote_average?: number;
	original_language?: string;
	countries: string[];
	tagline?: string;
	cast: CastMember[];
	crew: CrewGroup[];
	similar: SimilarTitle[];
	files: MediaFile[];
}
