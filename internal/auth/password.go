// Package auth implementa la autenticación del administrador de GALA
// (M-Servidor): hash argon2id, sesiones en BD, CSRF y límite de intentos.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Parámetros de argon2id. Modestos a propósito (hardware limitado): ~100 ms
// en un PC de escritorio y del orden de 0,3–0,6 s en un servidor flojo. Se
// ajustan aquí; los hashes ya guardados llevan sus propios parámetros, así que
// subirlos no invalida claves antiguas (se re-hashean en el siguiente login).
var (
	argonTime    uint32 = 2
	argonMemory  uint32 = 64 * 1024 // KiB = 64 MB
	argonThreads uint8  = 1
)

const (
	argonKeyLen  = 32
	argonSaltLen = 16

	MinPasswordLen = 8
	MaxPasswordLen = 128
)

// ErrPasswordPolicy se devuelve cuando una clave nueva no cumple la longitud.
var ErrPasswordPolicy = errors.New("la clave debe tener entre 8 y 128 caracteres")

// ValidatePassword aplica la única regla: longitud (sin reglas de composición).
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen || n > MaxPasswordLen {
		return ErrPasswordPolicy
	}
	return nil
}

// HashPassword devuelve el hash codificado ($argon2id$v=19$m=..,t=..,p=..$sal$hash).
func HashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generar sal: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

type hashParams struct {
	mem, time uint32
	threads   uint8
	salt, key []byte
}

func parseHash(enc string) (hashParams, error) {
	var p hashParams
	parts := strings.Split(enc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, errors.New("formato de hash no válido")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, errors.New("versión de argon2 no compatible")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.mem, &p.time, &p.threads); err != nil {
		return p, errors.New("parámetros de hash no válidos")
	}
	// Tope defensivo: un hash corrupto o manipulado no debe poder pedir GB.
	if p.mem == 0 || p.mem > 1024*1024 || p.time == 0 || p.time > 20 || p.threads == 0 {
		return p, errors.New("parámetros de hash fuera de rango")
	}
	var err error
	if p.salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return p, fmt.Errorf("sal: %w", err)
	}
	if p.key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil || len(p.key) == 0 {
		return p, errors.New("hash vacío o no válido")
	}
	return p, nil
}

// VerifyPassword comprueba pw contra el hash codificado en tiempo constante.
// needsRehash es true si el hash usa parámetros distintos a los vigentes.
func VerifyPassword(pw, enc string) (ok, needsRehash bool, err error) {
	p, err := parseHash(enc)
	if err != nil {
		return false, false, err
	}
	got := argon2.IDKey([]byte(pw), p.salt, p.time, p.mem, p.threads, uint32(len(p.key)))
	ok = subtle.ConstantTimeCompare(got, p.key) == 1
	needsRehash = p.mem != argonMemory || p.time != argonTime || p.threads != argonThreads
	return ok, needsRehash, nil
}
