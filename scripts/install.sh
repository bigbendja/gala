#!/bin/sh
# install.sh — instala GALA como servicio en un servidor Ubuntu/Debian (amd64).
#
# Dos caminos para conseguir el binario:
#   a) --binary RUTA      un binario linux-amd64 que ya tienes en el servidor
#                         (p. ej. copiado con scp). No hace falta token ni
#                         clonar el repositorio. Es el camino mientras el
#                         repositorio sea privado.
#   b) --download         baja el último release (o --version vX.Y.Z) de GitHub
#                         sin autenticarse y verifica su SHA256. Solo funciona
#                         cuando el repositorio es público.
#
# Es idempotente: puedes ejecutarlo otra vez para reparar o actualizar el binario.
# No toca gala.json, la base de datos ni la contraseña si ya existen.
#
# Uso:  sudo sh install.sh --binary /tmp/gala-linux-amd64 [opciones]
#       sudo sh install.sh --help

set -eu

GALA_HOME=${GALA_HOME:-/opt/gala}
REPO=${GALA_REPO:-bigbendja/gala}
ASSET=gala-linux-amd64

BINARY=""
SHA256=""
DOWNLOAD=0
VERSION=latest
SUBNET=""
MEDIA=/srv/media
PORT=7420
FIREWALL=1
APT=1

say() { printf '%s\n' "$*"; }
step() { printf '\n==> %s\n' "$*"; }
warn() { printf 'AVISO: %s\n' "$*" >&2; }
die() {
	printf 'ERROR: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'USAGE'
Instala GALA como servicio systemd en Ubuntu/Debian (amd64).

  sudo sh install.sh --binary RUTA      usa un binario linux-amd64 que ya tienes
  sudo sh install.sh --download         baja el último release de GitHub (repo público)

Opciones:
  --binary RUTA      binario gala-linux-amd64 ya copiado al servidor
  --sha256 HASH      comprueba el binario de --binary contra este SHA256
  --download         descargar de GitHub Releases (verifica el SHA256)
  --version vX.Y.Z   con --download: una versión concreta (por defecto, la última)
  --subnet CIDR      red local con acceso al puerto (p. ej. 192.168.1.0/24);
                     por defecto se detecta la de la tarjeta de red principal
  --media RUTA       carpeta de contenido a crear si no existe (por defecto /srv/media)
  --port N           puerto de GALA (por defecto 7420; solo en una instalación nueva)
  --no-firewall      no tocar ufw
  --skip-apt         no instalar paquetes (ffmpeg, curl) con apt
  -h, --help         esta ayuda

Si no indicas --binary ni --download, se busca gala-linux-amd64 junto a este
script o en la carpeta actual.
USAGE
}

# --- argumentos ---------------------------------------------------------------
while [ $# -gt 0 ]; do
	case "$1" in
	--binary) [ $# -ge 2 ] || die "--binary necesita una ruta"; BINARY=$2; shift 2 ;;
	--sha256) [ $# -ge 2 ] || die "--sha256 necesita un hash"; SHA256=$2; shift 2 ;;
	--download) DOWNLOAD=1; shift ;;
	--version) [ $# -ge 2 ] || die "--version necesita una versión (p. ej. v0.10.0)"; VERSION=$2; shift 2 ;;
	--subnet) [ $# -ge 2 ] || die "--subnet necesita una red (p. ej. 192.168.1.0/24)"; SUBNET=$2; shift 2 ;;
	--media) [ $# -ge 2 ] || die "--media necesita una ruta"; MEDIA=$2; shift 2 ;;
	--port) [ $# -ge 2 ] || die "--port necesita un número"; PORT=$2; shift 2 ;;
	--no-firewall) FIREWALL=0; shift ;;
	--skip-apt) APT=0; shift ;;
	-h | --help) usage; exit 0 ;;
	*) die "opción desconocida: $1 (mira --help)" ;;
	esac
done

case "$PORT" in '' | *[!0-9]*) die "--port debe ser un número" ;; esac
[ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] || die "--port fuera de rango (1-65535)"

