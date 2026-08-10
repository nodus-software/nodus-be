package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSPreflightAllowsPut(t *testing.T) {
	origin := "http://localhost:5173"
	handler := CORS([]string{origin})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
