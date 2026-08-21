package users

import (
	"context"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/internal/email"
	"nodus-health/pkg/logger"
)

// AuditRecorder is the narrow slice of the audit domain this service depends
// on, defined here (the consumer) rather than in the audit package.
type AuditRecorder interface {
	Record(ctx context.Context, entry audit.Entry) error
}

func queueLifecycleNotification(ctx context.Context, repo Repository, user *User, action string) error {
	copy := map[string]struct{ subject, body string }{
		"suspended":   {"Your Nodus Health account was suspended", "Your account was suspended by an organization administrator. Contact your administrator if you need assistance."},
		"restored":    {"Your Nodus Health account was restored", "Your account access was restored by an organization administrator."},
		"deactivated": {"Your Nodus Health account was deactivated", "Your account was deactivated by an organization administrator. Contact your administrator if this is unexpected."},
	}[action]
	tenantID := user.TenantID
	return repo.QueueEmail(ctx, email.Message{TenantID: &tenantID, Kind: email.SecurityNotification, To: user.Email, Subject: copy.subject, Text: copy.body, HTML: "<p>" + copy.body + "</p>", DedupeKey: "lifecycle:" + action + ":" + user.ID + ":" + time.Now().UTC().Format("2006-01-02")})
}

// Config is the subset of application configuration the users service
// needs.
type Config struct {
	// AccessReviewCycle is how far in the future next_access_review_due is
	// set whenever a review is recorded (quarterly, per Reg. 12(b)(iii)).
	AccessReviewCycle time.Duration
}

type Service struct {
	repo  Repository
	audit AuditRecorder
	log   *logger.Logger
	cfg   Config
}

func revokeUserAccess(ctx context.Context, repo Repository, userID string) error {
	operations := []func() error{
		func() error { return repo.RevokeSessionsByUser(ctx, userID) },
		func() error { return repo.RevokeRefreshTokensByUser(ctx, userID) },
		func() error { return repo.ConsumeLoginChallengesByUser(ctx, userID) },
		func() error { return repo.ConsumePasswordResetTokensByUser(ctx, userID) },
		func() error { return repo.ConsumeEnrollmentTokensByUser(ctx, userID) },
		func() error { return repo.ConsumeRecoverySessionsByUser(ctx, userID) },
	}
	for _, operation := range operations {
		if err := operation(); err != nil {
			return err
		}
	}
	return nil
}

func NewService(repo Repository, audit AuditRecorder, log *logger.Logger, cfg Config) *Service {
	return &Service{repo: repo, audit: audit, log: log, cfg: cfg}
}

func (s *Service) ListUsers(ctx context.Context, filter ListUsersFilter) ([]UserProfileResponse, error) {
	list, err := s.repo.ListUsers(ctx, filter)
	if err != nil {
		return nil, err
	}
	resp := make([]UserProfileResponse, 0, len(list))
	for _, u := range list {
		resp = append(resp, toUserProfileResponse(u))
	}
	return resp, nil
}

