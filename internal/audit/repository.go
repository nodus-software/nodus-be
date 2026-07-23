package audit

import "context"

// Repository persists audit entries. Append-only: there is intentionally no
// Update or Delete method.
type Repository interface {
	Insert(ctx context.Context, entry Entry) error
}
