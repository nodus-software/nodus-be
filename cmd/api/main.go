package main

import (
	"context"
	"os/signal"
	"syscall"

	"nodus-health/config"
	"nodus-health/internal/audit"
	auditpg "nodus-health/internal/audit/postgres"
	"nodus-health/internal/auth"
	authpg "nodus-health/internal/auth/postgres"
	"nodus-health/internal/platform/db"
	"nodus-health/internal/server"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
)

func main() {
	log := logger.NewLogger()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", "error", err.Error())
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DSN())
	if err != nil {
		log.Fatal("failed to connect to database", "error", err.Error())
	}
	defer pool.Close()

	mfaKey, err := security.DecodeKey(cfg.MFAEncryptionKey)
	if err != nil {
		log.Fatal("invalid MFA_ENCRYPTION_KEY", "error", err.Error())
	}

	auditRepo := auditpg.New(pool)
	auditService := audit.NewService(auditRepo)

	authRepo := authpg.New(pool)
	mailer := &auth.SMTPMailer{
		Host: cfg.SmtpHost, Port: cfg.SmtpPort, Sender: cfg.SmtpSender, Password: cfg.SmtpPassword,
	}

	authCfg := auth.Config{
		BaseURL: cfg.BaseUrl,

		JWTSecret:             cfg.JWTSecret,
		AccessTokenTTL:        cfg.AccessTokenTTL,
		RefreshTokenTTL:       cfg.RefreshTokenTTL,
		ChallengeTokenTTL:     cfg.ChallengeTokenTTL,
		PasswordResetTokenTTL: cfg.PasswordResetTokenTTL,

		BcryptCost: cfg.BcryptCost,

		TOTPIssuer:         cfg.TOTPIssuer,
		MFABackupCodeCount: cfg.MFABackupCodeCount,
		MFAEncryptionKey:   mfaKey,

		LockoutMaxAttempts: cfg.LockoutMaxAttempts,
		LockoutDuration:    cfg.LockoutDuration,

		PasswordResetMaxPerUsernamePerHour: cfg.PasswordResetMaxPerUsernamePerHour,
		PasswordResetMaxPerIPPerHour:       cfg.PasswordResetMaxPerIPPerHour,

		PasswordPolicy: auth.PasswordPolicy{
			MinLength:             cfg.PasswordMinLength,
			RequireUppercase:      cfg.PasswordRequireUppercase,
			RequireNumber:         cfg.PasswordRequireNumber,
			RequireSymbol:         cfg.PasswordRequireSymbol,
			RejectCommonPasswords: cfg.PasswordRejectCommon,
			MaxAgeDays:            cfg.PasswordMaxAgeDays,
		},
	}

	authService := auth.NewService(authRepo, auditService, mailer, log, authCfg)
	authHandler := auth.NewHandler(authService, cfg.JWTSecret, log)

	srv := server.New(server.Config{
		Port:           cfg.AppPort,
		AllowedOrigins: cfg.AllowedOrigins,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		IdleTimeout:    cfg.IdleTimeout,
		ShutdownGrace:  cfg.ShutdownGrace,
		RequestTimeout: cfg.WriteTimeout,
	}, log, authHandler)

	log.Info("starting nodus health api", "env", cfg.AppEnv, "port", cfg.AppPort)

	if err := srv.Run(ctx); err != nil {
		log.Fatal("server error", "error", err.Error())
	}

	log.Info("shutdown complete")
}
