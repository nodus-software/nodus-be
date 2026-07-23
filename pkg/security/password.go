package security

import "golang.org/x/crypto/bcrypt"

// HashPassword bcrypt-hashes a plaintext password at the given cost.
func HashPassword(plaintext string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword reports whether plaintext matches the given bcrypt hash.
func ComparePassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
