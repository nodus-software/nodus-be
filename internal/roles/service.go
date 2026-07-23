package roles

import (
	"context"

	"nodus-health/internal/audit"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/utility"
)

// AuditRecorder is the narrow slice of the audit domain this service depends
// on, defined here (the consumer) rather than in the audit package.
type AuditRecorder interface {
	Record(ctx context.Context, entry audit.Entry) error
}

type Service struct {
	repo  Repository
	audit AuditRecorder
	log   *logger.Logger
}

func NewService(repo Repository, audit AuditRecorder, log *logger.Logger) *Service {
	return &Service{repo: repo, audit: audit, log: log}
}

func (s *Service) ListRoles(ctx context.Context) ([]RoleResponse, error) {
	roles, err := s.repo.ListRolesWithPermissions(ctx)
	if err != nil {
		return nil, err
	}
	resp := make([]RoleResponse, 0, len(roles))
	for _, r := range roles {
		resp = append(resp, toRoleResponse(r))
	}
	return resp, nil
}

// CreateRole creates a new role and its permission set. Restricted to
// superusers per Reg. 12(b)(iv) — least-privilege role management must
// itself be a privileged operation, not something any admin permission
// grants.
func (s *Service) CreateRole(ctx context.Context, actorUserID string, req CreateRoleRequest) (*RoleResponse, error) {
	isSuperuser, err := s.repo.HasSuperuserRole(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !isSuperuser {
		return nil, ErrSuperuserRequired
	}

	permissions, err := s.repo.GetPermissionsByCodes(ctx, req.Permissions)
	if err != nil {
		return nil, err
	}
	if len(permissions) != len(dedupe(req.Permissions)) {
		return nil, ErrUnknownPermissions
	}

	roleID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}

	var created *Role
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		role, err := repo.CreateRole(ctx, Role{
			ID: roleID, Name: req.Name, Description: req.Description,
			IsSuperuserRole: req.IsSuperuserRole, RequiresProviderIdentifier: req.RequiresProviderIdentifier,
		})
		if err != nil {
			return err
		}
		for _, p := range permissions {
			if err := repo.AddRolePermission(ctx, role.ID, p.ID); err != nil {
				return err
			}
		}
		role.Permissions = req.Permissions
		created = role
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "role_created", Result: audit.ResultSuccess,
		TargetResource: created.ID, Metadata: map[string]any{"name": created.Name},
	})

	resp := toRoleResponse(*created)
	return &resp, nil
}

func dedupe(codes []string) []string {
	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}
