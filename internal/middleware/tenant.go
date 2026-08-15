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

type TenantHostConfig struct {
	BaseDomain      string
	AllowSlugHeader bool
}

// ResolveTenant resolves the left-most host label before authentication.
// X-Tenant-Slug is accepted only for local/test clients which do not have DNS
// subdomains; deployments should strip that header at their edge proxy.
func ResolveTenant(resolver TenantResolver, configs ...TenantHostConfig) func(http.Handler) http.Handler {
	cfg := TenantHostConfig{AllowSlugHeader: true}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	cfg.BaseDomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cfg.BaseDomain), "."))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/organizations" || r.URL.Path == "/organizations/check-slug" ||
				r.URL.Path == "/auth/organization-discovery/request" {
				if cfg.BaseDomain != "" {
					host := strings.ToLower(strings.TrimSuffix(r.Host, "."))
					if h, _, err := net.SplitHostPort(host); err == nil {
						host = h
					}
					if host != cfg.BaseDomain {
						response.BadRequest(w, "this endpoint is only available on the primary domain")
						return
					}
				}
				next.ServeHTTP(w, r)
				return
			}
			slug := ""
			if cfg.AllowSlugHeader {
				slug = strings.TrimSpace(r.Header.Get("X-Tenant-Slug"))
			}
			if slug == "" {
				host := strings.ToLower(strings.TrimSuffix(r.Host, "."))
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				if cfg.BaseDomain != "" {
					suffix := "." + cfg.BaseDomain
					if strings.HasSuffix(host, suffix) {
						candidate := strings.TrimSuffix(host, suffix)
						if candidate != "" && !strings.Contains(candidate, ".") {
							slug = candidate
						}
					}
				} else {
					parts := strings.Split(host, ".")
					if len(parts) >= 3 {
						slug = parts[0]
					}
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
