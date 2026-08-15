package organizations

import (
	"testing"

	"nodus-health/pkg/logger"
)

func TestReservedOrganizationSlugs(t *testing.T) {
	service := NewService(nil, nil, nil, Config{ReservedSlugs: []string{"billing"}}, logger.NewLogger())
	for _, slug := range []string{"app", "noreply", "billing", "BILLING"} {
		if !service.isReservedSlug(slug) {
			t.Fatalf("expected %q to be reserved", slug)
		}
	}
	if service.isReservedSlug("green-clinic") {
		t.Fatal("ordinary tenant slug must remain available")
	}
}

func TestTenantURL(t *testing.T) {
	service := NewService(nil, nil, nil, Config{TenantBaseDomain: "example.com", TenantURLScheme: "https"}, logger.NewLogger())
	got := service.tenantURL("green-clinic", "/login")
	if got != "https://green-clinic.example.com/login" {
		t.Fatalf("tenant URL = %q", got)
	}
}
