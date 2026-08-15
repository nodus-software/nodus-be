package invitation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/internal/auth"
	"nodus-health/internal/email"
	"nodus-health/internal/tenant"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

// AuditRecorder is the narrow slice of the audit domain this service depends
// on, defined here (the consumer) rather than in the audit package.
type AuditRecorder interface {
	Record(ctx context.Context, entry audit.Entry) error
}

// Config is the subset of application configuration the invitation service
// needs. Password policy is the Auth domain's own type/validator — reused
// here rather than re-implemented, so "what makes a valid password" has one
// definition shared by change-password, reset-password, and accept-invite.
type Config struct {
	BaseURL            string
	TenantBaseDomain   string
	TenantURLScheme    string
	TenantURLPort      string
	InviteTokenTTL     time.Duration
	EnrollmentTokenTTL time.Duration
	BcryptCost         int
	OrganizationName   string
	PasswordPolicy     auth.PasswordPolicy
	AccessReviewCycle  time.Duration
}

func (s *Service) tenantURL(identity tenant.Identity, path string) string {
	if s.cfg.TenantBaseDomain != "" && s.cfg.TenantURLScheme != "" {
		port := ""
		if s.cfg.TenantURLPort != "" {
			port = ":" + s.cfg.TenantURLPort
		}
		return fmt.Sprintf("%s://%s.%s%s%s", s.cfg.TenantURLScheme, identity.Slug, s.cfg.TenantBaseDomain, port, path)
	}
	return strings.TrimRight(s.cfg.BaseURL, "/") + path
}

type Service struct {
	repo   Repository
	audit  AuditRecorder
	mailer Mailer
	email  *email.Renderer
	log    *logger.Logger
	cfg    Config
}

func NewService(repo Repository, audit AuditRecorder, mailer Mailer, renderer *email.Renderer, log *logger.Logger, cfg Config) *Service {
	return &Service{repo: repo, audit: audit, mailer: mailer, email: renderer, log: log, cfg: cfg}
}

func (s *Service) sendInvitationEmail(ctx context.Context, recipientName, recipientEmail, inviteLink string, expiresAt time.Time, resend bool) {
	rendered, err := s.email.RenderStaffInvitation(email.StaffInvitationData{
		CommonData: email.CommonData{
			RecipientName: recipientName, OrganizationName: s.cfg.OrganizationName,
		},
		InviterName: "An administrator", InviteURL: inviteLink, ExpiresAt: expiresAt, IsResend: resend,
	})
	if err != nil {
		s.log.Error("failed to render invitation email", "error", err.Error())
		return
	}
	if err := s.mailer.SendHTML(ctx, recipientEmail, rendered.Subject, rendered.Text, rendered.HTML); err != nil {
		s.log.Error("failed to send invitation email", "error", err.Error())
	}
}

func (s *Service) invitationLink(ctx context.Context, rawToken string) (string, error) {
	identity, ok := tenant.FromContext(ctx)
	if !ok {
		return "", tenant.ErrMissing
	}
	return s.tenantURL(identity, "/invite?token="+url.QueryEscape(rawToken)), nil
}

