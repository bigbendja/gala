package httpprovider

import (
	"context"
	"net"
	"testing"
)

// TestValidateExternalURLBlocksPrivateIPLiterals cubre el bloqueo básico
// del Brief M4/M5 §6: 127.0.0.0/8, 10/8, 172.16/12, 192.168/16, 169.254/16.
func TestValidateExternalURLBlocksPrivateIPLiterals(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/x",
		"http://127.0.0.1:8080/x",
		"http://10.0.0.5/x",
		"http://172.16.0.1/x",
		"http://192.168.1.10/x",
		"http://169.254.169.254/latest/meta-data", // metadata endpoint clásico de SSRF en la nube
		"http://[::1]/x",
	}
	for _, u := range blocked {
		t.Run(u, func(t *testing.T) {
			err := ValidateExternalURL(context.Background(), u)
			if err != ErrBlockedTarget {
				t.Fatalf("ValidateExternalURL(%q) = %v, esperado ErrBlockedTarget", u, err)
			}
		})
	}
}

func TestValidateExternalURLAllowsPublicIPLiteral(t *testing.T) {
	// IP pública de documentación (RFC 5737 TEST-NET-1): no privada, no
	// loopback, no link-local — debe pasar la validación.
	if err := ValidateExternalURL(context.Background(), "http://93.184.216.34/x"); err != nil {
		t.Fatalf("ValidateExternalURL = %v, esperado nil (IP pública)", err)
	}
}

func TestValidateExternalURLRejectsNonHTTPScheme(t *testing.T) {
	for _, u := range []string{"ftp://example.com/x", "file:///etc/passwd", "javascript:alert(1)"} {
		if err := ValidateExternalURL(context.Background(), u); err == nil {
			t.Errorf("ValidateExternalURL(%q) = nil, esperado error (esquema no http/https)", u)
		}
	}
}

// TestValidateExternalURLBlocksResolvedPrivateIP cubre el caso donde el
// HOST (un nombre, no una IP literal) resuelve a una IP privada: se
// sustituye LookupIPAddr para no depender de DNS real.
func TestValidateExternalURLBlocksResolvedPrivateIP(t *testing.T) {
	orig := LookupIPAddr
	LookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("192.168.1.5")}}, nil
	}
	defer func() { LookupIPAddr = orig }()

	err := ValidateExternalURL(context.Background(), "http://intranet.interna.example/x")
	if err != ErrBlockedTarget {
		t.Fatalf("err = %v, esperado ErrBlockedTarget (el host resuelve a una IP privada)", err)
	}
}

func TestValidateExternalURLAllowsResolvedPublicIP(t *testing.T) {
	orig := LookupIPAddr
	LookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	defer func() { LookupIPAddr = orig }()

	if err := ValidateExternalURL(context.Background(), "http://ejemplo.net/x"); err != nil {
		t.Fatalf("err = %v, esperado nil (resuelve a IP pública)", err)
	}
}

// localhost y *.localhost son loopback por definición: se bloquean sin
// depender de lo que el resolvedor del sistema responda.
func TestValidateExternalURLBlocksLocalhostNames(t *testing.T) {
	orig := LookupIPAddr
	LookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil // aunque el DNS dijera "pública", el nombre manda
	}
	defer func() { LookupIPAddr = orig }()
	for _, u := range []string{"http://localhost/x", "http://LOCALHOST:7420/x", "http://app.localhost/x", "http://localhost./x"} {
		if err := ValidateExternalURL(context.Background(), u); err != ErrBlockedTarget {
			t.Errorf("ValidateExternalURL(%q) = %v, esperado ErrBlockedTarget", u, err)
		}
	}
}
