package roles

type RoleResponse struct {
	ID                         string   `json:"id"`
	Name                       string   `json:"name"`
	Description                string   `json:"description"`
	IsSuperuserRole            bool     `json:"is_superuser_role"`
	RequiresProviderIdentifier bool     `json:"requires_provider_identifier"`
	Permissions                []string `json:"permissions"`
}

type CreateRoleRequest struct {
	Name                       string   `json:"name" validate:"required"`
	Description                string   `json:"description"`
	IsSuperuserRole            bool     `json:"is_superuser_role"`
	RequiresProviderIdentifier bool     `json:"requires_provider_identifier"`
	Permissions                []string `json:"permissions" validate:"required,min=1,dive,required"`
}

func toRoleResponse(r Role) RoleResponse {
	return RoleResponse{
		ID: r.ID, Name: r.Name, Description: r.Description,
		IsSuperuserRole: r.IsSuperuserRole, RequiresProviderIdentifier: r.RequiresProviderIdentifier,
		Permissions: r.Permissions,
	}
}
