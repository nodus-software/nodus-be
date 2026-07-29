package postgres

import (
	"context"
	"errors"
	"time"

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
	created, err := r.q(ctx).CreateMFAFactor(ctx, sqlcgen.CreateMFAFactorParams{
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
	f, err := r.q(ctx).GetMFAFactorByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrFactorNotFound
		}
		return nil, err
	}
	return mfaFactorFromRow(f), nil
}

func (r *Repository) ListMFAFactorsByUser(ctx context.Context, userID string) ([]auth.MFAFactor, error) {
	rows, err := r.q(ctx).ListMFAFactorsByUser(ctx, userID)
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
	return r.q(ctx).ConfirmMFAFactor(ctx, id)
}

func (r *Repository) DeleteMFAFactor(ctx context.Context, id string) error {
	return r.q(ctx).DeleteMFAFactor(ctx, id)
}

func (r *Repository) CountConfirmedMFAFactors(ctx context.Context, userID string) (int, error) {
	n, err := r.q(ctx).CountConfirmedMFAFactors(ctx, userID)
	return int(n), err
}

func (r *Repository) CreateMFABackupCode(ctx context.Context, id, userID, codeHash string) error {
	return r.q(ctx).CreateMFABackupCode(ctx, sqlcgen.CreateMFABackupCodeParams{
		ID: id, UserID: userID, CodeHash: codeHash,
	})
}

func (r *Repository) GetUnusedMFABackupCodeIDByHash(ctx context.Context, userID, codeHash string) (string, error) {
	code, err := r.q(ctx).GetUnusedMFABackupCodeByHash(ctx, sqlcgen.GetUnusedMFABackupCodeByHashParams{
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
	return r.q(ctx).ConsumeMFABackupCode(ctx, id)
}

func (r *Repository) CountUnusedMFABackupCodes(ctx context.Context, userID string) (int, error) {
	n, err := r.q(ctx).CountUnusedMFABackupCodes(ctx, userID)
	return int(n), err
}

func (r *Repository) InvalidateMFABackupCodes(ctx context.Context, userID string) error {
	return r.q(ctx).InvalidateMFABackupCodes(ctx, userID)
}

func (r *Repository) GetEnrollmentTokenByHash(ctx context.Context, tokenHash string) (string, string, time.Time, bool, error) {
	token, err := r.q(ctx).GetEnrollmentTokenByHash(ctx, tokenHash)
	if err != nil {
		return "", "", time.Time{}, false, err
	}
	return token.ID, token.UserID, fromTimestamptz(token.ExpiresAt), token.Consumed, nil
}

func (r *Repository) ConsumeEnrollmentToken(ctx context.Context, id string) error {
	return r.q(ctx).ConsumeEnrollmentToken(ctx, id)
}

func (r *Repository) CreateWebAuthnCredential(ctx context.Context, c auth.WebAuthnCredential) error {
	return r.q(ctx).CreateWebAuthnCredential(ctx, sqlcgen.CreateWebAuthnCredentialParams{ID: c.ID, UserID: c.UserID, FactorID: c.FactorID, CredentialID: c.CredentialID, Credential: string(c.CredentialJSON)})
}

func (r *Repository) ListWebAuthnCredentialsByUser(ctx context.Context, userID string) ([]auth.WebAuthnCredential, error) {
	rows, err := r.q(ctx).ListWebAuthnCredentialsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]auth.WebAuthnCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, auth.WebAuthnCredential{ID: row.ID, UserID: row.UserID, FactorID: row.FactorID, CredentialID: row.CredentialID, CredentialJSON: row.Credential, CreatedAt: fromTimestamptz(row.CreatedAt)})
	}
	return out, nil
}

func (r *Repository) UpdateWebAuthnCredential(ctx context.Context, userID string, credentialID, credentialJSON []byte) error {
	return r.q(ctx).UpdateWebAuthnCredential(ctx, sqlcgen.UpdateWebAuthnCredentialParams{CredentialID: credentialID, UserID: userID, Credential: string(credentialJSON)})
}

func (r *Repository) CreateWebAuthnCeremony(ctx context.Context, c auth.WebAuthnCeremony) error {
	return r.q(ctx).CreateWebAuthnCeremony(ctx, sqlcgen.CreateWebAuthnCeremonyParams{ID: c.ID, UserID: c.UserID, LoginChallengeID: c.LoginChallengeID, EnrollmentTokenID: c.EnrollmentTokenID, Purpose: sqlcgen.WebauthnCeremonyPurpose(c.Purpose), Label: c.Label, SessionData: string(c.SessionData), ExpiresAt: toTimestamptz(c.ExpiresAt)})
}

func (r *Repository) GetWebAuthnCeremonyByID(ctx context.Context, id string) (*auth.WebAuthnCeremony, error) {
	row, err := r.q(ctx).GetWebAuthnCeremonyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrChallengeInvalid
		}
		return nil, err
	}
	return &auth.WebAuthnCeremony{ID: row.ID, UserID: row.UserID, Purpose: string(row.Purpose), Label: row.Label, LoginChallengeID: row.LoginChallengeID, EnrollmentTokenID: row.EnrollmentTokenID, SessionData: row.SessionData, ExpiresAt: fromTimestamptz(row.ExpiresAt), ConsumedAt: fromNullTimestamptz(row.ConsumedAt)}, nil
}

func (r *Repository) ConsumeWebAuthnCeremony(ctx context.Context, id string) error {
	n, err := r.q(ctx).ConsumeWebAuthnCeremony(ctx, id)
	if err != nil {
		return err
	}
	if n != 1 {
		return auth.ErrChallengeInvalid
	}
	return nil
}
func (r *Repository) DeletePendingTOTPFactors(ctx context.Context, userID string) error {
	return r.q(ctx).DeletePendingTOTPFactors(ctx, userID)
}
