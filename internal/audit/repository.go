package audit

import (
	"context"
	"time"
)

// Repository persists audit entries. Append-only: there is intentionally no
// Update or Delete method.
type Repository interface {
	Insert(ctx context.Context, entry Entry) error
	List(ctx context.Context, filter Filter, limit int) ([]Entry, error)
}

// Filter is the set of optional query parameters GET /audit-logs accepts. A
// nil field means "no filter on this dimension".
type Filter struct {
	UserID *string
	Action *string
	From   *time.Time
	To     *time.Time
}
