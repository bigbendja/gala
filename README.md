# GALA

**Tu cine, tus fuentes.** Cataloga y reproduce tu contenido de vídeo — archivos locales,
listas M3U, enlaces — en una sola galería. Sin configuración: doble clic y ya.
Funciona sin internet una vez catalogado, y comparte tu galería por wifi local
con los móviles de casa sin instalar nada en ellos.

> Estado: **M10a — Frontend: esqueleto + Galería + Ficha**. Ver `docs/` para la especificación y el plan de desarrollo.

## Reglas de hierro del proyecto
1. GALA nunca incluye, enlaza ni sugiere contenido o listas. Es una herramienta neutral: el usuario aporta su contenido (modelo VLC).
2. Todo funciona offline una vez catalogado.
3. Ninguna función exige entender servidores, puertos o "bibliotecas".

## Arranque rápido (Windows)
Dos scripts en la raíz del repo para no tener que levantar GALA a mano:

- **`gala.bat`** — uso diario. Se sitúa en la carpeta del repo, mata
  cualquier `gala.exe` anterior (evita el error de puerto ocupado), añade
  `ffmpeg\` al PATH de la sesión, compila `gala.exe` si no existe (avisando)
  y lo arranca — el propio binario abre el navegador solo. Si algo falla, la
  ventana NO se cierra de golpe: se queda en pausa para que puedas leer el
  error.
  ```
  gala.bat
  ```
- **`gala-build.bat`** — recompilar tras cambios de código. Mata instancias
  previas, reconstruye el frontend (`cd web && npm run build`) SOLO si algo
  bajo `web/src`, `web/static` o su configuración es más reciente que el
  último build (o si se lo pides explícitamente), y compila `gala.exe`.
  Al terminar, ya puedes lanzar `gala.bat`.
  ```
  gala-build.bat          # build del frontend solo si hace falta
  gala-build.bat --web    # fuerza reconstruir el frontend siempre
  ```

Ambos requieren Go y, la primera vez (o tras tocar el frontend), Node.js —
ver "Compilar" y "Frontend (desarrollo)" más abajo. `ffmpeg/` es opcional
(ver sección "ffmpeg" más abajo); sin él, GALA arranca igual y degrada a
direct play.

## Compilar
Requisitos: Go >= 1.22, Node.js >= 20.

El frontend (SvelteKit, `web/`) se compila a `web/dist/`, que **no se
versiona**: `go:embed` lo incorpora al binario, así que hay que construirlo
ANTES de cualquier `go build`/`go vet`/`go test` (si no, el build de Go falla
con "pattern all:dist: no matching files found").
```
cd web && npm ci && npm run build   # vuelca a web/dist/
cd ..
go build -o gala ./cmd/gala         # Linux/macOS
go build -o gala.exe ./cmd/gala     # Windows
```
Al ejecutarlo crea `gala.json` y `gala.db` junto al binario (portable) y abre
el navegador en `http://127.0.0.1:7420`. Flag útil: `--no-browser`.

## Frontend (desarrollo)
Ver `web/README.md` para el detalle completo (arranque con proxy a la API,
tests, sistema de diseño). Resumen rápido:
```
# Terminal 1: el backend real, sirviendo la API en :7420
go run ./cmd/gala

# Terminal 2: el frontend con recarga en caliente, proxy de /api/v1/* a :7420
cd web && npm install && npm run dev
```

## ffmpeg (desarrollo)
GALA usa ffmpeg/ffprobe para sondear contenedores/códecs y remuxear a HLS
cuando el contenedor no es compatible con el navegador (M4). **No se
empaquetan todavía** (eso es M11: build LGPL recortada por plataforma); en
desarrollo, colócalos a mano junto al ejecutable, en cualquiera de estos
sitios (se buscan en este orden):

1. `ffmpeg/ffmpeg(.exe)` y `ffmpeg/ffprobe(.exe)` — carpeta junto al binario
   (la misma que usará el empaquetado de M11).
2. `ffmpeg(.exe)` / `ffprobe(.exe)` sueltos, junto al binario.
3. El PATH del sistema.

Sin ffmpeg, GALA arranca igual: solo hace direct play (sin remux) y avisa una
vez en el log al arrancar. Descarga binarios estáticos desde
https://ffmpeg.org/download.html (build LGPL, sin componentes GPL, para
mantener la licencia de distribución).

Para generar la matriz de prueba sintética de M4 (barras SMPTE + tono, sin
depender de contenido real):
```
./scripts/make_testmedia.ps1   # Windows
./scripts/make_testmedia.sh    # Linux/macOS
```
Los clips se generan en `testdata/media/` (no se versionan; el script sí).

## Estructura
Ver `GALA_Plan_Desarrollo_MVP1.md` §4. El frontend (`web/`) es un proyecto
SvelteKit real (Svelte 5, `adapter-static` en modo SPA — sin SSR, fallback a
`index.html` para el enrutado del lado cliente). Su build se vuelca a
`web/dist/` y `go:embed` (`web/web.go`) lo incorpora al binario.

## Licencia
AGPL-3.0. Añade el texto completo de la licencia en `LICENSE` antes del primer
push público (https://www.gnu.org/licenses/agpl-3.0.txt). Las contribuciones
externas requieren firmar el CLA (ver CONTRIBUTING.md).
