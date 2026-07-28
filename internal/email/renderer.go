package email

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed templates/**/*.tmpl
var templateFiles embed.FS

//go:embed templates/assets/logo.png
var logoPNG []byte

// defaultLogoDataURI embeds the nodus+ health full-color logo directly into
// outgoing emails so branding renders reliably without depending on a
// publicly hosted asset URL.
var defaultLogoDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(logoPNG)

type Kind string

const (
	OrganizationActivation Kind = "organization_activation"
	StaffInvitation        Kind = "staff_invitation"
	PasswordResetRequested Kind = "password_reset_requested"
)

type CommonData struct {
	AppName          string
	AppURL           string
	LogoURL          string
	RecipientName    string
	OrganizationName string
	SupportEmail     string
	SupportURL       string
	PrivacyURL       string
	CurrentYear      int
	Preheader        string
	Heading          string
	CTAURL           string
	CTALabel         string
}

type OrganizationActivationData struct {
	CommonData
	ActivationURL string
	ExpiresAt     time.Time
}

type StaffInvitationData struct {
	CommonData
	InviterName string
	InviteURL   string
	ExpiresAt   time.Time
	IsResend    bool
}

type PasswordResetData struct {
	CommonData
	ResetURL  string
	ExpiresAt time.Time
}

type Rendered struct {
	Subject string
	HTML    string
	Text    string
}

type Renderer struct {
	defaults CommonData
}

func NewRenderer(defaults CommonData) *Renderer {
	if defaults.AppName == "" {
		defaults.AppName = "Nodus Health"
	}
	if defaults.CurrentYear == 0 {
		defaults.CurrentYear = time.Now().Year()
	}
	if defaults.LogoURL == "" {
		defaults.LogoURL = defaultLogoDataURI
	}
	return &Renderer{defaults: defaults}
}

func (r *Renderer) RenderOrganizationActivation(data OrganizationActivationData) (Rendered, error) {
	data.CommonData = r.withDefaults(data.CommonData)
	data.Heading = "Activate your organization"
	data.CTAURL = data.ActivationURL
	data.CTALabel = "Activate organization"
	data.Preheader = fmt.Sprintf("Activate %s and create your administrator credentials.", data.OrganizationName)
	return r.render(OrganizationActivation, fmt.Sprintf("Activate your %s organization", data.AppName), data)
}

func (r *Renderer) RenderStaffInvitation(data StaffInvitationData) (Rendered, error) {
	data.CommonData = r.withDefaults(data.CommonData)
	data.Heading = "Join " + data.OrganizationName
	data.CTAURL = data.InviteURL
	data.CTALabel = "Accept invitation"
	data.Preheader = fmt.Sprintf("You've been invited to join %s on %s.", data.OrganizationName, data.AppName)
	subject := "You've been invited to join " + data.OrganizationName
	if data.IsResend {
		data.Heading = "Your new invitation"
		data.Preheader = "A new invitation link has been issued for your account."
		subject = "Your new invitation to " + data.OrganizationName
	}
	return r.render(StaffInvitation, subject, data)
}

func (r *Renderer) RenderPasswordReset(data PasswordResetData) (Rendered, error) {
	data.CommonData = r.withDefaults(data.CommonData)
	data.Heading = "Reset your password"
	data.CTAURL = data.ResetURL
	data.CTALabel = "Reset password"
	data.Preheader = fmt.Sprintf("Reset the password for your %s account.", data.AppName)
	return r.render(PasswordResetRequested, "Reset your "+data.AppName+" password", data)
}

func (r *Renderer) withDefaults(data CommonData) CommonData {
	if data.AppName == "" {
		data.AppName = r.defaults.AppName
	}
	if data.AppURL == "" {
		data.AppURL = r.defaults.AppURL
	}
	if data.LogoURL == "" {
		data.LogoURL = r.defaults.LogoURL
	}
	if data.SupportEmail == "" {
		data.SupportEmail = r.defaults.SupportEmail
	}
	if data.SupportURL == "" {
		data.SupportURL = r.defaults.SupportURL
	}
	if data.PrivacyURL == "" {
		data.PrivacyURL = r.defaults.PrivacyURL
	}
	if data.CurrentYear == 0 {
		data.CurrentYear = r.defaults.CurrentYear
	}
	return data
}

func (r *Renderer) render(kind Kind, subject string, data any) (Rendered, error) {
	funcs := template.FuncMap{
		"formatTime": formatTime,
		// LogoURL is server-controlled branding (a config value or the embedded
		// data: URI below), never end-user input, so it's safe to mark as a
		// trusted URL. Without this, html/template's URL sanitizer strips
		// data: URIs and replaces the src with "#ZgotmplZ".
		"safeURL": func(s string) template.URL { return template.URL(s) },
	}
	htmlFiles := []string{
		"templates/layouts/base.html.tmpl",
		"templates/partials/header.html.tmpl",
		"templates/partials/button.html.tmpl",
		"templates/partials/footer.html.tmpl",
		filepath.ToSlash("templates/messages/" + string(kind) + ".html.tmpl"),
	}
	htmlTemplate, err := template.New("email").Funcs(funcs).ParseFS(templateFiles, htmlFiles...)
	if err != nil {
		return Rendered{}, fmt.Errorf("parse %s HTML template: %w", kind, err)
	}
	var htmlBody bytes.Buffer
	if err := htmlTemplate.ExecuteTemplate(&htmlBody, "base", data); err != nil {
		return Rendered{}, fmt.Errorf("render %s HTML template: %w", kind, err)
	}

	textFuncs := texttemplate.FuncMap{"formatTime": formatTime}
	textFiles := []string{
		"templates/layouts/base.txt.tmpl",
		filepath.ToSlash("templates/messages/" + string(kind) + ".txt.tmpl"),
	}
	textTemplate, err := texttemplate.New("email").Funcs(textFuncs).ParseFS(templateFiles, textFiles...)
	if err != nil {
		return Rendered{}, fmt.Errorf("parse %s text template: %w", kind, err)
	}
	var textBody bytes.Buffer
	if err := textTemplate.ExecuteTemplate(&textBody, "base", data); err != nil {
		return Rendered{}, fmt.Errorf("render %s text template: %w", kind, err)
	}

	return Rendered{Subject: subject, HTML: htmlBody.String(), Text: strings.TrimSpace(textBody.String()) + "\n"}, nil
}

func formatTime(value time.Time) string {
	return value.Format("2 January 2006 at 15:04 MST")
}
