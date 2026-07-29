package patients

import (
	"context"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/utility"
)

// AuditRecorder is the narrow slice of the audit domain this service
// depends on, defined here (the consumer) rather than in the audit package.
type AuditRecorder interface {
	Record(ctx context.Context, entry audit.Entry) error
}

// duplicateHighConfidence is the score threshold above which RegisterPatient
// blocks (with a 409) unless the caller explicitly overrides.
const duplicateHighConfidence = 0.9

type Config struct{}

type Service struct {
	repo  Repository
	audit AuditRecorder
	log   *logger.Logger
	cfg   Config
}

func NewService(repo Repository, auditRecorder AuditRecorder, log *logger.Logger, cfg Config) *Service {
	return &Service{repo: repo, audit: auditRecorder, log: log, cfg: cfg}
}

func parseDate(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil, ErrInvalidDate
	}
	return &t, nil
}

func (s *Service) ListPatients(ctx context.Context, filter ListPatientsFilter) ([]PatientResponse, int, error) {
	list, total, err := s.repo.ListPatients(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]PatientResponse, 0, len(list))
	for _, p := range list {
		resp = append(resp, toPatientResponse(p))
	}
	return resp, total, nil
}

func (s *Service) GetPatient(ctx context.Context, id string) (*PatientResponse, error) {
	p, err := s.repo.GetPatientByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := toPatientResponse(*p)
	return &resp, nil
}

func (s *Service) DuplicateCheck(ctx context.Context, fullName string, dob *time.Time, nationalID, phone *string) ([]DuplicateCandidateResponse, error) {
	candidates, err := s.repo.FindDuplicateCandidates(ctx, fullName, dob, nationalID, phone)
	if err != nil {
		return nil, err
	}
	resp := make([]DuplicateCandidateResponse, 0, len(candidates))
	for _, c := range candidates {
		resp = append(resp, toDuplicateCandidateResponse(c))
	}
	return resp, nil
}

// RegisterPatient handles all three intake modes: normal, unknown/
// unidentified (only gender + approx age required, full_name generated),
// and minor (requires a guardian - either an existing patient or a new one
// registered atomically in the same transaction). Unless overridden, it
// first re-runs the same similarity search as DuplicateCheck as a guard:
// a high-confidence match returns a *DuplicateError (mapped to 409 by the
// handler) rather than hard-blocking, since staff may legitimately be
// registering a different person who happens to share a name/DOB.
func (s *Service) RegisterPatient(ctx context.Context, actorUserID string, req RegisterPatientRequest) (*PatientResponse, error) {
	fullName := req.FullName
	if req.Unknown {
		fullName = unknownFullName(req.Gender, req.ApproxAgeYears)
	}

	dob, err := parseDate(req.DOB)
	if err != nil {
		return nil, err
	}

	if !req.DuplicateOverride && !req.Unknown && fullName != "" {
		candidates, err := s.repo.FindDuplicateCandidates(ctx, fullName, dob, req.NationalID, req.Phone)
		if err != nil {
			return nil, err
		}
		if len(candidates) > 0 && candidates[0].Score >= duplicateHighConfidence {
			return nil, &DuplicateError{Candidates: candidates}
		}
	}

	var guardianID *string
	var patientID string

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if req.Minor && req.Guardian != nil {
			switch req.Guardian.Mode {
			case "existing":
				if req.Guardian.PatientID == nil || *req.Guardian.PatientID == "" {
					return ErrInvalidGuardianSelection
				}
				if _, err := repo.GetPatientByID(ctx, *req.Guardian.PatientID); err != nil {
					return err
				}
				guardianID = req.Guardian.PatientID
			case "new":
				gDOB, err := parseDate(req.Guardian.DOB)
				if err != nil {
					return err
				}
				gID, err := utility.GenerateUUID()
				if err != nil {
					return err
				}
				mrn, err := repo.IssueMRN(ctx)
				if err != nil {
					return err
				}
				gender := GenderUnknown
				if req.Guardian.Gender != nil && *req.Guardian.Gender != "" {
					gender = Gender(*req.Guardian.Gender)
				}
				dobEstimated := false
				if req.Guardian.DOBEstimated != nil {
					dobEstimated = *req.Guardian.DOBEstimated
				}
				guardian := Patient{
					ID: gID, MRN: mrn, FullName: derefStr(req.Guardian.FullName), DOB: gDOB,
					DOBEstimated: dobEstimated, Gender: gender, Phone: req.Guardian.Phone,
					Address: req.Guardian.Address, NationalID: req.Guardian.NationalID,
				}
				if err := repo.InsertPatient(ctx, guardian); err != nil {
					return err
				}
				guardianID = &gID
			case "none":
				// no guardian linked
			default:
				return ErrInvalidGuardianSelection
			}
		}

		id, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		mrn, err := repo.IssueMRN(ctx)
		if err != nil {
			return err
		}
		gender := GenderUnknown
		if req.Gender != "" {
			gender = Gender(req.Gender)
		}
		var approxAge *int16
		if req.Unknown && req.ApproxAgeYears != nil {
			v := int16(*req.ApproxAgeYears)
			approxAge = &v
		}
		patient := Patient{
			ID: id, MRN: mrn, FullName: fullName, DOB: dob, DOBEstimated: req.DOBEstimated,
			ApproxAgeYears: approxAge, Gender: gender, Phone: req.Phone, Address: req.Address,
			NationalID: req.NationalID, GuardianID: guardianID,
		}
		if err := repo.InsertPatient(ctx, patient); err != nil {
			return err
		}
		patientID = id
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_registered", Result: audit.ResultSuccess,
		TargetResource: patientID, Metadata: map[string]any{"mrn": fullName, "unknown": req.Unknown, "minor": req.Minor},
	})

	return s.GetPatient(ctx, patientID)
}

