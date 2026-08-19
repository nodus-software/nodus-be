package organizations

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/auth"
	"nodus-health/internal/clinical"
	"nodus-health/internal/email"
	"nodus-health/internal/middleware"
	"nodus-health/internal/tenant"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/response"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9-]{3,32}$`)
var errInvalidSlug = errors.New("slug must be 3-32 lowercase letters, numbers, or hyphens")
var errDiscoveryRateLimited = errors.New("too many organization discovery requests")
var errDiscoveryAccountNotFound = errors.New("no active account was found for that email")
var errDiscoveryTokenInvalid = errors.New("invalid or expired sign-in handoff")

var mandatoryReservedSlugs = []string{"app", "api", "www", "admin", "mail", "info", "noreply", "support", "status"}

type Config struct {
	BaseURL          string
	TenantBaseDomain string
	TenantURLScheme  string
	TenantURLPort    string
	ReservedSlugs    []string
	BcryptCost       int
	PasswordPolicy   auth.PasswordPolicy
}

type Organization struct {
	ID               string    `json:"id"`
	OrganizationName string    `json:"organization_name"`
	Slug             string    `json:"slug"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

type RegisterRequest struct {
	OrganizationName string `json:"organization_name"`
	Slug             string `json:"slug"`
	AdminFullName    string `json:"admin_full_name"`
	AdminEmail       string `json:"admin_email"`
}

type UpdateRequest struct {
	OrganizationName *string `json:"organization_name"`
	Status           *string `json:"status"`
}

type AcceptRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type DiscoveryRequest struct {
	Email string `json:"email"`
}

type DiscoveryVerifyRequest struct {
	Token string `json:"token"`
	Slug  string `json:"slug"`
}

type DiscoveryResponse struct {
	Action   string `json:"action"`
	LoginURL string `json:"login_url,omitempty"`
}

type DiscoveryVerifyResponse struct {
	Email string `json:"email"`
}

type Service struct {
	pool             *pgxpool.Pool
	baseURL          string
	bcryptCost       int
	policy           auth.PasswordPolicy
	log              *logger.Logger
	email            *email.Renderer
	tenantBaseDomain string
	tenantURLScheme  string
	tenantURLPort    string
	reservedSlugs    map[string]struct{}
}

func NewService(pool *pgxpool.Pool, renderer *email.Renderer, cfg Config, log *logger.Logger) *Service {
	reserved := make(map[string]struct{}, len(mandatoryReservedSlugs)+len(cfg.ReservedSlugs))
	for _, slug := range append(mandatoryReservedSlugs, cfg.ReservedSlugs...) {
		reserved[strings.ToLower(strings.TrimSpace(slug))] = struct{}{}
	}
	return &Service{pool: pool, baseURL: cfg.BaseURL, bcryptCost: cfg.BcryptCost,
		policy: cfg.PasswordPolicy, log: log, email: renderer, tenantBaseDomain: cfg.TenantBaseDomain,
		tenantURLScheme: cfg.TenantURLScheme, tenantURLPort: cfg.TenantURLPort, reservedSlugs: reserved}
}

func (s *Service) isReservedSlug(slug string) bool {
	_, reserved := s.reservedSlugs[strings.ToLower(strings.TrimSpace(slug))]
	return reserved
}

func (s *Service) ValidateReservedSlugs(ctx context.Context) error {
	slugs := make([]string, 0, len(s.reservedSlugs))
	for slug := range s.reservedSlugs {
		slugs = append(slugs, slug)
	}
	var conflict string
	err := s.pool.QueryRow(ctx, `SELECT slug FROM organizations WHERE slug = ANY($1::text[]) LIMIT 1`, slugs).Scan(&conflict)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("organization slug %q is reserved; rename it before starting the API", conflict)
}

func (s *Service) tenantURL(slug, path string) string {
	port := ""
	if s.tenantURLPort != "" {
		port = ":" + s.tenantURLPort
	}
	return fmt.Sprintf("%s://%s.%s%s%s", s.tenantURLScheme, slug, s.tenantBaseDomain, port, path)
}

func normalizeEmail(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	return value, err == nil && strings.EqualFold(parsed.Address, value)
}

func (s *Service) RequestOrganizationDiscovery(ctx context.Context, emailAddress, ip string) (*DiscoveryResponse, error) {
	emailAddress, valid := normalizeEmail(emailAddress)
	if !valid {
		return nil, errors.New("a valid email is required")
	}
	emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(emailAddress)))
	var emailCount, ipCount int
	err := s.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM organization_discovery_requests WHERE email_hash=$1 AND created_at>now()-interval '1 hour'),
		(SELECT count(*) FROM organization_discovery_requests WHERE ip_address=$2 AND created_at>now()-interval '1 hour')`, emailHash, ip).Scan(&emailCount, &ipCount)
	if err != nil {
		return nil, err
	}
	if emailCount >= 5 || ipCount >= 20 {
		return nil, errDiscoveryRateLimited
	}
	requestID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO organization_discovery_requests(id,email_hash,ip_address) VALUES($1,$2,$3)`, requestID, emailHash, ip); err != nil {
		return nil, err
	}
	var slug string
	if err = s.pool.QueryRow(ctx, `SELECT slug FROM discover_active_organizations($1) LIMIT 1`, emailAddress).Scan(&slug); err == nil {
		rawToken, err := security.GenerateToken()
		if err != nil {
			return nil, err
		}
		tokenID, err := utility.GenerateUUID()
		if err != nil {
			return nil, err
		}
		if _, err = s.pool.Exec(ctx, `INSERT INTO organization_discovery_tokens(id,email,token_hash,expires_at) VALUES($1,$2,$3,$4)`,
			tokenID, emailAddress, security.HashToken(rawToken), time.Now().Add(15*time.Minute)); err != nil {
			return nil, err
		}
		return &DiscoveryResponse{Action: "login", LoginURL: s.tenantURL(slug, "/login?discovery_token="+url.QueryEscape(rawToken))}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	if err := s.reissuePendingActivation(ctx, emailAddress); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errDiscoveryAccountNotFound
		}
		return nil, err
	}
	return &DiscoveryResponse{Action: "activation_email_queued"}, nil
}

func (s *Service) reissuePendingActivation(ctx context.Context, emailAddress string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var tenantID, organizationName, slug, userID, fullName, recipient string
	err = tx.QueryRow(ctx, `SELECT tenant_id::text,organization_name,slug,user_id::text,full_name,email
		FROM discover_pending_registration($1)`, emailAddress).
		Scan(&tenantID, &organizationName, &slug, &userID, &fullName, &recipient)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return err
	}

	rawToken, err := security.GenerateToken()
	if err != nil {
		return err
	}
	tokenID, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	link := s.tenantURL(slug, "/activate-organization?token="+url.QueryEscape(rawToken))
	rendered, err := s.email.RenderOrganizationActivation(email.OrganizationActivationData{
		CommonData:    email.CommonData{RecipientName: fullName, OrganizationName: organizationName},
		ActivationURL: link, ExpiresAt: expiresAt,
	})
	if err != nil {
		return fmt.Errorf("render organization activation recovery email: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE organization_activation_tokens SET used_at=now()
		WHERE user_id=$1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_activation_tokens(id,tenant_id,user_id,token_hash,expires_at)
		VALUES($1,$2,$3,$4,$5)`, tokenID, tenantID, userID, security.HashToken(rawToken), expiresAt); err != nil {
		return err
	}
	if err = email.Enqueue(ctx, tx, email.Message{TenantID: &tenantID, Kind: email.OrganizationActivation, To: recipient,
		Subject: rendered.Subject, Text: rendered.Text, HTML: rendered.HTML, ExpiresAt: &expiresAt}); err != nil {
		return err
	}
	auditID, err := utility.GenerateUUID()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,tenant_id,user_id,action,result)
		VALUES($1,$2,$3,'organization_activation_reissued','success')`, auditID, tenantID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) VerifyOrganizationDiscovery(ctx context.Context, rawToken, slug string) (*DiscoveryVerifyResponse, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if strings.TrimSpace(rawToken) == "" || !slugPattern.MatchString(slug) {
		return nil, errDiscoveryTokenInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var tokenID, emailAddress string
	err = tx.QueryRow(ctx, `SELECT id::text,email FROM organization_discovery_tokens
		WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now() FOR UPDATE`, security.HashToken(rawToken)).Scan(&tokenID, &emailAddress)
	if err != nil {
		return nil, errDiscoveryTokenInvalid
	}
	var matches bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM discover_active_organizations($1) WHERE slug=$2)`, emailAddress, slug).Scan(&matches); err != nil {
		return nil, err
	}
	if !matches {
		return nil, errDiscoveryTokenInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE organization_discovery_tokens SET used_at=now() WHERE id=$1`, tokenID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &DiscoveryVerifyResponse{Email: emailAddress}, nil
}

func (s *Service) ResolveTenant(ctx context.Context, slug string) (tenant.Identity, error) {
	if s.isReservedSlug(slug) || !slugPattern.MatchString(slug) {
		return tenant.Identity{}, errors.New("organization not found")
	}
	var id, status string
	err := s.pool.QueryRow(ctx, `SELECT id::text, status::text FROM organizations WHERE slug=$1`, strings.ToLower(slug)).Scan(&id, &status)
	if err != nil || status == "suspended" {
		return tenant.Identity{}, errors.New("organization not found")
	}
	return tenant.Identity{ID: id, Slug: slug}, nil
}

func (s *Service) SlugAvailable(ctx context.Context, slug string) (bool, error) {
	if !slugPattern.MatchString(slug) || s.isReservedSlug(slug) {
		return false, nil
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE slug=$1)`, slug).Scan(&exists)
	return !exists, err
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (*Organization, error) {
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.AdminEmail = strings.ToLower(strings.TrimSpace(req.AdminEmail))
	if !slugPattern.MatchString(req.Slug) || s.isReservedSlug(req.Slug) {
		return nil, errInvalidSlug
	}
	tenantID, _ := utility.GenerateUUID()
	userID, _ := utility.GenerateUUID()
	roleID, _ := utility.GenerateUUID()
	activationID, _ := utility.GenerateUUID()
	rawToken, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)
	link := s.tenantURL(req.Slug, "/activate-organization?token="+url.QueryEscape(rawToken))
	rendered, err := s.email.RenderOrganizationActivation(email.OrganizationActivationData{
		CommonData:    email.CommonData{RecipientName: req.AdminFullName, OrganizationName: req.OrganizationName},
		ActivationURL: link, ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("render organization activation email: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var org Organization
	err = tx.QueryRow(ctx, `INSERT INTO organizations(id,organization_name,slug) VALUES($1,$2,$3)
		RETURNING id::text,organization_name,slug,status::text,created_at`,
		tenantID, req.OrganizationName, req.Slug).Scan(&org.ID, &org.OrganizationName, &org.Slug, &org.Status, &org.CreatedAt)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO roles(id,name,description,is_superuser_role) VALUES($1,'Administrator','Founding tenant administrator',true)`, roleID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,full_name,username,email,status) VALUES($1,$2,$3,$3,'invited')`, userID, req.AdminFullName, req.AdminEmail); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, userID, roleID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_activation_tokens(id,tenant_id,user_id,token_hash,expires_at) VALUES($1,$2,$3,$4,$5)`,
		activationID, tenantID, userID, security.HashToken(rawToken), expiresAt); err != nil {
		return nil, err
	}
	if err = seedPrescribingReferenceData(ctx, tx); err != nil {
		return nil, err
	}
	if err = seedAllergens(ctx, tx); err != nil {
		return nil, err
	}
	if err = seedClinicalTemplates(ctx, tx); err != nil {
		return nil, err
	}
	if err = seedDefaultOutpatientConfiguration(ctx, tx); err != nil {
		return nil, err
	}
	auditID, _ := utility.GenerateUUID()
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,tenant_id,user_id,action,result) VALUES($1,$2,NULL,'organization_registered','success')`, auditID, tenantID); err != nil {
		return nil, err
	}
	if err = email.Enqueue(ctx, tx, email.Message{TenantID: &tenantID, Kind: email.OrganizationActivation, To: req.AdminEmail,
		Subject: rendered.Subject, Text: rendered.Text, HTML: rendered.HTML, ExpiresAt: &expiresAt}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &org, nil
}

// The medication form picks dosage form, route and unit of measure from lists a
// facility configures, so a tenant with empty lists cannot record a medication
// properly. New tenants start with the same defaults migration 000016 gave
// existing ones; tenant_id comes from the stamp_tenant_id trigger, which reads
// the app.tenant_id set earlier in this transaction.
func seedPrescribingReferenceData(ctx context.Context, tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}) error {
	for _, list := range []struct {
		table   string
		entries []clinical.VocabularyEntry
	}{
		{"medication_dosage_forms", clinical.DefaultDosageForms},
		{"administration_routes", clinical.DefaultRoutes},
		{"units_of_measure", clinical.DefaultUnitsOfMeasure},
		{"prescription_frequencies", clinical.DefaultPrescriptionFrequencies},
		{"specimen_types", clinical.DefaultSpecimenTypes},
	} {
		codes := make([]string, len(list.entries))
		names := make([]string, len(list.entries))
		for i, entry := range list.entries {
			codes[i], names[i] = entry.Code, entry.Name
		}
		if _, err := tx.Exec(ctx, `INSERT INTO `+list.table+`(id,code,name)
			SELECT gen_random_uuid(),v.code,v.name FROM unnest($1::text[],$2::text[]) AS v(code,name)`, codes, names); err != nil {
			return err
		}
	}
	return nil
}

