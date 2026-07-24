#!/usr/bin/env bash
# GALA - genera la matriz de prueba de M4 (Brief M4 §7): clips sintéticos
# (barras SMPTE + tono) para probar decide/probe/streaming sin depender de
# contenido real. Los binarios resultantes NO se versionan (testdata/media/
# está en .gitignore); este script sí. Equivalente a make_testmedia.ps1
# (mismos nombres de archivo, misma matriz) para desarrollo fuera de Windows.
#
# Uso:
#   ./scripts/make_testmedia.sh
#   FFMPEG=/ruta/a/ffmpeg ./scripts/make_testmedia.sh
set -uo pipefail

FFMPEG="${FFMPEG:-ffmpeg}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${OUT_DIR:-$SCRIPT_DIR/../testdata/media}"
mkdir -p "$OUT_DIR"

new_clip() {
  local name="$1" duration="$2" vcodec="$3" acodec="$4" vbitrate="$5"
  local out="$OUT_DIR/$name"
  echo "Generando $name ..."
  if ! "$FFMPEG" -y \
      -f lavfi -i "smptebars=size=1280x720:rate=30" \
      -f lavfi -i "sine=frequency=440:sample_rate=48000" \
      -t "$duration" \
      -c:v "$vcodec" -b:v "$vbitrate" -pix_fmt yuv420p \
      -c:a "$acodec" -b:a 192k \
      "$out" >/dev/null 2>&1; then
    echo "AVISO: no se pudo generar $name (¿$vcodec/$acodec no disponibles en este ffmpeg?); se omite." >&2
    return
  fi
  local size
  size=$(stat -c%s "$out" 2>/dev/null || stat -f%z "$out")
  local size_mb=$((size / 1024 / 1024))
  if [ "$size" -lt $((10 * 1024 * 1024)) ]; then
    echo "AVISO: $name = ${size_mb} MB, por debajo del filtro de tamaño de M2 (10 MB); sube vbitrate." >&2
  else
    echo "  OK (${size_mb} MB)"
  fi
}

# --- Matriz corta (10 s): un caso por celda de contenedor x códecs (Brief §7) ---
new_clip "direct_h264_aac.mp4" 10  libx264 aac 15M
new_clip "remux_h264_aac.mkv"  10  libx264 aac 15M
new_clip "reject_hevc_aac.mkv" 10  libx265 aac 15M
new_clip "reject_h264_ac3.mkv" 10  libx264 ac3 15M

# --- Clips largos (180 s): seeking en direct y en remux (Brief §7) --------------
new_clip "seek_h264_aac_long.mp4" 180 libx264 aac 1500k
new_clip "seek_h264_aac_long.mkv" 180 libx264 aac 1500k

echo
echo "Listo. Clips en $OUT_DIR"
echo "go test ./internal/media/... -run TestProbe -v   # verifica contra estos clips"
