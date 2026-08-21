package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	AppEnv                    string
	AppPort                   string
	BaseUrl                   string
	TenantBaseDomain          string
	TenantURLScheme           string
	TenantURLPort             string
	ReservedOrganizationSlugs []string
	AllowedOrigins            []string
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
	IdleTimeout               time.Duration
	ShutdownGrace             time.Duration

	// Main Database
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string
	DBUrl      string

	// Test Database
	TestDBHost     string
	TestDBPort     string
	TestDBName     string
	TestDBUser     string
	TestDBPassword string
	TestDBSSLMode  string
	TestDBUrl      string

	// Redis
	RedisAddr     string
	RedisPassword string

	// Auth / JWT
	JWTSecret              string
	AccessTokenTTL         time.Duration
	RefreshTokenTTL        time.Duration
	SessionRefreshTokenTTL time.Duration
	RefreshCookieName      string
	RefreshCookieDomain    string
	RefreshCookieSecure    bool
	RefreshCookieSameSite  string

	// Fixed by the API contract, not environment-tunable.
	ChallengeTokenTTL     time.Duration
	PasswordResetTokenTTL time.Duration
	RecoveryEmailTokenTTL time.Duration
	RecoverySessionTTL    time.Duration
	RecoveryMaxAttempts   int
	InviteTokenTTL        time.Duration
	EnrollmentTokenTTL    time.Duration

	// Organization / access review
	OrganizationName  string
	AccessReviewCycle time.Duration

	// Password hashing
	BcryptCost int

	// Password complexity policy
	PasswordMinLength        int
	PasswordRequireUppercase bool
	PasswordRequireNumber    bool
	PasswordRequireSymbol    bool
	PasswordRejectCommon     bool
	PasswordMaxAgeDays       int

	// MFA
	TOTPIssuer            string
	MFABackupCodeCount    int
	MFAEncryptionKey      string
	WebAuthnRPDisplayName string
	WebAuthnRPID          string
	WebAuthnOrigins       []string
	WebAuthnCeremonyTTL   time.Duration

	// Lockout policy
	LockoutMaxAttempts           int
	LockoutDuration              time.Duration
	AuthSecurityMode             string
	AuthFailureObservationWindow time.Duration
	AuthFailureCycleWindow       time.Duration
	AuthFailureLockThreshold     int
	AuthFailureInitialLock       time.Duration
	AuthFailureMaximumLock       time.Duration
	AuthRateLimitHMACSecret      string
	AuthRateLimitIdentifierLimit int
	AuthRateLimitIPLimit         int
	AuthRateLimitTenantLimit     int
	AuthRateLimitContextLimit    int
	AuthRateLimitWindow          time.Duration
	TurnstileSecretKey           string
	TurnstileVerifyURL           string
	TurnstileTimeout             time.Duration
	TurnstileTestToken           string

	// Password reset rate limiting
	PasswordResetMaxPerUsernamePerHour int
	PasswordResetMaxPerIPPerHour       int

	// Email
	EmailProvider      string
	EmailFromName      string
	EmailFromAddress   string
	EmailHTTPTimeout   time.Duration
	ZeptoMailAPIURL    string
	ZeptoMailSendToken string
	ResendAPIURL       string
	ResendAPIKey       string
	EmailLogoURL       string
}

