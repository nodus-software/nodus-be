package test_auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pquerna/otp/totp"

	"nodus-health/internal/audit"
	"nodus-health/internal/auth"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

// TestConfig exposes the knobs asserted by these tests while Setup maps them
// to auth.Config. Keeping it here makes each test's security assumptions
// explicit rather than coupling test code to application configuration.
type TestConfig struct {
	BaseUrl                            string
	PasswordMinLength                  int
	LockoutMaxAttempts                 int
	PasswordResetMaxPerUsernamePerHour int
}

type Env struct {
	Cfg    TestConfig
	Router http.Handler
	Repo   *memoryRepo
	Mailer *memoryMailer
}

type sentMail struct{ To, Subject, Body string }
type memoryMailer struct{ Sent []sentMail }

func (m *memoryMailer) Send(_ context.Context, to, subject, body string) error {
	m.Sent = append(m.Sent, sentMail{to, subject, body})
	return nil
}
func (m *memoryMailer) LastToken() string {
	if len(m.Sent) == 0 {
		return ""
	}
	const marker = "?token="
	parts := strings.SplitN(m.Sent[len(m.Sent)-1].Body, marker, 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

type discardAudit struct{}

func (discardAudit) Record(context.Context, audit.Entry) error { return nil }

func Setup(t *testing.T) *Env {
	t.Helper()
	cfg := TestConfig{BaseUrl: "https://app.test", PasswordMinLength: 12, LockoutMaxAttempts: 3, PasswordResetMaxPerUsernamePerHour: 3}
	var encryptionKey [32]byte
	copy(encryptionKey[:], "test-only-mfa-encryption-key-32!")
	repo := newMemoryRepo()
	mailer := &memoryMailer{}
	service := auth.NewService(repo, discardAudit{}, mailer, logger.NewLogger(), auth.Config{
		BaseURL: cfg.BaseUrl, JWTSecret: "test-jwt-secret", AccessTokenTTL: time.Hour,
		RefreshTokenTTL: 24 * time.Hour, SessionRefreshTokenTTL: 2 * time.Hour, ChallengeTokenTTL: 5 * time.Minute,
		PasswordResetTokenTTL: time.Hour, BcryptCost: 4, TOTPIssuer: "Nodus Test",
		MFABackupCodeCount: 3, MFAEncryptionKey: encryptionKey, LockoutMaxAttempts: cfg.LockoutMaxAttempts,
		LockoutDuration: time.Hour, PasswordResetMaxPerUsernamePerHour: cfg.PasswordResetMaxPerUsernamePerHour,
		PasswordResetMaxPerIPPerHour: 100, PasswordPolicy: auth.PasswordPolicy{MinLength: cfg.PasswordMinLength, RequireUppercase: true, RequireNumber: true, RequireSymbol: true, RejectCommonPasswords: true},
	})
	r := chi.NewRouter()
	auth.NewHandler(service, "test-jwt-secret", logger.NewLogger()).RegisterRoutes(r)
	return &Env{Cfg: cfg, Router: r, Repo: repo, Mailer: mailer}
}

func (e *Env) JSON(t *testing.T, method, path, accessToken string, body any) *httptest.ResponseRecorder {
	return e.JSONWithCookie(t, method, path, accessToken, body, nil)
}

func (e *Env) JSONWithCookie(t *testing.T, method, path, accessToken string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(data))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.Router.ServeHTTP(rec, req)
	return rec
}
func Decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	// API success responses use {success: true, data: ...}; tests should deal
	// with the domain payload, not repeat transport-envelope plumbing.
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	data := envelope.Data
	if len(data) == 0 {
		data = rec.Body.Bytes()
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
}
func CurrentTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return code
}
func generateEd25519Keypair(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}
func signChallenge(priv ed25519.PrivateKey, challenge string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(challenge)))
}