# --- comprobaciones -------------------------------------------------------------
step "Comprobando el sistema"
[ "$(id -u)" -eq 0 ] || die "ejecútalo como administrador: sudo sh install.sh ..."
case "$(uname -m)" in
x86_64 | amd64) ;;
*) die "esta instalación es solo para amd64 (este equipo es $(uname -m))" ;;
esac
command -v systemctl >/dev/null 2>&1 || die "hace falta systemd (systemctl no está disponible)"
if [ -r /etc/os-release ]; then
	# shellcheck disable=SC1091
	. /etc/os-release
	case "${ID:-}${ID_LIKE:-}" in
	*ubuntu* | *debian*) ;;
	*) warn "pensado para Ubuntu/Debian; en ${PRETTY_NAME:-este sistema} puede fallar la instalación de paquetes" ;;
	esac
fi

# --- de dónde sale el binario ------------------------------------------------------
if [ -z "$BINARY" ] && [ "$DOWNLOAD" -eq 0 ]; then
	here=$(cd "$(dirname "$0")" 2>/dev/null && pwd || echo .)
	for cand in "$here/$ASSET" "./$ASSET"; do
		if [ -f "$cand" ]; then
			BINARY=$cand
			break
		fi
	done
fi
if [ -z "$BINARY" ] && [ "$DOWNLOAD" -eq 0 ]; then
	cat >&2 <<EOF
ERROR: no sé de dónde sacar el binario de GALA.

  - Si ya lo tienes en el servidor (por ejemplo copiado con scp):
        sudo sh install.sh --binary /ruta/a/$ASSET
  - Si el repositorio es público, para bajar el último release:
        sudo sh install.sh --download
EOF
	exit 1
fi
if [ -n "$BINARY" ] && [ ! -f "$BINARY" ]; then
	die "no existe el binario: $BINARY"
fi
[ -z "$BINARY" ] || [ "$DOWNLOAD" -eq 0 ] || die "usa --binary o --download, no los dos"

# --- paquetes --------------------------------------------------------------------------
if [ "$APT" -eq 1 ]; then
	step "Instalando ffmpeg y curl"
	command -v apt-get >/dev/null 2>&1 || die "no encuentro apt-get; instala ffmpeg y curl a mano y usa --skip-apt"
	DEBIAN_FRONTEND=noninteractive apt-get update -qq
	DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ffmpeg curl ca-certificates
else
	command -v ffmpeg >/dev/null 2>&1 || warn "ffmpeg no está instalado: GALA solo podrá reproducir directamente (sin remux)"
fi

# --- usuario y carpetas -------------------------------------------------------------------
step "Preparando el usuario 'gala' y las carpetas"
if ! id gala >/dev/null 2>&1; then
	useradd --system --user-group --home-dir "$GALA_HOME" --no-create-home --shell /usr/sbin/nologin gala
	say "Usuario de sistema 'gala' creado."
fi
FRESH=0
[ -f "$GALA_HOME/gala.json" ] || FRESH=1
mkdir -p "$GALA_HOME"
if [ ! -d "$MEDIA" ]; then
	mkdir -p "$MEDIA"
	chmod 755 "$MEDIA"
	say "Carpeta de contenido creada: $MEDIA"
fi

# --- el binario ----------------------------------------------------------------------------
step "Preparando el binario"
NEW="$GALA_HOME/gala.new"
rm -f "$NEW"
if [ "$DOWNLOAD" -eq 1 ]; then
	command -v curl >/dev/null 2>&1 || die "hace falta curl para --download"
	if [ "$VERSION" = latest ]; then
		BASE="https://github.com/$REPO/releases/latest/download"
	else
		BASE="https://github.com/$REPO/releases/download/$VERSION"
	fi
	TMP=$(mktemp -d)
	trap 'rm -rf "$TMP"' EXIT
	if ! curl -fsSL --retry 3 -o "$TMP/$ASSET" "$BASE/$ASSET" ||
		! curl -fsSL --retry 3 -o "$TMP/SHA256SUMS.txt" "$BASE/SHA256SUMS.txt"; then
		die "no se pudo descargar GALA desde GitHub. Si el repositorio es privado, la descarga anónima no funciona: copia el binario al servidor y usa --binary RUTA."
	fi
	want=$(awk -v f="$ASSET" '{n=$2; sub(/^\*/, "", n)} n==f {print $1; exit}' "$TMP/SHA256SUMS.txt")
	[ -n "$want" ] || die "SHA256SUMS.txt no incluye $ASSET"
	got=$(sha256sum "$TMP/$ASSET" | awk '{print $1}')
	[ "$want" = "$got" ] || die "el SHA256 de la descarga no coincide (esperado $want, obtenido $got); no se instala nada"
	say "Descarga verificada (SHA256 correcto)."
	cp "$TMP/$ASSET" "$NEW"
