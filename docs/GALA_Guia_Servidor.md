# GALA en un servidor Ubuntu — guía de instalación

Para tener GALA siempre encendido en un servidor (un PC viejo, un mini-PC…) y
verlo desde cualquier equipo o móvil de tu red, sin instalar nada en ellos.

> **Importante — solo red local.** GALA habla HTTP sin cifrar y la contraseña de
> administrador viaja en claro por la red. **No reenvíes el puerto del router a
> Internet** ni lo expongas de ninguna otra forma. Si necesitas administrarlo
> desde fuera de casa, usa un túnel SSH (ver más abajo).

## Qué necesitas

- Un servidor con **Ubuntu 22.04 o 24.04 de 64 bits (amd64)**. Sirve hardware modesto.
- Acceso por SSH con un usuario que pueda usar `sudo`.
- El binario `gala-linux-amd64` (ver el paso 1) y el fichero `install.sh`.
- Tu contenido en una carpeta del servidor (por defecto `/srv/media`).

## Cómo queda instalado

Todo vive en una sola carpeta, junto al ejecutable:

```
/opt/gala/gala             el programa
/opt/gala/gala-preflight   comprobación previa a cada arranque (revierte una actualización fallida)
/opt/gala/gala.json        configuración
/opt/gala/gala.db          base de datos (catálogo, sesiones…)
/opt/gala/cache/           carátulas y temporales
/srv/media/                tu contenido (o la carpeta que elijas)
/etc/systemd/system/gala.service
```

GALA corre con un usuario de sistema propio, `gala`, sin privilegios.

## 1. Conseguir el binario

Mientras el repositorio sea privado no se puede descargar anónimamente; tú lo
llevas al servidor. Elige una de estas dos vías, en **tu PC**.

**a) Desde la integración continua (recomendado).** Cada subida a `main` compila
`gala-linux-amd64` y lo guarda como artefacto:

```bash
gh run list --branch main --workflow CI --limit 1
gh run download <ID-del-run> -n gala-linux-amd64
```

**b) Compilándolo tú** (hace falta Go 1.22+ y Node 22):

```bash
cd web && npm ci && npm run build && cd ..
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o gala-linux-amd64 ./cmd/gala
```

Apunta el SHA256 para comprobarlo en el servidor:

```bash
sha256sum gala-linux-amd64
```

## 2. Copiar al servidor e instalar

```bash
scp gala-linux-amd64 scripts/install.sh usuario@IP-DEL-SERVIDOR:/tmp/
ssh usuario@IP-DEL-SERVIDOR
sudo sh /tmp/install.sh --binary /tmp/gala-linux-amd64 --sha256 <el-hash-de-arriba>
```

El instalador (se puede repetir cuantas veces quieras):

1. instala `ffmpeg` y `curl` con `apt`;
2. crea el usuario `gala` y las carpetas;
3. comprueba el binario (SHA256 y que se ejecuta) y lo coloca en `/opt/gala/gala`;
4. crea `gala.json` **solo si no existe**, instala `gala-preflight` y el servicio systemd,
   y arranca GALA;
5. añade una regla de `ufw` que permite el puerto **solo desde tu red local**
   (la detecta sola; si no puede, usa `--subnet 192.168.1.0/24`) y no activa `ufw`
   por ti: si lo tienes desactivado te avisa;
6. comprueba que GALA responde y te dice la dirección para abrirlo.

Opciones útiles: `--media /ruta`, `--port 7420`, `--subnet CIDR`, `--no-firewall`,
`--skip-apt`. `sh install.sh --help` las lista todas.

> **Antes de activar `ufw`:** permite SSH o te quedarás fuera.
> `sudo ufw allow OpenSSH && sudo ufw enable`

## 3. Primera configuración

1. **Contraseña de administrador.** Hazlo como el usuario `gala` (no como root, o la
   base de datos quedaría con otro dueño):

   ```bash
   sudo -u gala /opt/gala/gala admin set-password
   ```

   (Si estabas en una terminal, el instalador te lo ofrece al final.)

2. **Abre GALA** desde tu portátil o móvil: `http://IP-DEL-SERVIDOR:7420`.
3. Pulsa **Administrar**, entra con la contraseña y, en **Ajustes**, añade tus
   carpetas (por ejemplo `/srv/media`) y la clave de TMDB. Ya no hace falta volver al
   servidor para configurar nada.
