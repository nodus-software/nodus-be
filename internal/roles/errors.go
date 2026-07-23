package roles

import "errors"

var (
	ErrRoleNotFound       = errors.New("role not found")
	ErrRoleNameTaken      = errors.New("a role with this name already exists")
	ErrUnknownPermissions = errors.New("one or more permission codes do not exist")
	ErrSuperuserRequired  = errors.New("only a superuser may perform this action")
	ErrPermissionDenied   = errors.New("permission denied")
)
