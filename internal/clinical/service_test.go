package clinical

import (
	"context"
	"errors"
	"testing"
)

type transitionRepo struct {
	Repository
	entry  QueueEntry
	called bool
}

func (r *transitionRepo) GetQueueEntry(context.Context, string) (*QueueEntry, error) {
	x := r.entry
	return &x, nil
}
func (r *transitionRepo) TransitionQueueEntry(_ context.Context, x QueueEntry, status, target string, _ *string, _ *string, _ bool) (*QueueEntry, error) {
	r.called = true
	x.Status = status
	x.QueueID = target
	return &x, nil
}

func TestTransitionRejectsInvalidStateChange(t *testing.T) {
	r := &transitionRepo{entry: QueueEntry{ID: "entry", QueueID: "queue", Status: "waiting"}}
	s := NewService(r, nil)
	_, err := s.Transition(context.Background(), "actor", "entry", TransitionRequest{Status: "completed"})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
	if r.called {
		t.Fatal("repository should not be called for an invalid transition")
	}
}

func TestTransferRequiresReason(t *testing.T) {
	r := &transitionRepo{entry: QueueEntry{ID: "entry", QueueID: "one", Status: "waiting"}}
	s := NewService(r, nil)
	to := "two"
	_, err := s.Transition(context.Background(), "actor", "entry", TransitionRequest{Status: "transferred", QueueID: &to})
	if !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("expected reason required, got %v", err)
	}
}
