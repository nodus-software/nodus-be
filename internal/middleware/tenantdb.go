package middleware

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/platform/db"
	"nodus-health/internal/tenant"
	"nodus-health/pkg/response"
)

// TenantTransaction pins all repository calls in a request to one transaction
// and installs the tenant setting consumed by PostgreSQL RLS policies.
func TenantTransaction(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := tenant.FromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			tx, err := pool.Begin(r.Context())
			if err != nil {
				response.Internal(w)
				return
			}
			defer tx.Rollback(r.Context())
			if _, err := tx.Exec(r.Context(), "SELECT set_config('app.tenant_id', $1, true)", identity.ID); err != nil {
				response.Internal(w)
				return
			}
			ctx := db.WithExecutor(r.Context(), tx)
			next.ServeHTTP(w, r.WithContext(ctx))
			if err := tx.Commit(r.Context()); err != nil {
				return
			}
		})
	}
}
