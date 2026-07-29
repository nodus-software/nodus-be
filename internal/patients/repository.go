package patients

import (
	"context"
	"time"
)

// Repository persists everything owned by the Patient Management domain:
// patients themselves, their identifiers/consents/correction requests/
// activity log, and the per-tenant MRN counter.
type Repository interface {
	ListPatients(ctx context.Context, filter ListPatientsFilter) ([]Patient, int, error)
	GetPatientByID(ctx context.Context, id string) (*Patient, error)
	IssueMRN(ctx context.Context) (string, error)
	InsertPatient(ctx context.Context, p Patient) error
	FindDuplicateCandidates(ctx context.Context, fullName string, dob *time.Time, nationalID, phone *string) ([]DuplicateCandidate, error)
	UpdateContact(ctx context.Context, patientID string, phone, address *string) error
	MarkDeceased(ctx context.Context, patientID string, dateOfDeath time.Time) error
	SetMergedInto(ctx context.Context, awayID, keepID string) error
	ApplyCorrectionField(ctx context.Context, patientID, field, value string) error

	InsertCorrection(ctx context.Context, c Correction) error
	ListCorrections(ctx context.Context, patientID string) ([]Correction, error)
	GetCorrectionByID(ctx context.Context, id string) (*Correction, error)
	DecideCorrection(ctx context.Context, correctionID string, status CorrectionStatus, decidedBy string, decidedAt time.Time, note *string) error

	AddIdentifier(ctx context.Context, i Identifier) error
	RemoveIdentifier(ctx context.Context, patientID, identifierID string) error
	ListIdentifiers(ctx context.Context, patientID string) ([]Identifier, error)
	CountIdentifiers(ctx context.Context, patientID string) (int, error)

	ListConsents(ctx context.Context, patientID string) ([]Consent, error)
	UpsertConsent(ctx context.Context, patientID, scope string, granted bool, at time.Time) (*Consent, error)

	ListActivity(ctx context.Context, patientID string) ([]ActivityEntry, error)
	AddActivityEntry(ctx context.Context, e ActivityEntry) error

	ReassignIdentifiers(ctx context.Context, fromID, toID string) error
	ReassignConsents(ctx context.Context, fromID, toID string) error
	ReassignCorrections(ctx context.Context, fromID, toID string) error
	ReassignActivity(ctx context.Context, fromID, toID string) error

	// WithinTx runs fn with a Repository bound to a single database
	// transaction, committing on nil and rolling back otherwise.
	WithinTx(ctx context.Context, fn func(Repository) error) error
}
