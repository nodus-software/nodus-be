package users

import "errors"

var (
	ErrUserNotFound               = errors.New("user not found")
	ErrRoleNotFound               = errors.New("one or more roles not found")
	ErrNotLocked                  = errors.New("account is not currently locked")
	ErrProviderIdentifierRequired = errors.New("provider_identifier is required for the selected role(s)")
	ErrSuperuserRequired          = errors.New("only a superuser may assign a superuser role")
	ErrPermissionDenied           = errors.New("permission denied")
)