func unknownFullName(gender string, approxAge *int) string {
	genderLabel := "[Gender]"
	switch gender {
	case string(GenderMale):
		genderLabel = "Male"
	case string(GenderFemale):
		genderLabel = "Female"
	case string(GenderUnknown), "":
		genderLabel = "Unknown"
	}
	ageLabel := "[age]"
	if approxAge != nil {
		ageLabel = itoa(*approxAge)
	}
	return "Unknown " + genderLabel + ", approx. " + ageLabel + " yrs"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Service) UpdateContact(ctx context.Context, actorUserID, patientID, phone, address string) (*PatientResponse, error) {
	if err := s.repo.UpdateContact(ctx, patientID, &phone, &address); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_contact_updated", Result: audit.ResultSuccess, TargetResource: patientID,
	})
	return s.GetPatient(ctx, patientID)
}

func (s *Service) MarkDeceased(ctx context.Context, actorUserID, patientID string, req MarkDeceasedRequest) (*PatientResponse, error) {
	dateOfDeath, err := time.Parse(dateLayout, req.DateOfDeath)
	if err != nil {
		if dateOfDeath, err = time.Parse(time.RFC3339, req.DateOfDeath); err != nil {
			return nil, ErrInvalidDate
		}
	}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.MarkDeceased(ctx, patientID, dateOfDeath); err != nil {
			return err
		}
		note := "Marked as deceased (" + dateOfDeath.Format(dateLayout) + ")"
		if req.Source != nil && *req.Source != "" {
			note += " — source: " + *req.Source
		}
		return repo.AddActivityEntry(ctx, ActivityEntry{PatientID: patientID, UserID: &actorUserID, Kind: "system", Text: note})
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_marked_deceased", Result: audit.ResultSuccess, TargetResource: patientID,
		Metadata: map[string]any{"date_of_death": req.DateOfDeath, "source": req.Source},
	})
	return s.GetPatient(ctx, patientID)
}

func (s *Service) SubmitCorrection(ctx context.Context, actorUserID, patientID string, req SubmitCorrectionRequest) (*CorrectionResponse, error) {
	patient, err := s.repo.GetPatientByID(ctx, patientID)
	if err != nil {
		return nil, err
	}
	current := currentFieldValue(*patient, req.Field)

	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	c := Correction{
		ID: id, PatientID: patientID, Field: req.Field, CurrentValue: current,
		RequestedValue: req.RequestedValue, EvidenceNote: req.EvidenceNote, SubmittedBy: &actorUserID,
	}
	if err := s.repo.InsertCorrection(ctx, c); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_correction_submitted", Result: audit.ResultSuccess, TargetResource: patientID,
		Metadata: map[string]any{"field": req.Field, "requested_value": req.RequestedValue},
	})
	saved, err := s.repo.GetCorrectionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := toCorrectionResponse(*saved)
	return &resp, nil
}

