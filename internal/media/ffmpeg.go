// Package media localiza ffmpeg/ffprobe, sondea archivos de vídeo y decide
// cómo reproducirlos (Brief M4).
package media

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FFmpeg son las rutas localizadas de ffmpeg y ffprobe. Cualquiera de los
// dos puede quedar vacío si no se encontró: Available() es la única
// comprobación que debe usarse antes de invocarlos.
type FFmpeg struct {
	FFmpegPath  string
	FFprobePath string
}

// Available indica si tanto ffmpeg como ffprobe se localizaron. GALA nunca
// aborta el arranque por su ausencia (Brief M4 §1): sin ellos, el sistema
// asume direct play por extensión (media.Decide) y el navegador ya fallará
// si no puede reproducir (aviso honesto, no silencioso).
func (f *FFmpeg) Available() bool {
	return f != nil && f.FFmpegPath != "" && f.FFprobePath != ""
}

// Locate busca ffmpeg y ffprobe en orden: (1) carpeta "ffmpeg/" junto al
// ejecutable (así los empaquetará M11), (2) junto al propio ejecutable, (3)
// PATH del sistema. exeDir es el directorio del binario de GALA (Brief M4
// §1: "empaquetado real llega en M11"; en desarrollo el titular del proyecto
// coloca los binarios a mano, ver README). Registra un único Warn si falta
// alguno de los dos: no se repite en cada sondeo o petición de play.
func Locate(exeDir string, log *slog.Logger) *FFmpeg {
	f := &FFmpeg{
		FFmpegPath:  locateBinary(exeDir, "ffmpeg"),
		FFprobePath: locateBinary(exeDir, "ffprobe"),
	}
	if log != nil {
		switch {
		case f.FFmpegPath == "" && f.FFprobePath == "":
			log.Warn("ffmpeg/ffprobe no encontrados: solo direct play estará disponible (ver README para desarrollo)")
		case f.FFmpegPath == "":
			log.Warn("ffmpeg no encontrado (ffprobe sí): sin remux, solo direct play")
		case f.FFprobePath == "":
			log.Warn("ffprobe no encontrado (ffmpeg sí): sin sonda de medios, decisión por extensión")
		}
	}
	return f
}

func binaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// locateBinary prueba, en orden, exeDir/ffmpeg/<name>, exeDir/<name>, y PATH.
func locateBinary(exeDir, name string) string {
	fname := binaryName(name)

	candidate := filepath.Join(exeDir, "ffmpeg", fname)
	if isExecutableFile(candidate) {
		return candidate
	}
	candidate = filepath.Join(exeDir, fname)
	if isExecutableFile(candidate) {
		return candidate
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
