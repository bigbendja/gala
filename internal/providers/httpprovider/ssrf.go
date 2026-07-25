// Package httpprovider implementa providers.Provider para enlaces http(s)
// directos (Brief M5 §5). Se llama "httpprovider" en vez de "http" para no
// chocar con el import "net/http" en los archivos que necesitan ambos.
package httpprovider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

// ErrBlockedTarget señala que una URL apunta a una IP privada/loopback/
// link-local: nunca se sigue, ni para comprobar salud (Check) ni para
// descargar una lista M3U remota referenciada en una ingesta (Brief M4/M5
// §6: protección SSRF básica).
var ErrBlockedTarget = errors.New("httpprovider: la URL apunta a una red privada o local, bloqueada por seguridad")

// LookupIPAddr resuelve un host a sus IPs; variable de paquete (no una
// llamada directa a net.DefaultResolver) para poder sustituirla en tests por
// una resolución falsa — así se puede probar el camino "resuelve a una IP
// pública, sigue adelante" sin depender de DNS real ni de que un dominio de
// prueba exista de verdad (p. ej. el texto exacto del mockup de M5 usa
// "ejemplo.net", que no resuelve en ningún sitio).
var LookupIPAddr = net.DefaultResolver.LookupIPAddr

// ValidateExternalURL comprueba que rawURL es http(s) y que su host no
// resuelve a una IP privada/loopback/link-local (Brief M5 §6: bloqueo básico
// de 127.0.0.0/8, 10/8, 172.16/12, 192.168/16, 169.254/16 — y sus
// equivalentes IPv6 vía IsPrivate/IsLoopback/IsLinkLocal* de net.IP). Se usa
// antes de CUALQUIER petición saliente a una URL aportada por el usuario:
// Check() de este provider, y la descarga de listas M3U remotas en
// internal/ingest.
func ValidateExternalURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("httpprovider: URL no válida: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("httpprovider: esquema no permitido %q (solo http/https)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("httpprovider: URL sin host")
	}

	// Host ya es una IP literal (sin DNS que resolver).
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return ErrBlockedTarget
		}
		return nil
	}

	addrs, err := LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("httpprovider: no se pudo resolver %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("httpprovider: %q no resolvió a ninguna IP", host)
	}
	for _, a := range addrs {
		if isBlockedIP(a.IP) {
			return ErrBlockedTarget
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
