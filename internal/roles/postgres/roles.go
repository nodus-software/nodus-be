package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nodus-health/internal/roles"
	"nodus-health/internal/roles/postgres/sqlcgen"
)

const uniqueViolation = "23505"

func (r *Repository) ListRolesWithPermissions(ctx context.Context) ([]roles.Role, error) {
	rows, err := r.queries.ListRolesWithPermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]roles.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, roles.Role{
			ID: row.ID, Name: row.Name, Description: row.Description,
			IsSuperuserRole: row.IsSuperuserRole, RequiresProviderIdentifier: row.RequiresProviderIdentifier,
			Permissions: row.PermissionCodes,
		})
	}
	return out, nil
}

func (r *Repository) GetRoleByID(ctx context.Context, id string) (*roles.Role, error) {
	row, err := r.queries.GetRoleByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, roles.ErrRoleNotFound
		}
		return nil, err
	}
	return &roles.Role{
		ID: row.ID, Name: row.Name, Description: row.Description,
		IsSuperuserRole: row.IsSuperuserRole, RequiresProviderIdentifier: row.RequiresProviderIdentifier,
	}, nil
}

func (r *Repository) CreateRole(ctx context.Context, role roles.Role) (*roles.Role, error) {
	row, err := r.queries.CreateRole(ctx, sqlcgen.CreateRoleParams{
		ID: role.ID, Name: role.Name, Description: role.Description,
		IsSuperuserRole: role.IsSuperuserRole, RequiresProviderIdentifier: role.RequiresProviderIdentifier,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, roles.ErrRoleNameTaken
		}
		return nil, err
	}
	return &roles.Role{
		ID: row.ID, Name: row.Name, Description: row.Description,
		IsSuperuserRole: row.IsSuperuserRole, RequiresProviderIdentifier: row.RequiresProviderIdentifier,
	}, nil
}

func (r *Repository) GetPermissionsByCodes(ctx context.Context, codes []string) ([]roles.Permission, error) {
	rows, err := r.queries.GetPermissionsByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	out := make([]roles.Permission, 0, len(rows))
	for _, row := range rows {
		out = append(out, roles.Permission{ID: row.ID, Code: row.Code})
	}
	return out, nil
}

func (r *Repository) AddRolePermission(ctx context.Context, roleID, permissionID string) error {
	return r.queries.AddRolePermission(ctx, sqlcgen.AddRolePermissionParams{RoleID: roleID, PermissionID: permissionID})
}

func (r *Repository) HasSuperuserRole(ctx context.Context, userID string) (bool, error) {
	return r.queries.HasSuperuserRole(ctx, userID)
}
