# Authentication API documentation

`auth-api-contract.yaml` is the frontend-facing OpenAPI 3.0 contract.

The implementation wraps every JSON response:

- Success: `{ "success": true, "data": ... }`
- Paginated success: `{ "success": true, "data": ..., "meta": ... }`
- Error: `{ "success": false, "error": { "code": "...", "message": "...", "details": ... } }`
- `204 No Content` responses have no body.

The canonical source is the repository-root `auth-api-contract.yaml`; keep this
published copy synchronized whenever the contract changes.

## Refresh sessions and Remember me

The access token is returned in JSON and defaults to a 15-minute lifetime. The
rotating refresh token is never returned in JSON: the API sets it as an HttpOnly
cookie scoped to `/auth`. Frontends must use credentialed requests for MFA login,
refresh, and logout.

`remember_me` is submitted during `POST /auth/login/mfa`, because a session is not
created until MFA succeeds:

- `false` (default) creates a browser-session cookie and applies
  `SESSION_REFRESH_TOKEN_TTL` (24 hours by default) server-side.
- `true` creates a persistent cookie and applies `REFRESH_TOKEN_TTL` (30 days by
  default).

`POST /auth/refresh` takes no request body. It reads and rotates the cookie while
preserving the session's original Remember me choice. Logout revokes the session
and clears the cookie. Refresh-token hashes remain the only token representation
stored in PostgreSQL.

Production must use HTTPS with `REFRESH_COOKIE_SECURE=true`. `SameSite=Lax` is the
default and `Strict` is also supported. Cross-site cookie mode is intentionally not
supported without a separate CSRF-token design. Cross-origin, same-site frontends
require an exact `ALLOWED_ORIGINS` entry and credentialed CORS; wildcard origins are
not valid with cookies.

## Health check

`GET /health` is a public liveness probe. It does not require authentication,
tenant headers, or database access. A healthy process returns HTTP 200:

```json
{"success":true,"data":{"status":"ok"}}
```
