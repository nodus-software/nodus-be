package invitation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/internal/auth"
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
	InviteTokenTTL     time.Duration
	EnrollmentTokenTTL time.Duration
	BcryptCost         int
	OrganizationName   string
	PasswordPolicy     auth.PasswordPolicy
}

type Service struct {
	repo   Repository
	audit  AuditRecorder
	mailer Mailer
	log    *logger.Logger
	cfg    Config
}

func NewService(repo Repository, audit AuditRecorder, mailer Mailer, log *logger.Logger, cfg Config) *Service {
	return &Service{repo: repo, audit: audit, mailer: mailer, log: log, cfg: cfg}
}

// Invite creates a pending user (status=invited), assigns their roles, and
// emails a single-use accept-invite link. provider_identifier is required
// only when at least one selected role requires it (Reg. 12(b)(v)).
func (s *Service) Invite(ctx context.Context, actorUserID string, req InviteUserRequest) (*UserProfileResponse, error) {
	_, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err == nil {
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

	inviteLink := fmt.Sprintf("%s/accept-invite?token=%s", s.cfg.BaseURL, rawToken)
	if err := s.mailer.Send(ctx, req.Email, "You've been invited to Nodus Health", inviteLink); err != nil {
		s.log.Error("failed to send invitation email", "error", err.Error())
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "user_invited", Result: audit.ResultSuccess,
		TargetResource: userID, Metadata: map[string]any{"email": req.Email},
	})

	roleNames, err := s.repo.GetUserRoleNames(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserProfileResponse{
		ID: userID, FullName: req.FullName, Username: req.Email, Email: req.Email,
		ProviderIdentifier: providerIdentifier, Roles: roleNames, Permissions: []string{},
		Status: string(UserStatusInvited), MFAEnrolled: false,
	}, nil
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
	if user.Status != UserStatusInvited {
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

	inviteLink := fmt.Sprintf("%s/accept-invite?token=%s", s.cfg.BaseURL, rawToken)
	if err := s.mailer.Send(ctx, user.Email, "Your Nodus Health invitation", inviteLink); err != nil {
		s.log.Error("failed to send invitation email", "error", err.Error())
	}

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