else
	got=$(sha256sum "$BINARY" | awk '{print $1}')
	if [ -n "$SHA256" ]; then
		want=$(printf '%s' "$SHA256" | tr 'A-F' 'a-f')
		[ "$want" = "$got" ] || die "el SHA256 de $BINARY no coincide (esperado $want, obtenido $got); no se instala nada"
		say "SHA256 verificado."
	else
		say "SHA256 de $BINARY: $got"
		say "(compáralo con el de tu compilación; usa --sha256 para que se compruebe solo)"
	fi
	cp "$BINARY" "$NEW"
fi
chmod 0755 "$NEW"
chown gala:gala "$NEW"
# Comprobar que ejecuta en este equipo y es GALA, con el usuario sin privilegios.
if ! out=$(runuser -u gala -- "$NEW" --version 2>&1) || ! printf '%s' "$out" | grep -q '^GALA '; then
	rm -f "$NEW"
	die "el binario no se puede ejecutar aquí (¿es linux-amd64?): $out"
fi
say "$out"

# --- gala-preflight y la unit --------------------------------------------------------------
step "Instalando gala-preflight y el servicio systemd"
PRE="$GALA_HOME/gala-preflight"
cat >"$PRE.new" <<'GALA_PREFLIGHT_EOF'
#!/bin/sh
# gala-preflight — se ejecuta como ExecStartPre de la unit systemd, ANTES de
# cada arranque de GALA. Decide el rollback de una actualización sin depender
# de que el binario nuevo llegue a main(): un binario que ni arranca no puede
# revertirse a sí mismo, este script sí.
#
# Solo actúa si existe `update-pending` (junto al binario). El marcador es texto
# clave=valor: state, from_version, new_version, attempts, migraciones_nuevas.
# Siempre sale con 0: que systemd arranque lo que haya quedado en `gala`.
#
# Variables (para pruebas): GALA_DIR, GALA_BIN, GALA_MAX_ATTEMPTS (3),
# GALA_SELFCHECK_TIMEOUT (30 s).

DIR=${GALA_DIR:-$(cd "$(dirname "$0")" && pwd)}
BIN=${GALA_BIN:-$DIR/gala}
PREV="$BIN.prev"
NEW="$BIN.new"
M="$DIR/update-pending"
R="$DIR/update-result"
BK="$DIR/gala.db.pre-update.new"
DB="$DIR/gala.db"
MAX=${GALA_MAX_ATTEMPTS:-3}
TMO=${GALA_SELFCHECK_TIMEOUT:-30}

[ -f "$M" ] || exit 0

log() { echo "gala-preflight: $*" >&2; }
get() { sed -n "s/^$1=//p" "$M" | head -n 1 | tr -d '\r'; }

STATE=$(get state)
FROM=$(get from_version)
TO=$(get new_version)
ATT=$(get attempts)
MIG=$(get migraciones_nuevas)
case "$ATT" in '' | *[!0-9]*) ATT=0 ;; esac
[ "$MIG" = 1 ] || MIG=0

write_marker() { # state attempts migraciones
	printf 'state=%s\nfrom_version=%s\nnew_version=%s\nattempts=%s\nmigraciones_nuevas=%s\n' \
		"$1" "$FROM" "$TO" "$2" "$3" >"$M.tmp.$$" && mv -f "$M.tmp.$$" "$M"
}

write_result() { # status motivo
	printf 'status=%s\nfrom_version=%s\nnew_version=%s\nreason=%s\nat=%s\n' \
		"$1" "$FROM" "$TO" "$2" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$R.tmp.$$" && mv -f "$R.tmp.$$" "$R"
}

# Revierte al binario anterior (y a la BD si la versión nueva migró el esquema).
rollback() { # motivo
	log "revirtiendo a $FROM: $1"
	if [ -f "$PREV" ]; then
		mv -f "$PREV" "$BIN"
	fi
	if [ "$MIG" = 1 ] && [ -f "$BK" ]; then
		rm -f "$DB-wal" "$DB-shm"
		mv -f "$BK" "$DB"
	fi
	rm -f "$NEW" "$BK" "$M"
	write_result rolled_back "$1"
	exit 0
}

