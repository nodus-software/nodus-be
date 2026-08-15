package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"nodus-health/internal/tenant"
)

type tenantResolverStub struct{ slug string }

func (s *tenantResolverStub) ResolveTenant(_ context.Context, slug string) (tenant.Identity, error) {
	s.slug = slug
	return tenant.Identity{ID: "tenant-id", Slug: slug}, nil
}

func TestResolveTenantFromConfiguredSubdomain(t *testing.T) {
	resolver := &tenantResolverStub{}
	handler := ResolveTenant(resolver, TenantHostConfig{BaseDomain: "example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if identity, ok := tenant.FromContext(r.Context()); !ok || identity.Slug != "clinic" {
			t.Fatalf("tenant context = %#v, %v", identity, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "https://clinic.example.com/auth/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || resolver.slug != "clinic" {
		t.Fatalf("status=%d slug=%q", rec.Code, resolver.slug)
	}
}

func TestResolveTenantRejectsNestedAndForeignHosts(t *testing.T) {
	for _, host := range []string{"nested.clinic.example.com", "clinic.example.net", "example.com"} {
		t.Run(host, func(t *testing.T) {
			handler := ResolveTenant(&tenantResolverStub{}, TenantHostConfig{BaseDomain: "example.com"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("next handler must not run")
			}))
			req := httptest.NewRequest(http.MethodGet, "https://"+host+"/auth/me", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestResolveTenantIgnoresHeaderWhenDisabled(t *testing.T) {
	resolver := &tenantResolverStub{}
	handler := ResolveTenant(resolver, TenantHostConfig{BaseDomain: "example.com", AllowSlugHeader: false})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "https://clinic.example.com/auth/me", nil)
	req.Header.Set("X-Tenant-Slug", "attacker")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || resolver.slug != "clinic" {
		t.Fatalf("status=%d slug=%q", rec.Code, resolver.slug)
	}
}

func TestPublicTenantEndpointsRequirePrimaryDomain(t *testing.T) {
	handler := ResolveTenant(&tenantResolverStub{}, TenantHostConfig{BaseDomain: "example.com"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		host string
		want int
	}{{"example.com", http.StatusNoContent}, {"clinic.example.com", http.StatusBadRequest}} {
		req := httptest.NewRequest(http.MethodPost, "https://"+tc.host+"/auth/organization-discovery/request", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("host=%s status=%d want=%d", tc.host, rec.Code, tc.want)
		}
	}
}

func TestOrganizationDiscoveryVerificationRunsOnTenantDomain(t *testing.T) {
	resolver := &tenantResolverStub{}
	handler := ResolveTenant(resolver, TenantHostConfig{BaseDomain: "example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := tenant.FromContext(r.Context())
		if !ok || identity.Slug != "clinic" {
			t.Fatalf("tenant context = %#v, %v", identity, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "https://clinic.example.com/auth/organization-discovery/verify", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || resolver.slug != "clinic" {
		t.Fatalf("status=%d slug=%q body=%s", rec.Code, resolver.slug, rec.Body.String())
	}
}
