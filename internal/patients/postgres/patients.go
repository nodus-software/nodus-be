package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"nodus-health/internal/patients"
	"nodus-health/internal/patients/postgres/sqlcgen"
	"nodus-health/pkg/utility"
)

func toPatientStatuses(vals []string) []sqlcgen.PatientStatus {
	if len(vals) == 0 {
		return nil
	}
	out := make([]sqlcgen.PatientStatus, 0, len(vals))
	for _, v := range vals {
		out = append(out, sqlcgen.PatientStatus(v))
	}
	return out
}

func toPatientGenders(vals []string) []sqlcgen.PatientGender {
	if len(vals) == 0 {
		return nil
	}
	out := make([]sqlcgen.PatientGender, 0, len(vals))
	for _, v := range vals {
		out = append(out, sqlcgen.PatientGender(v))
	}
	return out
}

// insuranceFilter maps the "insured"/"uninsured" multi-select query values
// to a nullable boolean: both or neither selected means no filter.
func insuranceFilter(vals []string) *bool {
	var insured, uninsured bool
	for _, v := range vals {
		switch v {
		case "insured":
			insured = true
		case "uninsured":
			uninsured = true
		}
	}
	switch {
	case insured && !uninsured:
		t := true
		return &t
	case uninsured && !insured:
		f := false
		return &f
	default:
		return nil
	}
}

func toDomainPatient(p sqlcgen.Patient) patients.Patient {
	return patients.Patient{
		ID: p.ID, TenantID: p.TenantID, MRN: p.Mrn, FullName: p.FullName,
		DOB: fromNullDate(p.Dob), DOBEstimated: p.DobEstimated, ApproxAgeYears: p.ApproxAgeYears,
		Gender: patients.Gender(p.Gender), Phone: p.Phone, Address: p.Address, NationalID: p.NationalID,
		Status: patients.Status(p.Status), DateOfDeath: fromNullTimestamptz(p.DateOfDeath), Insured: p.Insured,
		GuardianID: p.GuardianID, MergedIntoID: p.MergedIntoID,
		CreatedAt: p.CreatedAt.Time, UpdatedAt: p.UpdatedAt.Time,
	}
}

func toDomainCorrection(c sqlcgen.PatientCorrection) patients.Correction {
	return patients.Correction{
		ID: c.ID, PatientID: c.PatientID, Field: c.Field, CurrentValue: c.CurrentValue,
		RequestedValue: c.RequestedValue, EvidenceNote: c.EvidenceNote, Status: patients.CorrectionStatus(c.Status),
		SubmittedBy: c.SubmittedBy, DecidedBy: c.DecidedBy, DecidedAt: fromNullTimestamptz(c.DecidedAt),
		DecisionNote: c.DecisionNote, CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time,
	}
}

func toDomainConsent(c sqlcgen.PatientConsent) patients.Consent {
	return patients.Consent{
		ID: c.ID, PatientID: c.PatientID, Scope: c.Scope, Granted: c.Granted,
		GrantedAt: fromNullTimestamptz(c.GrantedAt), RevokedAt: fromNullTimestamptz(c.RevokedAt),
		UpdatedAt: c.UpdatedAt.Time,
	}
}

func (r *Repository) ListPatients(ctx context.Context, filter patients.ListPatientsFilter) ([]patients.Patient, int, error) {
	page, perPage := filter.Page, filter.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	statuses := toPatientStatuses(filter.Status)
	genders := toPatientGenders(filter.Gender)
	insured := insuranceFilter(filter.Insurance)
	regFrom, regTo := toDate(filter.RegFrom), toDate(filter.RegTo)

	rows, err := r.q(ctx).ListPatients(ctx, sqlcgen.ListPatientsParams{
		Q: filter.Q, Statuses: statuses, Genders: genders, Insured: insured, RegFrom: regFrom, RegTo: regTo,
		OffsetVal: int32((page - 1) * perPage), LimitVal: int32(perPage),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q(ctx).CountPatients(ctx, sqlcgen.CountPatientsParams{
		Q: filter.Q, Statuses: statuses, Genders: genders, Insured: insured, RegFrom: regFrom, RegTo: regTo,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]patients.Patient, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainPatient(row))
	}
	return out, int(total), nil
}

func (r *Repository) GetPatientByID(ctx context.Context, id string) (*patients.Patient, error) {
	row, err := r.q(ctx).GetPatientByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, patients.ErrPatientNotFound
		}
		return nil, err
	}
	p := toDomainPatient(row)
	return &p, nil
}

