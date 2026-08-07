package organizations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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

type Mailer interface {
	SendHTML(ctx context.Context, to, subject, textBody, htmlBody string) error
}

type Service struct {
	pool       *pgxpool.Pool
	mailer     Mailer
	baseURL    string
	bcryptCost int
	policy     auth.PasswordPolicy
	log        *logger.Logger
	email      *email.Renderer
}

func NewService(pool *pgxpool.Pool, mailer Mailer, renderer *email.Renderer, baseURL string, bcryptCost int, policy auth.PasswordPolicy, log *logger.Logger) *Service {
	return &Service{
		pool: pool, mailer: mailer, baseURL: baseURL, bcryptCost: bcryptCost,
		policy: policy, log: log, email: renderer,
	}
}

func (s *Service) ResolveTenant(ctx context.Context, slug string) (tenant.Identity, error) {
	var id, status string
	err := s.pool.QueryRow(ctx, `SELECT id::text, status::text FROM organizations WHERE slug=$1`, strings.ToLower(slug)).Scan(&id, &status)
	if err != nil || status == "suspended" {
		return tenant.Identity{}, errors.New("organization not found")
	}
	return tenant.Identity{ID: id, Slug: slug}, nil
}

func (s *Service) SlugAvailable(ctx context.Context, slug string) (bool, error) {
	if !slugPattern.MatchString(slug) {
		return false, nil
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE slug=$1)`, slug).Scan(&exists)
	return !exists, err
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (*Organization, error) {
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.AdminEmail = strings.ToLower(strings.TrimSpace(req.AdminEmail))
	if !slugPattern.MatchString(req.Slug) {
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
		activationID, tenantID, userID, security.HashToken(rawToken), now.Add(24*time.Hour)); err != nil {
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
	auditID, _ := utility.GenerateUUID()
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,tenant_id,user_id,action,result) VALUES($1,$2,NULL,'organization_registered','success')`, auditID, tenantID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	link := fmt.Sprintf(
		"%s/activate-organization?token=%s&slug=%s",
		s.baseURL,
		url.QueryEscape(rawToken),
		url.QueryEscape(req.Slug),
	)
	rendered, err := s.email.RenderOrganizationActivation(email.OrganizationActivationData{
		CommonData: email.CommonData{
			RecipientName: req.AdminFullName, OrganizationName: req.OrganizationName,
		},
		ActivationURL: link,
		ExpiresAt:     now.Add(24 * time.Hour),
	})
	if err != nil {
		return nil, fmt.Errorf("render organization activation email: %w", err)
	}
	if err := s.mailer.SendHTML(ctx, req.AdminEmail, rendered.Subject, rendered.Text, rendered.HTML); err != nil {
		s.log.Error("failed to send organization activation", "error", err.Error())
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
		if _, err := tx.Exec(ctx, `INSERT INTO allergen_catalogue(id,code,name,category,aliases) VALUES(gen_random_uuid(),$1,$2,$3,$4)`, x.Code, x.Name, x.Category, x.Aliases); err != nil {
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
		if _, err = tx.Exec(ctx, `INSERT INTO clinical_template_versions(id,template_id,version,status,definition,published_at) VALUES($1,$2,1,'published',$3,now())`, versionID, templateID, definition); err != nil {
			return err
		}
	}
	return nil
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
	r.Post("/organizations/activations/{token}/accept", h.accept)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(h.jwtSecret, h.authorizer))
		r.Get("/organizations/current", h.current)
		r.With(middleware.RequirePermission("admin:organizations:write")).Patch("/organizations/current", h.update)
	})
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
			response.Conflict(w, "organization slug or admin email is already registered")
			return
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
