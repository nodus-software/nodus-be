package postgres

import (
	"context"

	"nodus-health/internal/auth"
)

func (r *Repository) GetRolesByUser(ctx context.Context, userID string) ([]auth.Role, error) {
	rows, err := r.q(ctx).GetRolesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles := make([]auth.Role, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, auth.Role{
			ID: row.ID, Name: row.Name, Description: row.Description,
			IsSuperuserRole: row.IsSuperuserRole, RequiresProviderIdentifier: row.RequiresProviderIdentifier,
		})
	}
	return roles, nil
}

func (r *Repository) GetEffectivePermissionsByUser(ctx context.Context, userID string) ([]string, error) {
	perms, err := r.q(ctx).GetEffectivePermissionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if perms == nil {
		perms = []string{}
	}
	return perms, nil
}
