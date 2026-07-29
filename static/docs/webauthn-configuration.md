# WebAuthn configuration

Configure `WEBAUTHN_RP_DISPLAY_NAME`, `WEBAUTHN_RP_ID`, `WEBAUTHN_ORIGINS` (space-separated absolute origins), and `WEBAUTHN_CEREMONY_TTL`. Development defaults are `Nodus Health`, `localhost`, `http://localhost:5173 http://localhost:3000`, and five minutes.

Production origins must use HTTPS. The RP ID is a domain without scheme or port and must be a registrable suffix of every allowed origin. Values are explicit configuration and are never inferred from request headers. Registration requires user verification, requests no attestation, and supports both platform passkeys and roaming hardware security keys.
