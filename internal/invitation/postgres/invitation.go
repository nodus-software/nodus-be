package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/invitation"
	"nodus-health/internal/invitation/postgres/sqlcgen"
)

func pendingUserFromRow(u sqlcgen.User) *invitation.PendingUser {
	return &invitation.PendingUser{
		ID: u.ID, FullName: u.FullName, Email: u.Email, ProviderIdentifier: u.ProviderIdentifier,
		Status: invitation.UserStatus(u.Status), PasswordSet: u.PasswordHash != nil,
	}
}

func invitationFromRow(i sqlcgen.Invitation) *invitation.Invitation {
	return &invitation.Invitation{
		ID: i.ID, UserID: i.UserID, InvitedBy: i.InvitedBy, TokenHash: i.TokenHash,
		ExpiresAt: fromTimestamptz(i.ExpiresAt), UsedAt: fromNullTimestamptz(i.UsedAt),
		CreatedAt: fromTimestamptz(i.CreatedAt),
	}
}

func reactivationTokenFromRow(t sqlcgen.ReactivationToken) *invitation.ReactivationToken {
	return &invitation.ReactivationToken{
		ID: t.ID, UserID: t.UserID, RequestedBy: t.RequestedBy, TokenHash: t.TokenHash,
		ExpiresAt: fromTimestamptz(t.ExpiresAt), UsedAt: fromNullTimestamptz(t.UsedAt),
		CreatedAt: fromTimestamptz(t.CreatedAt),
	}
}

func (r *Repository) GetRolesByIDs(ctx context.Context, ids []string) ([]invitation.Role, error) {
	rows, err := r.q(ctx).GetRolesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]invitation.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, invitation.Role{
			ID: row.ID, Name: row.Name, RequiresProviderIdentifier: row.RequiresProviderIdentifier,
		})
	}
	return out, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, tenantID, email string) (*invitation.PendingUser, error) {
	u, err := r.q(ctx).GetUserByEmail(ctx, sqlcgen.GetUserByEmailParams{TenantID: tenantID, Email: email})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrUserNotFound
		}
		return nil, err
	}
	return pendingUserFromRow(u), nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*invitation.PendingUser, error) {
	u, err := r.q(ctx).GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrUserNotFound
		}
		return nil, err
	}
	return pendingUserFromRow(u), nil
}

func (r *Repository) CreateInvitedUser(ctx context.Context, params invitation.CreateInvitedUserParams) error {
	_, err := r.q(ctx).CreateInvitedUser(ctx, sqlcgen.CreateInvitedUserParams{
		ID: params.ID, FullName: params.FullName, Username: params.Username,
		Email: params.Email, ProviderIdentifier: params.ProviderIdentifier,
	})
	return err
}

func (r *Repository) AssignUserRole(ctx context.Context, userID, roleID string) error {
	return r.q(ctx).AssignUserRole(ctx, sqlcgen.AssignUserRoleParams{UserID: userID, RoleID: roleID})
}

func (r *Repository) GetUserRoleNames(ctx context.Context, userID string) ([]string, error) {
	names, err := r.q(ctx).GetUserRoleNames(ctx, userID)
	if err != nil {
		return nil, err
	}
	if names == nil {
		names = []string{}
	}
	return names, nil
}

func (r *Repository) CreateInvitation(ctx context.Context, inv invitation.Invitation) error {
	return r.q(ctx).CreateInvitation(ctx, sqlcgen.CreateInvitationParams{
		ID: inv.ID, UserID: inv.UserID, InvitedBy: inv.InvitedBy,
		TokenHash: inv.TokenHash, ExpiresAt: toTimestamptz(inv.ExpiresAt),
	})
}

func (r *Repository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*invitation.Invitation, error) {
	row, err := r.q(ctx).GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrTokenInvalid
		}
		return nil, err
	}
	return invitationFromRow(row), nil
}

func (r *Repository) GetLatestInvitationByUserID(ctx context.Context, userID string) (*invitation.Invitation, error) {
	row, err := r.q(ctx).GetLatestInvitationByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrTokenInvalid
		}
		return nil, err
	}
	return invitationFromRow(row), nil
}

func (r *Repository) ConsumeInvitation(ctx context.Context, id string) error {
	return r.q(ctx).ConsumeInvitation(ctx, id)
}

func (r *Repository) ActivateUserWithPassword(ctx context.Context, userID, passwordHash string) error {
	return r.q(ctx).ActivateUserWithPassword(ctx, sqlcgen.ActivateUserWithPasswordParams{
		ID: userID, PasswordHash: &passwordHash,
	})
}

func (r *Repository) RestoreInvitedUser(ctx context.Context, userID string) error {
	return r.q(ctx).RestoreInvitedUser(ctx, userID)
}

func (r *Repository) CreateEnrollmentToken(ctx context.Context, token invitation.EnrollmentToken) error {
	return r.q(ctx).CreateEnrollmentToken(ctx, sqlcgen.CreateEnrollmentTokenParams{
		ID: token.ID, UserID: token.UserID, TokenHash: token.TokenHash, ExpiresAt: toTimestamptz(token.ExpiresAt),
	})
}

func (r *Repository) CreateReactivationToken(ctx context.Context, token invitation.ReactivationToken) error {
	return r.q(ctx).CreateReactivationToken(ctx, sqlcgen.CreateReactivationTokenParams{
		ID: token.ID, UserID: token.UserID, RequestedBy: token.RequestedBy,
		TokenHash: token.TokenHash, ExpiresAt: toTimestamptz(token.ExpiresAt),
	})
}

func (r *Repository) GetReactivationTokenByHash(ctx context.Context, tokenHash string) (*invitation.ReactivationToken, error) {
	row, err := r.q(ctx).GetReactivationTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrReactivationTokenInvalid
		}
		return nil, err
	}
	return reactivationTokenFromRow(row), nil
}

func (r *Repository) ConsumeReactivationToken(ctx context.Context, id string) error {
	rows, err := r.q(ctx).ConsumeReactivationToken(ctx, id)
	if err != nil {
		return err
	}
	if rows != 1 {
		return invitation.ErrReactivationTokenInvalid
	}
	return nil
}

func (r *Repository) ConsumeReactivationTokensByUser(ctx context.Context, userID string) error {
	return r.q(ctx).ConsumeReactivationTokensByUser(ctx, userID)
}

func (r *Repository) ResetMFAByUser(ctx context.Context, userID string) error {
	return r.q(ctx).ResetMFAByUser(ctx, userID)
}

func (r *Repository) ResetMFABackupCodesByUser(ctx context.Context, userID string) error {
	return r.q(ctx).ResetMFABackupCodesByUser(ctx, userID)
}

func (r *Repository) ActivateReactivatedUser(ctx context.Context, userID, passwordHash string, reviewedAt, nextReview time.Time) error {
	return r.q(ctx).ActivateReactivatedUser(ctx, sqlcgen.ActivateReactivatedUserParams{
		ID: userID, PasswordHash: &passwordHash, LastAccessReviewAt: toTimestamptz(reviewedAt),
		NextAccessReviewDue: toTimestamptz(nextReview),
	})
}

func (r *Repository) DeletePendingUser(ctx context.Context, userID string) error {
	return r.q(ctx).DeletePendingUser(ctx, userID)
}
