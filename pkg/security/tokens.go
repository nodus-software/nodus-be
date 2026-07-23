package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const randomTokenBytes = 32 // 256 bits

// GenerateToken returns a cryptographically random, URL-safe opaque token
// (256 bits of entropy) suitable for challenge tokens, refresh tokens,
// password reset tokens, and invite tokens. The raw value is what's handed
// to the client; only HashToken's output is ever persisted.
func GenerateToken() (string, error) {
	buf := make([]byte, randomTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the SHA-256 hex digest of a raw token, for at-rest
// storage. Lookups compare hashes, never raw values.
func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// GenerateBackupCode returns a short, human-typeable one-time MFA recovery
// code (e.g. "K7XP-L2QZ-9MNT-4RSC"), backed by 80 bits of randomness. Only
// HashToken's output is ever persisted.
func GenerateBackupCode() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate backup code: %w", err)
	}
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16], nil
}