func currentFieldValue(p Patient, field string) *string {
	switch field {
	case "full_name":
		return &p.FullName
	case "dob":
		return formatDate(p.DOB)
	case "gender":
		g := string(p.Gender)
		return &g
	case "national_id":
		return p.NationalID
	}
	return nil
}

func (s *Service) ListCorrections(ctx context.Context, patientID string) ([]CorrectionResponse, error) {
	list, err := s.repo.ListCorrections(ctx, patientID)
	if err != nil {
		return nil, err
	}
	resp := make([]CorrectionResponse, 0, len(list))
	for _, c := range list {
		resp = append(resp, toCorrectionResponse(c))
	}
	return resp, nil
}

// DecideCorrection actions or rejects a pending correction. Actioning
// writes the requested value onto the patient's protected field.
func (s *Service) DecideCorrection(ctx context.Context, actorUserID, correctionID string, req DecideCorrectionRequest) (*CorrectionResponse, error) {
	correction, err := s.repo.GetCorrectionByID(ctx, correctionID)
	if err != nil {
		return nil, err
	}
	if correction.Status != CorrectionPending {
		return nil, ErrCorrectionAlreadyDecided
	}

	now := time.Now()
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.DecideCorrection(ctx, correctionID, CorrectionStatus(req.Decision), actorUserID, now, req.DecisionNote); err != nil {
			return err
		}
		if req.Decision == string(CorrectionActioned) {
			if err := repo.ApplyCorrectionField(ctx, correction.PatientID, correction.Field, correction.RequestedValue); err != nil {
				return err
			}
			note := "Correction actioned: " + correction.Field + " → " + correction.RequestedValue
			if err := repo.AddActivityEntry(ctx, ActivityEntry{PatientID: correction.PatientID, UserID: &actorUserID, Kind: "system", Text: note}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_correction_decided", Result: audit.ResultSuccess, TargetResource: correction.PatientID,
		Metadata: map[string]any{"correction_id": correctionID, "decision": req.Decision},
	})
	saved, err := s.repo.GetCorrectionByID(ctx, correctionID)
	if err != nil {
		return nil, err
	}
	resp := toCorrectionResponse(*saved)
	return &resp, nil
}

func (s *Service) AddIdentifier(ctx context.Context, actorUserID, patientID string, req AddIdentifierRequest) (*IdentifierResponse, error) {
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	i := Identifier{ID: id, PatientID: patientID, IDType: req.IDType, IDValue: req.IDValue}
	if err := s.repo.AddIdentifier(ctx, i); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_identifier_added", Result: audit.ResultSuccess, TargetResource: patientID,
		Metadata: map[string]any{"id_type": req.IDType},
	})
	list, err := s.repo.ListIdentifiers(ctx, patientID)
	if err != nil {
		return nil, err
	}
	for _, l := range list {
		if l.ID == id {
			resp := toIdentifierResponse(l)
			return &resp, nil
		}
	}
	resp := toIdentifierResponse(i)
	return &resp, nil
}

func (s *Service) RemoveIdentifier(ctx context.Context, actorUserID, patientID, identifierID string) error {
	if err := s.repo.RemoveIdentifier(ctx, patientID, identifierID); err != nil {
		return err
	}
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_identifier_removed", Result: audit.ResultSuccess, TargetResource: patientID,
	})
	return nil
}

func (s *Service) ListIdentifiers(ctx context.Context, patientID string) ([]IdentifierResponse, error) {
	list, err := s.repo.ListIdentifiers(ctx, patientID)
	if err != nil {
		return nil, err
	}
	resp := make([]IdentifierResponse, 0, len(list))
	for _, i := range list {
		resp = append(resp, toIdentifierResponse(i))
	}
	return resp, nil
}

