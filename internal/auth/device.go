package auth

import "strings"

// DeviceName resume un User-Agent como «Chrome en Windows» para la lista de
// sesiones. No es una detección exacta: solo lo bastante clara para que el
// administrador reconozca sus dispositivos.
func DeviceName(ua string) string {
	l := strings.ToLower(ua)
	browser := "Navegador"
	switch {
	case strings.Contains(l, "edg/"):
		browser = "Edge"
	case strings.Contains(l, "opr/") || strings.Contains(l, "opera"):
		browser = "Opera"
	case strings.Contains(l, "firefox/"):
		browser = "Firefox"
	case strings.Contains(l, "chrome/") || strings.Contains(l, "crios/"):
		browser = "Chrome"
	case strings.Contains(l, "safari/"):
		browser = "Safari"
	case strings.Contains(l, "curl/"):
		browser = "curl"
	}
	osName := ""
	switch {
	case strings.Contains(l, "iphone"):
		osName = "iPhone"
	case strings.Contains(l, "ipad"):
		osName = "iPad"
	case strings.Contains(l, "android"):
		osName = "Android"
	case strings.Contains(l, "windows"):
		osName = "Windows"
	case strings.Contains(l, "mac os") || strings.Contains(l, "macintosh"):
		osName = "macOS"
	case strings.Contains(l, "linux") || strings.Contains(l, "x11"):
		osName = "Linux"
	}
	if osName == "" {
		return browser
	}
	return browser + " en " + osName
}
