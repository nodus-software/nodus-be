package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"nodus-health/pkg/utility"
)

const issuer = "nodus-health"

var (
	ErrTokenInvalid = errors.New("access token invalid")
	ErrTokenExpired = errors.New("access token expired")
)

// AccessClaims are the JWT claims embedded in every access token. Deliberately
// lean: permissions are never embedded here, so a role/permission change takes
// effect on the very next request instead of waiting for the token to expire —
// the authentication middleware re-resolves the user's current permissions
// from the database on every request using the subject/session identifiers
// below.
type AccessClaims struct {
	jwt.RegisteredClaims
	SessionID string `json:"session_id"`
	TenantID  string `json:"tid"`
}

// IssueAccessToken signs a new short-lived access token for userID/sessionID.
func IssueAccessToken(secret string, ttl time.Duration, userID, sessionID string, tenantIDs ...string) (token string, expiresAt time.Time, err error) {
	jti, err := utility.GenerateUUID()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate jti: %w", err)
	}

	now := time.Now()
	expiresAt = now.Add(ttl)

	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    issuer,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		SessionID: sessionID,
	}
	if len(tenantIDs) > 0 {
		claims.TenantID = tenantIDs[0]
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseAccessToken validates signature/expiry and returns the claims.
func ParseAccessToken(secret, tokenString string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	if !token.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}
