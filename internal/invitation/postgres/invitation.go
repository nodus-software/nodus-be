package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/invitation"
	"nodus-health/internal/invitation/postgres/sqlcgen"
)

func pendingUserFromRow(u sqlcgen.User) *invitation.PendingUser {
	return &invitation.PendingUser{
		ID: u.ID, FullName: u.FullName, Email: u.Email, Status: invitation.UserStatus(u.Status),
	}
}

func invitationFromRow(i sqlcgen.Invitation) *invitation.Invitation {
	return &invitation.Invitation{
		ID: i.ID, UserID: i.UserID, InvitedBy: i.InvitedBy, TokenHash: i.TokenHash,
		ExpiresAt: fromTimestamptz(i.ExpiresAt), UsedAt: fromNullTimestamptz(i.UsedAt),
		CreatedAt: fromTimestamptz(i.CreatedAt),
	}
}

func (r *Repository) GetRolesByIDs(ctx context.Context, ids []string) ([]invitation.Role, error) {
	rows, err := r.queries.GetRolesByIDs(ctx, ids)
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

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*invitation.PendingUser, error) {
	u, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrUserNotFound
		}
		return nil, err
	}
	return pendingUserFromRow(u), nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*invitation.PendingUser, error) {
	u, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrUserNotFound
		}
		return nil, err
	}
	return pendingUserFromRow(u), nil
}

func (r *Repository) CreateInvitedUser(ctx context.Context, params invitation.CreateInvitedUserParams) error {
	_, err := r.queries.CreateInvitedUser(ctx, sqlcgen.CreateInvitedUserParams{
		ID: params.ID, FullName: params.FullName, Username: params.Username,
		Email: params.Email, ProviderIdentifier: params.ProviderIdentifier,
	})
	return err
}

func (r *Repository) AssignUserRole(ctx context.Context, userID, roleID string) error {
	return r.queries.AssignUserRole(ctx, sqlcgen.AssignUserRoleParams{UserID: userID, RoleID: roleID})
}

func (r *Repository) GetUserRoleNames(ctx context.Context, userID string) ([]string, error) {
	names, err := r.queries.GetUserRoleNames(ctx, userID)
	if err != nil {
		return nil, err
	}
	if names == nil {
		names = []string{}
	}
	return names, nil
}

func (r *Repository) CreateInvitation(ctx context.Context, inv invitation.Invitation) error {
	return r.queries.CreateInvitation(ctx, sqlcgen.CreateInvitationParams{
		ID: inv.ID, UserID: inv.UserID, InvitedBy: inv.InvitedBy,
		TokenHash: inv.TokenHash, ExpiresAt: toTimestamptz(inv.ExpiresAt),
	})
}

func (r *Repository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*invitation.Invitation, error) {
	row, err := r.queries.GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrTokenInvalid
		}
		return nil, err
	}
	return invitationFromRow(row), nil
}

func (r *Repository) GetLatestInvitationByUserID(ctx context.Context, userID string) (*invitation.Invitation, error) {
	row, err := r.queries.GetLatestInvitationByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrTokenInvalid
		}
		return nil, err
	}
	return invitationFromRow(row), nil
}

func (r *Repository) ConsumeInvitation(ctx context.Context, id string) error {
	return r.queries.ConsumeInvitation(ctx, id)
}

func (r *Repository) ActivateUserWithPassword(ctx context.Context, userID, passwordHash string) error {
	return r.queries.ActivateUserWithPassword(ctx, sqlcgen.ActivateUserWithPasswordParams{
		ID: userID, PasswordHash: &passwordHash,
	})
}

func (r *Repository) CreateEnrollmentToken(ctx context.Context, token invitation.EnrollmentToken) error {
	return r.queries.CreateEnrollmentToken(ctx, sqlcgen.CreateEnrollmentTokenParams{
		ID: token.ID, UserID: token.UserID, TokenHash: token.TokenHash, ExpiresAt: toTimestamptz(token.ExpiresAt),
	})
}