# --- Reconciliar un corte durante el intercambio ---------------------------
# apply escribe el marcador (preparing) ANTES de los dos renombrados
# (gala → gala.prev, gala.new → gala) y lo pasa a swapped DESPUÉS.
if [ "$STATE" = preparing ]; then
	if [ ! -e "$BIN" ] && [ -f "$PREV" ]; then
		# Corte entre los dos renombrados: no hay `gala`. Vuelve el anterior.
		log "corte durante el intercambio: se restaura el binario anterior"
		mv -f "$PREV" "$BIN"
		rm -f "$NEW" "$BK" "$M"
		write_result rolled_back "La actualización se interrumpió durante la instalación; se mantiene $FROM."
		exit 0
	fi
	if [ -e "$BIN" ] && [ -f "$NEW" ]; then
		# El intercambio no llegó a hacerse: sigue el binario de siempre.
		log "la actualización se interrumpió antes de instalarse"
		rm -f "$NEW" "$BK" "$M"
		write_result rolled_back "La actualización se interrumpió antes de instalarse; se mantiene $FROM."
		exit 0
	fi
	# `gala` existe y ya no hay `gala.new`: el intercambio se hizo y solo faltó
	# anotarlo. Se trata como swapped.
	STATE=swapped
fi

# --- Arranque de una versión recién instalada ------------------------------
ATT=$((ATT + 1))
write_marker swapped "$ATT" "$MIG"

if [ "$ATT" -gt "$MAX" ]; then
	rollback "La versión nueva no llegó a arrancar bien tras $MAX intentos."
fi

# Autocomprobación del binario nuevo: abre la BD (aplica migraciones), NO liga
# ningún puerto. Salida 0 = sano; con `migraciones_aplicadas=N` en stdout.
if command -v timeout >/dev/null 2>&1; then
	OUT=$(timeout "$TMO" "$BIN" --selfcheck 2>&1)
else
	OUT=$("$BIN" --selfcheck 2>&1)
fi
RC=$?
if [ "$RC" -ne 0 ]; then
	log "autocomprobación fallida (código $RC): $OUT"
	rollback "La autocomprobación de la versión nueva falló."
fi
case "$OUT" in
*migraciones_aplicadas=[1-9]*)
	MIG=1
	write_marker swapped "$ATT" 1
	;;
esac
exit 0
GALA_PREFLIGHT_EOF
chmod 0755 "$PRE.new"
chown gala:gala "$PRE.new"
mv -f "$PRE.new" "$PRE"

UNIT=/etc/systemd/system/gala.service
cat >"$UNIT.new" <<'GALA_UNIT_EOF'
[Unit]
Description=GALA media center
After=network-online.target
Wants=network-online.target
# Sin límite de reintentos: systemd no debe rendirse antes de que gala-preflight
# tenga ocasión de revertir una actualización que no arranca.
StartLimitIntervalSec=0

[Service]
Type=simple
User=gala
Group=gala
WorkingDirectory=/opt/gala
# Antes de cada arranque: reconcilia/revierte una actualización a medias.
ExecStartPre=/opt/gala/gala-preflight
ExecStart=/opt/gala/gala --no-browser
# El auto-update reinicia GALA saliendo del proceso; systemd lo vuelve a levantar.
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/opt/gala

[Install]
WantedBy=multi-user.target
GALA_UNIT_EOF
if [ "$GALA_HOME" != /opt/gala ]; then
	sed -i "s#/opt/gala#$GALA_HOME#g" "$UNIT.new"
fi
chmod 0644 "$UNIT.new"
mv -f "$UNIT.new" "$UNIT"

# --- gala.json (solo en una instalación nueva) ---------------------------------------------------
if [ "$FRESH" -eq 1 ]; then
	cat >"$GALA_HOME/gala.json" <<EOF
{
  "port": $PORT,
  "bind": "0.0.0.0",
  "media_folders": [],
  "lan_pin": "",
  "tmdb_api_key": "",
  "home_hero_hidden": false,
  "update": { "channel": "stable", "auto_check": true }
}
EOF
	say "gala.json creado (puerto $PORT, accesible desde la red local)."
else
	say "gala.json ya existía: se conserva tal cual."
	PORT=$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$GALA_HOME/gala.json" | head -n 1)
	[ -n "$PORT" ] || PORT=7420
fi

