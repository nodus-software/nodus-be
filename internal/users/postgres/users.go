package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/users"
	"nodus-health/internal/users/postgres/sqlcgen"
)

func (r *Repository) ListUsers(ctx context.Context, filter users.ListUsersFilter) ([]users.User, error) {
	var status *sqlcgen.UserStatus
	if filter.Status != nil {
		s := sqlcgen.UserStatus(*filter.Status)
		status = &s
	}
	rows, err := r.q(ctx).ListUsers(ctx, sqlcgen.ListUsersParams{
		Role: filter.Role, Status: status, StaleAccess: filter.StaleAccess,
	})
	if err != nil {
		return nil, err
	}
	out := make([]users.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, users.User{
			ID: row.ID, TenantID: row.TenantID, FullName: row.FullName, Username: row.Username, Email: row.Email,
			ProviderIdentifier: row.ProviderIdentifier, Status: users.Status(row.Status),
			LockedUntil:         fromNullTimestamptz(row.LockedUntil),
			LastAccessReviewAt:  fromNullTimestamptz(row.LastAccessReviewAt),
			NextAccessReviewDue: fromNullTimestamptz(row.NextAccessReviewDue),
			DeactivatedAt:       fromNullTimestamptz(row.DeactivatedAt),
			DeactivatedBy:       row.DeactivatedBy,
			DeactivationReason:  row.DeactivationReason,
			SuspendedAt:         fromNullTimestamptz(row.SuspendedAt),
			SuspendedBy:         row.SuspendedBy,
			SuspensionReason:    row.SuspensionReason,
			InvitationExpiresAt: fromNullTimestamptz(row.InvitationExpiresAt),
			InvitationUsedAt:    fromNullTimestamptz(row.InvitationUsedAt),
			MFAEnrolled:         row.MfaEnrolled,
			RoleNames:           row.RoleNames,
			Permissions:         row.PermissionCodes,
		})
	}
	return out, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*users.User, error) {
	row, err := r.q(ctx).GetUserWithRolesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, users.ErrUserNotFound
		}
		return nil, err
	}
	return &users.User{
		ID: row.ID, TenantID: row.TenantID, FullName: row.FullName, Username: row.Username, Email: row.Email,
		ProviderIdentifier: row.ProviderIdentifier, Status: users.Status(row.Status),
		LockedUntil:         fromNullTimestamptz(row.LockedUntil),
		LastAccessReviewAt:  fromNullTimestamptz(row.LastAccessReviewAt),
		NextAccessReviewDue: fromNullTimestamptz(row.NextAccessReviewDue),
		DeactivatedAt:       fromNullTimestamptz(row.DeactivatedAt),
		DeactivatedBy:       row.DeactivatedBy,
		DeactivationReason:  row.DeactivationReason,
		SuspendedAt:         fromNullTimestamptz(row.SuspendedAt),
		SuspendedBy:         row.SuspendedBy,
		SuspensionReason:    row.SuspensionReason,
		InvitationExpiresAt: fromNullTimestamptz(row.InvitationExpiresAt),
		InvitationUsedAt:    fromNullTimestamptz(row.InvitationUsedAt),
		MFAEnrolled:         row.MfaEnrolled,
		RoleNames:           row.RoleNames,
		Permissions:         row.PermissionCodes,
	}, nil
}

