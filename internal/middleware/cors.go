package middleware

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// CORS allows explicitly configured origins and direct tenant subdomains of
// tenantBaseDomain using the configured tenant URL scheme and port.
func CORS(allowedOrigins []string, tenantBaseDomain, tenantURLScheme, tenantURLPort string) func(http.Handler) http.Handler {
	tenantBaseDomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(tenantBaseDomain), "."))
	tenantURLScheme = strings.ToLower(strings.TrimSpace(tenantURLScheme))
	tenantURLPort = strings.TrimSpace(tenantURLPort)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (slices.Contains(allowedOrigins, origin) || isTenantOrigin(origin, tenantBaseDomain, tenantURLScheme, tenantURLPort)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-Tenant-Slug")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isTenantOrigin(origin, baseDomain, scheme, port string) bool {
	if baseDomain == "" || scheme == "" {
		return false
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != scheme || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Port() != port {
		return false
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	suffix := "." + baseDomain
	if !strings.HasSuffix(host, suffix) {
		return false
	}

	slug := strings.TrimSuffix(host, suffix)
	return slug != "" && !strings.Contains(slug, ".")
}
