# M-Servidor — Notas de diseño (Auth y Auto-update)

Estado: **borrador para visto bueno**. No hay código de Auth ni de Auto-update
hasta que se aprueben estas notas. El Módulo 1 (Releases) ya está implementado
en `feat/releases`.

## 0. Decisiones cerradas (VB del titular)

1. **Sin `--config` ni `data_dir`.** Se mantiene «todo junto al ejecutable»:
   un único `/opt/gala/` con `gala` (binario), `gala.json`, `gala.db`, `cache/`
   y los ficheros de actualización. El §5 del encargo se corrige en ese sentido;
   `gala admin set-password` localiza `gala.json`/`gala.db` junto a su propio
   ejecutable (se ejecuta como `/opt/gala/gala admin set-password`).
2. **`media_dirs` fuera de `gala.json`**: las carpetas se gestionan por la UI.
3. **`x/crypto` antiguo compatible con Go 1.22** (p. ej. `v0.31.x`) y `x/term`;
   no se sube Go.
4. **Rollback con `gala-preflight`** en `ExecStartPre` (ver §2.4), con
   `--selfcheck`.
5. **argon2id t=2, m=64 MB, p=1** como constantes fáciles de cambiar; se
   re-mide en el servidor real.

Hecho ya conocido: hoy un cliente de red nunca puede administrar
(`lan.RemoteAllowed` es lista cerrada; el resto da 403). Auth no relaja eso:
añade una segunda vía (sesión de admin) sobre las mismas rutas.

---

## 1. Auth de administrador

### 1.1 Modelo de datos (migración `0008_admin_auth.sql`)

```sql
CREATE TABLE admin_auth (
  id INTEGER PRIMARY KEY CHECK (id = 1),      -- un único admin
  password_hash TEXT NOT NULL,                -- $argon2id$v=19$m=65536,t=2,p=1$sal$hash
  network_admin INTEGER NOT NULL DEFAULT 1,   -- toggle «Exigir clave para administrar desde la red»
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE sessions (
  id_hash TEXT PRIMARY KEY,                   -- SHA-256 del token; el token NUNCA se guarda
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL, last_seen TEXT NOT NULL,
  user_agent TEXT NOT NULL, ip TEXT NOT NULL
);
CREATE TABLE server_secret (id INTEGER PRIMARY KEY CHECK (id = 1), secret BLOB NOT NULL); -- 32 bytes, creado al primer arranque
```

- Token de sesión: 32 bytes de `crypto/rand` (256 bits), base64url, solo viaja en
  la cookie. En BD solo su SHA-256: una BD filtrada no da sesiones válidas.
- El secreto del servidor se usa para derivar el token CSRF y nunca sale por la API.
- El hash lleva sal y parámetros dentro; subir parámetros en el futuro no invalida
  claves antiguas (se re-hashea en el siguiente login correcto).

### 1.2 Parámetros argon2id (medidos)

Medido en un equipo de escritorio de 12 hilos (media de 5, 1 hilo, clave de 15 car.):

| t | m | tiempo |
|---|---|--------|
| 2 | 64 MB | **97 ms** |
| 3 | 64 MB | 148 ms |
| 2 | 32 MB | 44 ms |

Un servidor flojo (Atom/Celeron/Pi) suele ser 3–6× más lento → **~0,3–0,6 s** con
t=2, m=64 MB, p=1, dentro del objetivo. **Propuesta: t=2, m=64 MB, p=1**. Esto
es una medición en el PC de desarrollo, no en el servidor: antes de cerrar el
módulo la repito en el servidor real (te pediré un SSH o que ejecutes un
comando de medida de una línea) y ajusto si pasa de ~0,5 s.
Defensa de memoria: **máximo 2 verificaciones argon2 simultáneas** (semáforo);
cada una usa 64 MB y es un vector de DoS contra un servidor modesto.
Longitud de clave: mínimo 8, máximo 128 caracteres; sin reglas de composición.

### 1.3 Quién es admin

Una petición tiene permiso de administración si **(A) viene de localhost o
(B) trae sesión válida, con `network_admin` activo**.

- **Localhost** = `RemoteAddr` real en 127.0.0.0/8 o ::1 **y** cabecera `Host`
  también loopback (ya existe en `lan.IsLocal`; protege contra DNS rebinding).
  `X-Forwarded-For`/`X-Real-IP` se ignoran siempre.
