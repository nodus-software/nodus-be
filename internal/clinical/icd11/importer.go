package icd11

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const attribution = "International Classification of Diseases, Eleventh Revision (ICD-11), World Health Organization (WHO) 2019. Licensed under CC BY-ND 3.0 IGO."

func Commit(ctx context.Context, pool *pgxpool.Pool, wb *Workbook) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('nodus.icd11.import'))"); err != nil {
		return err
	}
	var existingID, checksum string
	err = tx.QueryRow(ctx, "SELECT id::text,COALESCE(source_checksum,'') FROM terminology_releases WHERE system='ICD-11' AND version=$1", wb.Version).Scan(&existingID, &checksum)
	if err == nil {
		if checksum != wb.Checksum {
			return fmt.Errorf("ICD-11 release %s already exists with a different checksum", wb.Version)
		}
		_, err = tx.Exec(ctx, "UPDATE terminology_releases SET active=(id=$1) WHERE system='ICD-11'", existingID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	releaseID, runID := uuid.New(), uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO terminology_releases(id,system,version,title,released_on,active,language,linearization,source_checksum,source_file,attribution) VALUES($1,'ICD-11',$2,$3,$4,false,'en','mms',$5,$6,$7)`, releaseID, wb.Version, wb.Title, wb.ReleasedOn, wb.Checksum, wb.SourceFile, attribution)
	if err != nil {
		return err
	}
	rows := make([][]any, 0, len(wb.Concepts))
	for _, c := range wb.Concepts {
		rows = append(rows, []any{uuid.New(), releaseID, c.Code, c.Display, c.Display, c.FoundationURI, c.LinearizationURI, c.SourceTitle, c.ChapterNo, c.ParentURI, "category", c.IsLeaf, c.IsResidual, true})
	}
	count, err := tx.CopyFrom(ctx, pgx.Identifier{"terminology_concepts"}, []string{"id", "release_id", "code", "display", "searchable_text", "foundation_uri", "linearization_uri", "source_title", "chapter_no", "parent_uri", "class_kind", "is_leaf", "is_residual", "primary_tabulation"}, pgx.CopyFromRows(rows))
	if err != nil {
		return err
	}
	if int(count) != len(wb.Concepts) {
		return fmt.Errorf("imported %d of %d concepts", count, len(wb.Concepts))
	}
	// Carry disabled choices to codes that still exist in the new release.
	_, err = tx.Exec(ctx, `INSERT INTO tenant_terminology_overrides(id,tenant_id,concept_id,enabled)
		SELECT gen_random_uuid(),o.tenant_id,n.id,o.enabled FROM tenant_terminology_overrides o
		JOIN terminology_concepts old ON old.id=o.concept_id JOIN terminology_releases r ON r.id=old.release_id AND r.system='ICD-11' AND r.active
		JOIN terminology_concepts n ON n.release_id=$1 AND n.code=old.code ON CONFLICT (tenant_id,concept_id) DO NOTHING`, releaseID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE terminology_releases SET active=(id=$1) WHERE system='ICD-11'", releaseID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO terminology_import_runs(id,system,version,source_file,source_checksum,status,total_rows,imported_rows,completed_at) VALUES($1,'ICD-11',$2,$3,$4,'committed',$5,$6,now())`, runID, wb.Version, wb.SourceFile, wb.Checksum, wb.TotalRows, len(wb.Concepts))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
