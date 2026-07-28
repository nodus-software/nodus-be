package users

import (
	"context"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/pkg/logger"
)

// AuditRecorder is the narrow slice of the audit domain this service depends
// on, defined here (the consumer) rather than in the audit package.
type AuditRecorder interface {
	Record(ctx context.Context, entry audit.Entry) error
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

// UpdateUser applies a role and/or status change. Assigning a superuser
// role requires the actor themselves be a superuser (Reg. 12(b)(iv)), and
// assigning any role that requires a provider identifier requires the user
// end up with one set (Reg. 12(b)(v)).
func (s *Service) UpdateUser(ctx context.Context, actorUserID, targetUserID string, req UpdateUserRequest) (*UserProfileResponse, error) {
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if user.Status == StatusInvited && req.Status != nil {
		return nil, ErrInvitationPending
	}

	if len(req.RoleIDs) > 0 {
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

		providerIdentifier := user.ProviderIdentifier
		if req.ProviderIdentifier != nil {
			providerIdentifier = req.ProviderIdentifier
		}
		if requiresProviderIdentifier && (providerIdentifier == nil || *providerIdentifier == "") {
			return nil, ErrProviderIdentifierRequired
		}
	}

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
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
		if req.Status != nil {
			if err := repo.UpdateUserStatus(ctx, targetUserID, *req.Status); err != nil {
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

	now := time.Now()
	nextDue := now.Add(s.cfg.AccessReviewCycle)

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.RecordAccessReview(ctx, targetUserID, now, nextDue); err != nil {
			return err
		}
		if req.Decision == "revoke_access" {
			return repo.UpdateUserStatus(ctx, targetUserID, string(StatusSuspended))
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