- **Rescate**: localhost nunca se somete a clave, rate-limit ni CSRF de sesión
  (mantiene sus defensas actuales: `Content-Type: application/json` y Host
  loopback). El túnel SSH (`ssh -L`) llega como loopback, así que sigue siendo
  la vía de rescate; y `gala admin set-password` no necesita autenticación (es
  acceso al disco).
- **Reverse proxy**: si alguien pone nginx en el mismo servidor, todo el tráfico
  parece loopback → todos serían admin. Se documenta, y se añade
  `"admin_localhost_bypass": true` (por defecto) en `gala.json`; con `false`,
  localhost también necesita sesión (el rescate pasa a ser solo el CLI).
- **Sin clave configurada**: un cliente de red que toque una ruta admin recibe
  `401` con `{"error":"La administración desde la red no está configurada. Pon una contraseña desde el equipo o por SSH.","code":"auth_not_configured"}`. Sin ningún cambio para localhost (regresión
  cubierta por test).

**Rutas de administración** (todo lo que hoy NO está en `RemoteAllowed`, salvo
lo marcado): `/folders*`, `/scan*`, `/ingest*`, `/pending`, `/items/{id}/confirm`,
`DELETE /items/{id}`, `/identify/*`, `/settings/*` (TMDB, home), `/health/*`,
`/update/*`, `/auth/password`, `/auth/logout-all`, `/auth/sessions`.
**Siguen solo-localhost aunque haya sesión** (decisión mía, por seguridad y
rescate): `/lan/toggle` y `/lan/pin` — apagar el Modo LAN o cambiar el PIN
desde la red te dejaría fuera o debilitaría el control; si lo quieres abierto
a sesión, se cambia con una línea.
Abiertas sin sesión (como hoy, bajo Modo LAN/PIN): galería, fichas, `/play`,
`/stream`, `/playback`, `/wish`, `/sources/{id}/failed`, `/ping`, `/version`,
`/healthz` y `/auth/{status,login,csrf}`.

**Relación con el PIN de Modo LAN**: independientes. El PIN controla *ver*; la
clave controla *administrar*. Un móvil con PIN no administra; un administrador
por red pasa también el PIN si está puesto (el guardián del PIN se evalúa antes).

### 1.4 Flujos

- **Primera clave**: `gala admin set-password`. Clave por prompt
  sin eco (`x/term`) o por stdin si no es TTY (`--stdin`); **nunca como
  argumento** (visible en `ps`). Escribe el hash en `admin_auth` **y borra todas
  las sesiones** en la misma transacción. Sirve para resetear. Abre la BD
  directamente (SQLite en WAL: no hace falta parar el servicio). Escritorio:
  Ajustes › Seguridad (solo localhost) puede poner la primera clave.
- **Login** `POST /auth/login {password}` → verifica (tiempo constante) → crea
  sesión → `Set-Cookie: gala_session=<token>; HttpOnly; SameSite=Lax; Path=/;
  Max-Age=30 días; Secure` solo si `r.TLS != nil`. Caducidad **deslizante** (se
  renueva en cada uso, escribiendo `last_seen` como mucho 1 vez / 5 min para no
  escribir en BD por petición). Duración configurable: `session_days` (def. 30).
  Devuelve también el token CSRF.
- **Logout** borra la sesión actual; **logout-all** todas.
- **Cambiar clave** `POST /auth/password {current,new}` (también desde localhost,
  donde `current` sigue exigiéndose si ya hay clave) → re-hash → borra el resto
  de sesiones, conserva la actual.
- **Estado** `GET /auth/status` → `{configured, authenticated, is_localhost,
  network_admin}`. **Sesiones** `GET /auth/sessions` (nombre de dispositivo
  derivado del User-Agent, IP, visto por última vez; marca «esta sesión»).
- **Toggle** `PUT /auth/network-admin {enabled}` (solo localhost, para que no se
  pueda cortar la rama en la que estás sentado).

### 1.5 CSRF

Token = `HMAC-SHA256(server_secret, id_hash)`, determinista por sesión (no se
guarda). Se entrega en el login y en `GET /auth/csrf`. Toda petición
`POST/PUT/PATCH/DELETE` autorizada **por sesión** debe llevar
`X-GALA-CSRF`, comparado en tiempo constante; si no, 403. Capas extra ya
existentes: `SameSite=Lax`, exigir `application/json` en escrituras. Las
peticiones desde localhost no usan sesión y no necesitan el token.

