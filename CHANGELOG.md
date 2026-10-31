# Cambios

Formato: una sección por versión. La CI usa la sección de la etiqueta como notas
del release.

## [0.10.1] - 2026-10-31

### Corregido
- `install.sh --download` fallaba en Ubuntu («URL rejected»): el script cargaba
  `/etc/os-release`, que pisaba su variable `VERSION`. La guía deja `--download` como
  camino recomendado ahora que el repositorio es público.

## [0.10.0] - 2026-10-31

GALA listo para un servidor: se instala con un script, se administra desde la red
con contraseña y se actualiza desde la propia app.

### Añadido
- **Administración con contraseña.** Desde otro dispositivo de la red, «Administrar»
  pide una contraseña para tocar carpetas, la clave de TMDB, «Por confirmar» o los
  ajustes. El propio equipo donde corre GALA sigue sin pedirla. Ajustes › Seguridad:
  poner o cambiar la contraseña, ver y cerrar sesiones, y permitir o no administrar
  desde la red. `gala admin set-password` la pone o la resetea por SSH.
- **Actualización desde la app.** Ajustes › Actualización busca versiones nuevas,
  descarga la última, verifica su SHA256 y reinicia GALA. Guarda la versión anterior
  (`gala.prev`) y una copia de la base de datos, y vuelve sola a la anterior si la
  nueva no arranca.
- **Instalación en servidor Ubuntu.** `install.sh` instala GALA como servicio systemd
  (`--binary` con un binario que ya tengas, o `--download` desde este release), con
  usuario propio y el cortafuegos abierto solo a tu red local. Guía en
  `docs/GALA_Guia_Servidor.md`.
- `gala --version`, `GET /api/v1/version` y `GET /healthz`; la versión se ve en
  Ajustes › Sistema.

### Seguridad
- La contraseña se guarda con argon2id; las sesiones, solo como hash.
- El acceso sin contraseña del propio equipo exige también que la petición venga de
  `localhost` (protege contra DNS rebinding).
- La contraseña y el tráfico viajan sin cifrar por la red local: no expongas GALA a
  Internet.

## [0.9.0] - 2026-10-30

Primera versión publicada con binarios para Windows, Linux y macOS.
