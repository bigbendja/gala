@echo off
setlocal
cd /d "%~dp0"

echo ============================================
echo   GALA - recompilar
echo ============================================

echo Cerrando instancias anteriores de gala.exe (si las hay)...
taskkill /F /IM gala.exe >nul 2>&1
timeout /t 1 /nobreak >nul

set "REBUILD_WEB=0"
if /i "%~1"=="--web" set "REBUILD_WEB=1"

if "%REBUILD_WEB%"=="0" (
    rem Sin --web: solo se reconstruye el frontend si algo bajo web\src,
    rem web\static o su configuracion es mas reciente que el ultimo build
    rem (web\dist\index.html). Evita esperar npm run build en cada recompilacion
    rem de Go cuando solo cambio codigo Go.
    powershell -NoProfile -Command ^
        "$src = Get-ChildItem -Path 'web\src','web\static','web\package.json','web\package-lock.json','web\vite.config.ts','web\tsconfig.json' -Recurse -File -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1 -ExpandProperty LastWriteTime; $out = (Get-Item 'web\dist\index.html' -ErrorAction SilentlyContinue).LastWriteTime; if (-not $out) { exit 1 } elseif ($src -and $src -gt $out) { exit 1 } else { exit 0 }"
    if errorlevel 1 set "REBUILD_WEB=1"
)

if "%REBUILD_WEB%"=="1" (
    echo.
    echo Construyendo el frontend ^(web\dist^)...
    pushd web
    call npm run build
    if errorlevel 1 (
        popd
        echo.
        echo ERROR: fallo el build del frontend. Revisa los mensajes de arriba.
        echo.
        pause
        exit /b 1
    )
    popd
    echo Frontend construido correctamente.
) else (
    echo.
    echo El frontend no tiene cambios: se omite npm run build.
    echo ^(Usa "gala-build.bat --web" para forzarlo.^)
)

echo.
echo Compilando gala.exe...
go build -o gala.exe .\cmd\gala
if errorlevel 1 (
    echo.
    echo ERROR: fallo la compilacion de Go. Revisa los mensajes de arriba.
    echo.
    pause
    exit /b 1
)

echo.
echo ============================================
echo   Listo. Ejecuta gala.bat para arrancar GALA.
echo ============================================
endlocal