// UpdateUser applies role and provider-identifier changes. Assigning a superuser
// role requires the actor themselves be a superuser (Reg. 12(b)(iv)), and
// assigning any role that requires a provider identifier requires the user
// end up with one set (Reg. 12(b)(v)).
func (s *Service) UpdateUser(ctx context.Context, actorUserID, targetUserID string, req UpdateUserRequest) (*UserProfileResponse, error) {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if req.Status != nil {
		return nil, ErrInvalidStatusTransition
	}
	targetIsSuperuser := false

	if len(req.RoleIDs) > 0 {
		targetIsSuperuser, err = s.repo.HasSuperuserRole(ctx, targetUserID)
		if err != nil {
			return nil, err
		}
		newRoles, err := s.repo.GetRolesByIDs(ctx, req.RoleIDs)
		if err != nil {
			return nil, err
		}
		if len(newRoles) != len(dedupe(req.RoleIDs)) {
			return nil, ErrRoleNotFound
		}

		var assigningSuperuser, requiresProviderIdentifier bool
		for _, r := range newRoles {
			assigningSuperuser = assigningSuperuser || r.IsSuperuserRole
			requiresProviderIdentifier = requiresProviderIdentifier || r.RequiresProviderIdentifier
		}

		if assigningSuperuser {
			actorIsSuperuser, err := s.repo.HasSuperuserRole(ctx, actorUserID)
			if err != nil {
				return nil, err
			}
			if !actorIsSuperuser {
				return nil, ErrSuperuserRequired
			}
		}
		if targetIsSuperuser {
			actorIsSuperuser, err := s.repo.HasSuperuserRole(ctx, actorUserID)
			if err != nil {
				return nil, err
			}
			if !actorIsSuperuser {
				return nil, ErrSuperuserRequired
			}
		}

		providerIdentifier := user.ProviderIdentifier
		if req.ProviderIdentifier != nil {
			providerIdentifier = req.ProviderIdentifier
		}
		if requiresProviderIdentifier && (providerIdentifier == nil || *providerIdentifier == "") {
			return nil, ErrProviderIdentifierRequired
		}
	}

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if len(req.RoleIDs) > 0 && targetIsSuperuser {
			newRoles, err := repo.GetRolesByIDs(ctx, req.RoleIDs)
			if err != nil {
				return err
			}
			keepsSuperuser := false
			for _, role := range newRoles {
				keepsSuperuser = keepsSuperuser || role.IsSuperuserRole
			}
			if !keepsSuperuser {
				if err := repo.LockUserLifecycle(ctx); err != nil {
					return err
				}
				count, err := repo.CountOtherActiveSuperusers(ctx, targetUserID)
				if err != nil {
					return err
				}
				if count == 0 {
					return ErrLastSuperuser
				}
			}
		}
		if len(req.RoleIDs) > 0 {
			if err := repo.ReplaceUserRoles(ctx, targetUserID, req.RoleIDs); err != nil {
				return err
			}
		}
		if req.ProviderIdentifier != nil {
			if err := repo.SetProviderIdentifier(ctx, targetUserID, *req.ProviderIdentifier); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "user_updated", Result: audit.ResultSuccess,
		TargetResource: targetUserID, Metadata: map[string]any{"role_ids": req.RoleIDs, "status": req.Status},
	})

	return s.getProfile(ctx, targetUserID)
}

func (s *Service) SuspendUser(ctx context.Context, actorUserID, targetUserID, reason string) (*UserProfileResponse, error) {
	if actorUserID == targetUserID {
		return nil, ErrSelfDeactivation
	}
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if user.Status == StatusSuspended {
		return s.getProfile(ctx, targetUserID)
	}
	if user.Status == StatusInvited {
		return nil, ErrInvitationPending
	}
	if user.Status != StatusActive {
		return nil, ErrInvalidStatusTransition
	}
	targetIsSuperuser, err := s.repo.HasSuperuserRole(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if targetIsSuperuser {
		actorIsSuperuser, err := s.repo.HasSuperuserRole(ctx, actorUserID)
		if err != nil {
			return nil, err
		}
		if !actorIsSuperuser {
			return nil, ErrSuperuserRequired
		}
	}
	now := time.Now()
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if targetIsSuperuser {
			if err := repo.LockUserLifecycle(ctx); err != nil {
				return err
			}
			count, err := repo.CountOtherActiveSuperusers(ctx, targetUserID)
			if err != nil {
				return err
			}
			if count == 0 {
				return ErrLastSuperuser
			}
		}
		if err := repo.SuspendUser(ctx, targetUserID, actorUserID, reason, now); err != nil {
			return err
		}
		if err := revokeUserAccess(ctx, repo, targetUserID); err != nil {
			return err
		}
		return queueLifecycleNotification(ctx, repo, user, "suspended")
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{UserID: &actorUserID, TargetUserID: &targetUserID, Action: "user_suspended", Result: audit.ResultSuccess, ReasonCode: reason})
	return s.getProfile(ctx, targetUserID)
}

