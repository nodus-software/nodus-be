package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/auth"
	"nodus-health/internal/auth/postgres/sqlcgen"
)

func mfaFactorFromRow(f sqlcgen.MfaFactor) *auth.MFAFactor {
	return &auth.MFAFactor{
		ID: f.ID, UserID: f.UserID, Type: auth.MFAFactorType(f.Type), Label: f.Label,
		SecretEncrypted: f.SecretEncrypted, PublicKey: f.PublicKey,
		ConfirmedAt: fromNullTimestamptz(f.ConfirmedAt), CreatedAt: fromTimestamptz(f.CreatedAt),
	}
}

func (r *Repository) CreateMFAFactor(ctx context.Context, factor auth.MFAFactor) (*auth.MFAFactor, error) {
	created, err := r.queries.CreateMFAFactor(ctx, sqlcgen.CreateMFAFactorParams{
		ID: factor.ID, UserID: factor.UserID, Type: sqlcgen.MfaFactorType(factor.Type),
		Label: factor.Label, SecretEncrypted: factor.SecretEncrypted, PublicKey: factor.PublicKey,
		ConfirmedAt: toNullTimestamptz(factor.ConfirmedAt),
	})
	if err != nil {
		return nil, err
	}
	return mfaFactorFromRow(created), nil
}

func (r *Repository) GetMFAFactorByID(ctx context.Context, id string) (*auth.MFAFactor, error) {
	f, err := r.queries.GetMFAFactorByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrFactorNotFound
		}
		return nil, err
	}
	return mfaFactorFromRow(f), nil
}

func (r *Repository) ListMFAFactorsByUser(ctx context.Context, userID string) ([]auth.MFAFactor, error) {
	rows, err := r.queries.ListMFAFactorsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	factors := make([]auth.MFAFactor, 0, len(rows))
	for _, row := range rows {
		factors = append(factors, *mfaFactorFromRow(row))
	}
	return factors, nil
}

func (r *Repository) ConfirmMFAFactor(ctx context.Context, id string) error {
	return r.queries.ConfirmMFAFactor(ctx, id)
}

func (r *Repository) DeleteMFAFactor(ctx context.Context, id string) error {
	return r.queries.DeleteMFAFactor(ctx, id)
}

func (r *Repository) CountConfirmedMFAFactors(ctx context.Context, userID string) (int, error) {
	n, err := r.queries.CountConfirmedMFAFactors(ctx, userID)
	return int(n), err
}

func (r *Repository) CreateMFABackupCode(ctx context.Context, id, userID, codeHash string) error {
	return r.queries.CreateMFABackupCode(ctx, sqlcgen.CreateMFABackupCodeParams{
		ID: id, UserID: userID, CodeHash: codeHash,
	})
}

func (r *Repository) GetUnusedMFABackupCodeIDByHash(ctx context.Context, userID, codeHash string) (string, error) {
	code, err := r.queries.GetUnusedMFABackupCodeByHash(ctx, sqlcgen.GetUnusedMFABackupCodeByHashParams{
		UserID: userID, CodeHash: codeHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return code.ID, nil
}

func (r *Repository) ConsumeMFABackupCode(ctx context.Context, id string) error {
	return r.queries.ConsumeMFABackupCode(ctx, id)
}
