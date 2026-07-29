package patients

import "errors"

var (
	ErrPatientNotFound          = errors.New("patient not found")
	ErrGuardianNotFound         = errors.New("guardian patient not found")
	ErrIdentifierNotFound       = errors.New("identifier not found")
	ErrCorrectionNotFound       = errors.New("correction request not found")
	ErrCorrectionAlreadyDecided = errors.New("correction request has already been decided")
	ErrCannotMergeSelf          = errors.New("cannot merge a patient record with itself")
	ErrAlreadyMerged            = errors.New("one or both patient records have already been merged")
	ErrInvalidGuardianSelection = errors.New("invalid guardian selection")
	ErrInvalidDate              = errors.New("invalid date")
)

// DuplicateError is returned by RegisterPatient when a high-confidence
// duplicate is found and the caller did not set duplicate_override. The
// handler maps this to 409 with the candidates surfaced in error.details,
// rather than a hard block, mirroring the FE's "this is a different
// person, continue registering" override.
type DuplicateError struct {
	Candidates []DuplicateCandidate
}

func (e *DuplicateError) Error() string {
	return "a possible duplicate patient record was found"
}
