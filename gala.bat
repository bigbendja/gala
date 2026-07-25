@echo off
setlocal
cd /d "%~dp0"

echo ============================================
echo   GALA - arranque
echo ============================================

echo Cerrando instancias anteriores de gala.exe (si las hay)...
taskkill /F /IM gala.exe >nul 2>&1
timeout /t 1 /nobreak >nul

rem ffmpeg/ffprobe de desarrollo: se colocan a mano en la carpeta ffmpeg\
rem junto al binario (ver README). Anadirla al PATH de esta sesion no
rem afecta al resto del sistema.
set "PATH=%~dp0ffmpeg;%PATH%"

if not exist "%~dp0gala.exe" (
    echo.
    echo No se encontro gala.exe: compilando por primera vez...
    go build -o gala.exe .\cmd\gala
    if errorlevel 1 (
        echo.
        echo ERROR: no se pudo compilar GALA.
        echo Comprueba que tienes Go instalado ^(go version^) y que el
        echo frontend esta construido al menos una vez:
        echo     cd web ^&^& npm ci ^&^& npm run build
        echo.
        pause
        exit /b 1
    )
    echo Compilado correctamente.
)

echo.
echo Arrancando GALA... se abrira el navegador solo.
echo ^(Deja esta ventana abierta; cierrala o pulsa Ctrl+C para apagar GALA.^)
echo.
"%~dp0gala.exe"
set "EXITCODE=%ERRORLEVEL%"

if not "%EXITCODE%"=="0" (
    echo.
    echo ERROR: GALA se cerro con un error ^(codigo %EXITCODE%^).
    echo Revisa los mensajes de arriba.
    echo.
    pause
    exit /b %EXITCODE%
)

endlocal
