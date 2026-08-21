package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimitStore interface {
	Increment(ctx context.Context, scope, value string, ttl time.Duration) (int64, error)
	Count(ctx context.Context, scope, value string) (int64, error)
}

type TurnstileVerifier interface {
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

type SecurityControls struct {
	RateLimits RateLimitStore
	Turnstile  TurnstileVerifier
}

type RedisRateLimitStore struct {
	client *redis.Client
	secret []byte
}

func NewRedisRateLimitStore(address, password, secret string) *RedisRateLimitStore {
	return &RedisRateLimitStore{
		client: redis.NewClient(&redis.Options{Addr: address, Password: password, DialTimeout: 500 * time.Millisecond, ReadTimeout: 500 * time.Millisecond, WriteTimeout: 500 * time.Millisecond}),
		secret: []byte(secret),
	}
}

func (s *RedisRateLimitStore) key(scope, value string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(value))))
	return "auth:" + scope + ":" + hex.EncodeToString(mac.Sum(nil))
}

func (s *RedisRateLimitStore) Increment(ctx context.Context, scope, value string, ttl time.Duration) (int64, error) {
	key := s.key(scope, value)
	result, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, key)
		pipe.ExpireNX(ctx, key, ttl)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return result[0].(*redis.IntCmd).Val(), nil
}

func (s *RedisRateLimitStore) Count(ctx context.Context, scope, value string) (int64, error) {
	n, err := s.client.Get(ctx, s.key(scope, value)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return n, err
}

type CloudflareTurnstileVerifier struct {
	secret string
	url    string
	client *http.Client
}

func NewCloudflareTurnstileVerifier(secret, verifyURL string, timeout time.Duration) *CloudflareTurnstileVerifier {
	return &CloudflareTurnstileVerifier{secret: secret, url: verifyURL, client: &http.Client{Timeout: timeout}}
}

func (v *CloudflareTurnstileVerifier) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	if strings.TrimSpace(v.secret) == "" || strings.TrimSpace(token) == "" {
		return false, nil
	}
	form := url.Values{"secret": {v.secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.url, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, errors.New("turnstile returned status " + strconv.Itoa(resp.StatusCode))
	}
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Success, nil
}

// DeterministicTurnstileVerifier is intended only for development and tests.
type DeterministicTurnstileVerifier struct{ Token string }

func (v DeterministicTurnstileVerifier) Verify(_ context.Context, token, _ string) (bool, error) {
	return v.Token != "" && hmac.Equal([]byte(token), []byte(v.Token)), nil
}
