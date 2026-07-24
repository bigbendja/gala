<#
GALA - genera la matriz de prueba de M4 (Brief M4 §7): clips sintéticos
(barras SMPTE + tono) para probar decide/probe/streaming sin depender de
contenido real. Los binarios resultantes NO se versionan (testdata/media/
está en .gitignore); este script sí.

Requiere ffmpeg en PATH (o -FFmpeg <ruta>). Si un códec concreto no está en
tu build de ffmpeg (p. ej. libx265 recortado), ese clip se omite con un
aviso; el resto de la matriz se genera igual.

Uso:
  .\scripts\make_testmedia.ps1
  .\scripts\make_testmedia.ps1 -FFmpeg C:\gala-f0\ffmpeg\ffmpeg.exe
#>
param(
    [string]$FFmpeg = "ffmpeg",
    [string]$OutDir = (Join-Path $PSScriptRoot "..\testdata\media")
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

function New-Clip {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][int]$DurationSeconds,
        [Parameter(Mandatory)][string]$VideoCodec,
        [Parameter(Mandatory)][string]$AudioCodec,
        [Parameter(Mandatory)][string]$VideoBitrate
    )
    $out = Join-Path $OutDir $Name
    $ffArgs = @(
        "-y",
        "-f", "lavfi", "-i", "smptebars=size=1280x720:rate=30",
        "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
        "-t", "$DurationSeconds",
        "-c:v", $VideoCodec, "-b:v", $VideoBitrate, "-pix_fmt", "yuv420p",
        "-c:a", $AudioCodec, "-b:a", "192k",
        $out
    )
    Write-Host "Generando $Name ..."
    & $FFmpeg @ffArgs *> $null
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $out)) {
        Write-Warning "No se pudo generar $Name (¿$VideoCodec/$AudioCodec no disponibles en este ffmpeg?); se omite."
        return
    }
    $sizeBytes = (Get-Item $out).Length
    $sizeMB = [math]::Round($sizeBytes / 1MB, 1)
    if ($sizeBytes -lt 10MB) {
        Write-Warning "$Name = $sizeMB MB, por debajo del filtro de tamaño de M2 (10 MB); sube VideoBitrate."
    } else {
        Write-Host "  OK ($sizeMB MB)"
    }
}

# --- Matriz corta (10 s): un caso por celda de contenedor x códecs (Brief §7) ---
New-Clip -Name "direct_h264_aac.mp4" -DurationSeconds 10 -VideoCodec libx264 -AudioCodec aac -VideoBitrate 15M
New-Clip -Name "remux_h264_aac.mkv"  -DurationSeconds 10 -VideoCodec libx264 -AudioCodec aac -VideoBitrate 15M
New-Clip -Name "reject_hevc_aac.mkv" -DurationSeconds 10 -VideoCodec libx265 -AudioCodec aac -VideoBitrate 15M
New-Clip -Name "reject_h264_ac3.mkv" -DurationSeconds 10 -VideoCodec libx264 -AudioCodec ac3 -VideoBitrate 15M

# --- Clips largos (180 s): seeking en direct y en remux (Brief §7) --------------
New-Clip -Name "seek_h264_aac_long.mp4" -DurationSeconds 180 -VideoCodec libx264 -AudioCodec aac -VideoBitrate 1500k
New-Clip -Name "seek_h264_aac_long.mkv" -DurationSeconds 180 -VideoCodec libx264 -AudioCodec aac -VideoBitrate 1500k

Write-Host ""
Write-Host "Listo. Clips en $OutDir"
Write-Host "go test ./internal/media/... -run TestProbe -v   # verifica contra estos clips"