func (s *Service) RestoreUser(ctx context.Context, actorUserID, targetUserID, reason string) (*UserProfileResponse, error) {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if user.Status == StatusActive {
		return s.getProfile(ctx, targetUserID)
	}
	if user.Status != StatusSuspended {
		return nil, ErrInvalidStatusTransition
	}
	if err := s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.RestoreUser(ctx, targetUserID); err != nil {
			return err
		}
		return queueLifecycleNotification(ctx, repo, user, "restored")
	}); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{UserID: &actorUserID, TargetUserID: &targetUserID, Action: "user_restored", Result: audit.ResultSuccess, ReasonCode: reason})
	return s.getProfile(ctx, targetUserID)
}

// DeactivateUser permanently removes an activated staff member's access
// while retaining their identity and role history for record attribution.
func (s *Service) DeactivateUser(ctx context.Context, actorUserID, targetUserID, reason string) (*UserProfileResponse, error) {
	if actorUserID == targetUserID {
		_ = s.audit.Record(ctx, audit.Entry{UserID: &actorUserID, Action: "user_deactivated", Result: audit.ResultFailure, TargetResource: targetUserID, Metadata: map[string]any{"reason": reason, "error": ErrSelfDeactivation.Error()}})
		return nil, ErrSelfDeactivation
	}
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if user.Status == StatusInvited {
		_ = s.audit.Record(ctx, audit.Entry{UserID: &actorUserID, Action: "user_deactivated", Result: audit.ResultFailure, TargetResource: targetUserID, Metadata: map[string]any{"reason": reason, "error": ErrInvitationPending.Error()}})
		return nil, ErrInvitationPending
	}
	if user.Status == StatusDeactivated {
		_ = s.audit.Record(ctx, audit.Entry{UserID: &actorUserID, Action: "user_deactivated", Result: audit.ResultFailure, TargetResource: targetUserID, Metadata: map[string]any{"reason": reason, "error": ErrInvalidStatusTransition.Error()}})
		return nil, ErrInvalidStatusTransition
	}

	targetIsSuperuser, err := s.repo.HasSuperuserRole(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if targetIsSuperuser {
		actorIsSuperuser, err := s.repo.HasSuperuserRole(ctx, actorUserID)
		if err != nil {
			return nil, err
		}
		if !actorIsSuperuser {
			return nil, ErrSuperuserRequired
		}
	}
	now := time.Now()
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if targetIsSuperuser && user.Status == StatusActive {
			if err := repo.LockUserLifecycle(ctx); err != nil {
				return err
			}
			count, err := repo.CountOtherActiveSuperusers(ctx, targetUserID)
			if err != nil {
				return err
			}
			if count == 0 {
				return ErrLastSuperuser
			}
		}
		if err := repo.DeactivateUser(ctx, targetUserID, actorUserID, reason, now); err != nil {
			return err
		}
		if err := revokeUserAccess(ctx, repo, targetUserID); err != nil {
			return err
		}
		return queueLifecycleNotification(ctx, repo, user, "deactivated")
	})
	if err != nil {
		_ = s.audit.Record(ctx, audit.Entry{UserID: &actorUserID, Action: "user_deactivated", Result: audit.ResultFailure, TargetResource: targetUserID, Metadata: map[string]any{"reason": reason, "error": err.Error()}})
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "user_deactivated", Result: audit.ResultSuccess,
		TargetResource: targetUserID, Metadata: map[string]any{"reason": reason},
	})
	return s.getProfile(ctx, targetUserID)
}