### 1.6 Rate limiting y registro

Reutiliza el esquema de `lan` (5 fallos por IP → bloqueo de 30 s que se duplica
hasta 15 min; tope global 30 fallos/10 min → 5 min). Un login en bloqueo
responde `429` con `Retry-After`. Mensaje único para cualquier fallo («Clave
incorrecta») → sin enumeración (de todos modos hay un solo admin). Se registra
(`slog`) IP, momento y resultado de cada intento; **jamás la clave** ni el
cuerpo de la petición. Estado en memoria (se reinicia con el servicio; aceptable
a esta escala).

### 1.7 Riesgos asumidos

- La clave viaja **en claro por HTTP** en la LAN: se documenta en la guía y en
  Ajustes › Seguridad («solo para redes de confianza; no la expongas a
  Internet»). HTTPS queda como mejora futura.
- Quien tenga una shell en el servidor es «localhost» y por tanto admin: es
  intrínseco al modelo de rescate.
- Estado de rate-limit en memoria; un reinicio lo borra.

### 1.8 Tests previstos

Unitarios: hash/verify (incl. formato y re-hash), sesión crear/expirar/deslizar,
revocación, rate-limit (progresión y tope global), CSRF. Integración: login OK/KO,
ruta admin sin sesión → 401, con sesión → 200, sesión sin CSRF → 403, bypass
localhost, `X-Forwarded-For` ignorado, `Host` falso, logout-all, cambiar clave
invalida las demás, `set-password` borra sesiones. **Regresión**: sin clave,
localhost conserva admin total.

---

## 2. Auto-update y rollback

### 2.1 Alcance y premisas

- **`/healthz`** (público, sin PIN): 200 `{"status":"ok"}` si la BD responde
  (`PingContext`); si no, 503. Que responda ya implica listener vivo.
- **Aplicar actualización solo si GALA corre bajo systemd** (variable
  `INVOCATION_ID`) en Linux. En escritorio (Windows/macOS) no hay quien relance
  el proceso: la UI enseña «Hay una versión nueva» + enlace a la descarga, sin
  botón de actualizar. Decisión mía; lo contrario es improvisar un relanzador.
- `check` y `apply` son solo-admin. `apply` **no recibe URL del cliente**: usa
  la release ya consultada y guardada en el servidor.

### 2.2 `GET /update/check`

Consulta `api.github.com/repos/bigbendja/gala/releases/latest` (no incluye
prereleases/borradores = canal `stable`). Caché 24 h en memoria y disco; el botón
«Buscar actualizaciones» fuerza una consulta (mínimo 1 min entre consultas).
Compara semver; solo ofrece **versiones mayores a la actual** (sin downgrade).
Sin red o 403/5xx de GitHub → `200 {"checked":false,"error":"No se pudo
comprobar las actualizaciones"}`; nada se rompe. Nunca consulta por su cuenta
salvo que `update.auto_check` sea true (1×/día).

### 2.3 `POST /update/apply` — secuencia

1. Exclusión mutua (una sola actualización) y comprobación de **reproducción
   activa**: contador de peticiones en vuelo en `/stream/*` + sesiones HLS vivas.
   Si hay → `409 {"code":"playback_active","count":N}`; la UI avisa y ofrece
   «Actualizar igualmente» (`{"force":true}`). No se difiere solo.
2. Espacio libre ≥ 2× tamaño del binario junto al binario.
3. Descargar **solo `https`** y solo a hosts de GitHub (`github.com`,
   `objects.githubusercontent.com`, validado también en las redirecciones),
   límite de 200 MB y timeout; a `gala.new` en el mismo directorio (mismo
   sistema de ficheros → `rename` atómico). Progreso en `GET /update/status`.
4. Verificar **SHA256** contra `SHA256SUMS.txt` de la misma release.
   *Límite honesto*: binario y checksum salen del mismo sitio; esto protege de
   corrupción/truncado, **no** de una cuenta de GitHub comprometida. Firmar los
   checksums (minisign/ed25519) sería el siguiente paso (lo anoto en
   ISSUES-MVP2, no lo implemento).
5. `chmod +x` y **ejecutar `gala.new --version`** (argumentos fijos, sin shell):
   debe imprimir la versión esperada. Detecta binario de otra arquitectura o roto
   *antes* de tocar nada.
