package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var ErrPermanent = errors.New("permanent email delivery failure")

type Delivery struct {
	ID      string
	To      string
	Subject string
	Text    string
	HTML    string
}

type Provider interface {
	Name() string
	Send(context.Context, Delivery) (string, error)
}

type ProviderConfig struct {
	Name         string
	FromName     string
	FromAddress  string
	ZeptoURL     string
	ZeptoToken   string
	ResendURL    string
	ResendAPIKey string
	HTTPClient   *http.Client
}

func NewProvider(cfg ProviderConfig) (Provider, error) {
	if strings.TrimSpace(cfg.FromAddress) == "" {
		return nil, errors.New("EMAIL_FROM_ADDRESS is required")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Name)) {
	case "zeptomail":
		if cfg.ZeptoToken == "" {
			return nil, errors.New("ZEPTOMAIL_SEND_TOKEN is required when EMAIL_PROVIDER=zeptomail")
		}
		return &ZeptoMailProvider{url: cfg.ZeptoURL, token: cfg.ZeptoToken, fromName: cfg.FromName, fromAddress: cfg.FromAddress, client: cfg.HTTPClient}, nil
	case "resend":
		if cfg.ResendAPIKey == "" {
			return nil, errors.New("RESEND_API_KEY is required when EMAIL_PROVIDER=resend")
		}
		return &ResendProvider{url: cfg.ResendURL, apiKey: cfg.ResendAPIKey, fromName: cfg.FromName, fromAddress: cfg.FromAddress, client: cfg.HTTPClient}, nil
	default:
		return nil, fmt.Errorf("EMAIL_PROVIDER must be zeptomail or resend, got %q", cfg.Name)
	}
}

func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nodus-health-api")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		deliveryErr := fmt.Errorf("email provider returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
			return "", fmt.Errorf("%w: %v", ErrPermanent, deliveryErr)
		}
		return "", deliveryErr
	}
	return string(responseBody), nil
}

type ZeptoMailProvider struct {
	url, token, fromName, fromAddress string
	client                            *http.Client
}

func (p *ZeptoMailProvider) Name() string { return "zeptomail" }

func (p *ZeptoMailProvider) Send(ctx context.Context, message Delivery) (string, error) {
	payload := map[string]any{
		"from":    map[string]string{"address": p.fromAddress, "name": p.fromName},
		"to":      []any{map[string]any{"email_address": map[string]string{"address": message.To}}},
		"subject": message.Subject, "textbody": message.Text, "htmlbody": message.HTML,
		"client_reference": message.ID,
	}
	raw, err := postJSON(ctx, p.client, p.url, map[string]string{"Authorization": "Zoho-enczapikey " + p.token}, payload)
	if err != nil {
		return "", err
	}
	var response struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal([]byte(raw), &response)
	return response.RequestID, nil
}

type ResendProvider struct {
	url, apiKey, fromName, fromAddress string
	client                             *http.Client
}

func (p *ResendProvider) Name() string { return "resend" }

func (p *ResendProvider) Send(ctx context.Context, message Delivery) (string, error) {
	from := p.fromAddress
	if p.fromName != "" {
		from = fmt.Sprintf("%s <%s>", p.fromName, p.fromAddress)
	}
	payload := map[string]any{"from": from, "to": []string{message.To}, "subject": message.Subject, "text": message.Text, "html": message.HTML}
	raw, err := postJSON(ctx, p.client, p.url, map[string]string{"Authorization": "Bearer " + p.apiKey, "Idempotency-Key": message.ID}, payload)
	if err != nil {
		return "", err
	}
	var response struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(raw), &response)
	return response.ID, nil
}