func (r *Repository) IssueMRN(ctx context.Context) (string, error) {
	n, err := r.q(ctx).IssuePatientMRNNumber(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MRN-%d", n), nil
}

func (r *Repository) InsertPatient(ctx context.Context, p patients.Patient) error {
	return r.q(ctx).InsertPatient(ctx, sqlcgen.InsertPatientParams{
		ID: p.ID, Mrn: p.MRN, FullName: p.FullName, Dob: toDate(p.DOB), DobEstimated: p.DOBEstimated,
		ApproxAgeYears: p.ApproxAgeYears, Gender: sqlcgen.PatientGender(p.Gender), Phone: p.Phone,
		Address: p.Address, NationalID: p.NationalID, Insured: p.Insured, GuardianID: p.GuardianID,
	})
}

func (r *Repository) FindDuplicateCandidates(ctx context.Context, fullName string, dob *time.Time, nationalID, phone *string) ([]patients.DuplicateCandidate, error) {
	rows, err := r.q(ctx).FindDuplicateCandidates(ctx, sqlcgen.FindDuplicateCandidatesParams{
		FullName: fullName, Dob: toDate(dob), NationalID: nationalID, Phone: phone,
	})
	if err != nil {
		return nil, err
	}
	out := make([]patients.DuplicateCandidate, 0, len(rows))
	for _, row := range rows {
		score := float64(row.NameScore)
		var matched []string
		if row.DobExact != nil && *row.DobExact {
			score += 0.4
			matched = append(matched, "dob")
		}
		if row.NationalIDExact != nil && *row.NationalIDExact {
			score += 0.5
			matched = append(matched, "national_id")
		}
		if row.PhoneExact != nil && *row.PhoneExact {
			score += 0.2
			matched = append(matched, "phone")
		}
		confidence := "low"
		switch {
		case score >= 0.9:
			confidence = "high"
		case score >= 0.5:
			confidence = "medium"
		}
		p := patients.Patient{
			ID: row.ID, TenantID: row.TenantID, MRN: row.Mrn, FullName: row.FullName,
			DOB: fromNullDate(row.Dob), DOBEstimated: row.DobEstimated, ApproxAgeYears: row.ApproxAgeYears,
			Gender: patients.Gender(row.Gender), Phone: row.Phone, Address: row.Address, NationalID: row.NationalID,
			Status: patients.Status(row.Status), DateOfDeath: fromNullTimestamptz(row.DateOfDeath), Insured: row.Insured,
			GuardianID: row.GuardianID, MergedIntoID: row.MergedIntoID,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		}
		out = append(out, patients.DuplicateCandidate{Patient: p, Score: score, Confidence: confidence, MatchedOn: matched})
	}
	return out, nil
}

func (r *Repository) UpdateContact(ctx context.Context, patientID string, phone, address *string) error {
	return r.q(ctx).UpdatePatientContact(ctx, sqlcgen.UpdatePatientContactParams{ID: patientID, Phone: phone, Address: address})
}

func (r *Repository) MarkDeceased(ctx context.Context, patientID string, dateOfDeath time.Time) error {
	return r.q(ctx).MarkPatientDeceased(ctx, sqlcgen.MarkPatientDeceasedParams{ID: patientID, DateOfDeath: toTimestamptz(dateOfDeath)})
}

func (r *Repository) SetMergedInto(ctx context.Context, awayID, keepID string) error {
	return r.q(ctx).SetPatientMergedInto(ctx, sqlcgen.SetPatientMergedIntoParams{ID: awayID, MergedIntoID: &keepID})
}

func (r *Repository) ApplyCorrectionField(ctx context.Context, patientID, field, value string) error {
	switch field {
	case "full_name":
		return r.q(ctx).UpdatePatientFullName(ctx, sqlcgen.UpdatePatientFullNameParams{ID: patientID, FullName: value})
	case "dob":
		t, err := time.Parse("2006-01-02", value)
		if err != nil {
			return patients.ErrInvalidDate
		}
		return r.q(ctx).UpdatePatientDOB(ctx, sqlcgen.UpdatePatientDOBParams{ID: patientID, Column2: toDate(&t)})
	case "gender":
		return r.q(ctx).UpdatePatientGender(ctx, sqlcgen.UpdatePatientGenderParams{ID: patientID, Column2: sqlcgen.PatientGender(value)})
	case "national_id":
		v := value
		return r.q(ctx).UpdatePatientNationalID(ctx, sqlcgen.UpdatePatientNationalIDParams{ID: patientID, NationalID: &v})
	}
	return nil
}

func (r *Repository) InsertCorrection(ctx context.Context, c patients.Correction) error {
	return r.q(ctx).InsertCorrection(ctx, sqlcgen.InsertCorrectionParams{
		ID: c.ID, PatientID: c.PatientID, Field: c.Field, CurrentValue: c.CurrentValue,
		RequestedValue: c.RequestedValue, EvidenceNote: c.EvidenceNote, SubmittedBy: c.SubmittedBy,
	})
}

func (r *Repository) ListCorrections(ctx context.Context, patientID string) ([]patients.Correction, error) {
	rows, err := r.q(ctx).ListCorrections(ctx, patientID)
	if err != nil {
		return nil, err
	}
	out := make([]patients.Correction, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainCorrection(row))
	}
	return out, nil
}

