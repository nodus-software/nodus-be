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

type resourceRepo struct {
	Repository
	created     *Resource
	impact      *DeactivationImpact
	deactivated bool
	cascade     bool
}

func (r *resourceRepo) CreateResource(_ context.Context, _ string, resource Resource) (*Resource, error) {
	r.created = &resource
	return &resource, nil
}

func (r *resourceRepo) DeactivationImpact(context.Context, string, string) (*DeactivationImpact, error) {
	return r.impact, nil
}

func (r *resourceRepo) DeactivateResource(_ context.Context, _ string, _ string, cascade bool) (*ResourceLifecycleResult, error) {
	r.deactivated = true
	r.cascade = cascade
	return &ResourceLifecycleResult{Root: r.impact.Root}, nil
}

func TestCreateBedRequiresRoom(t *testing.T) {
	r := &resourceRepo{}
	s := NewService(r, noopAudit{})

	_, err := s.CreateResource(context.Background(), "actor", "beds", CreateResourceRequest{Code: "B-1", Name: "Bed 1"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected room requirement validation, got %v", err)
	}
	if r.created != nil {
		t.Fatal("repository should not be called without a room")
	}
}

func TestCreateBedPassesRoomToRepository(t *testing.T) {
	r := &resourceRepo{}
	s := NewService(r, noopAudit{})
	roomID := "room-1"

	_, err := s.CreateResource(context.Background(), "actor", "beds", CreateResourceRequest{Code: "B-1", Name: "Bed 1", RoomID: &roomID})
	if err != nil {
		t.Fatalf("create bed: %v", err)
	}
	if r.created == nil || r.created.RoomID == nil || *r.created.RoomID != roomID {
		t.Fatalf("expected room %q to reach repository, got %#v", roomID, r.created)
	}
}

func TestPrescribingVocabulariesAreResourceKinds(t *testing.T) {
	for _, kind := range []string{DosageFormKind, RouteKind, UnitOfMeasureKind, PrescriptionFrequencyKind, SpecimenTypeKind} {
		r := &resourceRepo{}
		s := NewService(r, noopAudit{})
		if _, err := s.CreateResource(context.Background(), "actor", kind, CreateResourceRequest{Code: "tablet", Name: "Tablet"}); err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		// Vocabularies are flat lists, so nothing parent-shaped should be set.
		if r.created == nil || r.created.DepartmentID != nil || r.created.WardID != nil {
			t.Fatalf("expected a parentless %s entry, got %#v", kind, r.created)
		}
	}
}

func TestVocabularyCodesAreCanonicalLowercase(t *testing.T) {
	r := &resourceRepo{}
	s := NewService(r, noopAudit{})
	_, err := s.CreateResource(context.Background(), "actor", PrescriptionFrequencyKind, CreateResourceRequest{Code: " BID ", Name: " Twice daily "})
	if err != nil {
		t.Fatal(err)
	}
	if r.created.Code != "BID" || r.created.Name != "Twice daily" {
		t.Fatalf("unexpected normalized resource: %#v", r.created)
	}
}

func TestDeactivateRequiresReason(t *testing.T) {
	r := &resourceRepo{}
	s := NewService(r, noopAudit{})
	_, err := s.DeactivateResource(context.Background(), "actor", "wards", "ward", DeactivateResourceRequest{})
	if !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("expected reason requirement, got %v", err)
	}
}

func TestDeactivateBlocksActiveChildrenWithoutCascade(t *testing.T) {
	r := &resourceRepo{impact: &DeactivationImpact{
		Root:              ResourceReference{Kind: "wards", ID: "ward", Name: "Ward"},
		ActiveDescendants: []ResourceReference{{Kind: "rooms", ID: "room", Name: "Room"}},
	}}
	s := NewService(r, noopAudit{})
	_, err := s.DeactivateResource(context.Background(), "actor", "wards", "ward", DeactivateResourceRequest{Reason: "Closing", Cascade: false})
	if !errors.Is(err, ErrActiveDescendants) {
		t.Fatalf("expected active descendant conflict, got %v", err)
	}
	if r.deactivated {
		t.Fatal("repository must not deactivate without explicit cascade")
	}
}

func TestDeactivateExplicitCascade(t *testing.T) {
	r := &resourceRepo{impact: &DeactivationImpact{
		Root:              ResourceReference{Kind: "wards", ID: "ward", Name: "Ward"},
		ActiveDescendants: []ResourceReference{{Kind: "rooms", ID: "room", Name: "Room"}},
	}}
	s := NewService(r, noopAudit{})
	_, err := s.DeactivateResource(context.Background(), "actor", "wards", "ward", DeactivateResourceRequest{Reason: "Closing", Cascade: true})
	if err != nil {
		t.Fatalf("deactivate hierarchy: %v", err)
	}
	if !r.deactivated || !r.cascade {
		t.Fatal("expected explicit cascade to reach repository")
	}
}

func TestDeactivateBlocksOperationalUseEvenWithCascade(t *testing.T) {
	r := &resourceRepo{impact: &DeactivationImpact{
		Root:                ResourceReference{Kind: "rooms", ID: "room", Name: "Room"},
		OperationalBlockers: []OperationalBlocker{{Type: "bed_occupied", Count: 1}},
	}}
	s := NewService(r, noopAudit{})
	_, err := s.DeactivateResource(context.Background(), "actor", "rooms", "room", DeactivateResourceRequest{Reason: "Closing", Cascade: true})
	if !errors.Is(err, ErrOperationalUse) {
		t.Fatalf("expected operational blocker, got %v", err)
	}
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
