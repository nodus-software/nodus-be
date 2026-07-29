package test_patients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"nodus-health/internal/audit"
	"nodus-health/internal/patients"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

type discardAudit struct{}

func (discardAudit) Record(context.Context, audit.Entry) error { return nil }

type stubAuthorizer struct {
	mu          sync.Mutex
	permissions map[string][]string
}

func (a *stubAuthorizer) Authorize(_ context.Context, userID, _ string) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.permissions[userID], nil
}

func (a *stubAuthorizer) grant(userID string, permissions []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissions[userID] = permissions
}

const testJWTSecret = "test-jwt-secret"

type Env struct {
	Router     http.Handler
	Repo       *memoryRepo
	authorizer *stubAuthorizer
}

func Setup(t *testing.T) *Env {
	t.Helper()
	repo := newMemoryRepo()
	service := patients.NewService(repo, discardAudit{}, logger.NewLogger(), patients.Config{})
	authorizer := &stubAuthorizer{permissions: map[string][]string{}}
	handler := patients.NewHandler(service, authorizer, testJWTSecret, logger.NewLogger())
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return &Env{Router: r, Repo: repo, authorizer: authorizer}
}

func (e *Env) NewActor(t *testing.T, permissions ...string) (userID, accessToken string) {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	e.authorizer.grant(id, permissions)
	token, _, err := security.IssueAccessToken(testJWTSecret, time.Hour, id, "session")
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}

// CreatePatient adds a patient row directly to the repo, bypassing
// registration - useful for tests that just need an existing record to act
// on (merge, mark-deceased, corrections, ...).
func (e *Env) CreatePatient(t *testing.T, mutate func(*patients.Patient)) string {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	p := patients.Patient{
		ID: id, MRN: "MRN-" + id[:8], FullName: "Test Patient", Gender: patients.GenderUnknown,
		Status: patients.StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if mutate != nil {
		mutate(&p)
	}
	e.Repo.patients[id] = p
	return id
}

func (e *Env) JSON(t *testing.T, method, path, accessToken string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(data))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	rec := httptest.NewRecorder()
	e.Router.ServeHTTP(rec, req)
	return rec
}

func Decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	data := envelope.Data
	if len(data) == 0 {
		data = rec.Body.Bytes()
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
}

// memoryRepo is a complete, deliberately small Repository implementation
// giving handler tests deterministic isolated state. It does not exercise
// real SQL (tenant RLS, pg_trgm scoring) - FindDuplicateCandidates returns
// a preset list a test can populate directly, the same way other domains'
// memoryRepo fakes stub out DB-only behavior (e.g. CountOtherActiveSuperusers
// in tests/test_users).
type memoryRepo struct {
	sync.Mutex
	patients            map[string]patients.Patient
	identifiers         map[string]patients.Identifier
	consents            map[string]patients.Consent // key: patientID+"|"+scope
	corrections         map[string]patients.Correction
	activity            map[string][]patients.ActivityEntry
	duplicateCandidates []patients.DuplicateCandidate
	mrnSeq              int
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		patients: map[string]patients.Patient{}, identifiers: map[string]patients.Identifier{},
		consents: map[string]patients.Consent{}, corrections: map[string]patients.Correction{},
		activity: map[string][]patients.ActivityEntry{},
	}
}