func (r *Repository) ReplaceUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	if err := r.q(ctx).DeleteUserRoles(ctx, userID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := r.q(ctx).InsertUserRole(ctx, sqlcgen.InsertUserRoleParams{UserID: userID, RoleID: roleID}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) SuspendUser(ctx context.Context, userID, actorUserID, reason string, suspendedAt time.Time) error {
	return r.q(ctx).SuspendUser(ctx, sqlcgen.SuspendUserParams{ID: userID, SuspendedAt: toTimestamptz(suspendedAt), SuspendedBy: &actorUserID, SuspensionReason: &reason})
}
func (r *Repository) RestoreUser(ctx context.Context, userID string) error {
	return r.q(ctx).RestoreUser(ctx, userID)
}

func (r *Repository) SetProviderIdentifier(ctx context.Context, userID, identifier string) error {
	return r.q(ctx).SetProviderIdentifier(ctx, sqlcgen.SetProviderIdentifierParams{ID: userID, ProviderIdentifier: &identifier})
}

func (r *Repository) RecordAccessReview(ctx context.Context, userID string, reviewedAt, nextDue time.Time) error {
	return r.q(ctx).RecordAccessReview(ctx, sqlcgen.RecordAccessReviewParams{
		ID: userID, LastAccessReviewAt: toTimestamptz(reviewedAt), NextAccessReviewDue: toTimestamptz(nextDue),
	})
}

func (r *Repository) UnlockUser(ctx context.Context, userID string) error {
	return r.q(ctx).UnlockUser(ctx, userID)
}

func (r *Repository) GetRolesByIDs(ctx context.Context, ids []string) ([]users.Role, error) {
	rows, err := r.q(ctx).GetRolesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]users.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, users.Role{
			ID: row.ID, Name: row.Name, IsSuperuserRole: row.IsSuperuserRole,
			RequiresProviderIdentifier: row.RequiresProviderIdentifier,
		})
	}
	return out, nil
}

func (r *Repository) HasSuperuserRole(ctx context.Context, userID string) (bool, error) {
	return r.q(ctx).HasSuperuserRole(ctx, userID)
}

func (r *Repository) CountOtherActiveSuperusers(ctx context.Context, userID string) (int64, error) {
	return r.q(ctx).CountOtherActiveSuperusers(ctx, userID)
}

func (r *Repository) LockUserLifecycle(ctx context.Context) error {
	return r.q(ctx).LockUserLifecycle(ctx)
}

func (r *Repository) DeactivateUser(ctx context.Context, userID, actorUserID, reason string, deactivatedAt time.Time) error {
	return r.q(ctx).DeactivateUser(ctx, sqlcgen.DeactivateUserParams{ID: userID, DeactivatedAt: toTimestamptz(deactivatedAt), DeactivatedBy: &actorUserID, DeactivationReason: &reason})
}

func (r *Repository) RevokeSessionsByUser(ctx context.Context, userID string) error {
	return r.q(ctx).RevokeSessionsByUser(ctx, userID)
}

func (r *Repository) RevokeRefreshTokensByUser(ctx context.Context, userID string) error {
	return r.q(ctx).RevokeRefreshTokensByUser(ctx, userID)
}

func (r *Repository) ConsumeLoginChallengesByUser(ctx context.Context, userID string) error {
	return r.q(ctx).ConsumeLoginChallengesByUser(ctx, userID)
}

func (r *Repository) ConsumePasswordResetTokensByUser(ctx context.Context, userID string) error {
	return r.q(ctx).ConsumePasswordResetTokensByUser(ctx, userID)
}

func (r *Repository) ConsumeEnrollmentTokensByUser(ctx context.Context, userID string) error {
	return r.q(ctx).ConsumeEnrollmentTokensByUser(ctx, userID)
}

func (r *Repository) ConsumeRecoverySessionsByUser(ctx context.Context, userID string) error {
	return r.q(ctx).ConsumeRecoverySessionsByUser(ctx, userID)
}
func (r *Repository) GetTemporaryRestrictionsByUser(ctx context.Context, userID string) ([]users.TemporaryRestriction, error) {
	rows, err := r.q(ctx).GetTemporaryRestrictionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]users.TemporaryRestriction, 0, len(rows))
	for _, row := range rows {
		out = append(out, users.TemporaryRestriction{Mechanism: row.Mechanism, FailureCount: int(row.FailureCount), NextAttemptAt: fromNullTimestamptz(row.NextAttemptAt), LockedUntil: fromNullTimestamptz(row.LockedUntil)})
	}
	return out, nil
}
