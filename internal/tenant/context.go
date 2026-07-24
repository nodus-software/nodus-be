package tenant

import (
	"context"
	"errors"
)

var ErrMissing = errors.New("tenant is not resolved")

type contextKey struct{}

type Identity struct {
	ID   string
	Slug string
}

func WithContext(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok && identity.ID != ""
}

func ID(ctx context.Context) (string, error) {
	identity, ok := FromContext(ctx)
	if !ok {
		return "", ErrMissing
	}
	return identity.ID, nil
}
