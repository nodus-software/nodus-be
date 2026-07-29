# MFA recovery and legacy biometric rollout

MFA, least privilege, tenant isolation and attributable audit events support a DHA security assessment; deploying this code does not itself establish certification.

## Rollout preflight

Before migration 000009, take a tested recovery point and list users whose only confirmed factor is the retired custom biometric key:

```sql
SELECT u.tenant_id, u.id, u.email
FROM users u
JOIN mfa_factors b ON b.user_id=u.id AND b.type='biometric' AND b.confirmed_at IS NOT NULL
WHERE NOT EXISTS (
  SELECT 1 FROM mfa_factors t
  WHERE t.user_id=u.id AND t.type='totp' AND t.confirmed_at IS NOT NULL
);
```

Contact affected users and arrange an administrator-approved reset and strong-factor reenrollment. Mixed TOTP/biometric users retain TOTP and recovery-code access. The migration deletes legacy keys because their Ed25519 material is not a WebAuthn credential. Its down migration restores schema compatibility only; deleted keys cannot be recovered.

## Normal administrator-assisted reset

Verify the requester and target, record an operational reason and support ticket, and use the application MFA-reset action. Never transmit or record a token. Confirm that sessions were revoked, the registered-address notification was delivered, the user confirmed their existing password, enrolled a new strong factor, saved new recovery codes, and subsequently logged in. Retain the audit event IDs with the ticket.

## Sole administrator

1. Require identity and organization-authority evidence, a support ticket, and approval by two named support staff.
2. Resolve the exact tenant and user independently and create a database recovery point.
3. Use only the narrowly scoped application recovery operation. Do not set a known password, insert a factor, or bypass password confirmation.
4. Confirm revocation of sessions, refresh tokens, login challenges, ceremonies, old factors and recovery codes; issue only an expiring, single-use recovery action.
5. Notify the account owner through the registered channel. Require normal strong-factor enrollment and recovery-code acknowledgement before login.
6. Review audit evidence and close or escalate the incident. Roll back only from the recovery point; remember that legacy biometric credentials cannot be recreated.

Email and SMS OTP are intentionally excluded because they create a weaker downgrade path. Email is used only for security notification and delivery of an administrator-approved, short-lived link that still requires the user's password.