func (s *Service) ListConsents(ctx context.Context, patientID string) ([]ConsentResponse, error) {
	list, err := s.repo.ListConsents(ctx, patientID)
	if err != nil {
		return nil, err
	}
	resp := make([]ConsentResponse, 0, len(list))
	for _, c := range list {
		resp = append(resp, toConsentResponse(c))
	}
	return resp, nil
}

func (s *Service) SetConsent(ctx context.Context, actorUserID, patientID, scope string, req SetConsentRequest) (*ConsentResponse, error) {
	now := time.Now()
	c, err := s.repo.UpsertConsent(ctx, patientID, scope, req.Granted, now)
	if err != nil {
		return nil, err
	}
	action := "Revoked"
	if req.Granted {
		action = "Granted"
	}
	_ = s.repo.AddActivityEntry(ctx, ActivityEntry{
		PatientID: patientID, UserID: &actorUserID, Kind: "system", Text: action + " consent: " + scope,
	})
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_consent_updated", Result: audit.ResultSuccess, TargetResource: patientID,
		Metadata: map[string]any{"scope": scope, "granted": req.Granted},
	})
	resp := toConsentResponse(*c)
	return &resp, nil
}

func (s *Service) ListActivity(ctx context.Context, patientID string) ([]ActivityEntryResponse, error) {
	list, err := s.repo.ListActivity(ctx, patientID)
	if err != nil {
		return nil, err
	}
	resp := make([]ActivityEntryResponse, 0, len(list))
	for _, e := range list {
		resp = append(resp, toActivityEntryResponse(e))
	}
	return resp, nil
}

func (s *Service) AddActivityNote(ctx context.Context, actorUserID, patientID string, req AddActivityNoteRequest) (*ActivityEntryResponse, error) {
	if err := s.repo.AddActivityEntry(ctx, ActivityEntry{PatientID: patientID, UserID: &actorUserID, Kind: "note", Text: req.Text}); err != nil {
		return nil, err
	}
	list, err := s.repo.ListActivity(ctx, patientID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrPatientNotFound
	}
	resp := toActivityEntryResponse(list[0])
	return &resp, nil
}

// MergePatients combines two patient records using an alias/redirect model:
// the "away" record is kept (never deleted) but marked status=merged with
// merged_into_id pointing at the survivor, so old MRN lookups can still
// resolve and redirect. All child records (identifiers, consents,
// corrections, activity) are reassigned to the survivor.
func (s *Service) MergePatients(ctx context.Context, actorUserID string, req MergePatientsRequest) (*PatientResponse, error) {
	if req.KeepPatientID == req.AwayPatientID {
		return nil, ErrCannotMergeSelf
	}
	keep, err := s.repo.GetPatientByID(ctx, req.KeepPatientID)
	if err != nil {
		return nil, err
	}
	away, err := s.repo.GetPatientByID(ctx, req.AwayPatientID)
	if err != nil {
		return nil, err
	}
	if keep.Status == StatusMerged || away.Status == StatusMerged {
		return nil, ErrAlreadyMerged
	}

	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ReassignIdentifiers(ctx, away.ID, keep.ID); err != nil {
			return err
		}
		if err := repo.ReassignConsents(ctx, away.ID, keep.ID); err != nil {
			return err
		}
		if err := repo.ReassignCorrections(ctx, away.ID, keep.ID); err != nil {
			return err
		}
		if err := repo.ReassignActivity(ctx, away.ID, keep.ID); err != nil {
			return err
		}
		if err := repo.SetMergedInto(ctx, away.ID, keep.ID); err != nil {
			return err
		}
		note := "Merged in duplicate record " + away.MRN + " (" + away.FullName + ") — now an alias. Reason: " + req.Reason
		return repo.AddActivityEntry(ctx, ActivityEntry{PatientID: keep.ID, UserID: &actorUserID, Kind: "system", Text: note})
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &actorUserID, Action: "patient_merged", Result: audit.ResultSuccess, TargetResource: keep.ID,
		Metadata: map[string]any{"away_patient_id": away.ID, "reason": req.Reason},
	})

	return s.GetPatient(ctx, keep.ID)
}