// RecordAccessReview records a quarterly access-review decision. A
// revoke_access decision immediately suspends the account; the other
// decisions only update the review timestamps.
func (s *Service) RecordAccessReview(ctx context.Context, actorUserID, targetUserID string, req AccessReviewRequest) (*UserProfileResponse, error) {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if user.Status == StatusInvited {
		return nil, ErrInvitationPending
	}
	revoking := req.Decision == "revoke_access" && user.Status != StatusSuspended
	if revoking && actorUserID == targetUserID {
		return nil, ErrSelfDeactivation
	}
	targetIsSuperuser := false
	if revoking {
		targetIsSuperuser, err = s.repo.HasSuperuserRole(ctx, targetUserID)
		if err != nil {
			return nil, err
		}
		if targetIsSuperuser {
			actorIsSuperuser, err := s.repo.HasSuperuserRole(ctx, actorUserID)
			if err != nil {
				return nil, err
			}
			if !actorIsSuperuser {
				return nil, ErrSuperuserRequired
			}
		}
	}

	now := time.Now()
	nextDue := now.Add(s.cfg.AccessReviewCycle)

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if revoking && targetIsSuperuser && user.Status == StatusActive {
			if err := repo.LockUserLifecycle(ctx); err != nil {
				return err
			}
			count, err := repo.CountOtherActiveSuperusers(ctx, targetUserID)
			if err != nil {
				return err
			}
			if count == 0 {
				return ErrLastSuperuser
			}
		}
		if err := repo.RecordAccessReview(ctx, targetUserID, now, nextDue); err != nil {
			return err
		}
		if req.Decision == "revoke_access" {
			if err := repo.SuspendUser(ctx, targetUserID, actorUserID, req.Notes, now); err != nil {
				return err
			}
			if revoking {
				if err := revokeUserAccess(ctx, repo, targetUserID); err != nil {
					return err
				}
				return queueLifecycleNotification(ctx, repo, user, "suspended")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "access_review_recorded", Result: audit.ResultSuccess,
		TargetResource: targetUserID,
		Metadata:       map[string]any{"decision": req.Decision, "reviewed_by": req.ReviewedBy, "notes": req.Notes},
	})

	return s.getProfile(ctx, targetUserID)
}

func (s *Service) GetSecurityStatus(ctx context.Context, userID string) (*SecurityStatusResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	restrictions, err := s.repo.GetTemporaryRestrictionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	response := &SecurityStatusResponse{UserID: user.ID, Status: string(user.Status), SuspendedAt: user.SuspendedAt, SuspendedBy: user.SuspendedBy, SuspensionReason: user.SuspensionReason, DeactivatedAt: user.DeactivatedAt, DeactivatedBy: user.DeactivatedBy, DeactivationReason: user.DeactivationReason, TemporaryRestrictions: make([]TemporaryRestrictionResponse, 0, len(restrictions))}
	for _, restriction := range restrictions {
		response.TemporaryRestrictions = append(response.TemporaryRestrictions, TemporaryRestrictionResponse{Mechanism: restriction.Mechanism, FailureCount: restriction.FailureCount, NextAttemptAt: restriction.NextAttemptAt, LockedUntil: restriction.LockedUntil})
	}
	return response, nil
}

// UnlockUser clears the lockout counter/timestamp set by repeated failed
// logins. It does not touch the password or MFA factors — those remain the
// user's own unless separately reset.
func (s *Service) UnlockUser(ctx context.Context, actorUserID, targetUserID, reason string) error {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if !user.IsLocked(time.Now()) {
		return ErrNotLocked
	}
	if err := s.repo.UnlockUser(ctx, targetUserID); err != nil {
		return err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "account_unlocked", Result: audit.ResultSuccess,
		TargetResource: targetUserID, Metadata: map[string]any{"reason": reason},
	})
	return nil
}

func (s *Service) getProfile(ctx context.Context, userID string) (*UserProfileResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	resp := toUserProfileResponse(*user)
	return &resp, nil
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