// Invite creates a pending user (status=invited), assigns their roles, and
// emails a single-use accept-invite link. provider_identifier is required
// only when at least one selected role requires it (Reg. 12(b)(v)).
func (s *Service) Invite(ctx context.Context, actorUserID string, req InviteUserRequest) (*UserProfileResponse, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	existing, err := s.repo.GetUserByEmail(ctx, tenantID, req.Email)
	if err == nil {
		if (existing.Status == UserStatusInvited || existing.Status == UserStatusSuspended) && !existing.PasswordSet {
			if err := s.Resend(ctx, actorUserID, existing.ID); err != nil {
				return nil, err
			}
			roleNames, err := s.repo.GetUserRoleNames(ctx, existing.ID)
			if err != nil {
				return nil, err
			}
			return &UserProfileResponse{
				ID: existing.ID, TenantID: tenantID, FullName: existing.FullName, Username: existing.Email,
				Email: existing.Email, ProviderIdentifier: existing.ProviderIdentifier, Roles: roleNames,
				Permissions: []string{}, Status: string(UserStatusInvited), MFAEnrolled: false,
			}, nil
		}
		return nil, ErrEmailAlreadyExists
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}

	roleIDs := dedupe(req.RoleIDs)
	rolesFound, err := s.repo.GetRolesByIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	if len(rolesFound) != len(roleIDs) {
		return nil, ErrRoleNotFound
	}

	var requiresProviderIdentifier bool
	for _, r := range rolesFound {
		requiresProviderIdentifier = requiresProviderIdentifier || r.RequiresProviderIdentifier
	}
	if requiresProviderIdentifier && req.ProviderIdentifier == "" {
		return nil, ErrProviderIdentifierRequired
	}

	var providerIdentifier *string
	if req.ProviderIdentifier != "" {
		providerIdentifier = &req.ProviderIdentifier
	}

	userID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	invitationID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	rawToken, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.CreateInvitedUser(ctx, CreateInvitedUserParams{
			ID: userID, FullName: req.FullName, Username: req.Email, Email: req.Email,
			ProviderIdentifier: providerIdentifier,
		}); err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			if err := repo.AssignUserRole(ctx, userID, roleID); err != nil {
				return err
			}
		}
		return repo.CreateInvitation(ctx, Invitation{
			ID: invitationID, UserID: userID, InvitedBy: actorUserID,
			TokenHash: security.HashToken(rawToken), ExpiresAt: now.Add(s.cfg.InviteTokenTTL),
		})
	})
	if err != nil {
		return nil, err
	}

	inviteLink, err := s.invitationLink(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	s.sendInvitationEmail(ctx, req.FullName, req.Email, inviteLink, now.Add(s.cfg.InviteTokenTTL), false)

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "user_invited", Result: audit.ResultSuccess,
		TargetResource: userID,
	})

	roleNames, err := s.repo.GetUserRoleNames(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserProfileResponse{
		ID: userID, TenantID: tenantID, FullName: req.FullName, Username: req.Email, Email: req.Email,
		ProviderIdentifier: providerIdentifier, Roles: roleNames, Permissions: []string{},
		Status: string(UserStatusInvited), MFAEnrolled: false,
	}, nil
}

func (s *Service) reactivationLink(ctx context.Context, rawToken string) (string, error) {
	identity, ok := tenant.FromContext(ctx)
	if !ok {
		return "", tenant.ErrMissing
	}
	return s.tenantURL(identity, "/reactivate?token="+url.QueryEscape(rawToken)), nil
}

func (s *Service) sendReactivationEmail(ctx context.Context, user *PendingUser, link string, expiresAt time.Time) {
	rendered, err := s.email.RenderAccountReactivation(email.AccountReactivationData{
		CommonData:      email.CommonData{RecipientName: user.FullName, OrganizationName: s.cfg.OrganizationName},
		ReactivationURL: link, ExpiresAt: expiresAt,
	})
	if err != nil {
		s.log.Error("failed to render account reactivation email", "error", err.Error())
		return
	}
	if err := s.mailer.SendHTML(ctx, user.Email, rendered.Subject, rendered.Text, rendered.HTML); err != nil {
		s.log.Error("failed to send account reactivation email", "error", err.Error())
	}
}

// RequestReactivation keeps the account inaccessible while issuing a
// one-time link for fresh password and MFA setup.
func (s *Service) RequestReactivation(ctx context.Context, actorUserID, targetUserID, reason string) error {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if user.Status != UserStatusDeactivated {
		return ErrNotDeactivated
	}
	rawToken, err := security.GenerateToken()
	if err != nil {
		return err
	}
	tokenID, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	now := time.Now()
	expiresAt := now.Add(s.cfg.InviteTokenTTL)
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ConsumeReactivationTokensByUser(ctx, targetUserID); err != nil {
			return err
		}
		return repo.CreateReactivationToken(ctx, ReactivationToken{
			ID: tokenID, UserID: targetUserID, RequestedBy: actorUserID,
			TokenHash: security.HashToken(rawToken), ExpiresAt: expiresAt,
		})
	})
	if err != nil {
		return err
	}
	link, err := s.reactivationLink(ctx, rawToken)
	if err != nil {
		return err
	}
	s.sendReactivationEmail(ctx, user, link, expiresAt)
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "user_reactivation_requested", Result: audit.ResultSuccess,
		TargetResource: targetUserID, Metadata: map[string]any{"reason": reason},
	})
	return nil
}

