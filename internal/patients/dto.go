package patients

import "time"

const dateLayout = "2006-01-02"

// ListPatientsFilter mirrors the query parameters on GET /patients. A nil/
// empty field means "no filter". Status/Gender/Insurance accept multiple
// values (comma-separated on the wire) for multi-select filtering.
type ListPatientsFilter struct {
	Q         *string
	Status    []string
	Gender    []string
	Insurance []string // "insured" | "uninsured"
	RegFrom   *time.Time
	RegTo     *time.Time
	Page      int
	PerPage   int
}

type GuardianSelectionRequest struct {
	Mode         string  `json:"mode" validate:"required,oneof=existing new none"`
	PatientID    *string `json:"patient_id,omitempty"`
	FullName     *string `json:"full_name,omitempty"`
	DOB          *string `json:"dob,omitempty"`
	DOBEstimated *bool   `json:"dob_estimated,omitempty"`
	Gender       *string `json:"gender,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	NationalID   *string `json:"national_id,omitempty"`
	Address      *string `json:"address,omitempty"`
}

// RegisterPatientRequest is the POST /patients body. Supports three intake
// modes (normal, Unknown/unidentified, Minor+Guardian) - see service.go's
// RegisterPatient for the branching logic.
type RegisterPatientRequest struct {
	Unknown           bool                      `json:"unknown"`
	Minor             bool                      `json:"minor"`
	FullName          string                    `json:"full_name"`
	DOB               *string                   `json:"dob"`
	DOBEstimated      bool                      `json:"dob_estimated"`
	Gender            string                    `json:"gender" validate:"omitempty,oneof=male female unknown"`
	ApproxAgeYears    *int                      `json:"approx_age_years"`
	Phone             *string                   `json:"phone"`
	Address           *string                   `json:"address"`
	NationalID        *string                   `json:"national_id"`
	Guardian          *GuardianSelectionRequest `json:"guardian,omitempty"`
	DuplicateOverride bool                      `json:"duplicate_override"`
}

type PatientResponse struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	MRN            string    `json:"mrn"`
	FullName       string    `json:"full_name"`
	DOB            *string   `json:"dob"`
	DOBEstimated   bool      `json:"dob_estimated"`
	ApproxAgeYears *int      `json:"approx_age_years,omitempty"`
	Gender         string    `json:"gender"`
	Phone          *string   `json:"phone"`
	Address        *string   `json:"address"`
	NationalID     *string   `json:"national_id"`
	Status         string    `json:"status"`
	DateOfDeath    *string   `json:"date_of_death,omitempty"`
	Insured        bool      `json:"insured"`
	GuardianID     *string   `json:"guardian_id,omitempty"`
	MergedIntoID   *string   `json:"merged_into_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

func formatDateTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func toPatientResponse(p Patient) PatientResponse {
	var approxAge *int
	if p.ApproxAgeYears != nil {
		v := int(*p.ApproxAgeYears)
		approxAge = &v
	}
	return PatientResponse{
		ID: p.ID, TenantID: p.TenantID, MRN: p.MRN, FullName: p.FullName,
		DOB: formatDate(p.DOB), DOBEstimated: p.DOBEstimated, ApproxAgeYears: approxAge,
		Gender: string(p.Gender), Phone: p.Phone, Address: p.Address, NationalID: p.NationalID,
		Status: string(p.Status), DateOfDeath: formatDateTime(p.DateOfDeath), Insured: p.Insured,
		GuardianID: p.GuardianID, MergedIntoID: p.MergedIntoID,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

type DuplicateCandidateResponse struct {
	Patient    PatientResponse `json:"patient"`
	Score      float64         `json:"score"`
	Confidence string          `json:"confidence"`
	MatchedOn  []string        `json:"matched_on"`
}

func toDuplicateCandidateResponse(c DuplicateCandidate) DuplicateCandidateResponse {
	return DuplicateCandidateResponse{
		Patient: toPatientResponse(c.Patient), Score: c.Score, Confidence: c.Confidence, MatchedOn: c.MatchedOn,
	}
}

type MarkDeceasedRequest struct {
	DateOfDeath string  `json:"date_of_death" validate:"required"`
	Source      *string `json:"source,omitempty"`
}

type SubmitCorrectionRequest struct {
	Field          string  `json:"field" validate:"required,oneof=full_name dob gender national_id"`
	RequestedValue string  `json:"requested_value" validate:"required"`
	EvidenceNote   *string `json:"evidence_note,omitempty"`
}

type DecideCorrectionRequest struct {
	Decision     string  `json:"decision" validate:"required,oneof=actioned rejected"`
	DecisionNote *string `json:"decision_note,omitempty"`
}

type CorrectionResponse struct {
	ID             string  `json:"id"`
	PatientID      string  `json:"patient_id"`
	Field          string  `json:"field"`
	CurrentValue   *string `json:"current_value,omitempty"`
	RequestedValue string  `json:"requested_value"`
	EvidenceNote   *string `json:"evidence_note,omitempty"`
	Status         string  `json:"status"`
	SubmittedBy    *string `json:"submitted_by,omitempty"`
	DecidedBy      *string `json:"decided_by,omitempty"`
	DecidedAt      *string `json:"decided_at,omitempty"`
	DecisionNote   *string `json:"decision_note,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

func toCorrectionResponse(c Correction) CorrectionResponse {
	return CorrectionResponse{
		ID: c.ID, PatientID: c.PatientID, Field: c.Field, CurrentValue: c.CurrentValue,
		RequestedValue: c.RequestedValue, EvidenceNote: c.EvidenceNote, Status: string(c.Status),
		SubmittedBy: c.SubmittedBy, DecidedBy: c.DecidedBy, DecidedAt: formatDateTime(c.DecidedAt),
		DecisionNote: c.DecisionNote, CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}
}

type AddIdentifierRequest struct {
	IDType  string `json:"id_type" validate:"required"`
	IDValue string `json:"id_value" validate:"required"`
}

type IdentifierResponse struct {
	ID            string  `json:"id"`
	PatientID     string  `json:"patient_id"`
	IDType        string  `json:"id_type"`
	IDValue       string  `json:"id_value"`
	VerifiedLabel string  `json:"verified_label"`
	CreatedAt     string  `json:"created_at"`
	VerifiedAt    *string `json:"verified_at,omitempty"`
}

func toIdentifierResponse(i Identifier) IdentifierResponse {
	label := "Pending verification"
	if i.VerifiedAt != nil {
		label = "Verified"
	}
	return IdentifierResponse{
		ID: i.ID, PatientID: i.PatientID, IDType: i.IDType, IDValue: i.IDValue,
		VerifiedLabel: label, CreatedAt: i.CreatedAt.Format(time.RFC3339), VerifiedAt: formatDateTime(i.VerifiedAt),
	}
}

type SetConsentRequest struct {
	Granted bool `json:"granted"`
}

type ConsentResponse struct {
	ID        string  `json:"id"`
	PatientID string  `json:"patient_id"`
	Scope     string  `json:"scope"`
	Granted   bool    `json:"granted"`
	GrantedAt *string `json:"granted_at,omitempty"`
	RevokedAt *string `json:"revoked_at,omitempty"`
}

func toConsentResponse(c Consent) ConsentResponse {
	return ConsentResponse{
		ID: c.ID, PatientID: c.PatientID, Scope: c.Scope, Granted: c.Granted,
		GrantedAt: formatDateTime(c.GrantedAt), RevokedAt: formatDateTime(c.RevokedAt),
	}
}

type AddActivityNoteRequest struct {
	Text string `json:"text" validate:"required"`
}

type ActivityEntryResponse struct {
	ID        string  `json:"id"`
	PatientID string  `json:"patient_id"`
	UserID    *string `json:"user_id,omitempty"`
	Kind      string  `json:"kind"`
	Text      string  `json:"text"`
	CreatedAt string  `json:"created_at"`
}

func toActivityEntryResponse(e ActivityEntry) ActivityEntryResponse {
	return ActivityEntryResponse{
		ID: e.ID, PatientID: e.PatientID, UserID: e.UserID, Kind: e.Kind, Text: e.Text,
		CreatedAt: e.CreatedAt.Format(time.RFC3339),
	}
}

type MergePatientsRequest struct {
	KeepPatientID string `json:"keep_patient_id" validate:"required"`
	AwayPatientID string `json:"away_patient_id" validate:"required"`
	Reason        string `json:"reason" validate:"required,max=500"`
}