# --- colocar el binario y arrancar ----------------------------------------------------------------
WAS_ACTIVE=0
if systemctl is-active --quiet gala 2>/dev/null; then WAS_ACTIVE=1; fi
mv -f "$NEW" "$GALA_HOME/gala"
chown -R gala:gala "$GALA_HOME"
chmod 640 "$GALA_HOME/gala.json"

step "Arrancando GALA"
systemctl daemon-reload
systemctl enable gala >/dev/null 2>&1 || true
if [ "$WAS_ACTIVE" -eq 1 ]; then
	systemctl restart gala
	say "Servicio reiniciado con el binario nuevo."
else
	systemctl start gala
fi

# --- cortafuegos -----------------------------------------------------------------------------------
if [ "$FIREWALL" -eq 1 ]; then
	step "Cortafuegos"
	if [ -z "$SUBNET" ]; then
		dev=$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* dev \([^ ]*\).*/\1/p' | head -n 1 || true)
		if [ -n "$dev" ]; then
			SUBNET=$(ip -4 route show dev "$dev" scope link 2>/dev/null | awk '$1 ~ /\// {print $1; exit}' || true)
		fi
	fi
	if ! command -v ufw >/dev/null 2>&1; then
		warn "ufw no está instalado: no se ha abierto ningún puerto. Si usas otro cortafuegos, permite el puerto $PORT/tcp solo desde tu red local."
	elif [ -z "$SUBNET" ]; then
		warn "no pude detectar tu red local. Ejecuta: sudo ufw allow from 192.168.1.0/24 to any port $PORT proto tcp (con tu red)."
	else
		ufw allow from "$SUBNET" to any port "$PORT" proto tcp comment 'GALA' >/dev/null
		say "ufw: permitido el puerto $PORT/tcp solo desde $SUBNET."
		if ! ufw status 2>/dev/null | grep -qi '^Status: active'; then
			warn "ufw está desactivado: la regla está guardada pero no se aplica. Cuando quieras activarlo, permite antes SSH (sudo ufw allow OpenSSH) y luego: sudo ufw enable"
		fi
	fi
fi

# --- comprobar que responde ----------------------------------------------------------------------------
step "Comprobando que GALA responde"
UP=0
if command -v curl >/dev/null 2>&1; then
	i=0
	while [ "$i" -lt 30 ]; do
		if curl -fsS -o /dev/null "http://127.0.0.1:$PORT/healthz" 2>/dev/null; then
			UP=1
			break
		fi
		i=$((i + 1))
		sleep 1
	done
fi
if [ "$UP" -eq 1 ]; then
	say "GALA está en marcha."
else
	warn "GALA no respondió en 30 s. Mira: sudo systemctl status gala  y  sudo journalctl -u gala -n 50"
fi

IP=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
[ -n "$IP" ] || IP="IP-DEL-SERVIDOR"

# --- contraseña (opcional, solo si hay teclado y es una instalación nueva) -----------------------------------
if [ "$FRESH" -eq 1 ] && [ -t 0 ] && [ -t 1 ]; then
	printf '\n¿Poner ahora la contraseña de administrador? [S/n] '
	read -r ans || ans=n
	case "$ans" in
	n | N | no | NO) ;;
	*) runuser -u gala -- "$GALA_HOME/gala" admin set-password || warn "no se pudo poner la contraseña; hazlo después (mira los siguientes pasos)" ;;
	esac
fi

cat <<EOF

==================================================================
 GALA instalado.

 Siguientes pasos
 1. Pon la contraseña de administrador (si no lo hiciste arriba).
    IMPORTANTE: como el usuario 'gala', no como root:
        sudo -u gala $GALA_HOME/gala admin set-password
 2. Desde otro equipo de tu red, abre:  http://$IP:$PORT
    Pulsa «Administrar», entra con la contraseña y, en Ajustes,
    añade tus carpetas (por ejemplo $MEDIA) y la clave de TMDB.
 3. El contenido debe poder leerlo el usuario 'gala':
        sudo chmod -R a+rX $MEDIA

 NO expongas el puerto $PORT a Internet: la contraseña y el tráfico
 viajan sin cifrar. Solo para tu red local (o por túnel SSH:
 ssh -L $PORT:localhost:$PORT usuario@servidor).

 Actualizar: botón en Ajustes › Actualización, o repite este script.
 Estado del servicio:  sudo systemctl status gala
==================================================================
EOF
