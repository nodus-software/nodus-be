package clinical

import "errors"

var (
	ErrNotFound          = errors.New("clinical resource not found")
	ErrInvalidInput      = errors.New("invalid clinical request")
	ErrInvalidTransition = errors.New("invalid queue transition")
	ErrReasonRequired    = errors.New("a reason is required for this action")
	ErrConflict          = errors.New("the clinical operation conflicts with current state")
	ErrActiveVisit       = errors.New("patient already has an active outpatient visit")
	ErrVisitIncomplete   = errors.New("outpatient visit has not completed consultation")
	ErrFormIncomplete    = errors.New("encounter form has not been submitted")
	ErrActiveDescendants = errors.New("active child configurations must be deactivated first")
	ErrOperationalUse    = errors.New("configuration is currently in operational use")
	ErrInactiveParent    = errors.New("the parent configuration must be active first")
	ErrRoutingMissing    = errors.New("no active routing rule matches this workflow event")
)

type LifecycleConflictError struct {
	Cause  error
	Impact *DeactivationImpact
}

type ActiveVisitConflictError struct {
	Visit      Visit       `json:"active_visit"`
	QueueEntry *QueueEntry `json:"queue_entry,omitempty"`
}

func (e *ActiveVisitConflictError) Error() string { return ErrActiveVisit.Error() }
func (e *ActiveVisitConflictError) Unwrap() error { return ErrActiveVisit }

func (e *LifecycleConflictError) Error() string { return e.Cause.Error() }
func (e *LifecycleConflictError) Unwrap() error { return e.Cause }
