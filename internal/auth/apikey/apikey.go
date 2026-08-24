// Package apikey issues and verifies ApplyTrail API keys.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

const (
	// Prefix marks a key as ApplyTrail's, so a leaked credential can be
	// recognised in a log or a secret scanner.
	Prefix = "at_live_"
	// secretBytes is the entropy behind each key: 256 bits.
	secretBytes = 32
	// lookupChars is how much of the secret goes into the indexed prefix. It
	// narrows verification to one row without revealing enough to guess the
	// rest.
	lookupChars = 8
)

// Generated is a freshly minted key. Secret is returned exactly once, at
// creation; only Prefix and Hash are ever persisted.
type Generated struct {
	Secret string
	Prefix string
	Hash   string
}

// Generate mints a new key.
func Generate() (Generated, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return Generated{}, fmt.Errorf("read random bytes: %w", err)
	}

	// URL-safe base64 without padding keeps the key copy-pasteable into a
	// header, a shell, and a JSON config without escaping.
	body := base64.RawURLEncoding.EncodeToString(buf)
	secret := Prefix + body

	return Generated{
		Secret: secret,
		Prefix: Prefix + body[:lookupChars],
		Hash:   Hash(secret),
	}, nil
}

// Hash returns the stored digest of a key.
//
// A plain SHA-256 is deliberate, not an oversight. Password hashes need a slow
// KDF because human-chosen passwords have little entropy and must survive an
// offline guessing attack. An ApplyTrail key is 256 bits of CSPRNG output, so
// there is nothing to guess: brute force is infeasible regardless of the hash's
// speed. Using argon2 here would add tens of milliseconds and tens of megabytes
// to every single authenticated request, which on a scale-to-zero container is
// a real cost paid for no security gain.
//
// What this does buy: a database leak does not hand over working credentials.
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// LookupPrefix extracts the indexed prefix from a presented key, so
// verification reads one row instead of hashing every key in the table.
//
// It returns false for anything that is not shaped like an ApplyTrail key,
// which lets the caller reject obvious garbage before touching the database.
func LookupPrefix(secret string) (string, bool) {
	if !strings.HasPrefix(secret, Prefix) {
		return "", false
	}
	body := secret[len(Prefix):]
	if len(body) < lookupChars {
		return "", false
	}
	return Prefix + body[:lookupChars], true
}

// Verify reports whether secret matches the stored hash, in constant time so
// that comparison timing cannot be used to recover a key byte by byte.
func Verify(secret, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(Hash(secret)), []byte(storedHash)) == 1
}

// Redact renders a key for display: enough to identify which key it is,
// nothing usable.
func Redact(prefix string) string { return prefix + "…" }

// ParseScopes is re-exported for CLI convenience.
var ParseScopes = domain.ParseScopes
