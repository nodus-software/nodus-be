package auth

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	wa "github.com/go-webauthn/webauthn/webauthn"

	"nodus-health/internal/audit"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

type webAuthnUser struct {
	user        *User
	credentials []wa.Credential
}

func (u webAuthnUser) WebAuthnID() []byte                   { return []byte(u.user.ID) }
func (u webAuthnUser) WebAuthnName() string                 { return u.user.Email }
func (u webAuthnUser) WebAuthnDisplayName() string          { return u.user.FullName }
func (u webAuthnUser) WebAuthnCredentials() []wa.Credential { return u.credentials }

func (s *Service) webAuthnUser(ctx context.Context, userID string) (webAuthnUser, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return webAuthnUser{}, err
	}
	rows, err := s.repo.ListWebAuthnCredentialsByUser(ctx, userID)
	if err != nil {
		return webAuthnUser{}, err
	}
	credentials := make([]wa.Credential, 0, len(rows))
	for _, row := range rows {
		var c wa.Credential
		if err := json.Unmarshal(row.CredentialJSON, &c); err != nil {
			return webAuthnUser{}, err
		}
		credentials = append(credentials, c)
	}
	return webAuthnUser{user: u, credentials: credentials}, nil
}

func (s *Service) generateRecoveryCodes(ctx context.Context, repo Repository, userID string) ([]string, error) {
	codes := make([]string, 0, s.cfg.MFABackupCodeCount)
	for range s.cfg.MFABackupCodeCount {
		raw, err := security.GenerateBackupCode()
		if err != nil {
			return nil, err
		}
		id, err := utility.GenerateUUID()
		if err != nil {
			return nil, err
		}
		if err := repo.CreateMFABackupCode(ctx, id, userID, security.HashToken(security.NormalizeRecoveryCode(raw))); err != nil {
			return nil, err
		}
		codes = append(codes, raw)
	}
	return codes, nil
}

func (s *Service) BeginWebAuthnRegistration(ctx context.Context, userID, enrollmentTokenID string, req WebAuthnRegistrationOptionsRequest) (*WebAuthnRegistrationOptionsResponse, error) {
	if s.webauthnErr != nil || s.webauthn == nil {
		return nil, ErrWebAuthnUnavailable
	}
	req.Label = strings.TrimSpace(req.Label)
	if req.Label == "" || len(req.Label) > 80 {
		return nil, ErrWebAuthnInvalid
	}
	u, err := s.webAuthnUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if enrollmentTokenID == "" && !security.ComparePassword(u.user.PasswordHash, req.CurrentPassword) {
		return nil, ErrCurrentPasswordInvalid
	}
	creation, session, err := s.webauthn.BeginRegistration(u)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	session.UserVerification = protocol.VerificationRequired
	encoded, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	var enrollmentID *string
	if enrollmentTokenID != "" {
		enrollmentID = &enrollmentTokenID
	}
	if err := s.repo.CreateWebAuthnCeremony(ctx, WebAuthnCeremony{ID: id, UserID: userID, Purpose: "registration", Label: req.Label, EnrollmentTokenID: enrollmentID, SessionData: encoded, ExpiresAt: time.Now().Add(s.cfg.WebAuthnCeremonyTTL)}); err != nil {
		return nil, err
	}
	return &WebAuthnRegistrationOptionsResponse{CeremonyID: id, PublicKey: creation.Response}, nil
}

func (s *Service) FinishWebAuthnRegistration(ctx context.Context, userID, enrollmentTokenID string, req WebAuthnRegistrationVerifyRequest) (*WebAuthnRegistrationVerifyResponse, error) {
	ceremony, err := s.repo.GetWebAuthnCeremonyByID(ctx, req.CeremonyID)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	if ceremony.UserID != userID || ceremony.Purpose != "registration" || ceremony.ConsumedAt != nil || !ceremony.ExpiresAt.After(time.Now()) {
		return nil, ErrWebAuthnInvalid
	}
	if ceremony.EnrollmentTokenID != nil && *ceremony.EnrollmentTokenID != enrollmentTokenID {
		return nil, ErrWebAuthnInvalid
	}
	if ceremony.EnrollmentTokenID == nil && enrollmentTokenID != "" {
		return nil, ErrWebAuthnInvalid
	}
	u, err := s.webAuthnUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var session wa.SessionData
	if err := json.Unmarshal(ceremony.SessionData, &session); err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(req.Credential)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	credential, err := s.webauthn.CreateCredential(u, session, parsed)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	factorID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	credentialRowID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := &WebAuthnRegistrationVerifyResponse{}
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ConsumeWebAuthnCeremony(ctx, ceremony.ID); err != nil {
			return err
		}
		factor, err := repo.CreateMFAFactor(ctx, MFAFactor{ID: factorID, UserID: userID, Type: MFAFactorWebAuthn, Label: ceremony.Label, ConfirmedAt: &now})
		if err != nil {
			return err
		}
		if err := repo.CreateWebAuthnCredential(ctx, WebAuthnCredential{ID: credentialRowID, UserID: userID, FactorID: factorID, CredentialID: credential.ID, CredentialJSON: credentialJSON}); err != nil {
			return err
		}
		if err := repo.DeletePendingTOTPFactors(ctx, userID); err != nil {
			return err
		}
		remaining, err := repo.CountUnusedMFABackupCodes(ctx, userID)
		if err != nil {
			return err
		}
		if remaining == 0 {
			if result.RecoveryCodes, err = s.generateRecoveryCodes(ctx, repo, userID); err != nil {
				return err
			}
		}
		if enrollmentTokenID != "" {
			if err := repo.ConsumeEnrollmentToken(ctx, enrollmentTokenID); err != nil {
				return err
			}
		}
		result.Factor = MFAFactorResponse{ID: factor.ID, Type: string(factor.Type), Label: factor.Label, CreatedAt: factor.CreatedAt}
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{UserID: &userID, Action: "webauthn_credential_registered", Result: audit.ResultSuccess, Metadata: map[string]any{"label": ceremony.Label}})
	return result, nil
}

