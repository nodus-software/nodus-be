package audit

import (
	"context"
	"time"

	"nodus-health/pkg/utility"
)

// Service records audit entries. Domains elsewhere depend on it through their
// own narrow "Recorder"-style interface (accept interfaces, return structs) —
// this package never imports any other domain.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Record assigns an ID/timestamp and persists the entry.
func (s *Service) Record(ctx context.Context, entry Entry) error {
	id, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	entry.ID = id
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	return s.repo.Insert(ctx, entry)
}
