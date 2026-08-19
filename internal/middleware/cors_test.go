package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSPreflightAllowsPut(t *testing.T) {
	origin := "http://localhost:5173"
	handler := CORS([]string{origin}, "example.com", "https", "")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("preflight request must not reach the next handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/clinical/encounters/encounter-id/form", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
	}
	methods := response.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(methods, http.MethodPut) {
		t.Fatalf("expected PUT in Access-Control-Allow-Methods, got %q", methods)
	}
}

func TestCORSAllowsTenantSubdomain(t *testing.T) {
	origin := "https://tenant-one.nodus.co.ke"
	handler := CORS(nil, "nodus.co.ke", "https", "")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", origin)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("expected tenant origin %q to be allowed, got %q", origin, got)
	}
}

func TestCORSRejectsNonTenantOrigins(t *testing.T) {
	tests := []string{
		"https://nodus.co.ke",
		"https://nodus.co.ke.attacker.com",
		"https://tenant.nodus.co.ke.attacker.com",
		"https://nested.tenant.nodus.co.ke",
		"http://tenant.nodus.co.ke",
		"https://tenant.nodus.co.ke:8443",
	}

	for _, origin := range tests {
		t.Run(origin, func(t *testing.T) {
			handler := CORS(nil, "nodus.co.ke", "https", "")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.Header.Set("Origin", origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)

			if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("expected origin to be rejected, got Access-Control-Allow-Origin %q", got)
			}
		})
	}
}