func (s *Service) ValidateReactivation(ctx context.Context, rawToken string) (*ValidateReactivationResponse, error) {
	token, err := s.repo.GetReactivationTokenByHash(ctx, security.HashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if token.IsUsed() {
		return nil, ErrReactivationTokenInvalid
	}
	if token.IsExpired(time.Now()) {
		return nil, ErrReactivationTokenExpired
	}
	user, err := s.repo.GetUserByID(ctx, token.UserID)
	if err != nil || user.Status != UserStatusDeactivated {
		return nil, ErrReactivationTokenInvalid
	}
	return &ValidateReactivationResponse{FullName: user.FullName, Email: user.Email, Organization: s.cfg.OrganizationName}, nil
}

func (s *Service) AcceptReactivation(ctx context.Context, rawToken, password string) (*EnrollmentTokenResponse, error) {
	token, err := s.repo.GetReactivationTokenByHash(ctx, security.HashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if token.IsUsed() {
		return nil, ErrReactivationTokenInvalid
	}
	now := time.Now()
	if token.IsExpired(now) {
		return nil, ErrReactivationTokenExpired
	}
	user, err := s.repo.GetUserByID(ctx, token.UserID)
	if err != nil || user.Status != UserStatusDeactivated {
		return nil, ErrReactivationTokenInvalid
	}
	if violations := auth.ValidatePasswordPolicy(password, s.cfg.PasswordPolicy); len(violations) > 0 {
		return nil, &PolicyViolationError{Violations: violations}
	}
	passwordHash, err := security.HashPassword(password, s.cfg.BcryptCost)
	if err != nil {
		return nil, err
	}
	rawEnrollment, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	enrollmentID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ConsumeReactivationToken(ctx, token.ID); err != nil {
			return err
		}
		if err := repo.ConsumeReactivationTokensByUser(ctx, token.UserID); err != nil {
			return err
		}
		if err := repo.ResetMFABackupCodesByUser(ctx, token.UserID); err != nil {
			return err
		}
		if err := repo.ResetMFAByUser(ctx, token.UserID); err != nil {
			return err
		}
		if err := repo.ActivateReactivatedUser(ctx, token.UserID, passwordHash, now, now.Add(s.cfg.AccessReviewCycle)); err != nil {
			return err
		}
		return repo.CreateEnrollmentToken(ctx, EnrollmentToken{
			ID: enrollmentID, UserID: token.UserID, TokenHash: security.HashToken(rawEnrollment),
			ExpiresAt: now.Add(s.cfg.EnrollmentTokenTTL),
		})
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &token.UserID, Action: "user_reactivated", Result: audit.ResultSuccess,
		TargetResource: token.UserID, Metadata: map[string]any{"requested_by": token.RequestedBy},
	})
	return &EnrollmentTokenResponse{EnrollmentToken: rawEnrollment}, nil
}

func (s *Service) CancelInvitation(ctx context.Context, actorUserID, targetUserID, reason string) error {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if user.Status != UserStatusInvited || user.PasswordSet {
		return ErrNotPending
	}
	if err := s.repo.DeletePendingUser(ctx, targetUserID); err != nil {
		return err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "invitation_cancelled", Result: audit.ResultSuccess,
		TargetResource: targetUserID, Metadata: map[string]any{"reason": reason},
	})
	return nil
}

