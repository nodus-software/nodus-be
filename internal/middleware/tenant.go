package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"nodus-health/internal/tenant"
	"nodus-health/pkg/response"
)

type TenantResolver interface {
	ResolveTenant(ctx context.Context, slug string) (tenant.Identity, error)
}

// ResolveTenant resolves the left-most host label before authentication.
// X-Tenant-Slug is accepted only for local/test clients which do not have DNS
// subdomains; deployments should strip that header at their edge proxy.
func ResolveTenant(resolver TenantResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/organizations" || r.URL.Path == "/organizations/check-slug" {
				next.ServeHTTP(w, r)
				return
			}
			slug := strings.TrimSpace(r.Header.Get("X-Tenant-Slug"))
			if slug == "" {
				host := r.Host
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				parts := strings.Split(host, ".")
				if len(parts) >= 3 {
					slug = parts[0]
				}
			}
			if slug == "" {
				response.BadRequest(w, "tenant subdomain is required")
				return
			}
			identity, err := resolver.ResolveTenant(r.Context(), slug)
			if err != nil {
				response.NotFound(w, "organization not found")
				return
			}
			next.ServeHTTP(w, r.WithContext(tenant.WithContext(r.Context(), identity)))
		})
	}
}
