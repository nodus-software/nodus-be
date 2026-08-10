package test_triage

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
	"nodus-health/internal/clinical"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

const testJWTSecret = "triage-test-jwt-secret"

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

type memoryRepo struct {
	clinical.Repository
	entry           clinical.QueueEntry
	encounter       *clinical.Encounter
	form            *clinical.EncounterForm
	startCalls      int
	queueKind       string
	triageCompleted bool
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{queueKind: "triage", entry: clinical.QueueEntry{
		ID: "entry-1", QueueID: "triage-queue", QueueName: "Outpatient Triage Queue",
		ServicePointKind: "triage",
		SubjectType:      "visit", SubjectID: "visit-1", PatientID: "patient-1",
		PatientName: "Test Patient", PatientMRN: "MRN-001", Status: "waiting",
		JoinedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}
}

func (r *memoryRepo) GetQueueEntry(context.Context, string) (*clinical.QueueEntry, error) {
	x := r.entry
	return &x, nil
}

func (r *memoryRepo) TransitionQueueEntry(_ context.Context, entry clinical.QueueEntry, status, target string, _, _ *string, _ bool) (*clinical.QueueEntry, error) {
	if entry.Status != r.entry.Status {
		return nil, clinical.ErrConflict
	}
	r.entry.Status = status
	r.entry.QueueID = target
	r.entry.UpdatedAt = time.Now().UTC()
	x := r.entry
	return &x, nil
}

func (r *memoryRepo) StartEncounter(_ context.Context, entryID string, encounter clinical.Encounter, formID, actor string) (*clinical.EncounterStart, error) {
	if entryID != r.entry.ID {
		return nil, clinical.ErrNotFound
	}
	if r.entry.SubjectType != "visit" || r.queueKind != encounter.EncounterType {
		return nil, clinical.ErrInvalidInput
	}
	if (r.entry.Status != "called" && r.entry.Status != "in_service") || r.encounter != nil {
		return nil, clinical.ErrConflict
	}
	if encounter.EncounterType == "consultation" && !r.triageCompleted {
		return nil, clinical.ErrInvalidTransition
	}
	definition := clinical.DefaultTriageTemplate
	if encounter.EncounterType == "consultation" {
		definition = clinical.DefaultConsultationTemplate
	}
	r.startCalls++
	r.entry.Status = "in_service"
	encounter.VisitID = r.entry.SubjectID
	r.encounter = &encounter
	r.form = &clinical.EncounterForm{
		ID: formID, EncounterID: encounter.ID, Status: "draft", Revision: 0,
		Answers: map[string]json.RawMessage{},
		TemplateVersion: clinical.ClinicalTemplateVersion{
			ID: encounter.EncounterType + "-template-v1", TemplateID: encounter.EncounterType + "-template", Version: 1,
			Status: "published", Definition: definition,
		},
	}
	return &clinical.EncounterStart{QueueEntry: r.entry, Encounter: encounter, Form: *r.form}, nil
}

func (r *memoryRepo) GetEncounter(_ context.Context, id string) (*clinical.Encounter, error) {
	if r.encounter == nil || r.encounter.ID != id {
		return nil, clinical.ErrNotFound
	}
	x := *r.encounter
	return &x, nil
}

func (r *memoryRepo) GetEncounterForm(_ context.Context, id string) (*clinical.EncounterForm, error) {
	if r.form == nil || r.form.EncounterID != id {
		return nil, clinical.ErrNotFound
	}
	x := *r.form
	return &x, nil
}

type Env struct {
	Router     http.Handler
	Repo       *memoryRepo
	authorizer *stubAuthorizer
}

func Setup(t *testing.T) *Env {
	t.Helper()
	repo := newMemoryRepo()
	authorizer := &stubAuthorizer{permissions: map[string][]string{}}
	service := clinical.NewService(repo, discardAudit{})
	handler := clinical.NewHandler(service, authorizer, testJWTSecret, logger.NewLogger())
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	return &Env{Router: router, Repo: repo, authorizer: authorizer}
}

func (e *Env) NewActor(t *testing.T, permissions ...string) string {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	e.authorizer.mu.Lock()
	e.authorizer.permissions[id] = permissions
	e.authorizer.mu.Unlock()
	token, _, err := security.IssueAccessToken(testJWTSecret, time.Hour, id, "session")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (e *Env) JSON(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload string
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = string(encoded)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	e.Router.ServeHTTP(recorder, req)
	return recorder
}

func decodeData(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response envelope: %v; body=%s", err, recorder.Body.String())
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("decode response data: %v; body=%s", err, recorder.Body.String())
	}
}
