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

func (r *Repository) UpdateUserStatus(ctx context.Context, userID, status string) error {
	return r.q(ctx).UpdateUserStatus(ctx, sqlcgen.UpdateUserStatusParams{ID: userID, Status: sqlcgen.UserStatus(status)})
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