func seedAllergens(ctx context.Context, tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}) error {
	for _, x := range clinical.DefaultAllergens {
		aliases := x.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO allergen_catalogue(id,code,name,category,aliases) VALUES(gen_random_uuid(),$1,$2,$3,$4)`, x.Code, x.Name, x.Category, aliases); err != nil {
			return err
		}
	}
	return nil
}

func seedClinicalTemplates(ctx context.Context, tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}) error {
	for _, x := range []struct {
		code, name, description, encounterType string
		definition                             clinical.TemplateDefinition
	}{
		{"outpatient-triage", "Outpatient Triage", "Core outpatient triage observations", "triage", clinical.DefaultTriageTemplate},
		{"outpatient-consultation", "Outpatient Consultation", "Practical outpatient consultation note", "consultation", clinical.DefaultConsultationTemplate},
	} {
		templateID, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		versionID, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		definition, err := json.Marshal(x.definition)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO clinical_templates(id,code,name,description,encounter_type,is_default) VALUES($1,$2,$3,$4,$5,true)`, templateID, x.code, x.name, x.description, x.encounterType); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO clinical_template_versions(id,template_id,version,status,definition,published_at) VALUES($1,$2,1,'published',$3,now())`, versionID, templateID, string(definition)); err != nil {
			return err
		}
	}
	return nil
}

func seedDefaultOutpatientConfiguration(ctx context.Context, tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}) error {
	_, err := tx.Exec(ctx, `
		WITH departments_seed(code, name, description) AS (VALUES
			('OPD', 'Outpatient Department', 'Default outpatient clinical services'),
			('LAB', 'Laboratory Department', 'Default diagnostic laboratory services'),
			('PHARM', 'Pharmacy Department', 'Default medication dispensing services')
		)
		INSERT INTO departments(id, code, name, description)
		SELECT gen_random_uuid(), code, name, description FROM departments_seed;

		WITH service_points_seed(department_code, code, name, kind) AS (VALUES
			('OPD', 'OPD-TRIAGE', 'Outpatient Triage', 'triage'),
			('OPD', 'OPD-CONSULT', 'General Consultation', 'consultation'),
			('LAB', 'LAB-MAIN', 'Main Laboratory', 'laboratory'),
			('PHARM', 'PHARM-MAIN', 'Main Pharmacy', 'pharmacy')
		)
		INSERT INTO service_points(id, department_id, code, name, kind)
		SELECT gen_random_uuid(), d.id, s.code, s.name, s.kind
		FROM service_points_seed s
		JOIN departments d ON d.code=s.department_code
			AND d.tenant_id=NULLIF(current_setting('app.tenant_id', true), '')::uuid;

		WITH queues_seed(service_point_code, code, name) AS (VALUES
			('OPD-TRIAGE', 'OPD-TRIAGE-Q', 'Outpatient Triage Queue'),
			('OPD-CONSULT', 'OPD-CONSULT-Q', 'General Consultation Queue'),
			('LAB-MAIN', 'LAB-Q', 'Laboratory Orders Queue'),
			('PHARM-MAIN', 'PHARM-Q', 'Prescription Queue')
		)
		INSERT INTO queues(id, service_point_id, code, name)
		SELECT gen_random_uuid(), sp.id, s.code, s.name
		FROM queues_seed s
		JOIN service_points sp ON sp.code=s.service_point_code
			AND sp.tenant_id=NULLIF(current_setting('app.tenant_id', true), '')::uuid;

		WITH rules(name, event_type, visit_type, encounter_type, order_kind, service_category, queue_code, priority) AS (VALUES
			('Default outpatient check-in to triage', 'visit.created', 'outpatient', NULL, NULL, NULL, 'OPD-TRIAGE-Q', 0::smallint),
			('Default completed triage to consultation', 'encounter.completed', 'outpatient', 'triage', NULL, NULL, 'OPD-CONSULT-Q', 0::smallint),
			('Default laboratory order routing', 'order.created', NULL, NULL, 'service', 'laboratory', 'LAB-Q', 0::smallint),
			('Default medication order routing', 'order.created', NULL, NULL, 'medication', 'pharmacy', 'PHARM-Q', 0::smallint),
			('Default reviewed order to consultation', 'order.review_ready', 'outpatient', NULL, NULL, NULL, 'OPD-CONSULT-Q', 0::smallint)
		)
		INSERT INTO queue_routing_rules(id, name, event_type, visit_type, encounter_type, order_kind, service_category, target_queue_id, priority)
		SELECT gen_random_uuid(), r.name, r.event_type, r.visit_type::clinical_visit_type,
		       r.encounter_type::clinical_encounter_type, r.order_kind, r.service_category, q.id, r.priority
		FROM rules r
		JOIN queues q ON q.code=r.queue_code
			AND q.tenant_id=NULLIF(current_setting('app.tenant_id', true), '')::uuid`)
	return err
}

func (s *Service) Current(ctx context.Context) (*Organization, error) {
	id, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	var org Organization
	err = s.pool.QueryRow(ctx, `SELECT id::text,organization_name,slug,status::text,created_at FROM organizations WHERE id=$1`, id).
		Scan(&org.ID, &org.OrganizationName, &org.Slug, &org.Status, &org.CreatedAt)
	return &org, err
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (*Organization, error) {
	id, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	var org Organization
	err = s.pool.QueryRow(ctx, `UPDATE organizations SET
		organization_name=COALESCE($2,organization_name),
		status=COALESCE($3::organization_status,status)
		WHERE id=$1 RETURNING id::text,organization_name,slug,status::text,created_at`,
		id, req.OrganizationName, req.Status).Scan(&org.ID, &org.OrganizationName, &org.Slug, &org.Status, &org.CreatedAt)
	return &org, err
}

func (s *Service) Accept(ctx context.Context, rawToken, password string, enrollmentTTL time.Duration) (string, error) {
	if violations := auth.ValidatePasswordPolicy(password, s.policy); len(violations) > 0 {
		return "", fmt.Errorf("password fails policy")
	}
	hash, err := security.HashPassword(password, s.bcryptCost)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	resolvedTenantID, err := tenant.ID(ctx)
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, resolvedTenantID); err != nil {
		return "", err
	}
	var activationID, tenantID, userID string
	err = tx.QueryRow(ctx, `SELECT id::text,tenant_id::text,user_id::text FROM organization_activation_tokens
		WHERE token_hash=$1 AND tenant_id=$2 AND used_at IS NULL AND expires_at>now()`, security.HashToken(rawToken), resolvedTenantID).
		Scan(&activationID, &tenantID, &userID)
	if err != nil {
		return "", errors.New("invalid or expired activation token")
	}
	enrollmentID, _ := utility.GenerateUUID()
	enrollmentRaw, _ := security.GenerateToken()
	if _, err = tx.Exec(ctx, `UPDATE organization_activation_tokens SET used_at=now() WHERE id=$1`, activationID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2,status='active' WHERE id=$1`, userID, hash); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE organizations SET status='active' WHERE id=$1`, tenantID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO enrollment_tokens(id,user_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`,
		enrollmentID, userID, security.HashToken(enrollmentRaw), time.Now().Add(enrollmentTTL)); err != nil {
		return "", err
	}
	return enrollmentRaw, tx.Commit(ctx)
}