func (s *Service) BeginWebAuthnLogin(ctx context.Context, req WebAuthnLoginOptionsRequest) (*WebAuthnLoginOptionsResponse, error) {
	challenge, err := s.repo.GetLoginChallengeByHash(ctx, security.HashToken(req.ChallengeToken))
	if err != nil {
		return nil, ErrChallengeInvalid
	}
	now := time.Now()
	if challenge.IsConsumed() {
		return nil, ErrChallengeInvalid
	}
	if challenge.IsExpired(now) {
		return nil, ErrChallengeExpired
	}
	u, err := s.webAuthnUser(ctx, challenge.UserID)
	if err != nil {
		return nil, err
	}
	if len(u.credentials) == 0 {
		return nil, ErrWebAuthnUnavailable
	}
	assertion, session, err := s.webauthn.BeginLogin(u)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	session.UserVerification = protocol.VerificationRequired
	encoded, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	expires := now.Add(s.cfg.WebAuthnCeremonyTTL)
	if challenge.ExpiresAt.Before(expires) {
		expires = challenge.ExpiresAt
	}
	if err := s.repo.CreateWebAuthnCeremony(ctx, WebAuthnCeremony{ID: id, UserID: challenge.UserID, Purpose: "authentication", LoginChallengeID: &challenge.ID, SessionData: encoded, ExpiresAt: expires}); err != nil {
		return nil, err
	}
	return &WebAuthnLoginOptionsResponse{CeremonyID: id, PublicKey: assertion.Response}, nil
}

func (s *Service) FinishWebAuthnLogin(ctx context.Context, req WebAuthnLoginVerifyRequest, ip, userAgent, deviceLabel string) (*TokenPairResponse, error) {
	challenge, err := s.repo.GetLoginChallengeByHash(ctx, security.HashToken(req.ChallengeToken))
	if err != nil {
		return nil, ErrChallengeInvalid
	}
	now := time.Now()
	if challenge.IsConsumed() {
		return nil, ErrChallengeInvalid
	}
	if challenge.IsExpired(now) {
		return nil, ErrChallengeExpired
	}
	ceremony, err := s.repo.GetWebAuthnCeremonyByID(ctx, req.CeremonyID)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	if ceremony.UserID != challenge.UserID || ceremony.Purpose != "authentication" || ceremony.LoginChallengeID == nil || *ceremony.LoginChallengeID != challenge.ID || ceremony.ConsumedAt != nil || !ceremony.ExpiresAt.After(now) {
		return nil, ErrWebAuthnInvalid
	}
	u, err := s.webAuthnUser(ctx, challenge.UserID)
	if err != nil {
		return nil, err
	}
	var session wa.SessionData
	if err := json.Unmarshal(ceremony.SessionData, &session); err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(req.Credential)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	credential, err := s.webauthn.ValidateLogin(u, session, parsed)
	if err != nil {
		return nil, ErrWebAuthnInvalid
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	user, err := s.repo.GetUserByID(ctx, challenge.UserID)
	if err != nil {
		return nil, err
	}
	var pair *TokenPairResponse
	err = s.repo.WithinTx(ctx, func(repo Repository) error {
		if err := repo.ConsumeWebAuthnCeremony(ctx, ceremony.ID); err != nil {
			return err
		}
		if err := repo.UpdateWebAuthnCredential(ctx, user.ID, credential.ID, encoded); err != nil {
			return err
		}
		var sessionErr error
		pair, sessionErr = s.establishSession(ctx, user, challenge, ip, userAgent, deviceLabel, req.RememberMe, "", now)
		return sessionErr
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Entry{UserID: &user.ID, Action: "login_success", IPAddress: ip, Result: audit.ResultSuccess, Metadata: map[string]any{"method": "webauthn"}})
	return pair, nil
}
