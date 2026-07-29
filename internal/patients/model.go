package patients

import "time"

type Status string

const (
	StatusActive   Status = "active"
	StatusDeceased Status = "deceased"
	StatusMerged   Status = "merged"
)

type Gender string

const (
	GenderMale    Gender = "male"
	GenderFemale  Gender = "female"
	GenderUnknown Gender = "unknown"
)

type CorrectionStatus string

const (
	CorrectionPending  CorrectionStatus = "pending"
	CorrectionActioned CorrectionStatus = "actioned"
	CorrectionRejected CorrectionStatus = "rejected"
)

// CorrectableFields are the "protected" patient fields that can only change
// via a submitted-then-decided correction request, never a direct PATCH.
var CorrectableFields = map[string]bool{
	"full_name":   true,
	"dob":         true,
	"gender":      true,
	"national_id": true,
}

type Patient struct {
	ID             string
	TenantID       string
	MRN            string
	FullName       string
	DOB            *time.Time
	DOBEstimated   bool
	ApproxAgeYears *int16
	Gender         Gender
	Phone          *string
	Address        *string
	NationalID     *string
	Status         Status
	DateOfDeath    *time.Time
	Insured        bool
	GuardianID     *string
	MergedIntoID   *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Identifier struct {
	ID         string
	PatientID  string
	IDType     string
	IDValue    string
	VerifiedAt *time.Time
	CreatedAt  time.Time
}

type Consent struct {
	ID        string
	PatientID string
	Scope     string
	Granted   bool
	GrantedAt *time.Time
	RevokedAt *time.Time
	UpdatedAt time.Time
}

type Correction struct {
	ID             string
	PatientID      string
	Field          string
	CurrentValue   *string
	RequestedValue string
	EvidenceNote   *string
	Status         CorrectionStatus
	SubmittedBy    *string
	DecidedBy      *string
	DecidedAt      *time.Time
	DecisionNote   *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ActivityEntry struct {
	ID        string
	PatientID string
	UserID    *string
	Kind      string
	Text      string
	CreatedAt time.Time
}

// DuplicateCandidate is one ranked result from a similarity search against
// existing patients in the tenant, used both by the standalone
// duplicate-check endpoint and as a server-side guard during registration.
type DuplicateCandidate struct {
	Patient    Patient
	Score      float64
	Confidence string // high | medium | low
	MatchedOn  []string
}