func (r *memoryRepo) ListPatients(_ context.Context, filter patients.ListPatientsFilter) ([]patients.Patient, int, error) {
	var out []patients.Patient
	for _, p := range r.patients {
		if len(filter.Status) > 0 && !containsStr(filter.Status, string(p.Status)) {
			continue
		}
		out = append(out, p)
	}
	total := len(out)
	page, perPage := filter.Page, filter.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	start := (page - 1) * perPage
	if start > len(out) {
		start = len(out)
	}
	end := start + perPage
	if end > len(out) {
		end = len(out)
	}
	return out[start:end], total, nil
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (r *memoryRepo) GetPatientByID(_ context.Context, id string) (*patients.Patient, error) {
	p, ok := r.patients[id]
	if !ok {
		return nil, patients.ErrPatientNotFound
	}
	v := p
	return &v, nil
}

func (r *memoryRepo) IssueMRN(context.Context) (string, error) {
	r.mrnSeq++
	return "MRN-TEST-" + itoa(r.mrnSeq), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func (r *memoryRepo) InsertPatient(_ context.Context, p patients.Patient) error {
	if p.Status == "" {
		p.Status = patients.StatusActive
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	p.UpdatedAt = p.CreatedAt
	r.patients[p.ID] = p
	return nil
}

func (r *memoryRepo) FindDuplicateCandidates(context.Context, string, *time.Time, *string, *string) ([]patients.DuplicateCandidate, error) {
	return r.duplicateCandidates, nil
}

func (r *memoryRepo) UpdateContact(_ context.Context, patientID string, phone, address *string) error {
	p, ok := r.patients[patientID]
	if !ok {
		return patients.ErrPatientNotFound
	}
	p.Phone, p.Address = phone, address
	r.patients[patientID] = p
	return nil
}

func (r *memoryRepo) MarkDeceased(_ context.Context, patientID string, dateOfDeath time.Time) error {
	p, ok := r.patients[patientID]
	if !ok {
		return patients.ErrPatientNotFound
	}
	p.Status = patients.StatusDeceased
	p.DateOfDeath = &dateOfDeath
	r.patients[patientID] = p
	return nil
}

func (r *memoryRepo) SetMergedInto(_ context.Context, awayID, keepID string) error {
	p, ok := r.patients[awayID]
	if !ok {
		return patients.ErrPatientNotFound
	}
	p.Status = patients.StatusMerged
	p.MergedIntoID = &keepID
	r.patients[awayID] = p
	return nil
}

func (r *memoryRepo) ApplyCorrectionField(_ context.Context, patientID, field, value string) error {
	p, ok := r.patients[patientID]
	if !ok {
		return patients.ErrPatientNotFound
	}
	switch field {
	case "full_name":
		p.FullName = value
	case "dob":
		t, err := time.Parse("2006-01-02", value)
		if err != nil {
			return patients.ErrInvalidDate
		}
		p.DOB = &t
	case "gender":
		p.Gender = patients.Gender(value)
	case "national_id":
		v := value
		p.NationalID = &v
	}
	r.patients[patientID] = p
	return nil
}

func (r *memoryRepo) InsertCorrection(_ context.Context, c patients.Correction) error {
	c.Status = patients.CorrectionPending
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	r.corrections[c.ID] = c
	return nil
}

func (r *memoryRepo) ListCorrections(_ context.Context, patientID string) ([]patients.Correction, error) {
	var out []patients.Correction
	for _, c := range r.corrections {
		if c.PatientID == patientID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *memoryRepo) GetCorrectionByID(_ context.Context, id string) (*patients.Correction, error) {
	c, ok := r.corrections[id]
	if !ok {
		return nil, patients.ErrCorrectionNotFound
	}
	v := c
	return &v, nil
}

func (r *memoryRepo) DecideCorrection(_ context.Context, correctionID string, status patients.CorrectionStatus, decidedBy string, decidedAt time.Time, note *string) error {
	c, ok := r.corrections[correctionID]
	if !ok {
		return patients.ErrCorrectionNotFound
	}
	c.Status = status
	c.DecidedBy = &decidedBy
	c.DecidedAt = &decidedAt
	c.DecisionNote = note
	r.corrections[correctionID] = c
	return nil
}

func (r *memoryRepo) AddIdentifier(_ context.Context, i patients.Identifier) error {
	i.CreatedAt = time.Now()
	r.identifiers[i.ID] = i
	return nil
}

func (r *memoryRepo) RemoveIdentifier(_ context.Context, _, identifierID string) error {
	delete(r.identifiers, identifierID)
	return nil
}

func (r *memoryRepo) ListIdentifiers(_ context.Context, patientID string) ([]patients.Identifier, error) {
	var out []patients.Identifier
	for _, i := range r.identifiers {
		if i.PatientID == patientID {
			out = append(out, i)
		}
	}
	return out, nil
}

func (r *memoryRepo) CountIdentifiers(_ context.Context, patientID string) (int, error) {
	n := 0
	for _, i := range r.identifiers {
		if i.PatientID == patientID {
			n++
		}
	}
	return n, nil
}

func (r *memoryRepo) ListConsents(_ context.Context, patientID string) ([]patients.Consent, error) {
	var out []patients.Consent
	for _, c := range r.consents {
		if c.PatientID == patientID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *memoryRepo) UpsertConsent(_ context.Context, patientID, scope string, granted bool, at time.Time) (*patients.Consent, error) {
	key := patientID + "|" + scope
	c, ok := r.consents[key]
	if !ok {
		id, err := utility.GenerateUUID()
		if err != nil {
			return nil, err
		}
		c = patients.Consent{ID: id, PatientID: patientID, Scope: scope}
	}
	c.Granted = granted
	if granted {
		c.GrantedAt = &at
	} else {
		c.RevokedAt = &at
	}
	r.consents[key] = c
	v := c
	return &v, nil
}

func (r *memoryRepo) ListActivity(_ context.Context, patientID string) ([]patients.ActivityEntry, error) {
	return r.activity[patientID], nil
}

func (r *memoryRepo) AddActivityEntry(_ context.Context, e patients.ActivityEntry) error {
	id, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	e.ID = id
	e.CreatedAt = time.Now()
	r.activity[e.PatientID] = append([]patients.ActivityEntry{e}, r.activity[e.PatientID]...)
	return nil
}

func (r *memoryRepo) ReassignIdentifiers(_ context.Context, fromID, toID string) error {
	for id, i := range r.identifiers {
		if i.PatientID == fromID {
			i.PatientID = toID
			r.identifiers[id] = i
		}
	}
	return nil
}

func (r *memoryRepo) ReassignConsents(_ context.Context, fromID, toID string) error {
	for key, c := range r.consents {
		if c.PatientID != fromID {
			continue
		}
		newKey := toID + "|" + c.Scope
		if _, exists := r.consents[newKey]; exists {
			continue
		}
		c.PatientID = toID
		r.consents[newKey] = c
		delete(r.consents, key)
	}
	return nil
}

func (r *memoryRepo) ReassignCorrections(_ context.Context, fromID, toID string) error {
	for id, c := range r.corrections {
		if c.PatientID == fromID {
			c.PatientID = toID
			r.corrections[id] = c
		}
	}
	return nil
}

func (r *memoryRepo) ReassignActivity(_ context.Context, fromID, toID string) error {
	r.activity[toID] = append(r.activity[toID], r.activity[fromID]...)
	delete(r.activity, fromID)
	return nil
}

func (r *memoryRepo) WithinTx(_ context.Context, fn func(patients.Repository) error) error {
	return fn(r)
}
