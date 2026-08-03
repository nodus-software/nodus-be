package clinical

import "errors"

var (
	ErrNotFound          = errors.New("clinical resource not found")
	ErrInvalidInput      = errors.New("invalid clinical request")
	ErrInvalidTransition = errors.New("invalid queue transition")
	ErrReasonRequired    = errors.New("a reason is required for this queue action")
	ErrConflict          = errors.New("the clinical operation conflicts with current state")
	ErrActiveVisit       = errors.New("patient already has an active outpatient visit")
	ErrVisitIncomplete   = errors.New("outpatient visit has not completed consultation")
)
