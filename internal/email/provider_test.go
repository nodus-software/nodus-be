package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestResendProviderRequest(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer resend-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "message-id" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["from"] != "Nodus Health <no-reply@nodus.test>" || body["subject"] != "Welcome" {
			t.Fatalf("unexpected payload: %#v", body)
		}
		return response(http.StatusOK, `{"id":"provider-id"}`), nil
	})}
	provider, err := NewProvider(ProviderConfig{Name: "resend", FromName: "Nodus Health", FromAddress: "no-reply@nodus.test",
		ResendURL: "https://resend.test/emails", ResendAPIKey: "resend-secret", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	id, err := provider.Send(context.Background(), Delivery{ID: "message-id", To: "founder@example.com", Subject: "Welcome", Text: "text", HTML: "<p>text</p>"})
	if err != nil || id != "provider-id" {
		t.Fatalf("Send() = %q, %v", id, err)
	}
}

func TestZeptoMailProviderRequest(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Zoho-enczapikey zepto-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["client_reference"] != "message-id" || body["htmlbody"] != "<p>text</p>" {
			t.Fatalf("unexpected payload: %#v", body)
		}
		return response(http.StatusOK, `{"request_id":"zepto-id"}`), nil
	})}
	provider, err := NewProvider(ProviderConfig{Name: "zeptomail", FromAddress: "no-reply@nodus.test",
		ZeptoURL: "https://zepto.test/email", ZeptoToken: "zepto-secret", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	id, err := provider.Send(context.Background(), Delivery{ID: "message-id", To: "founder@example.com", Subject: "Welcome", HTML: "<p>text</p>"})
	if err != nil || id != "zepto-id" {
		t.Fatalf("Send() = %q, %v", id, err)
	}
}

func TestProviderClassifiesInvalidPayloadAsPermanent(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusUnprocessableEntity, "invalid recipient"), nil
	})}
	provider, err := NewProvider(ProviderConfig{Name: "resend", FromAddress: "no-reply@nodus.test",
		ResendURL: "https://resend.test/emails", ResendAPIKey: "secret", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Send(context.Background(), Delivery{ID: "message-id", To: "bad", Subject: "subject"})
	if !errors.Is(err, ErrPermanent) {
		t.Fatalf("expected permanent error, got %v", err)
	}
}
