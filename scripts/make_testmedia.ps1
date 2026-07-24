<#
GALA - genera la matriz de prueba de M4 (Brief M4 §7): clips sintéticos
(barras SMPTE + tono) para probar decide/probe/streaming sin depender de
contenido real. Los binarios resultantes NO se versionan (testdata/media/
está en .gitignore); este script sí.

Busca ffmpeg en este orden (igual que internal/media/ffmpeg.go): (1) carpeta
ffmpeg/ junto al repo, (2) ffmpeg.exe suelto junto al repo, (3) PATH del
sistema. O pásalo explícito con -FFmpeg <ruta>.

Si un códec concreto no está en tu build de ffmpeg (p. ej. libx265
recortado), ese clip se omite con un aviso; el resto de la matriz se genera
igual. Si un clip queda por debajo del filtro de tamaño de M2 (10 MB), el
script reintenta automáticamente con más bitrate antes de rendirse.

Uso:
  .\scripts\make_testmedia.ps1
  .\scripts\make_testmedia.ps1 -FFmpeg C:\ruta\a\ffmpeg.exe
#>
param(
    [string]$FFmpeg = "",
    [string]$OutDir = (Join-Path $PSScriptRoot "..\testdata\media")
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# --- Localizar ffmpeg ------------------------------------------------------

function Resolve-FFmpegPath {
    param([string]$Explicit)
    if ($Explicit) {
        if (Test-Path $Explicit) { return (Resolve-Path $Explicit).Path }
        Write-Error "No existe el ffmpeg indicado: $Explicit"
        exit 1
    }
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $candidates = @(
        (Join-Path $repoRoot "ffmpeg\ffmpeg.exe"),
        (Join-Path $repoRoot "ffmpeg.exe")
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) { return $c }
    }
    $onPath = Get-Command "ffmpeg" -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }
    Write-Error "No se encontró ffmpeg: ni en '$repoRoot\ffmpeg\', ni junto al repo, ni en PATH. Instálalo o pásalo con -FFmpeg <ruta>."
    exit 1
}

$resolvedFFmpeg = Resolve-FFmpegPath -Explicit $FFmpeg
Write-Host "Usando ffmpeg: $resolvedFFmpeg"
Write-Host ""

# --- Generación de un clip, con reintento si se queda corto ---------------

# minClipBytes: filtro de tamaño de M2 (Brief M4 §7: "todos > 10 MB").
$minClipBytes = 10MB
$maxAttempts = 3
# bitrateGrowthFactor: cuánto se sube el bitrate en cada reintento si el
# clip generado se queda por debajo de minClipBytes.
$bitrateGrowthFactor = 1.8