// Load reads from a .env file if present, then falls back to system env vars.
// Missing required variables cause a fatal error at startup.
func Load() (*Config, error) {
	// Look in the current directory and its parents. This supports running
	// from the repository root, cmd/api, or through a debugger whose working
	// directory is a package directory.
	if path := findDotEnv(); path != "" {
		if err := godotenv.Load(path); err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
	} else {
		log.Println("No .env file found, reading from system environment")
	}

	appEnv := getEnv("APP_ENV", "development")
	development := appEnv != "production"

	cfg := &Config{
		AppEnv:                    appEnv,
		AppPort:                   getEnv("APP_PORT", "8080"),
		BaseUrl:                   getEnv("BASE_URL", "http://localhost"),
		TenantBaseDomain:          strings.ToLower(strings.TrimSpace(getEnv("TENANT_BASE_DOMAIN", "localhost"))),
		TenantURLScheme:           strings.ToLower(strings.TrimSpace(getEnv("TENANT_URL_SCHEME", developmentDefault(development, "http", "https")))),
		TenantURLPort:             strings.TrimSpace(getEnv("TENANT_URL_PORT", "")),
		ReservedOrganizationSlugs: strings.Fields(strings.ToLower(getEnv("RESERVED_ORGANIZATION_SLUGS", ""))),
		AllowedOrigins:            strings.Fields(getEnv("ALLOWED_ORIGINS", "")),
		ReadTimeout:               getEnvDuration("READ_TIMEOUT", 15*time.Second),
		WriteTimeout:              getEnvDuration("WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:               getEnvDuration("IDLE_TIMEOUT", 60*time.Second),
		ShutdownGrace:             getEnvDuration("SHUTDOWN_GRACE", 10*time.Second),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", developmentDefault(development, "5433", "5432")),
		DBName:     requiredWithDevDefault("DB_NAME", development, "nodus_health"),
		DBUser:     requiredWithDevDefault("DB_USER", development, "nodus"),
		DBPassword: requiredWithDevDefault("DB_PASSWORD", development, "nodus"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),
		DBUrl:      getEnv("DB_URL", ""),

		TestDBHost:     getEnv("TEST_DB_HOST", "localhost"),
		TestDBPort:     getEnv("TEST_DB_PORT", developmentDefault(development, "5433", "5432")),
		TestDBName:     getEnv("TEST_DB_NAME", "nodus_health_test"),
		TestDBUser:     getEnv("TEST_DB_USER", "nodus"),
		TestDBPassword: getEnv("TEST_DB_PASSWORD", "nodus"),
		TestDBSSLMode:  getEnv("TEST_DB_SSL_MODE", "disable"),
		TestDBUrl:      getEnv("TEST_DB_URL", ""),

		RedisAddr:     getEnv("REDIS_ADDRESS", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		JWTSecret:              requiredWithDevDefault("JWT_SECRET", development, "development-only-change-me"),
		AccessTokenTTL:         getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:        getEnvDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		SessionRefreshTokenTTL: getEnvDuration("SESSION_REFRESH_TOKEN_TTL", 24*time.Hour),
		RefreshCookieName:      getEnv("REFRESH_COOKIE_NAME", "nodus_refresh"),
		RefreshCookieDomain:    getEnv("REFRESH_COOKIE_DOMAIN", ""),
		RefreshCookieSecure:    getEnvBool("REFRESH_COOKIE_SECURE", !development),
		RefreshCookieSameSite:  strings.ToLower(getEnv("REFRESH_COOKIE_SAME_SITE", "lax")),

		ChallengeTokenTTL:     5 * time.Minute,
		PasswordResetTokenTTL: 15 * time.Minute,
		RecoveryEmailTokenTTL: getEnvDuration("RECOVERY_EMAIL_TOKEN_TTL", 10*time.Minute),
		RecoverySessionTTL:    getEnvDuration("RECOVERY_SESSION_TTL", 10*time.Minute),
		RecoveryMaxAttempts:   getEnvInt("RECOVERY_MAX_ATTEMPTS", 5),
		InviteTokenTTL:        24 * time.Hour,
		EnrollmentTokenTTL:    30 * time.Minute,

		OrganizationName:  getEnv("ORGANIZATION_NAME", "Nodus Health"),
		AccessReviewCycle: getEnvDuration("ACCESS_REVIEW_CYCLE", 90*24*time.Hour),

		BcryptCost: getEnvInt("BCRYPT_COST", 12),

		PasswordMinLength:        getEnvInt("PASSWORD_MIN_LENGTH", 12),
		PasswordRequireUppercase: getEnvBool("PASSWORD_REQUIRE_UPPERCASE", true),
		PasswordRequireNumber:    getEnvBool("PASSWORD_REQUIRE_NUMBER", true),
		PasswordRequireSymbol:    getEnvBool("PASSWORD_REQUIRE_SYMBOL", true),
		PasswordRejectCommon:     getEnvBool("PASSWORD_REJECT_COMMON", true),
		PasswordMaxAgeDays:       getEnvInt("PASSWORD_MAX_AGE_DAYS", 90),

		TOTPIssuer:            getEnv("TOTP_ISSUER", "Nodus Health"),
		MFABackupCodeCount:    getEnvInt("MFA_BACKUP_CODE_COUNT", 10),
		MFAEncryptionKey:      requiredWithDevDefault("MFA_ENCRYPTION_KEY", development, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="),
		WebAuthnRPDisplayName: getEnv("WEBAUTHN_RP_DISPLAY_NAME", "Nodus Health"),
		WebAuthnRPID:          getEnv("WEBAUTHN_RP_ID", "localhost"),
		WebAuthnOrigins:       strings.Fields(getEnv("WEBAUTHN_ORIGINS", "http://localhost:5173 http://localhost:3000")),
		WebAuthnCeremonyTTL:   getEnvDuration("WEBAUTHN_CEREMONY_TTL", 5*time.Minute),

		LockoutMaxAttempts:           getEnvInt("LOCKOUT_MAX_ATTEMPTS", 5),
		LockoutDuration:              getEnvDuration("LOCKOUT_DURATION", 15*time.Minute),
		AuthSecurityMode:             strings.ToLower(getEnv("AUTH_SECURITY_MODE", "observation")),
		AuthFailureObservationWindow: getEnvDuration("AUTH_FAILURE_OBSERVATION_WINDOW", 15*time.Minute),
		AuthFailureCycleWindow:       getEnvDuration("AUTH_FAILURE_CYCLE_WINDOW", 24*time.Hour),
		AuthFailureLockThreshold:     getEnvInt("AUTH_FAILURE_LOCK_THRESHOLD", 10),
		AuthFailureInitialLock:       getEnvDuration("AUTH_FAILURE_INITIAL_LOCK", 15*time.Minute),
		AuthFailureMaximumLock:       getEnvDuration("AUTH_FAILURE_MAXIMUM_LOCK", time.Hour),
		AuthRateLimitHMACSecret:      requiredWithDevDefault("AUTH_RATE_LIMIT_HMAC_SECRET", development, "development-only-rate-limit-secret"),
		AuthRateLimitIdentifierLimit: getEnvInt("AUTH_RATE_LIMIT_IDENTIFIER_LIMIT", 30),
		AuthRateLimitIPLimit:         getEnvInt("AUTH_RATE_LIMIT_IP_LIMIT", 300),
		AuthRateLimitTenantLimit:     getEnvInt("AUTH_RATE_LIMIT_TENANT_LIMIT", 3000),
		AuthRateLimitContextLimit:    getEnvInt("AUTH_RATE_LIMIT_CONTEXT_LIMIT", 100),
		AuthRateLimitWindow:          getEnvDuration("AUTH_RATE_LIMIT_WINDOW", 15*time.Minute),
		TurnstileSecretKey:           getEnv("TURNSTILE_SECRET_KEY", ""),
		TurnstileVerifyURL:           getEnv("TURNSTILE_VERIFY_URL", "https://challenges.cloudflare.com/turnstile/v0/siteverify"),
		TurnstileTimeout:             getEnvDuration("TURNSTILE_TIMEOUT", 3*time.Second),
		TurnstileTestToken:           getEnv("TURNSTILE_TEST_TOKEN", "development-turnstile-pass"),

		PasswordResetMaxPerUsernamePerHour: 5,
		PasswordResetMaxPerIPPerHour:       20,

		EmailProvider:      getEnv("EMAIL_PROVIDER", ""),
		EmailFromName:      getEnv("EMAIL_FROM_NAME", "Nodus Health"),
		EmailFromAddress:   getEnv("EMAIL_FROM_ADDRESS", ""),
		EmailHTTPTimeout:   getEnvDuration("EMAIL_HTTP_TIMEOUT", 10*time.Second),
		ZeptoMailAPIURL:    getEnv("ZEPTOMAIL_API_URL", "https://api.zeptomail.com/v1.1/email"),
		ZeptoMailSendToken: getEnv("ZEPTOMAIL_SEND_TOKEN", ""),
		ResendAPIURL:       getEnv("RESEND_API_URL", "https://api.resend.com/emails"),
		ResendAPIKey:       getEnv("RESEND_API_KEY", ""),

		EmailLogoURL: getEnv("EMAIL_LOGO_URL", ""),
	}

	if cfg.SessionRefreshTokenTTL <= 0 || cfg.RefreshTokenTTL <= 0 || cfg.SessionRefreshTokenTTL > cfg.RefreshTokenTTL {
		return nil, fmt.Errorf("SESSION_REFRESH_TOKEN_TTL must be positive and no greater than REFRESH_TOKEN_TTL")
	}
	if cfg.RefreshCookieName == "" || strings.ContainsAny(cfg.RefreshCookieName, "\t\r\n ;,") {
		return nil, fmt.Errorf("REFRESH_COOKIE_NAME is invalid")
	}
	if cfg.RefreshCookieSameSite != "lax" && cfg.RefreshCookieSameSite != "strict" {
		return nil, fmt.Errorf("REFRESH_COOKIE_SAME_SITE must be lax or strict")
	}
	if cfg.WebAuthnRPID == "" || len(cfg.WebAuthnOrigins) == 0 || cfg.WebAuthnCeremonyTTL <= 0 {
		return nil, fmt.Errorf("WebAuthn RP ID, origins, and ceremony TTL must be configured")
	}
	if cfg.TenantBaseDomain == "" || strings.Contains(cfg.TenantBaseDomain, ":") {
		return nil, fmt.Errorf("TENANT_BASE_DOMAIN must be a hostname without a port")
	}
	if cfg.TenantURLScheme != "http" && cfg.TenantURLScheme != "https" {
		return nil, fmt.Errorf("TENANT_URL_SCHEME must be http or https")
	}
	if cfg.TenantURLPort != "" {
		port, err := strconv.Atoi(cfg.TenantURLPort)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("TENANT_URL_PORT must be empty or a valid TCP port")
		}
	}
	if cfg.AuthSecurityMode != "observation" && cfg.AuthSecurityMode != "enforcement" {
		return nil, fmt.Errorf("AUTH_SECURITY_MODE must be observation or enforcement")
	}
	if cfg.AppEnv == "production" && cfg.AuthSecurityMode == "enforcement" && (cfg.AuthRateLimitHMACSecret == "" || cfg.TurnstileSecretKey == "") {
		return nil, fmt.Errorf("AUTH_RATE_LIMIT_HMAC_SECRET and TURNSTILE_SECRET_KEY are required for production enforcement")
	}
	if cfg.AuthFailureObservationWindow <= 0 || cfg.AuthFailureCycleWindow <= 0 || cfg.AuthFailureLockThreshold <= 0 || cfg.AuthFailureInitialLock <= 0 || cfg.AuthFailureMaximumLock < cfg.AuthFailureInitialLock {
		return nil, fmt.Errorf("authentication failure policy values are invalid")
	}

	return cfg, nil
}

func findDotEnv() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func developmentDefault(development bool, dev, production string) string {
	if development {
		return dev
	}
	return production
}

// requiredWithDevDefault keeps production fail-closed while making a fresh
// local checkout work with docker-compose's documented credentials.
func requiredWithDevDefault(key string, development bool, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	if development {
		return defaultValue
	}
	return requireEnv(key)
}

// DSN builds the PostgreSQL connection string from config values.
func (c *Config) DSN() string {
	if c.DBUrl != "" {
		return c.DBUrl
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

// TestDSN builds the PostgreSQL connection string for the test database.
func (c *Config) TestDSN() string {
	if c.TestDBUrl != "" {
		return c.TestDBUrl
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.TestDBUser, c.TestDBPassword, c.TestDBHost, c.TestDBPort, c.TestDBName, c.TestDBSSLMode,
	)
}

// IsProd returns true when running in production mode.
func (c *Config) IsProd() bool {
	return c.AppEnv == "production"
}

// helpers

// getEnv returns the env var value or a default if unset.
func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

// requireEnv returns the env var value or fatals if unset/empty.
func requireEnv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		log.Fatalf("required environment variable %q is not set", key)
	}
	return val
}

// getEnvInt returns the env var parsed as int, or a default on failure.
func getEnvInt(key string, defaultVal int) int {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		log.Printf("warning: %q is not a valid integer, using default %d", key, defaultVal)
		return defaultVal
	}
	return i
}

// getEnvBool returns the env var parsed as a bool, or a default on failure.
func getEnvBool(key string, defaultVal bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		log.Printf("warning: %q is not a valid bool, using default %t", key, defaultVal)
		return defaultVal
	}
	return b
}

// getEnvDuration returns the env var parsed as a time.Duration, or a default on failure.
func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		log.Printf("warning: %q is not a valid duration, using default %s", key, defaultVal)
		return defaultVal
	}
	return d
}
