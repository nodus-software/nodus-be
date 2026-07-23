package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	AppEnv         string
	AppPort        string
	BaseUrl        string
	AllowedOrigins []string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	ShutdownGrace  time.Duration

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
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// Fixed by the API contract, not environment-tunable.
	ChallengeTokenTTL     time.Duration
	PasswordResetTokenTTL time.Duration
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
	TOTPIssuer         string
	MFABackupCodeCount int
	MFAEncryptionKey   string

	// Lockout policy
	LockoutMaxAttempts int
	LockoutDuration    time.Duration

	// Password reset rate limiting
	PasswordResetMaxPerUsernamePerHour int
	PasswordResetMaxPerIPPerHour       int

	// Email
	SmtpHost     string
	SmtpPort     string
	SmtpSender   string
	SmtpPassword string
}

// Load reads from a .env file if present, then falls back to system env vars.
// Missing required variables cause a fatal error at startup.
func Load() (*Config, error) {
	// Attempt to load .env — log a notice if not found but don't fail,
	// since production environments typically use system env vars instead.
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from system environment")
	}

	cfg := &Config{
		AppEnv:         getEnv("APP_ENV", "development"),
		AppPort:        getEnv("APP_PORT", "8080"),
		BaseUrl:        getEnv("BASE_URL", "http://localhost"),
		AllowedOrigins: strings.Fields(getEnv("ALLOWED_ORIGINS", "")),
		ReadTimeout:    getEnvDuration("READ_TIMEOUT", 15*time.Second),
		WriteTimeout:   getEnvDuration("WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:    getEnvDuration("IDLE_TIMEOUT", 60*time.Second),
		ShutdownGrace:  getEnvDuration("SHUTDOWN_GRACE", 10*time.Second),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBName:     requireEnv("DB_NAME"),
		DBUser:     requireEnv("DB_USER"),
		DBPassword: requireEnv("DB_PASSWORD"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),
		DBUrl:      getEnv("DB_URL", ""),

		TestDBHost:     getEnv("TEST_DB_HOST", "localhost"),
		TestDBPort:     getEnv("TEST_DB_PORT", "5432"),
		TestDBName:     requireEnv("TEST_DB_NAME"),
		TestDBUser:     requireEnv("TEST_DB_USER"),
		TestDBPassword: requireEnv("TEST_DB_PASSWORD"),
		TestDBSSLMode:  getEnv("TEST_DB_SSL_MODE", "disable"),
		TestDBUrl:      getEnv("TEST_DB_URL", ""),

		RedisAddr:     getEnv("REDIS_ADDRESS", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		JWTSecret:       requireEnv("JWT_SECRET"),
		AccessTokenTTL:  getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: getEnvDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),

		ChallengeTokenTTL:     5 * time.Minute,
		PasswordResetTokenTTL: 15 * time.Minute,
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

		TOTPIssuer:         getEnv("TOTP_ISSUER", "Nodus Health"),
		MFABackupCodeCount: getEnvInt("MFA_BACKUP_CODE_COUNT", 10),
		MFAEncryptionKey:   requireEnv("MFA_ENCRYPTION_KEY"),

		LockoutMaxAttempts: getEnvInt("LOCKOUT_MAX_ATTEMPTS", 5),
		LockoutDuration:    getEnvDuration("LOCKOUT_DURATION", 15*time.Minute),

		PasswordResetMaxPerUsernamePerHour: 5,
		PasswordResetMaxPerIPPerHour:       20,

		SmtpHost:     getEnv("SMTP_HOST", ""),
		SmtpPort:     getEnv("SMTP_PORT", ""),
		SmtpSender:   getEnv("SMTP_SENDER", ""),
		SmtpPassword: getEnv("SMTP_PASSWORD", ""),
	}

	return cfg, nil
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