function New-Clip {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][int]$DurationSeconds,
        [Parameter(Mandatory)][string]$VideoCodec,
        [Parameter(Mandatory)][string]$AudioCodec,
        [Parameter(Mandatory)][int]$VideoBitrateKbps
    )
    $out = Join-Path $OutDir $Name
    $bitrate = $VideoBitrateKbps

    for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
        Write-Host "Generando $Name (intento $attempt/$maxAttempts, -b:v ${bitrate}k) ..."

        # -hide_banner -loglevel error: ffmpeg escribe su banner y progreso
        # por stderr; en PowerShell 5.1, redirigir el stderr de un comando
        # nativo (2>, *>, 2>&1) envuelve cada línea en un NativeCommandError
        # y pone $?=$false AUNQUE el proceso termine con código 0 — con
        # $ErrorActionPreference="Stop" eso aborta el script entero sin
        # generar nada, aunque el mismo comando funcione perfectamente a
        # mano. La solución es no redirigir nada: silenciar ffmpeg en el
        # origen y comprobar el resultado por $LASTEXITCODE + el archivo.
        #
        # testsrc2 (no smptebars): smptebars es una imagen ESTÁTICA, y con
        # contenido casi sin cambios frame a frame, libx264/libx265 en modo
        # ABR usa muchísimos menos bits de los pedidos por -b:v (verificado:
        # a 64.8 Mbps generaba un mp4 de 0,2 MB) — el bitrate objetivo no
        # sirve de nada si no hay complejidad visual que codificar. testsrc2
        # tiene patrones en movimiento (relojes, degradados) que sí obligan
        # al codificador a acercarse al bitrate pedido.
        & $resolvedFFmpeg -y -hide_banner -loglevel error `
            -f lavfi -i "testsrc2=size=1280x720:rate=30" `
            -f lavfi -i "sine=frequency=440:sample_rate=48000" `
            -t "$DurationSeconds" `
            -c:v $VideoCodec -b:v "${bitrate}k" -pix_fmt yuv420p `
            -c:a $AudioCodec -b:a 192k `
            $out
        $exitCode = $LASTEXITCODE

        if ($exitCode -ne 0 -or -not (Test-Path $out)) {
            Write-Warning "No se pudo generar $Name (ffmpeg salió con código $exitCode; ¿$VideoCodec/$AudioCodec no disponibles en este build?); se omite."
            return [pscustomobject]@{ Name = $Name; SizeMB = 0; Status = "omitido" }
        }

        $sizeBytes = (Get-Item $out).Length
        $sizeMB = [math]::Round($sizeBytes / 1MB, 1)
        if ($sizeBytes -ge $minClipBytes) {
            Write-Host "  OK ($sizeMB MB)"
            return [pscustomobject]@{ Name = $Name; SizeMB = $sizeMB; Status = "OK" }
        }

        if ($attempt -lt $maxAttempts) {
            Write-Warning "$Name = $sizeMB MB, por debajo del filtro de tamaño de M2 (10 MB); subiendo bitrate y reintentando..."
            $bitrate = [int]([math]::Ceiling($bitrate * $bitrateGrowthFactor))
        } else {
            Write-Warning "$Name = $sizeMB MB tras $maxAttempts intentos, sigue por debajo de 10 MB."
            return [pscustomobject]@{ Name = $Name; SizeMB = $sizeMB; Status = "bajo 10 MB" }
        }
    }
}

# Bitrates verificados a mano con testsrc2 contra un ffmpeg 8.1 real:
# 12000 kbps/10s -> ~13-14 MB; 600 kbps/180s -> ~17 MB. Margen cómodo sobre
# el filtro de tamaño de M2 (10 MB) sin generar archivos innecesariamente
# grandes.

# --- Matriz corta (10 s): un caso por celda de contenedor x códecs (Brief §7) ---
$results = @()
$results += New-Clip -Name "direct_h264_aac.mp4" -DurationSeconds 10 -VideoCodec libx264 -AudioCodec aac -VideoBitrateKbps 12000
$results += New-Clip -Name "remux_h264_aac.mkv"  -DurationSeconds 10 -VideoCodec libx264 -AudioCodec aac -VideoBitrateKbps 12000
$results += New-Clip -Name "reject_hevc_aac.mkv" -DurationSeconds 10 -VideoCodec libx265 -AudioCodec aac -VideoBitrateKbps 12000
$results += New-Clip -Name "reject_h264_ac3.mkv" -DurationSeconds 10 -VideoCodec libx264 -AudioCodec ac3 -VideoBitrateKbps 12000

# --- Clips largos (180 s): seeking en direct y en remux (Brief §7) --------------
$results += New-Clip -Name "seek_h264_aac_long.mp4" -DurationSeconds 180 -VideoCodec libx264 -AudioCodec aac -VideoBitrateKbps 600
$results += New-Clip -Name "seek_h264_aac_long.mkv" -DurationSeconds 180 -VideoCodec libx264 -AudioCodec aac -VideoBitrateKbps 600

# --- Resumen ---------------------------------------------------------------

Write-Host ""
Write-Host "=== Resumen ==="
$results | Format-Table -Property Name, SizeMB, Status -AutoSize

$problems = $results | Where-Object { $_.Status -ne "OK" }
if ($problems) {
    Write-Warning "$($problems.Count) de $($results.Count) clip(s) con problemas (omitidos o por debajo de 10 MB). Revisa si tu build de ffmpeg trae libx265, o sube VideoBitrateKbps a mano en el script."
} else {
    Write-Host "Los $($results.Count) clips superan el filtro de tamaño de M2 (10 MB)."
}

Write-Host ""
Write-Host "Clips en $OutDir"
Write-Host "go test ./internal/media/... -run TestProbeAgainstGeneratedClips -v   # verifica contra estos clips"