6. **Copia de seguridad de la BD**: `VACUUM INTO 'gala.db.pre-update'` (copia
   consistente en caliente) y anotar en el marcador la última migración aplicada.
7. `rename gala → gala.prev`, `rename gala.new → gala`; escribir marcador
   `update-pending` (fsync); responder `202`; cierre ordenado (drena
   peticiones, cierra BD) y `os.Exit(0)` → `Restart=always` levanta el nuevo.

### 2.4 Rollback — `gala-preflight` + `--selfcheck` (aprobado)

El rollback no puede depender de que el binario nuevo llegue a `main()`, así que
lo decide un script POSIX `sh` mínimo (`/opt/gala/gala-preflight`, ~30 líneas)
puesto como `ExecStartPre=` de la unit. Solo toca ficheros de `/opt/gala`, corre
como el usuario `gala` y es compatible con `NoNewPrivileges=true`.

Antes de cada arranque, si existe `update-pending` (fichero de texto
`clave=valor`: `new_version`, `from_version`, `attempts`, `migraciones_nuevas`):

1. `attempts += 1`.
2. Ejecuta **`gala --selfcheck`** con el binario nuevo y un `timeout` (30 s).
   `--selfcheck` abre la BD (aplica migraciones, `PingContext`), carga la
   configuración, **no liga ningún puerto**, no escanea ni arranca workers, y
   sale 0 si todo va bien o ≠0 con el motivo en stderr.
3. Si falla, o si `attempts > 3` (arranques que murieron luego): **restaura**
   `gala.prev → gala` (y `gala.db.pre-update → gala.db` si
   `migraciones_nuevas=1`), borra `update-pending`, escribe `update-result`
   (`status=rolled_back`, versiones y motivo) y **el preflight sale 0** → systemd
   arranca el binario viejo.

Desde Go solo queda el camino feliz: tras arrancar, auto-chequeo de `/healthz`
(30 s); si falla, salir con código 1 (el intento ya contó); si pasa y el proceso
aguanta 60 s estable, borrar `update-pending` y escribir `update-result`
(`status=ok`). `update/status` lo lee y la UI avisa («La actualización a vX
falló; se revirtió a vY»). Así se cubren: no arranca (selfcheck), arranca y
muere (attempts), arranca pero está roto (healthz).

Detalles:
- **Migraciones**: solo hacia delante y el binario viejo no detecta un esquema
  más nuevo. Por eso el paso 6 de §2.3 hace `VACUUM INTO gala.db.pre-update` y el
  rollback lo restaura si hubo migraciones nuevas. Se pierde lo escrito entre
  la actualización y el rollback (~1 min). El backup se borra a las 24 h del éxito.
- `gala.prev` se conserva tras un éxito (rollback manual gratis) hasta la
  siguiente actualización.
- La unit lleva `StartLimitIntervalSec=0` para que systemd no deje de reintentar
  antes de que el preflight revierta.
- El script lo instala `install.sh` y no se autoactualiza (a propósito).
  **La unit del Módulo 4 y este módulo se diseñan juntos.**

### 2.5 `gala-update` (instalaciones desde fuente)

Script separado, sin relación con lo anterior: `git pull`, `npm ci && npm run
build`, `go build` con las mismas ldflags, `sudo systemctl restart gala`. Es la
única parte que usa `sudo`, y solo en la vía de desarrollo.

### 2.6 Tests previstos

Comparación semver; caché y tolerancia sin red (GitHub simulado con
`httptest`); descarga: checksum correcto/incorrecto, host no permitido, tamaño
excedido, truncado; `--version` incorrecto aborta sin tocar nada; swap y marcador;
bloqueo por reproducción activa; **tabla de rollback** del preflight (intento
1–3 OK, intento 4 revierte, con y sin migraciones nuevas) probada contra el
script real en un directorio temporal.

---

## 3. Pendiente

- Medir argon2id en el servidor real (§1.2).
- ¿Servidor amd64 o ARM? (si ARM, añadir `linux-arm64` al release).
- Visto bueno de esta nota (Auth y rollback) antes de cualquier código.
- Mockups: `docs/briefs/GALA_Mockup_Login.html` trae login + Seguridad +
  Actualización.
- Aún abiertos de §1.3–1.4: `/lan/toggle` y `/lan/pin` solo-localhost, y
  `network_admin` solo-localhost con `admin_localhost_bypass`.