func (e *Env) CreateUser(t *testing.T, username, email, password string) string {
	t.Helper()
	hash, err := security.HashPassword(password, 4)
	if err != nil {
		t.Fatal(err)
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	e.Repo.users[id] = auth.User{ID: id, Username: username, Email: email, FullName: username, PasswordHash: hash, Status: auth.UserStatusActive, CreatedAt: time.Now()}
	return id
}
func (e *Env) EnrollTOTP(t *testing.T, userID string) string {
	t.Helper()
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Nodus Test", AccountName: userID})
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := security.EncryptString(e.Repo.key, key.Secret())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := utility.GenerateUUID()
	now := time.Now()
	e.Repo.factors[id] = auth.MFAFactor{ID: id, UserID: userID, Type: auth.MFAFactorTOTP, SecretEncrypted: &encrypted, ConfirmedAt: &now, CreatedAt: now}
	return key.Secret()
}
func (e *Env) EnrollBiometric(t *testing.T, userID, publicKey string) {
	t.Helper()
	id, _ := utility.GenerateUUID()
	now := time.Now()
	e.Repo.factors[id] = auth.MFAFactor{ID: id, UserID: userID, Type: auth.MFAFactorBiometric, PublicKey: &publicKey, ConfirmedAt: &now, CreatedAt: now}
}
func (e *Env) CreateSession(t *testing.T, userID string) string {
	t.Helper()
	id, _ := utility.GenerateUUID()
	now := time.Now()
	e.Repo.sessions[id] = auth.Session{ID: id, UserID: userID, CreatedAt: now, LastActiveAt: now}
	return id
}
func (e *Env) IssueAccessToken(t *testing.T, userID, sessionID string) string {
	t.Helper()
	token, _, err := security.IssueAccessToken("test-jwt-secret", time.Hour, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func (e *Env) CompleteLogin(t *testing.T, email, password, secret string) (string, string) {
	return e.CompleteLoginRemember(t, email, password, secret, false)
}

func (e *Env) CompleteLoginRemember(t *testing.T, email, password, secret string, rememberMe bool) (string, string) {
	t.Helper()
	challenge := loginToChallenge(t, e, email, password)
	rec := e.JSON(t, http.MethodPost, "/auth/login/mfa", "", map[string]any{"challenge_token": challenge, "method": "totp", "code": CurrentTOTPCode(t, secret), "remember_me": rememberMe})
	if rec.Code != http.StatusOK {
		t.Fatalf("complete login: %d %s", rec.Code, rec.Body.String())
	}
	var pair struct {
		AccessToken string `json:"access_token"`
	}
	Decode(t, rec, &pair)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "nodus_refresh" || cookies[0].Value == "" {
		t.Fatalf("expected HttpOnly refresh cookie, got %#v", cookies)
	}
	if !cookies[0].HttpOnly {
		t.Fatal("refresh cookie must be HttpOnly")
	}
	return pair.AccessToken, cookies[0].Value
}

func loginToChallenge(t *testing.T, env *Env, email, password string) string {
	t.Helper()
	rec := env.JSON(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": email, "password": password,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login step 1 failed: %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		ChallengeToken string `json:"challenge_token"`
	}
	Decode(t, rec, &response)
	return response.ChallengeToken
}

type AuthedUser struct{ UserID, SessionID, AccessToken string }

func (e *Env) NewAuthedUser(t *testing.T, permissions ...string) AuthedUser {
	t.Helper()
	id := e.CreateUser(t, "user", "user@example.com", "Sup3rSecret!Pass")
	e.Repo.roles[id] = []auth.Role{{ID: "role", Name: "tester"}}
	e.Repo.permissions[id] = permissions
	sid := e.CreateSession(t, id)
	return AuthedUser{UserID: id, SessionID: sid, AccessToken: e.IssueAccessToken(t, id, sid)}
}

// memoryRepo is a complete, deliberately small Repository implementation.
// It gives handler tests deterministic isolated state while exercising all
// auth service code and middleware exactly as production wiring does.
type resetAttempt struct {
	username, ip string
	at           time.Time
}
type backupCode struct {
	userID, hash string
	used         bool
}
type enrollmentToken struct {
	id, userID, hash string
	expiresAt        time.Time
	consumed         bool
}
type memoryRepo struct {
	sync.Mutex
	users       map[string]auth.User
	challenges  map[string]auth.LoginChallenge
	sessions    map[string]auth.Session
	refresh     map[string]auth.RefreshToken
	resets      map[string]auth.PasswordResetToken
	factors     map[string]auth.MFAFactor
	backup      map[string]backupCode
	attempts    []resetAttempt
	roles       map[string][]auth.Role
	permissions map[string][]string
	enrollments map[string]enrollmentToken
	key         [32]byte
}

func newMemoryRepo() *memoryRepo {
	r := &memoryRepo{users: map[string]auth.User{}, challenges: map[string]auth.LoginChallenge{}, sessions: map[string]auth.Session{}, refresh: map[string]auth.RefreshToken{}, resets: map[string]auth.PasswordResetToken{}, factors: map[string]auth.MFAFactor{}, backup: map[string]backupCode{}, roles: map[string][]auth.Role{}, permissions: map[string][]string{}, enrollments: map[string]enrollmentToken{}}
	copy(r.key[:], "test-only-mfa-encryption-key-32!")
	return r
}
func (r *memoryRepo) GetUserByUsername(_ context.Context, name string) (*auth.User, error) {
	for _, u := range r.users {
		if u.Username == name {
			v := u
			return &v, nil
		}
	}
	return nil, auth.ErrUserNotFound
}
func (r *memoryRepo) GetUserByEmail(_ context.Context, email string) (*auth.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			v := u
			return &v, nil
		}
	}
	return nil, auth.ErrUserNotFound
}
func (r *memoryRepo) GetUserByID(_ context.Context, id string) (*auth.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return &u, nil
}
func (r *memoryRepo) IncrementFailedLoginAttempts(_ context.Context, id string) (int, error) {
	u, e := r.GetUserByID(context.Background(), id)
	if e != nil {
		return 0, e
	}
	u.FailedLoginAttempts++
	r.users[id] = *u
	return u.FailedLoginAttempts, nil
}
func (r *memoryRepo) LockUser(_ context.Context, id string, until time.Time) error {
	u, e := r.GetUserByID(context.Background(), id)
	if e != nil {
		return e
	}
	u.LockedUntil = &until
	r.users[id] = *u
	return nil
}
func (r *memoryRepo) ResetFailedLoginAttempts(_ context.Context, id string) error {
	u, e := r.GetUserByID(context.Background(), id)
	if e != nil {
		return e
	}
	u.FailedLoginAttempts = 0
	r.users[id] = *u
	return nil
}
func (r *memoryRepo) CreateLoginChallenge(_ context.Context, c auth.LoginChallenge) error {
	c.CreatedAt = time.Now()
	r.challenges[c.ID] = c
	return nil
}
func (r *memoryRepo) GetLoginChallengeByHash(_ context.Context, h string) (*auth.LoginChallenge, error) {
	for _, c := range r.challenges {
		if c.ChallengeTokenHash == h {
			v := c
			return &v, nil
		}
	}
	return nil, auth.ErrChallengeInvalid
}
func (r *memoryRepo) ConsumeLoginChallenge(_ context.Context, id string) error {
	c, ok := r.challenges[id]
	if !ok {
		return auth.ErrChallengeInvalid
	}
	n := time.Now()
	c.ConsumedAt = &n
	r.challenges[id] = c
	return nil
}
func (r *memoryRepo) CreateSession(_ context.Context, s auth.Session) error {
	n := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = n
	}
	if s.LastActiveAt.IsZero() {
		s.LastActiveAt = n
	}
	r.sessions[s.ID] = s
	return nil
}
func (r *memoryRepo) GetSessionByID(_ context.Context, id string) (*auth.Session, error) {
	s, ok := r.sessions[id]
	if !ok {
		return nil, auth.ErrSessionNotFound
	}
	return &s, nil
}
func (r *memoryRepo) ListActiveSessionsByUser(_ context.Context, uid string) (out []auth.Session, e error) {
	for _, s := range r.sessions {
		if s.UserID == uid && !s.IsRevoked() {
			out = append(out, s)
		}
	}
	return out, nil
}
func (r *memoryRepo) RevokeSession(_ context.Context, id string) error {
	s, e := r.GetSessionByID(context.Background(), id)
	if e != nil {
		return e
	}
	n := time.Now()
	s.RevokedAt = &n
	r.sessions[id] = *s
	return nil
}
func (r *memoryRepo) RevokeSessionsByUser(_ context.Context, uid string) error {
	return r.revokeSessions(uid, "")
}
func (r *memoryRepo) RevokeSessionsByUserExceptSession(_ context.Context, uid, except string) error {
	return r.revokeSessions(uid, except)
}
func (r *memoryRepo) revokeSessions(uid, except string) error {
	for id, s := range r.sessions {
		if s.UserID == uid && id != except {
			n := time.Now()
			s.RevokedAt = &n
			r.sessions[id] = s
		}
	}
	return nil
}
func (r *memoryRepo) TouchSessionLastActive(_ context.Context, id string) error {
	s, e := r.GetSessionByID(context.Background(), id)
	if e != nil {
		return e
	}
	s.LastActiveAt = time.Now()
	r.sessions[id] = *s
	return nil
}
func (r *memoryRepo) CreateRefreshToken(_ context.Context, t auth.RefreshToken) error {
	t.CreatedAt = time.Now()
	r.refresh[t.ID] = t
	return nil
}
func (r *memoryRepo) GetRefreshTokenByHash(_ context.Context, h string) (*auth.RefreshToken, error) {
	for _, t := range r.refresh {
		if t.TokenHash == h {
			v := t
			return &v, nil
		}
	}
	return nil, auth.ErrRefreshTokenInvalid
}
func (r *memoryRepo) RevokeRefreshToken(_ context.Context, id string) error {
	t, ok := r.refresh[id]
	if !ok {
		return auth.ErrRefreshTokenInvalid
	}
	if t.RevokedAt != nil {
		return auth.ErrRefreshTokenRevoked
	}
	n := time.Now()
	t.RevokedAt = &n
	r.refresh[id] = t
	return nil
}
func (r *memoryRepo) RevokeRefreshTokensByUser(_ context.Context, uid string) error {
	return r.revokeRefresh(uid, "")
}
func (r *memoryRepo) RevokeRefreshTokensByUserExceptSession(_ context.Context, uid, except string) error {
	return r.revokeRefresh(uid, except)
}
func (r *memoryRepo) revokeRefresh(uid, except string) error {
	for id, t := range r.refresh {
		if t.UserID == uid && t.SessionID != except {
			n := time.Now()
			t.RevokedAt = &n
			r.refresh[id] = t
		}
	}
	return nil
}
func (r *memoryRepo) RevokeRefreshTokensBySession(_ context.Context, sid string) error {
	for id, t := range r.refresh {
		if t.SessionID == sid {
			n := time.Now()
			t.RevokedAt = &n
			r.refresh[id] = t
		}
	}
	return nil
}
func (r *memoryRepo) UpdatePasswordHash(_ context.Context, uid, h string) error {
	u, e := r.GetUserByID(context.Background(), uid)
	if e != nil {
		return e
	}
	u.PasswordHash = h
	r.users[uid] = *u
	return nil
}
func (r *memoryRepo) CreatePasswordResetToken(_ context.Context, t auth.PasswordResetToken) error {
	t.CreatedAt = time.Now()
	r.resets[t.ID] = t
	return nil
}
func (r *memoryRepo) GetPasswordResetTokenByHash(_ context.Context, h string) (*auth.PasswordResetToken, error) {
	for _, t := range r.resets {
		if t.TokenHash == h {
			v := t
			return &v, nil
		}
	}
	return nil, auth.ErrResetTokenInvalid
}
func (r *memoryRepo) ConsumePasswordResetToken(_ context.Context, id string) error {
	t, ok := r.resets[id]
	if !ok {
		return auth.ErrResetTokenInvalid
	}
	n := time.Now()
	t.UsedAt = &n
	r.resets[id] = t
	return nil
}
func (r *memoryRepo) InvalidateOtherPasswordResetTokens(_ context.Context, uid, except string) error {
	for id, t := range r.resets {
		if t.UserID == uid && id != except {
			n := time.Now()
			t.UsedAt = &n
			r.resets[id] = t
		}
	}
	return nil
}
func (r *memoryRepo) RecordPasswordResetAttempt(_ context.Context, _, name, ip string) error {
	r.attempts = append(r.attempts, resetAttempt{name, ip, time.Now()})
	return nil
}
func (r *memoryRepo) CountPasswordResetAttemptsByUsername(_ context.Context, name string, since time.Time) (n int, e error) {
	for _, a := range r.attempts {
		if a.username == name && a.at.After(since) {
			n++
		}
	}
	return n, nil
}
func (r *memoryRepo) CountPasswordResetAttemptsByIP(_ context.Context, ip string, since time.Time) (n int, e error) {
	for _, a := range r.attempts {
		if a.ip == ip && a.at.After(since) {
			n++
		}
	}
	return n, nil
}
func (r *memoryRepo) CreateMFAFactor(_ context.Context, f auth.MFAFactor) (*auth.MFAFactor, error) {
	f.CreatedAt = time.Now()
	r.factors[f.ID] = f
	return &f, nil
}
func (r *memoryRepo) GetMFAFactorByID(_ context.Context, id string) (*auth.MFAFactor, error) {
	f, ok := r.factors[id]
	if !ok {
		return nil, auth.ErrFactorNotFound
	}
	return &f, nil
}
func (r *memoryRepo) ListMFAFactorsByUser(_ context.Context, uid string) (out []auth.MFAFactor, e error) {
	for _, f := range r.factors {
		if f.UserID == uid {
			out = append(out, f)
		}
	}
	return out, nil
}
func (r *memoryRepo) ConfirmMFAFactor(_ context.Context, id string) error {
	f, e := r.GetMFAFactorByID(context.Background(), id)
	if e != nil {
		return e
	}
	n := time.Now()
	f.ConfirmedAt = &n
	r.factors[id] = *f
	return nil
}
func (r *memoryRepo) DeleteMFAFactor(_ context.Context, id string) error {
	if _, ok := r.factors[id]; !ok {
		return auth.ErrFactorNotFound
	}
	delete(r.factors, id)
	return nil
}
func (r *memoryRepo) CountConfirmedMFAFactors(ctx context.Context, uid string) (int, error) {
	fs, _ := r.ListMFAFactorsByUser(ctx, uid)
	n := 0
	for _, f := range fs {
		if f.IsConfirmed() {
			n++
		}
	}
	return n, nil
}
func (r *memoryRepo) CreateMFABackupCode(_ context.Context, id, uid, h string) error {
	r.backup[id] = backupCode{uid, h, false}
	return nil
}
func (r *memoryRepo) GetUnusedMFABackupCodeIDByHash(_ context.Context, uid, h string) (string, error) {
	for id, b := range r.backup {
		if b.userID == uid && b.hash == h && !b.used {
			return id, nil
		}
	}
	return "", nil
}
func (r *memoryRepo) ConsumeMFABackupCode(_ context.Context, id string) error {
	b, ok := r.backup[id]
	if !ok {
		return auth.ErrMFACodeInvalid
	}
	b.used = true
	r.backup[id] = b
	return nil
}
func (r *memoryRepo) GetEnrollmentTokenByHash(_ context.Context, hash string) (string, string, time.Time, bool, error) {
	for _, token := range r.enrollments {
		if token.hash == hash {
			return token.id, token.userID, token.expiresAt, token.consumed, nil
		}
	}
	return "", "", time.Time{}, false, auth.ErrEnrollmentTokenInvalid
}
func (r *memoryRepo) ConsumeEnrollmentToken(_ context.Context, id string) error {
	token, ok := r.enrollments[id]
	if !ok {
		return auth.ErrEnrollmentTokenInvalid
	}
	token.consumed = true
	r.enrollments[id] = token
	return nil
}
func (r *memoryRepo) GetRolesByUser(_ context.Context, uid string) ([]auth.Role, error) {
	return r.roles[uid], nil
}
func (r *memoryRepo) GetEffectivePermissionsByUser(_ context.Context, uid string) ([]string, error) {
	return r.permissions[uid], nil
}
func (r *memoryRepo) WithinTx(_ context.Context, fn func(auth.Repository) error) error { return fn(r) }