// ValidateToken previews an invitation before the accept-invite page is
// shown, without consuming it.
func (s *Service) ValidateToken(ctx context.Context, rawToken string) (*ValidateInviteResponse, error) {
	inv, err := s.repo.GetInvitationByTokenHash(ctx, security.HashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if inv.IsUsed() {
		return nil, ErrTokenInvalid
	}
	if inv.IsExpired(time.Now()) {
		return nil, ErrTokenExpired
	}

	user, err := s.repo.GetUserByID(ctx, inv.UserID)
	if err != nil {
		return nil, err
	}

	return &ValidateInviteResponse{FullName: user.FullName, Email: user.Email, Organization: s.cfg.OrganizationName}, nil
}

// Accept sets the invitee's password, activates the account, invalidates
// the invite token, and issues a short-lived enrollment token scoped only
// to completing MFA setup — never a real session, so accepting an invite
// alone can never bypass the mandatory MFA step.
func (s *Service) Accept(ctx context.Context, rawToken, password string) (*EnrollmentTokenResponse, error) {
	inv, err := s.repo.GetInvitationByTokenHash(ctx, security.HashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if inv.IsUsed() {
		return nil, ErrTokenInvalid
	}
	now := time.Now()
	if inv.IsExpired(now) {
		return nil, ErrTokenExpired
	}

	if violations := auth.ValidatePasswordPolicy(password, s.cfg.PasswordPolicy); len(violations) > 0 {
		return nil, &PolicyViolationError{Violations: violations}
	}

	hash, err := security.HashPassword(password, s.cfg.BcryptCost)
	if err != nil {
		return nil, err
	}

	rawEnrollment, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	enrollmentID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ConsumeInvitation(ctx, inv.ID); err != nil {
			return err
		}
		if err := repo.ActivateUserWithPassword(ctx, inv.UserID, hash); err != nil {
			return err
		}
		return repo.CreateEnrollmentToken(ctx, EnrollmentToken{
			ID: enrollmentID, UserID: inv.UserID,
			TokenHash: security.HashToken(rawEnrollment), ExpiresAt: now.Add(s.cfg.EnrollmentTokenTTL),
		})
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &inv.UserID, Action: "invitation_accepted", Result: audit.ResultSuccess,
	})

	return &EnrollmentTokenResponse{EnrollmentToken: rawEnrollment}, nil
}

// Resend invalidates the prior invitation token and issues a new one. The
// contract's path parameter is named "token", but an authenticated admin
// resending an invite never has the invitee's raw secret token — only the
// invitee does — so this endpoint treats the path segment as the invited
// user's ID instead, which is what an admin actually has on hand (e.g. from
// GET /users).
func (s *Service) Resend(ctx context.Context, actorUserID, targetUserID string) error {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if user.Status != UserStatusInvited && !(user.Status == UserStatusSuspended && !user.PasswordSet) {
		return ErrNotPending
	}

	latest, err := s.repo.GetLatestInvitationByUserID(ctx, targetUserID)
	if err != nil {
		return err
	}

	rawToken, err := security.GenerateToken()
	if err != nil {
		return err
	}
	newID, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	now := time.Now()

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if user.Status != UserStatusInvited {
			if err := repo.RestoreInvitedUser(ctx, targetUserID); err != nil {
				return err
			}
		}
		if !latest.IsUsed() {
			if err := repo.ConsumeInvitation(ctx, latest.ID); err != nil {
				return err
			}
		}
		return repo.CreateInvitation(ctx, Invitation{
			ID: newID, UserID: targetUserID, InvitedBy: actorUserID,
			TokenHash: security.HashToken(rawToken), ExpiresAt: now.Add(s.cfg.InviteTokenTTL),
		})
	})
	if err != nil {
		return err
	}

	inviteLink, err := s.invitationLink(ctx, rawToken)
	if err != nil {
		return err
	}
	s.sendInvitationEmail(ctx, user.FullName, user.Email, inviteLink, now.Add(s.cfg.InviteTokenTTL), true)

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "invitation_resent", Result: audit.ResultSuccess, TargetResource: targetUserID,
	})
	return nil
}

func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
