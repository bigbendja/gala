#!/usr/bin/env bash
# GALA - genera la matriz de prueba de M4 (Brief M4 §7): clips sintéticos
# (barras SMPTE + tono) para probar decide/probe/streaming sin depender de
# contenido real. Los binarios resultantes NO se versionan (testdata/media/
# está en .gitignore); este script sí. Equivalente a make_testmedia.ps1
# (mismos nombres de archivo, misma matriz) para desarrollo fuera de Windows.
#
# Busca ffmpeg en este orden (igual que internal/media/ffmpeg.go): (1)
# carpeta ffmpeg/ junto al repo, (2) ffmpeg suelto junto al repo, (3) PATH.
# O pásalo explícito con FFMPEG=/ruta/a/ffmpeg.
#
# Si un códec concreto no está en tu build de ffmpeg (p. ej. libx265
# recortado), ese clip se omite con un aviso; el resto de la matriz se
# genera igual. Si un clip queda por debajo del filtro de tamaño de M2
# (10 MB), el script reintenta automáticamente con más bitrate.
#
# Uso:
#   ./scripts/make_testmedia.sh
#   FFMPEG=/ruta/a/ffmpeg ./scripts/make_testmedia.sh
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT_DIR="${OUT_DIR:-$SCRIPT_DIR/../testdata/media}"
mkdir -p "$OUT_DIR"

# --- Localizar ffmpeg -------------------------------------------------------

resolve_ffmpeg() {
  if [ -n "${FFMPEG:-}" ]; then
    if [ -x "$FFMPEG" ] || command -v "$FFMPEG" >/dev/null 2>&1; then
      echo "$FFMPEG"
      return 0
    fi
    echo "No existe el ffmpeg indicado: $FFMPEG" >&2
    return 1
  fi
  for candidate in "$REPO_ROOT/ffmpeg/ffmpeg" "$REPO_ROOT/ffmpeg"; do
    if [ -x "$candidate" ]; then
      echo "$candidate"
      return 0
    fi
  done
  if command -v ffmpeg >/dev/null 2>&1; then
    command -v ffmpeg
    return 0
  fi
  echo "No se encontró ffmpeg: ni en '$REPO_ROOT/ffmpeg/', ni junto al repo, ni en PATH. Instálalo o pásalo con FFMPEG=/ruta/a/ffmpeg." >&2
  return 1
}

FFMPEG_BIN="$(resolve_ffmpeg)" || exit 1
echo "Usando ffmpeg: $FFMPEG_BIN"
echo

# --- Generación de un clip, con reintento si se queda corto -----------------

MIN_CLIP_BYTES=$((10 * 1024 * 1024)) # filtro de tamaño de M2 (Brief §7)
MAX_ATTEMPTS=3
BITRATE_GROWTH="1.8"

results_name=()
results_size=()
results_status=()

new_clip() {
  local name="$1" duration="$2" vcodec="$3" acodec="$4" bitrate_kbps="$5"
  local out="$OUT_DIR/$name"
  local attempt=1

  while [ "$attempt" -le "$MAX_ATTEMPTS" ]; do
    echo "Generando $name (intento $attempt/$MAX_ATTEMPTS, -b:v ${bitrate_kbps}k) ..."
    # testsrc2 (no smptebars): smptebars es una imagen ESTÁTICA; con
    # contenido casi sin cambios frame a frame, libx264/libx265 en ABR usa
    # muchísimos menos bits de los pedidos por -b:v (verificado: a 64,8 Mbps
    # generaba un mp4 de 0,2 MB) — testsrc2 tiene patrones en movimiento que
    # sí obligan al codificador a acercarse al bitrate pedido.
    if ! "$FFMPEG_BIN" -y -hide_banner -loglevel error \
        -f lavfi -i "testsrc2=size=1280x720:rate=30" \
        -f lavfi -i "sine=frequency=440:sample_rate=48000" \
        -t "$duration" \
        -c:v "$vcodec" -b:v "${bitrate_kbps}k" -pix_fmt yuv420p \
        -c:a "$acodec" -b:a 192k \
        "$out"; then
      echo "AVISO: no se pudo generar $name (¿$vcodec/$acodec no disponibles en este ffmpeg?); se omite." >&2
      results_name+=("$name"); results_size+=("0"); results_status+=("omitido")
      return
    fi

    local size size_mb
    size=$(stat -c%s "$out" 2>/dev/null || stat -f%z "$out")
    size_mb=$((size / 1024 / 1024))
    if [ "$size" -ge "$MIN_CLIP_BYTES" ]; then
      echo "  OK (${size_mb} MB)"
      results_name+=("$name"); results_size+=("$size_mb"); results_status+=("OK")
      return
    fi

    if [ "$attempt" -lt "$MAX_ATTEMPTS" ]; then
      echo "AVISO: $name = ${size_mb} MB, por debajo del filtro de tamaño de M2 (10 MB); subiendo bitrate y reintentando..." >&2
      bitrate_kbps=$(awk -v b="$bitrate_kbps" -v g="$BITRATE_GROWTH" 'BEGIN { printf "%d", (b * g) + 1 }')
    else
      echo "AVISO: $name = ${size_mb} MB tras $MAX_ATTEMPTS intentos, sigue por debajo de 10 MB." >&2
      results_name+=("$name"); results_size+=("$size_mb"); results_status+=("bajo 10 MB")
      return
    fi
    attempt=$((attempt + 1))
  done
}

# Bitrates verificados a mano con testsrc2 contra un ffmpeg 8.1 real:
# 12000 kbps/10s -> ~13-14 MB; 600 kbps/180s -> ~17 MB. Margen cómodo sobre
# el filtro de tamaño de M2 (10 MB) sin generar archivos innecesariamente
# grandes.

# --- Matriz corta (10 s): un caso por celda de contenedor x códecs (Brief §7) ---
new_clip "direct_h264_aac.mp4" 10  libx264 aac 12000
new_clip "remux_h264_aac.mkv"  10  libx264 aac 12000
new_clip "reject_hevc_aac.mkv" 10  libx265 aac 12000
new_clip "audiotranscode_h264_ac3.mkv" 10  libx264 ac3 12000

# --- Clips largos (180 s): seeking en direct y en remux (Brief §7) --------------
new_clip "seek_h264_aac_long.mp4" 180 libx264 aac 600
new_clip "seek_h264_aac_long.mkv" 180 libx264 aac 600

# --- Resumen -----------------------------------------------------------------

echo
echo "=== Resumen ==="
problems=0
for i in "${!results_name[@]}"; do
  printf "  %-24s %6s MB   %s\n" "${results_name[$i]}" "${results_size[$i]}" "${results_status[$i]}"
  [ "${results_status[$i]}" != "OK" ] && problems=$((problems + 1))
done
if [ "$problems" -gt 0 ]; then
  echo "AVISO: $problems de ${#results_name[@]} clip(s) con problemas (omitidos o por debajo de 10 MB). Revisa si tu build de ffmpeg trae libx265, o sube el bitrate a mano en el script." >&2
else
  echo "Los ${#results_name[@]} clips superan el filtro de tamaño de M2 (10 MB)."
fi

echo
echo "Clips en $OUT_DIR"
echo "go test ./internal/media/... -run TestProbeAgainstGeneratedClips -v   # verifica contra estos clips"
