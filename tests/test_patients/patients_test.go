package test_patients

import (
	"net/http"
	"testing"

	"nodus-health/internal/patients"
)

func TestListPatients_FiltersByStatus(t *testing.T) {
	env := Setup(t)
	env.CreatePatient(t, func(p *patients.Patient) { p.Status = patients.StatusActive })
	env.CreatePatient(t, func(p *patients.Patient) { p.Status = patients.StatusDeceased })
	_, actorToken := env.NewActor(t, "patients:read")

	rec := env.JSON(t, http.MethodGet, "/patients?status=deceased", actorToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var list []patients.PatientResponse
	Decode(t, rec, &list)
	if len(list) != 1 || list[0].Status != "deceased" {
		t.Fatalf("expected one deceased patient, got %+v", list)
	}
}

func TestRegisterPatient_Succeeds(t *testing.T) {
	env := Setup(t)
	_, actorToken := env.NewActor(t, "patients:write")

	rec := env.JSON(t, http.MethodPost, "/patients", actorToken, patients.RegisterPatientRequest{
		FullName: "Amina Hassan", Gender: "female", Phone: strPtr("0712345678"),
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patients.PatientResponse
	Decode(t, rec, &resp)
	if resp.FullName != "Amina Hassan" || resp.MRN == "" {
		t.Fatalf("expected registered patient with MRN, got %+v", resp)
	}
}

func TestRegisterPatient_UnknownIntake_GeneratesFullName(t *testing.T) {
	env := Setup(t)
	_, actorToken := env.NewActor(t, "patients:write")
	age := 30

	rec := env.JSON(t, http.MethodPost, "/patients", actorToken, patients.RegisterPatientRequest{
		Unknown: true, Gender: "male", ApproxAgeYears: &age,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patients.PatientResponse
	Decode(t, rec, &resp)
	if resp.FullName != "Unknown Male, approx. 30 yrs" {
		t.Fatalf("expected generated unknown-patient name, got %q", resp.FullName)
	}
}

func TestRegisterPatient_HighConfidenceDuplicate_Returns409WithCandidates(t *testing.T) {
	env := Setup(t)
	_, actorToken := env.NewActor(t, "patients:write")
	existing := env.CreatePatient(t, func(p *patients.Patient) { p.FullName = "Brian Otieno" })
	env.Repo.duplicateCandidates = []patients.DuplicateCandidate{
		{Patient: env.Repo.patients[existing], Score: 0.95, Confidence: "high", MatchedOn: []string{"dob"}},
	}

	rec := env.JSON(t, http.MethodPost, "/patients", actorToken, patients.RegisterPatientRequest{
		FullName: "Brian Otieno", Gender: "male",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}

	// Overriding should bypass the guard even though the same duplicate exists.
	rec2 := env.JSON(t, http.MethodPost, "/patients", actorToken, patients.RegisterPatientRequest{
		FullName: "Brian Otieno", Gender: "male", DuplicateOverride: true,
	})
	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 with override, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestRegisterPatient_MinorWithExistingGuardian_LinksGuardian(t *testing.T) {
	env := Setup(t)
	_, actorToken := env.NewActor(t, "patients:write")
	guardianID := env.CreatePatient(t, func(p *patients.Patient) { p.FullName = "Guardian Parent" })

	rec := env.JSON(t, http.MethodPost, "/patients", actorToken, patients.RegisterPatientRequest{
		Minor: true, FullName: "Baby Otieno", Gender: "unknown",
		Guardian: &patients.GuardianSelectionRequest{Mode: "existing", PatientID: &guardianID},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patients.PatientResponse
	Decode(t, rec, &resp)
	if resp.GuardianID == nil || *resp.GuardianID != guardianID {
		t.Fatalf("expected guardian_id=%s, got %+v", guardianID, resp.GuardianID)
	}
}

func TestMarkDeceased_SetsStatusAndDate(t *testing.T) {
	env := Setup(t)
	target := env.CreatePatient(t, nil)
	_, actorToken := env.NewActor(t, "patients:write")

	rec := env.JSON(t, http.MethodPost, "/patients/"+target+"/mark-deceased", actorToken, patients.MarkDeceasedRequest{
		DateOfDeath: "2026-01-15",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patients.PatientResponse
	Decode(t, rec, &resp)
	if resp.Status != "deceased" || resp.DateOfDeath == nil {
		t.Fatalf("expected deceased status with date_of_death set, got %+v", resp)
	}
}

func TestSubmitAndDecideCorrection_ActionedAppliesField(t *testing.T) {
	env := Setup(t)
	target := env.CreatePatient(t, func(p *patients.Patient) { p.FullName = "Old Name" })
	_, writerToken := env.NewActor(t, "patients:write")

	rec := env.JSON(t, http.MethodPost, "/patients/"+target+"/corrections", writerToken, patients.SubmitCorrectionRequest{
		Field: "full_name", RequestedValue: "New Name",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var correction patients.CorrectionResponse
	Decode(t, rec, &correction)

	rec2 := env.JSON(t, http.MethodPost, "/patients/corrections/"+correction.ID+"/action", writerToken, patients.DecideCorrectionRequest{
		Decision: "actioned",
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	updated, err := env.Repo.GetPatientByID(nil, target) //nolint:staticcheck // memoryRepo ignores ctx
	if err != nil {
		t.Fatal(err)
	}
	if updated.FullName != "New Name" {
		t.Fatalf("expected actioned correction to apply full_name, got %q", updated.FullName)
	}
}

func TestSubmitCorrection_DecidingTwiceReturns409(t *testing.T) {
	env := Setup(t)
	target := env.CreatePatient(t, nil)
	_, writerToken := env.NewActor(t, "patients:write")

	rec := env.JSON(t, http.MethodPost, "/patients/"+target+"/corrections", writerToken, patients.SubmitCorrectionRequest{
		Field: "national_id", RequestedValue: "12345678",
	})
	var correction patients.CorrectionResponse
	Decode(t, rec, &correction)

	env.JSON(t, http.MethodPost, "/patients/corrections/"+correction.ID+"/action", writerToken, patients.DecideCorrectionRequest{Decision: "rejected"})
	rec2 := env.JSON(t, http.MethodPost, "/patients/corrections/"+correction.ID+"/action", writerToken, patients.DecideCorrectionRequest{Decision: "actioned"})
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 deciding an already-decided correction, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestMergePatients_ReassignsChildRecordsAndAliasesAwayRecord(t *testing.T) {
	env := Setup(t)
	keep := env.CreatePatient(t, func(p *patients.Patient) { p.FullName = "Amina Hassan" })
	away := env.CreatePatient(t, func(p *patients.Patient) { p.FullName = "Amina H." })
	_, actorToken := env.NewActor(t, "patients:merge")

	env.Repo.identifiers["ident-1"] = patients.Identifier{ID: "ident-1", PatientID: away, IDType: "National ID", IDValue: "123"}

	rec := env.JSON(t, http.MethodPost, "/patients/merge", actorToken, patients.MergePatientsRequest{
		KeepPatientID: keep, AwayPatientID: away, Reason: "Same person, confirmed by phone",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	awayPatient, err := env.Repo.GetPatientByID(nil, away) //nolint:staticcheck
	if err != nil {
		t.Fatal(err)
	}
	if awayPatient.Status != patients.StatusMerged || awayPatient.MergedIntoID == nil || *awayPatient.MergedIntoID != keep {
		t.Fatalf("expected away patient marked merged into keep, got %+v", awayPatient)
	}
	if env.Repo.identifiers["ident-1"].PatientID != keep {
		t.Fatalf("expected identifier reassigned to keep patient, got %+v", env.Repo.identifiers["ident-1"])
	}
}

func TestMergePatients_SelfMergeReturns409(t *testing.T) {
	env := Setup(t)
	target := env.CreatePatient(t, nil)
	_, actorToken := env.NewActor(t, "patients:merge")

	rec := env.JSON(t, http.MethodPost, "/patients/merge", actorToken, patients.MergePatientsRequest{
		KeepPatientID: target, AwayPatientID: target, Reason: "oops",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for self-merge, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMergePatients_AlreadyMergedReturns409(t *testing.T) {
	env := Setup(t)
	keep := env.CreatePatient(t, nil)
	away := env.CreatePatient(t, func(p *patients.Patient) { p.Status = patients.StatusMerged })
	_, actorToken := env.NewActor(t, "patients:merge")

	rec := env.JSON(t, http.MethodPost, "/patients/merge", actorToken, patients.MergePatientsRequest{
		KeepPatientID: keep, AwayPatientID: away, Reason: "already merged before",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 merging an already-merged record, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMergePatients_WithoutPermissionReturns403(t *testing.T) {
	env := Setup(t)
	keep := env.CreatePatient(t, nil)
	away := env.CreatePatient(t, nil)
	_, actorToken := env.NewActor(t, "patients:write") // write, not merge

	rec := env.JSON(t, http.MethodPost, "/patients/merge", actorToken, patients.MergePatientsRequest{
		KeepPatientID: keep, AwayPatientID: away, Reason: "test",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddAndRemoveIdentifier(t *testing.T) {
	env := Setup(t)
	target := env.CreatePatient(t, nil)
	_, actorToken := env.NewActor(t, "patients:write")

	rec := env.JSON(t, http.MethodPost, "/patients/"+target+"/identifiers", actorToken, patients.AddIdentifierRequest{
		IDType: "National ID", IDValue: "12345678",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var identifier patients.IdentifierResponse
	Decode(t, rec, &identifier)
	if identifier.VerifiedLabel != "Pending verification" {
		t.Fatalf("expected new identifiers to start pending verification, got %q", identifier.VerifiedLabel)
	}

	rec2 := env.JSON(t, http.MethodDelete, "/patients/"+target+"/identifiers/"+identifier.ID, actorToken, nil)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestSetConsent_IsIdempotent(t *testing.T) {
	env := Setup(t)
	target := env.CreatePatient(t, nil)
	_, actorToken := env.NewActor(t, "patients:write")

	rec := env.JSON(t, http.MethodPut, "/patients/"+target+"/consents/data_sharing", actorToken, patients.SetConsentRequest{Granted: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var consent patients.ConsentResponse
	Decode(t, rec, &consent)
	if !consent.Granted {
		t.Fatalf("expected granted=true, got %+v", consent)
	}

	rec2 := env.JSON(t, http.MethodPut, "/patients/"+target+"/consents/data_sharing", actorToken, patients.SetConsentRequest{Granted: false})
	Decode(t, rec2, &consent)
	if consent.Granted {
		t.Fatalf("expected granted=false after revoking, got %+v", consent)
	}
}

func strPtr(s string) *string { return &s }
