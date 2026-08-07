package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"nodus-health/internal/clinical"
)

const conceptSelect = `SELECT c.id::text,c.code,c.display,r.version,c.linearization_uri,c.chapter_no,COALESCE(o.enabled,true)
 FROM terminology_concepts c JOIN terminology_releases r ON r.id=c.release_id
 LEFT JOIN tenant_terminology_overrides o ON o.concept_id=c.id
 WHERE r.system='ICD-11' AND r.active AND c.active AND c.primary_tabulation`

func scanConcept(row pgx.Row) (*clinical.Concept, error) {
	var x clinical.Concept
	var uri, chapter *string
	e := row.Scan(&x.ID, &x.Code, &x.Display, &x.Version, &uri, &chapter, &x.Enabled)
	if uri != nil {
		x.URI = *uri
	}
	if chapter != nil {
		x.Chapter = *chapter
	}
	return &x, e
}

func (r *Repository) SearchICD11Concepts(c context.Context, q string, limit int) ([]clinical.Concept, error) {
	like := "%" + q + "%"
	rows, e := r.exec(c).Query(c, conceptSelect+` AND COALESCE(o.enabled,true) AND (c.code ILIKE $1 OR c.searchable_text ILIKE $1) ORDER BY CASE WHEN c.code ILIKE $2 THEN 0 ELSE 1 END,c.code LIMIT $3`, like, q+"%", limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Concept{}
	for rows.Next() {
		x, e := scanConcept(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}
func (r *Repository) ListDiagnosisConcepts(c context.Context, f clinical.DiagnosisFilters) ([]clinical.Concept, int, error) {
	args := []any{}
	where := ""
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		where += fmt.Sprintf(" AND (c.code ILIKE $%d OR c.searchable_text ILIKE $%d)", len(args), len(args))
	}
	if f.Chapter != "" {
		args = append(args, f.Chapter)
		where += fmt.Sprintf(" AND c.chapter_no=$%d", len(args))
	}
	if f.Availability == "enabled" {
		where += " AND COALESCE(o.enabled,true)"
	} else if f.Availability == "disabled" {
		where += " AND NOT COALESCE(o.enabled,true)"
	}
	var total int
	if e := r.exec(c).QueryRow(c, "SELECT count(*) FROM ("+conceptSelect+where+") x", args...).Scan(&total); e != nil {
		return nil, 0, e
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, e := r.exec(c).Query(c, conceptSelect+where+fmt.Sprintf(" ORDER BY c.code LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []clinical.Concept{}
	for rows.Next() {
		x, e := scanConcept(rows)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, *x)
	}
	return out, total, rows.Err()
}
func (r *Repository) SetDiagnosisConceptEnabled(c context.Context, id string, enabled bool) (*clinical.Concept, error) {
	var exists bool
	if e := r.exec(c).QueryRow(c, `SELECT EXISTS(SELECT 1 FROM terminology_concepts c JOIN terminology_releases r ON r.id=c.release_id WHERE c.id=$1 AND r.system='ICD-11' AND r.active AND c.primary_tabulation)`, id).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return nil, clinical.ErrNotFound
	}
	if enabled {
		_, _ = r.exec(c).Exec(c, "DELETE FROM tenant_terminology_overrides WHERE concept_id=$1", id)
	} else {
		_, e := r.exec(c).Exec(c, `INSERT INTO tenant_terminology_overrides(id,concept_id,enabled) VALUES(gen_random_uuid(),$1,false) ON CONFLICT (tenant_id,concept_id) DO UPDATE SET enabled=false`, id)
		if e != nil {
			return nil, e
		}
	}
	return scanConcept(r.exec(c).QueryRow(c, conceptSelect+" AND c.id=$1", id))
}
func (r *Repository) ResolveICD11Concept(c context.Context, id, code string) (*clinical.Concept, error) {
	q := conceptSelect + " AND COALESCE(o.enabled,true)"
	var row pgx.Row
	if id != "" {
		row = r.exec(c).QueryRow(c, q+" AND c.id=$1", id)
	} else {
		row = r.exec(c).QueryRow(c, q+" AND c.code=$1", code)
	}
	x, e := scanConcept(row)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrInvalidInput
	}
	return x, e
}

const allergenCols = "id::text,code,name,category,aliases,active,created_at,updated_at"

func scanAllergen(row pgx.Row) (*clinical.Allergen, error) {
	var x clinical.Allergen
	e := row.Scan(&x.ID, &x.Code, &x.Name, &x.Category, &x.Aliases, &x.Active, &x.CreatedAt, &x.UpdatedAt)
	return &x, e
}
func (r *Repository) ListAllergens(c context.Context) ([]clinical.Allergen, error) {
	rows, e := r.exec(c).Query(c, "SELECT "+allergenCols+" FROM allergen_catalogue ORDER BY category,name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []clinical.Allergen{}
	for rows.Next() {
		x, e := scanAllergen(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}
func (r *Repository) CreateAllergen(c context.Context, x clinical.Allergen) (*clinical.Allergen, error) {
	y, e := scanAllergen(r.exec(c).QueryRow(c, "INSERT INTO allergen_catalogue(id,code,name,category,aliases) VALUES($1,$2,$3,$4,$5) RETURNING "+allergenCols, x.ID, x.Code, x.Name, x.Category, x.Aliases))
	return y, normalizeResourceError(e)
}
func (r *Repository) UpdateAllergen(c context.Context, id string, in clinical.UpdateAllergenRequest) (*clinical.Allergen, error) {
	x, e := scanAllergen(r.exec(c).QueryRow(c, "UPDATE allergen_catalogue SET code=COALESCE($2,code),name=COALESCE($3,name),category=COALESCE($4,category),aliases=COALESCE($5,aliases) WHERE id=$1 RETURNING "+allergenCols, id, in.Code, in.Name, in.Category, in.Aliases))
	return x, normalizeResourceError(e)
}
func (r *Repository) SetAllergenActive(c context.Context, id string, active bool) (*clinical.Allergen, error) {
	x, e := scanAllergen(r.exec(c).QueryRow(c, "UPDATE allergen_catalogue SET active=$2 WHERE id=$1 RETURNING "+allergenCols, id, active))
	return x, normalizeResourceError(e)
}
func (r *Repository) GetActiveAllergen(c context.Context, id string) (*clinical.Allergen, error) {
	x, e := scanAllergen(r.exec(c).QueryRow(c, "SELECT "+allergenCols+" FROM allergen_catalogue WHERE id=$1 AND active", id))
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrInvalidInput
	}
	return x, e
}

func normalizeAliases(xs []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		k := strings.ToLower(x)
		if x != "" && !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	return out
}
