package main

import (
	"context"
	"net/http"
	"os/signal"
	"strings"
	"syscall"

	"nodus-health/config"
	"nodus-health/internal/audit"
	auditpg "nodus-health/internal/audit/postgres"
	"nodus-health/internal/auth"
	authpg "nodus-health/internal/auth/postgres"
	"nodus-health/internal/clinical"
	clinicalpg "nodus-health/internal/clinical/postgres"
	"nodus-health/internal/email"
	"nodus-health/internal/invitation"
	invitationpg "nodus-health/internal/invitation/postgres"
	"nodus-health/internal/organizations"
	"nodus-health/internal/patients"
	patientspg "nodus-health/internal/patients/postgres"
	"nodus-health/internal/platform/db"
	"nodus-health/internal/roles"
	rolespg "nodus-health/internal/roles/postgres"
	"nodus-health/internal/server"
	"nodus-health/internal/users"
	userspg "nodus-health/internal/users/postgres"
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
	if cfg.AppEnv == "production" {
		if err := db.ValidateTenantRuntimeRole(ctx, pool); err != nil {
			log.Fatal("unsafe database runtime role", "error", err.Error())
		}
	}

	mfaKey, err := security.DecodeKey(cfg.MFAEncryptionKey)
	if err != nil {
		log.Fatal("invalid MFA_ENCRYPTION_KEY", "error", err.Error())
	}

	auditRepo := auditpg.New(pool)
	auditService := audit.NewService(auditRepo)

	authRepo := authpg.New(pool)
	emailRenderer := email.NewRenderer(email.CommonData{
		AppName: "Nodus Health", AppURL: cfg.BaseUrl, LogoURL: cfg.EmailLogoURL,
	})
	if cfg.AppEnv == "production" && strings.TrimSpace(cfg.EmailProvider) == "" {
		log.Fatal("EMAIL_PROVIDER is required in production")
	}
	if strings.TrimSpace(cfg.EmailProvider) != "" {
		provider, err := email.NewProvider(email.ProviderConfig{
			Name: cfg.EmailProvider, FromName: cfg.EmailFromName, FromAddress: cfg.EmailFromAddress,
			ZeptoURL: cfg.ZeptoMailAPIURL, ZeptoToken: cfg.ZeptoMailSendToken,
			ResendURL: cfg.ResendAPIURL, ResendAPIKey: cfg.ResendAPIKey,
			HTTPClient: &http.Client{Timeout: cfg.EmailHTTPTimeout},
		})
		if err != nil {
			log.Fatal("invalid email provider configuration", "error", err.Error())
		}
		go email.NewWorker(pool, provider, log).Run(ctx)
	}

	authCfg := auth.Config{
		BaseURL:          cfg.BaseUrl,
		TenantBaseDomain: cfg.TenantBaseDomain,
		TenantURLScheme:  cfg.TenantURLScheme,
		TenantURLPort:    cfg.TenantURLPort,

		JWTSecret:              cfg.JWTSecret,
		AccessTokenTTL:         cfg.AccessTokenTTL,
		RefreshTokenTTL:        cfg.RefreshTokenTTL,
		SessionRefreshTokenTTL: cfg.SessionRefreshTokenTTL,
		ChallengeTokenTTL:      cfg.ChallengeTokenTTL,
		PasswordResetTokenTTL:  cfg.PasswordResetTokenTTL,

		BcryptCost: cfg.BcryptCost,

		TOTPIssuer:            cfg.TOTPIssuer,
		MFABackupCodeCount:    cfg.MFABackupCodeCount,
		MFAEncryptionKey:      mfaKey,
		WebAuthnRPDisplayName: cfg.WebAuthnRPDisplayName,
		WebAuthnRPID:          cfg.WebAuthnRPID,
		WebAuthnOrigins:       cfg.WebAuthnOrigins,
		WebAuthnCeremonyTTL:   cfg.WebAuthnCeremonyTTL,

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

	authService := auth.NewService(authRepo, auditService, emailRenderer, log, authCfg)
	sameSite := http.SameSiteLaxMode
	switch cfg.RefreshCookieSameSite {
	case "strict":
		sameSite = http.SameSiteStrictMode
	}
	authHandler := auth.NewHandler(authService, cfg.JWTSecret, log, auth.RefreshCookieConfig{
		Name: cfg.RefreshCookieName, Domain: cfg.RefreshCookieDomain,
		Secure: cfg.RefreshCookieSecure, SameSite: sameSite,
	})

	// authService.Authorize (session/user validity + effective permissions)
	// is the single Authorizer every domain's Authenticate middleware uses,
	// so a revoked session or role change takes effect on the very next
	// request no matter which domain's endpoint is called.
	rolesRepo := rolespg.New(pool)
	rolesService := roles.NewService(rolesRepo, auditService, log)
	rolesHandler := roles.NewHandler(rolesService, authService, cfg.JWTSecret, log)

	usersRepo := userspg.New(pool)
	usersService := users.NewService(usersRepo, auditService, log, users.Config{
		AccessReviewCycle: cfg.AccessReviewCycle,
	})
	usersHandler := users.NewHandler(usersService, authService, cfg.JWTSecret, log)

	invitationRepo := invitationpg.New(pool)
	invitationService := invitation.NewService(invitationRepo, auditService, emailRenderer, log, invitation.Config{
		BaseURL:            cfg.BaseUrl,
		TenantBaseDomain:   cfg.TenantBaseDomain,
		TenantURLScheme:    cfg.TenantURLScheme,
		TenantURLPort:      cfg.TenantURLPort,
		InviteTokenTTL:     cfg.InviteTokenTTL,
		EnrollmentTokenTTL: cfg.EnrollmentTokenTTL,
		BcryptCost:         cfg.BcryptCost,
		OrganizationName:   cfg.OrganizationName,
		PasswordPolicy:     authCfg.PasswordPolicy,
		AccessReviewCycle:  cfg.AccessReviewCycle,
	})
	invitationHandler := invitation.NewHandler(invitationService, authService, cfg.JWTSecret, log)

	auditHandler := audit.NewHandler(auditService, authService, cfg.JWTSecret, log)

	patientsRepo := patientspg.New(pool)
	patientsService := patients.NewService(patientsRepo, auditService, log, patients.Config{})
	patientsHandler := patients.NewHandler(patientsService, authService, cfg.JWTSecret, log)

	clinicalRepo := clinicalpg.New(pool)
	clinicalService := clinical.NewService(clinicalRepo, auditService)
	clinicalHandler := clinical.NewHandler(clinicalService, authService, cfg.JWTSecret, log)

	organizationService := organizations.NewService(
		pool, emailRenderer, organizations.Config{
			BaseURL: cfg.BaseUrl, TenantBaseDomain: cfg.TenantBaseDomain, TenantURLScheme: cfg.TenantURLScheme, TenantURLPort: cfg.TenantURLPort,
			ReservedSlugs: cfg.ReservedOrganizationSlugs, BcryptCost: cfg.BcryptCost, PasswordPolicy: authCfg.PasswordPolicy,
		}, log,
	)
	if err := organizationService.ValidateReservedSlugs(ctx); err != nil {
		log.Fatal("invalid reserved organization slug configuration", "error", err.Error())
	}
	organizationHandler := organizations.NewHandler(organizationService, authService, cfg.JWTSecret, cfg.EnrollmentTokenTTL)

	srv := server.New(server.Config{
		Port:                  cfg.AppPort,
		AllowedOrigins:        cfg.AllowedOrigins,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		IdleTimeout:           cfg.IdleTimeout,
		ShutdownGrace:         cfg.ShutdownGrace,
		RequestTimeout:        cfg.WriteTimeout,
		TenantResolver:        organizationService,
		TenantPool:            pool,
		TenantBaseDomain:      cfg.TenantBaseDomain,
		TenantURLScheme:       cfg.TenantURLScheme,
		TenantURLPort:         cfg.TenantURLPort,
		AllowTenantSlugHeader: cfg.AppEnv != "production",
	}, log, organizationHandler, authHandler, rolesHandler, usersHandler, invitationHandler, auditHandler, patientsHandler, clinicalHandler)

	log.Info("starting nodus health api", "env", cfg.AppEnv, "port", cfg.AppPort)

	if err := srv.Run(ctx); err != nil {
		log.Fatal("server error", "error", err.Error())
	}

	log.Info("shutdown complete")
}
