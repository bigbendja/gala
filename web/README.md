# GALA — frontend (web/)

SPA en SvelteKit (Svelte 5, TypeScript, `@sveltejs/adapter-static` en modo
SPA: sin SSR, con fallback a `index.html` para el enrutado del lado
cliente). El binario Go (`web/web.go`, `go:embed`) sirve esta build
compilada; no hay Node en producción.

Referencia visual vinculante: `docs/GALA_Mockup.html` (raíz del repo).

## Desarrollar

Necesitas el backend real corriendo (aunque sea con datos de `--demo`),
porque este proyecto no mockea la API: consume `/api/v1/*` de verdad.

```sh
# Terminal 1 — backend real en :7420
cd ..
go run ./cmd/gala --demo

# Terminal 2 — frontend con recarga en caliente
cd web
npm install
npm run dev -- --open
```

`vite.config.ts` reenvía `/api/*` a `http://127.0.0.1:7420` (ver `server.proxy`),
así que el código de la SPA siempre pide rutas relativas (`/api/v1/items`,
etc.) tanto en desarrollo como embebido en el binario — nunca hace falta una
URL base distinta según el entorno.

## Compilar

```sh
npm run build      # vuelca a web/dist/ — EXACTAMENTE lo que embebe web/web.go
npm run preview    # sirve esa build para probarla suelta, sin el binario Go
```

`web/dist/` **no se versiona** (`.gitignore`): se regenera en cada build,
local o en CI (`.github/workflows/ci.yml` ejecuta `npm ci && npm run build`
antes de cualquier `go build`/`go vet`/`go test`, porque `go:embed` necesita
que el directorio exista y tenga contenido).

## Tests

```sh
npm test          # Vitest (unit + component, jsdom + @testing-library/svelte)
npm run check     # svelte-check (tipos)
```

## Sistema de diseño

`src/lib/styles/`: `tokens.css` (colores/tipografías EXACTOS del mockup,
`:root`), `fonts.css` (`@font-face` de las 3 fuentes empaquetadas en
`src/lib/styles/fonts/*.woff2` — self-hosted, nunca Google Fonts: GALA
funciona sin internet), `base.css` (reset + `prefers-reduced-motion`).

Componentes base en `src/lib/components/`: `HealthDot` (mapa salud→color,
ver `src/lib/health.ts`), `Card`, `Row` (fila horizontal virtualizada — ver
más abajo), `Hero`, `SourceStack`, `Topbar`, `Footer`.

## Virtualización sin librerías

`Row.svelte` no usa ninguna librería de virtualización: aprovecha que
`display:grid; grid-auto-flow:column; grid-auto-columns:172px` reserva el
ancho de scroll completo aunque solo unas pocas tarjetas tengan
`grid-column` explícito — el resto de columnas quedan vacías pero ocupan su
hueco. Combinado con paginación perezosa (`onNeedMore` en `+page.svelte`,
que pide la siguiente página de `/api/v1/items` cuando el scroll se acerca
al final de lo cargado), una fila de miles de items scrollea fluido sin
montar miles de nodos DOM ni traerse el catálogo entero de una vez.

## Cliente de API

`src/lib/api/client.ts` + `src/lib/api/types.ts`: wrapper mínimo sobre
`fetch`, tipado según el JSON REAL que devuelve el backend (no hay campos
inventados). Errores de red o HTTP se traducen a `ApiError` con mensaje en
español.

## Alcance de esta entrega (M10a)

Ver CLAUDE.md (bullet M10a) para el detalle completo: qué se implementó, qué
se anotó como endpoint faltante, y qué decisiones de diseño se tomaron donde
el mockup no llegaba. Reproductor, modal de ingesta/"Por confirmar", ajustes
y búsqueda funcional llegan en M10b/M10c — no están aquí a propósito.
