# GALA

**Tu cine, tus fuentes.** Cataloga y reproduce tu contenido de vídeo — archivos locales,
listas M3U, enlaces — en una sola galería. Sin configuración: doble clic y ya.
Funciona sin internet una vez catalogado, y comparte tu galería por wifi local
con los móviles de casa sin instalar nada en ellos.

> Estado: **F0 — esqueleto**. Ver `docs/` para la especificación y el plan de desarrollo.

## Reglas de hierro del proyecto
1. GALA nunca incluye, enlaza ni sugiere contenido o listas. Es una herramienta neutral: el usuario aporta su contenido (modelo VLC).
2. Todo funciona offline una vez catalogado.
3. Ninguna función exige entender servidores, puertos o "bibliotecas".

## Compilar (F0)
Requisitos: Go >= 1.22.
```
go build -o gala ./cmd/gala        # Linux/macOS
go build -o gala.exe ./cmd/gala    # Windows
```
Al ejecutarlo crea `gala.json` y `gala.db` junto al binario (portable) y abre
el navegador en `http://127.0.0.1:7420`. Flag útil: `--no-browser`.

## Estructura
Ver `GALA_Plan_Desarrollo_MVP1.md` §4. El frontend real (SvelteKit) sustituirá
al placeholder de `web/dist/` en M10; el build de SvelteKit (adapter-static)
se vuelca a ese mismo directorio y `go:embed` lo incorpora al binario.

## Licencia
AGPL-3.0. Añade el texto completo de la licencia en `LICENSE` antes del primer
push público (https://www.gnu.org/licenses/agpl-3.0.txt). Las contribuciones
externas requieren firmar el CLA (ver CONTRIBUTING.md).