type Handler struct {
	service       *Service
	authorizer    middleware.Authorizer
	jwtSecret     string
	enrollmentTTL time.Duration
}

func NewHandler(s *Service, authorizer middleware.Authorizer, jwtSecret string, enrollmentTTL time.Duration) *Handler {
	return &Handler{service: s, authorizer: authorizer, jwtSecret: jwtSecret, enrollmentTTL: enrollmentTTL}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/organizations/check-slug", h.checkSlug)
	r.Post("/organizations", h.register)
	r.Post("/auth/organization-discovery/request", h.requestDiscovery)
	r.Post("/auth/organization-discovery/verify", h.verifyDiscovery)
	r.Post("/organizations/activations/{token}/accept", h.accept)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))
		r.Get("/organizations/current", h.current)
		r.With(middleware.RequirePermission("admin:organizations:write")).Patch("/organizations/current", h.update)
	})
}

func requestIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) requestDiscovery(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[DiscoveryRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.RequestOrganizationDiscovery(r.Context(), req.Email, requestIP(r))
	if err != nil {
		if errors.Is(err, errDiscoveryRateLimited) {
			response.Error(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error())
			return
		}
		if err.Error() == "a valid email is required" {
			response.BadRequest(w, err.Error())
			return
		}
		if errors.Is(err, errDiscoveryAccountNotFound) {
			response.NotFound(w, err.Error())
			return
		}
		h.service.log.Error("organization discovery request failed", "error", err.Error())
		response.Internal(w)
		return
	}
	response.OK(w, result)
}

func (h *Handler) verifyDiscovery(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[DiscoveryVerifyRequest](w, r)
	if !ok {
		return
	}
	result, err := h.service.VerifyOrganizationDiscovery(r.Context(), req.Token, req.Slug)
	if err != nil {
		response.BadRequest(w, errDiscoveryTokenInvalid.Error())
		return
	}
	response.OK(w, result)
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var value T
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		response.BadRequest(w, "invalid request body")
		return value, false
	}
	return value, true
}

