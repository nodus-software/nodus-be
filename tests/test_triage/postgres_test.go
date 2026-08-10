package test_triage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	pgxdriver "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"nodus-health/internal/auth"
	"nodus-health/internal/clinical"
	clinicalpostgres "nodus-health/internal/clinical/postgres"
	"nodus-health/internal/email"
	"nodus-health/internal/organizations"
	"nodus-health/internal/platform/db"
	migrationfiles "nodus-health/migrations"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/utility"
)

type stubMailer struct{}

func (stubMailer) SendHTML(context.Context, string, string, string, string) error { return nil }

func mustUUID(t *testing.T) string {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func migrateDatabase(t *testing.T, dsn string, version uint) {
	t.Helper()
	source, err := iofs.New(migrationfiles.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	driver, err := pgxdriver.WithInstance(database, &pgxdriver.Config{})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err = runner.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate database to version %d: %v", version, err)
	}
	_, _ = runner.Close()
}

func temporaryDatabase(t *testing.T) string {
	t.Helper()
	baseDSN := os.Getenv("TEST_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL triage integration test")
	}
	ctx := context.Background()
	baseConfig, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		t.Fatal(err)
	}
	adminPool, err := pgxpool.NewWithConfig(ctx, baseConfig)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := "nodus_triage_" + strings.ReplaceAll(mustUUID(t), "-", "")
	if _, err = adminPool.Exec(ctx, `CREATE DATABASE "`+databaseName+`"`); err != nil {
		adminPool.Close()
		t.Fatalf("create temporary database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+databaseName+`" WITH (FORCE)`)
		adminPool.Close()
	})
	testConfig := baseConfig.Copy()
	testConfig.ConnConfig.Database = databaseName
	return testConfig.ConnString()
}

func runtimeDatabase(t *testing.T, ownerDSN string) string {
	t.Helper()
	ctx := context.Background()
	ownerPool, err := pgxpool.New(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerPool.Close()
	_, err = ownerPool.Exec(ctx, `DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='nodus_triage_test_app') THEN
			CREATE ROLE nodus_triage_test_app LOGIN PASSWORD 'nodus';
		END IF;
	END $$;
	ALTER ROLE nodus_triage_test_app NOSUPERUSER NOBYPASSRLS;
	GRANT USAGE ON SCHEMA public TO nodus_triage_test_app;
	GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO nodus_triage_test_app;
	GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO nodus_triage_test_app;`)
	if err != nil {
		t.Fatalf("configure runtime test role: %v", err)
	}
	parsed, err := url.Parse(ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword("nodus_triage_test_app", "nodus")
	return parsed.String()
}

func TestPostgresTriageDefaultsStartAndRouting(t *testing.T) {
	dsn := temporaryDatabase(t)
	migrateDatabase(t, dsn, 19)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	legacyTenantID := mustUUID(t)
	if _, err = pool.Exec(ctx, `INSERT INTO organizations(id,organization_name,slug,status) VALUES($1,'Existing Clinic',$2,'active')`, legacyTenantID, "existing-"+legacyTenantID[:8]); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	pool.Close()

	migrateDatabase(t, dsn, 22)
	runtimeDSN := runtimeDatabase(t, dsn)
	pool, err = pgxpool.New(ctx, runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	assertTenantDefaults(t, pool, legacyTenantID)

	organizationService := organizations.NewService(
		pool, stubMailer{}, email.NewRenderer(email.CommonData{}), "http://localhost", 4,
		auth.PasswordPolicy{}, logger.NewLogger(),
	)
	createdOrganization, err := organizationService.Register(ctx, organizations.RegisterRequest{
		OrganizationName: "New Clinic", Slug: "new-clinic-" + legacyTenantID[:6],
		AdminFullName: "Founding Admin", AdminEmail: "admin-" + legacyTenantID[:6] + "@example.test",
	})
	if err != nil {
		t.Fatalf("register organization: %v", err)
	}
	assertTenantDefaults(t, pool, createdOrganization.ID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", legacyTenantID); err != nil {
		t.Fatal(err)
	}
	tenantContext := db.WithExecutor(ctx, tx)
	repository := clinicalpostgres.New(pool)
	service := clinical.NewService(repository, discardAudit{})

	actorID := mustUUID(t)
	patientID := mustUUID(t)
	visitID := mustUUID(t)
	entryID := mustUUID(t)
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,full_name,username,email,status) VALUES($1,'Triage Nurse',$2,$3,'active')`, actorID, "nurse-"+actorID[:8], "nurse-"+actorID[:8]+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO patients(id,mrn,full_name,gender) VALUES($1,$2,'Test Patient','unknown')`, patientID, "MRN-"+patientID[:8]); err != nil {
		t.Fatal(err)
	}
	var triageQueueID string
	if err = tx.QueryRow(ctx, "SELECT id FROM queues WHERE code='OPD-TRIAGE-Q'").Scan(&triageQueueID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO clinical_visits(id,patient_id,visit_type,status,created_by) VALUES($1,$2,'outpatient','active',$3)`, visitID, patientID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status) VALUES($1,$2,'visit',$3,$4,'waiting')`, entryID, triageQueueID, visitID, patientID); err != nil {
		t.Fatal(err)
	}
	called, err := service.Transition(tenantContext, actorID, entryID, clinical.TransitionRequest{Status: "called"})
	if err != nil {
		t.Fatalf("call triage queue entry: %v", err)
	}
	if called.Status != "called" {
		t.Fatalf("expected called queue entry, got %+v", called)
	}
	var calledAtSet bool
	if err = tx.QueryRow(ctx, "SELECT called_at IS NOT NULL FROM queue_entries WHERE id=$1", entryID).Scan(&calledAtSet); err != nil {
		t.Fatal(err)
	}
	if !calledAtSet {
		t.Fatal("expected called_at to be set")
	}

	started, err := service.StartTriage(tenantContext, actorID, entryID)
	if err != nil {
		t.Fatalf("start triage: %v", err)
	}
	if started.QueueEntry.Status != "in_service" || started.Form.TemplateVersion.Status != "published" {
		t.Fatalf("unexpected triage start: %+v", started)
	}
	if _, err = service.StartTriage(tenantContext, actorID, entryID); !errors.Is(err, clinical.ErrConflict) {
		t.Fatalf("expected repeated triage start conflict, got %v", err)
	}
	if _, err = tx.Exec(ctx, "UPDATE clinical_encounter_forms SET status='submitted',submitted_at=now() WHERE encounter_id=$1", started.Encounter.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := service.CompleteEncounter(tenantContext, actorID, started.Encounter.ID, clinical.CompleteEncounterRequest{})
	if err != nil {
		t.Fatalf("complete triage: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("expected completed encounter, got %+v", completed)
	}
	var queueCode, queueStatus string
	if err = tx.QueryRow(ctx, `SELECT q.code,e.status::text FROM queue_entries e JOIN queues q ON q.id=e.queue_id
		WHERE e.subject_type='visit' AND e.subject_id=$1 AND e.status IN ('waiting','called','in_service','paused')`, visitID).Scan(&queueCode, &queueStatus); err != nil {
		t.Fatal(err)
	}
	if queueCode != "OPD-CONSULT-Q" || queueStatus != "waiting" {
		t.Fatalf("expected waiting consultation entry, got queue=%s status=%s", queueCode, queueStatus)
	}
	var completedHistory int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM queue_entry_history WHERE queue_entry_id=$1 AND to_status='completed'`, entryID).Scan(&completedHistory); err != nil {
		t.Fatal(err)
	}
	if completedHistory != 1 {
		t.Fatalf("expected triage completion history, got %d rows", completedHistory)
	}

	var consultationEntryID string
	if err = tx.QueryRow(ctx, `SELECT e.id FROM queue_entries e JOIN queues q ON q.id=e.queue_id
		WHERE e.subject_id=$1 AND q.code='OPD-CONSULT-Q'`, visitID).Scan(&consultationEntryID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Transition(tenantContext, actorID, consultationEntryID, clinical.TransitionRequest{Status: "in_service"}); !errors.Is(err, clinical.ErrEncounterStartRequired) {
		t.Fatalf("expected a bare in_service transition on the consultation queue to be refused, got %v", err)
	}
	if _, err = service.Transition(tenantContext, actorID, consultationEntryID, clinical.TransitionRequest{Status: "called"}); err != nil {
		t.Fatalf("call consultation queue entry: %v", err)
	}
	consultation, err := service.StartConsultation(tenantContext, actorID, consultationEntryID)
	if err != nil {
		t.Fatalf("start consultation: %v", err)
	}
	if consultation.QueueEntry.Status != "in_service" || consultation.Encounter.EncounterType != "consultation" || consultation.Encounter.Status != "in_progress" {
		t.Fatalf("unexpected consultation start: %+v", consultation)
	}
	if consultation.Form.Status != "draft" || consultation.Form.TemplateVersion.Status != "published" || len(consultation.Form.TemplateVersion.Definition.Sections) == 0 {
		t.Fatalf("expected a pinned published consultation form, got %+v", consultation.Form)
	}
	if consultation.Encounter.ServicePointID == nil {
		t.Fatal("expected the consultation encounter to be staffed at the queue's service point")
	}
	visible, err := service.ListQueueEntries(tenantContext, consultation.QueueEntry.QueueID)
	if err != nil {
		t.Fatal(err)
	}
	onBoard := false
	for _, entry := range visible {
		if entry.ID == consultationEntryID {
			onBoard = true
		}
	}
	if !onBoard {
		t.Fatal("a patient in consultation must stay on the queue board until the encounter completes")
	}
	if _, err = service.StartConsultation(tenantContext, actorID, consultationEntryID); !errors.Is(err, clinical.ErrConflict) {
		t.Fatalf("expected repeated consultation start conflict, got %v", err)
	}

	// A patient stranded in_service without an encounter — the state a bare
	// transition used to produce — must still be recoverable.
	strandedPatientID := mustUUID(t)
	strandedVisitID := mustUUID(t)
	strandedEntryID := mustUUID(t)
	var consultationQueueID, consultationServicePointID string
	if err = tx.QueryRow(ctx, "SELECT id,service_point_id FROM queues WHERE code='OPD-CONSULT-Q'").Scan(&consultationQueueID, &consultationServicePointID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO patients(id,mrn,full_name,gender) VALUES($1,$2,'Stranded Patient','unknown')`, strandedPatientID, "MRN-"+strandedPatientID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO clinical_visits(id,patient_id,visit_type,status,created_by) VALUES($1,$2,'outpatient','active',$3)`, strandedVisitID, strandedPatientID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO clinical_encounters(id,visit_id,service_point_id,encounter_type,status,clinician_id,started_at,ended_at)
		VALUES($1,$2,$3,'triage','completed',$4,now(),now())`, mustUUID(t), strandedVisitID, consultationServicePointID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,called_at) VALUES($1,$2,'visit',$3,$4,'in_service',now())`, strandedEntryID, consultationQueueID, strandedVisitID, strandedPatientID); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.StartConsultation(tenantContext, actorID, strandedEntryID)
	if err != nil {
		t.Fatalf("recover stranded consultation entry: %v", err)
	}
	if recovered.QueueEntry.Status != "in_service" || recovered.Encounter.EncounterType != "consultation" || recovered.Form.TemplateVersion.Status != "published" {
		t.Fatalf("unexpected recovered consultation: %+v", recovered)
	}

	// Consultation may not be started before triage has been completed.
	untriagedPatientID := mustUUID(t)
	untriagedVisitID := mustUUID(t)
	untriagedEntryID := mustUUID(t)
	if _, err = tx.Exec(ctx, `INSERT INTO patients(id,mrn,full_name,gender) VALUES($1,$2,'Untriaged Patient','unknown')`, untriagedPatientID, "MRN-"+untriagedPatientID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO clinical_visits(id,patient_id,visit_type,status,created_by) VALUES($1,$2,'outpatient','active',$3)`, untriagedVisitID, untriagedPatientID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,called_at) VALUES($1,$2,'visit',$3,$4,'called',now())`, untriagedEntryID, consultationQueueID, untriagedVisitID, untriagedPatientID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.StartConsultation(tenantContext, actorID, untriagedEntryID); !errors.Is(err, clinical.ErrInvalidTransition) {
		t.Fatalf("expected consultation before triage to be refused, got %v", err)
	}
	if _, err = service.StartTriage(tenantContext, actorID, consultationEntryID); !errors.Is(err, clinical.ErrInvalidInput) {
		t.Fatalf("expected triage start on a consultation queue to be refused, got %v", err)
	}

	rollbackPatientID := mustUUID(t)
	rollbackVisitID := mustUUID(t)
	rollbackEntryID := mustUUID(t)
	if _, err = tx.Exec(ctx, `INSERT INTO patients(id,mrn,full_name,gender) VALUES($1,$2,'Routing Rollback Patient','unknown')`, rollbackPatientID, "MRN-"+rollbackPatientID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO clinical_visits(id,patient_id,visit_type,status,created_by) VALUES($1,$2,'outpatient','active',$3)`, rollbackVisitID, rollbackPatientID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO queue_entries(id,queue_id,subject_type,subject_id,patient_id,status,called_at) VALUES($1,$2,'visit',$3,$4,'called',now())`, rollbackEntryID, triageQueueID, rollbackVisitID, rollbackPatientID); err != nil {
		t.Fatal(err)
	}
	rollbackStart, err := service.StartTriage(tenantContext, actorID, rollbackEntryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE clinical_encounter_forms SET status='submitted',submitted_at=now() WHERE encounter_id=$1", rollbackStart.Encounter.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE queue_routing_rules SET active=false WHERE event_type='encounter.completed' AND encounter_type='triage'"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.CompleteEncounter(tenantContext, actorID, rollbackStart.Encounter.ID, clinical.CompleteEncounterRequest{}); !errors.Is(err, clinical.ErrRoutingMissing) {
		t.Fatalf("expected missing routing error, got %v", err)
	}
	var encounterStatus, entryStatus string
	if err = tx.QueryRow(ctx, "SELECT status::text FROM clinical_encounters WHERE id=$1", rollbackStart.Encounter.ID).Scan(&encounterStatus); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, "SELECT status::text FROM queue_entries WHERE id=$1", rollbackEntryID).Scan(&entryStatus); err != nil {
		t.Fatal(err)
	}
	if encounterStatus != "in_progress" || entryStatus != "in_service" {
		t.Fatalf("routing failure partially committed: encounter=%s entry=%s", encounterStatus, entryStatus)
	}
}

func assertTenantDefaults(t *testing.T, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenantID); err != nil {
		t.Fatal(err)
	}
	var currentUser, tenantSetting string
	if err = tx.QueryRow(ctx, "SELECT current_user,current_setting('app.tenant_id',true)").Scan(&currentUser, &tenantSetting); err != nil {
		t.Fatal(err)
	}
	if currentUser != "nodus_triage_test_app" || tenantSetting != tenantID {
		t.Fatalf("unexpected database security context: user=%s tenant=%s", currentUser, tenantSetting)
	}
	var templates, rules, visibleTenants int
	if err = tx.QueryRow(ctx, "SELECT count(DISTINCT tenant_id) FROM clinical_templates").Scan(&visibleTenants); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM clinical_templates t JOIN clinical_template_versions v ON v.template_id=t.id
		WHERE t.tenant_id=$1 AND v.tenant_id=$1 AND t.encounter_type='triage' AND t.is_default AND t.archived_at IS NULL AND v.status='published'`, tenantID).Scan(&templates); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM queue_routing_rules r JOIN queues q ON q.id=r.target_queue_id
		WHERE r.tenant_id=$1 AND q.tenant_id=$1 AND r.event_type='encounter.completed' AND r.encounter_type='triage' AND q.code='OPD-CONSULT-Q' AND r.active`, tenantID).Scan(&rules); err != nil {
		t.Fatal(err)
	}
	if templates != 1 || rules != 1 || visibleTenants != 1 {
		t.Fatalf("tenant %s defaults as %s (setting %s): templates=%d consultation_rules=%d visible_tenants=%d", tenantID, currentUser, tenantSetting, templates, rules, visibleTenants)
	}
}