4. El usuario `gala` debe poder **leer** tu contenido:

   ```bash
   sudo chmod -R a+rX /srv/media
   ```

Para ver GALA solo con un PIN en el móvil, activa el Modo LAN y el PIN desde el propio
servidor (ver «Túnel SSH»); el PIN protege *ver*, la contraseña protege *administrar*.

## Actualizar

- **Desde la app:** Ajustes › Actualización › «Buscar actualizaciones». Descarga la
  versión nueva, la verifica y reinicia GALA sola; si la versión nueva no arranca, se
  vuelve a la anterior automáticamente. *Requiere que el repositorio sea público (hoy no
  lo es: el botón dirá «No se pudo comprobar»).*
- **A mano (mientras tanto):** repite el paso 1 y 2 con el binario nuevo. `install.sh`
  conserva tu configuración, tu base de datos y tu contraseña, y reinicia el servicio.
- **Si compilas en el servidor** (solo desarrollo): `scripts/gala-update` hace
  `git pull`, compila y reinicia.

### Volver a la versión anterior a mano

Cada actualización desde la app deja el programa anterior como `gala.prev` y una copia
de la base de datos de antes de actualizar, `gala.db.pre-update`. Si algo queda mal
aunque GALA arranque:

```bash
sudo systemctl stop gala
sudo mv /opt/gala/gala /opt/gala/gala.malo
sudo mv /opt/gala/gala.prev /opt/gala/gala
# Solo si ves datos raros tras actualizar: vuelve también a la base de datos de antes.
sudo -u gala rm -f /opt/gala/gala.db-wal /opt/gala/gala.db-shm
sudo -u gala cp /opt/gala/gala.db.pre-update /opt/gala/gala.db
sudo systemctl start gala
```

(Lo que hayas hecho en GALA después de actualizar —progreso de reproducción, listas—
se pierde al restaurar la copia.)

## Contraseña olvidada

Por SSH, en cualquier momento:

```bash
sudo -u gala /opt/gala/gala admin set-password
```

Cierra todas las sesiones abiertas. Desde el propio servidor (o por túnel SSH) GALA
siempre se puede administrar sin contraseña: no te puedes quedar fuera.

## Túnel SSH

Para administrar sin abrir nada a la red, o desde fuera de casa:

```bash
ssh -L 7420:localhost:7420 usuario@IP-DEL-SERVIDOR
```

y abre `http://localhost:7420` en tu equipo. Para GALA eres «el propio servidor»: sin
contraseña y con acceso al Modo LAN, al PIN y a todo Ajustes.

## Día a día

```bash
sudo systemctl status gala          # ¿está en marcha?
sudo journalctl -u gala -n 100      # registro
sudo systemctl restart gala         # reiniciar
curl http://localhost:7420/healthz  # {"status":"ok"} si todo va bien
```

## Si algo falla

| Síntoma | Qué mirar |
|---|---|
| No abre desde otro equipo | `sudo ufw status` (¿regla del puerto 7420 desde tu red?), y que el equipo esté en la misma red. |
| «No se pudo comprobar las actualizaciones» | Normal con el repositorio privado o sin Internet. No afecta a nada más. |
| No ve mis películas | Permisos: `sudo chmod -R a+rX /srv/media`; y que la carpeta esté añadida en Ajustes. |
| Solo reproduce algunos vídeos | `ffmpeg` no instalado: `sudo apt install ffmpeg`. |
| «El disco está lleno» al actualizar | Libera espacio; no se ha cambiado nada. |
| El servicio no arranca | `sudo journalctl -u gala -n 50`. Si fue tras actualizar, `gala-preflight` revierte solo tras 3 intentos. |

## Camino definitivo: descarga anónima (cuando el repositorio sea público)

Se publicará `install.sh` junto a cada release, y bastará con:

```bash
curl -fsSL https://github.com/bigbendja/gala/releases/latest/download/install.sh | sudo sh -s -- --download
```

`--download` baja el último release, verifica su SHA256 contra `SHA256SUMS.txt` y
continúa igual que arriba (`--version vX.Y.Z` fija una versión). Hoy no funciona
porque el repositorio es privado.

## Quitar GALA

```bash
sudo systemctl disable --now gala
sudo rm /etc/systemd/system/gala.service && sudo systemctl daemon-reload
sudo rm -rf /opt/gala      # borra también la base de datos y la configuración
sudo userdel gala
```

Tu contenido en `/srv/media` no se toca.