func (h *Handler) checkSlug(w http.ResponseWriter, r *http.Request) {
	available, err := h.service.SlugAvailable(r.Context(), r.URL.Query().Get("slug"))
	if err != nil {
		h.service.log.Error("failed to check organization slug", "error", err.Error())
		response.Internal(w)
		return
	}
	response.OK(w, map[string]bool{"available": available})
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[RegisterRequest](w, r)
	if !ok {
		return
	}
	org, err := h.service.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, errInvalidSlug) {
			response.Validation(w, map[string]string{"slug": err.Error()})
			return
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "organizations_slug_key":
				response.Conflict(w, "organization code is already registered")
				return
			case "users_email_lower_key", "users_tenant_email_key", "users_tenant_username_key":
				response.Conflict(w, "admin email is already registered")
				return
			}
		}
		h.service.log.Error("failed to register organization", "error", err.Error())
		response.Internal(w)
		return
	}
	response.Created(w, org)
}
func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[AcceptRequest](w, r)
	if !ok {
		return
	}
	if req.Token == "" {
		req.Token = chi.URLParam(r, "token")
	}
	token, err := h.service.Accept(r.Context(), req.Token, req.Password, h.enrollmentTTL)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	response.OK(w, map[string]string{"enrollment_token": token})
}
func (h *Handler) current(w http.ResponseWriter, r *http.Request) {
	org, err := h.service.Current(r.Context())
	if err != nil {
		response.NotFound(w, "organization not found")
		return
	}
	response.OK(w, org)
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[UpdateRequest](w, r)
	if !ok {
		return
	}
	org, err := h.service.Update(r.Context(), req)
	if err != nil {
		response.Internal(w)
		return
	}
	response.OK(w, org)
}