func (r *Repository) GetCorrectionByID(ctx context.Context, id string) (*patients.Correction, error) {
	row, err := r.q(ctx).GetCorrectionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, patients.ErrCorrectionNotFound
		}
		return nil, err
	}
	c := toDomainCorrection(row)
	return &c, nil
}

func (r *Repository) DecideCorrection(ctx context.Context, correctionID string, status patients.CorrectionStatus, decidedBy string, decidedAt time.Time, note *string) error {
	return r.q(ctx).DecideCorrection(ctx, sqlcgen.DecideCorrectionParams{
		ID: correctionID, Column2: sqlcgen.PatientCorrectionStatus(status), Column3: decidedBy,
		DecidedAt: toTimestamptz(decidedAt), DecisionNote: note,
	})
}

func (r *Repository) AddIdentifier(ctx context.Context, i patients.Identifier) error {
	return r.q(ctx).InsertIdentifier(ctx, sqlcgen.InsertIdentifierParams{ID: i.ID, PatientID: i.PatientID, IDType: i.IDType, IDValue: i.IDValue})
}

func (r *Repository) RemoveIdentifier(ctx context.Context, patientID, identifierID string) error {
	return r.q(ctx).DeleteIdentifier(ctx, sqlcgen.DeleteIdentifierParams{ID: identifierID, PatientID: patientID})
}

func (r *Repository) ListIdentifiers(ctx context.Context, patientID string) ([]patients.Identifier, error) {
	rows, err := r.q(ctx).ListIdentifiers(ctx, patientID)
	if err != nil {
		return nil, err
	}
	out := make([]patients.Identifier, 0, len(rows))
	for _, row := range rows {
		out = append(out, patients.Identifier{
			ID: row.ID, PatientID: row.PatientID, IDType: row.IDType, IDValue: row.IDValue,
			VerifiedAt: fromNullTimestamptz(row.VerifiedAt), CreatedAt: row.CreatedAt.Time,
		})
	}
	return out, nil
}

func (r *Repository) CountIdentifiers(ctx context.Context, patientID string) (int, error) {
	n, err := r.q(ctx).CountIdentifiers(ctx, patientID)
	return int(n), err
}

func (r *Repository) ListConsents(ctx context.Context, patientID string) ([]patients.Consent, error) {
	rows, err := r.q(ctx).ListConsents(ctx, patientID)
	if err != nil {
		return nil, err
	}
	out := make([]patients.Consent, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainConsent(row))
	}
	return out, nil
}

func (r *Repository) UpsertConsent(ctx context.Context, patientID, scope string, granted bool, at time.Time) (*patients.Consent, error) {
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	var grantedAt, revokedAt pgtype.Timestamptz
	if granted {
		grantedAt = toTimestamptz(at)
	} else {
		revokedAt = toTimestamptz(at)
	}
	row, err := r.q(ctx).UpsertConsent(ctx, sqlcgen.UpsertConsentParams{
		ID: id, PatientID: patientID, Scope: scope, Granted: granted, GrantedAt: grantedAt, RevokedAt: revokedAt,
	})
	if err != nil {
		return nil, err
	}
	c := toDomainConsent(row)
	return &c, nil
}

func (r *Repository) ListActivity(ctx context.Context, patientID string) ([]patients.ActivityEntry, error) {
	rows, err := r.q(ctx).ListActivity(ctx, patientID)
	if err != nil {
		return nil, err
	}
	out := make([]patients.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, patients.ActivityEntry{
			ID: row.ID, PatientID: row.PatientID, UserID: row.UserID, Kind: row.Kind, Text: row.Text, CreatedAt: row.CreatedAt.Time,
		})
	}
	return out, nil
}

func (r *Repository) AddActivityEntry(ctx context.Context, e patients.ActivityEntry) error {
	id, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	return r.q(ctx).InsertActivity(ctx, sqlcgen.InsertActivityParams{ID: id, PatientID: e.PatientID, UserID: e.UserID, Kind: e.Kind, Text: e.Text})
}

func (r *Repository) ReassignIdentifiers(ctx context.Context, fromID, toID string) error {
	return r.q(ctx).ReassignIdentifiers(ctx, sqlcgen.ReassignIdentifiersParams{PatientID: fromID, PatientID_2: toID})
}

func (r *Repository) ReassignConsents(ctx context.Context, fromID, toID string) error {
	return r.q(ctx).ReassignConsents(ctx, sqlcgen.ReassignConsentsParams{PatientID: fromID, PatientID_2: toID})
}

func (r *Repository) ReassignCorrections(ctx context.Context, fromID, toID string) error {
	return r.q(ctx).ReassignCorrections(ctx, sqlcgen.ReassignCorrectionsParams{PatientID: fromID, PatientID_2: toID})
}

func (r *Repository) ReassignActivity(ctx context.Context, fromID, toID string) error {
	return r.q(ctx).ReassignActivity(ctx, sqlcgen.ReassignActivityParams{PatientID: fromID, PatientID_2: toID})
}
