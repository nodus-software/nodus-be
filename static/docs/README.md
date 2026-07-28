# Authentication API documentation

`auth-api-contract.yaml` is the frontend-facing OpenAPI 3.0 contract.

The implementation wraps every JSON response:

- Success: `{ "success": true, "data": ... }`
- Paginated success: `{ "success": true, "data": ..., "meta": ... }`
- Error: `{ "success": false, "error": { "code": "...", "message": "...", "details": ... } }`
- `204 No Content` responses have no body.

The canonical source is the repository-root `auth-api-contract.yaml`; keep this
published copy synchronized whenever the contract changes.

## Health check

`GET /health` is a public liveness probe. It does not require authentication,
tenant headers, or database access. A healthy process returns HTTP 200:

```json
{"success":true,"data":{"status":"ok"}}
```
