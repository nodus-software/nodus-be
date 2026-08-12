// Command email-preview renders sample emails to local files without sending
// them. It is intended for development and design review only.
package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nodus-health/internal/email"
)

type preview struct {
	name   string
	render func(*email.Renderer) (email.Rendered, error)
}

func main() {
	outputFlag := flag.String("output", "tmp/email-previews", "directory to write rendered previews")
	flag.Parse()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}
	outputDir := *outputFlag
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(repoRoot, outputDir)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatal(fmt.Errorf("create preview directory: %w", err))
	}

	renderer := email.NewRenderer(email.CommonData{
		AppName:      "Nodus Health",
		AppURL:       "https://nodus.example.test",
		LogoURL:      "https://res.cloudinary.com/dwjwrng2b/image/upload/v1786552657/full_logo_no_bg_eqgkg0.png",
		SupportEmail: "support@nodushealth.example",
		SupportURL:   "https://nodus.example.test/support",
		PrivacyURL:   "https://nodus.example.test/privacy",
		CurrentYear:  time.Now().Year(),
	})

	expires := time.Now().Add(24 * time.Hour).Truncate(time.Minute)
	previews := []preview{
		{
			name: "organization_activation",
			render: func(r *email.Renderer) (email.Rendered, error) {
				return r.RenderOrganizationActivation(email.OrganizationActivationData{
					CommonData:    email.CommonData{RecipientName: "Amina", OrganizationName: "Nodus Family Clinic"},
					ActivationURL: "https://nodus.example.test/activate-organization?token=preview-token&slug=nodus-family-clinic",
					ExpiresAt:     expires,
				})
			},
		},
		{
			name: "staff_invitation",
			render: func(r *email.Renderer) (email.Rendered, error) {
				return r.RenderStaffInvitation(email.StaffInvitationData{
					CommonData:  email.CommonData{RecipientName: "Kamau", OrganizationName: "Nodus Family Clinic"},
					InviterName: "Amina Wanjiku",
					InviteURL:   "https://nodus.example.test/accept-invite?token=preview-token",
					ExpiresAt:   expires,
				})
			},
		},
		{
			name: "staff_invitation_resent",
			render: func(r *email.Renderer) (email.Rendered, error) {
				return r.RenderStaffInvitation(email.StaffInvitationData{
					CommonData: email.CommonData{RecipientName: "Kamau", OrganizationName: "Nodus Family Clinic"},
					InviteURL:  "https://nodus.example.test/accept-invite?token=new-preview-token",
					ExpiresAt:  expires,
					IsResend:   true,
				})
			},
		},
		{
			name: "password_reset_requested",
			render: func(r *email.Renderer) (email.Rendered, error) {
				return r.RenderPasswordReset(email.PasswordResetData{
					CommonData: email.CommonData{RecipientName: "Amina"},
					ResetURL:   "https://nodus.example.test/reset-password?token=preview-token",
					ExpiresAt:  expires,
				})
			},
		},
	}

	var index strings.Builder
	index.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Nodus email previews</title></head><body style=\"margin:40px;font-family:Arial,sans-serif;color:#353940;background:#f6f7f9\"><main style=\"max-width:760px;margin:auto\"><h1>Email previews</h1><p>Generated locally from the production templates. These links do not perform real actions.</p><ul style=\"line-height:2\">")
	for _, item := range previews {
		rendered, err := item.render(renderer)
		if err != nil {
			fatal(fmt.Errorf("render %s: %w", item.name, err))
		}
		if err := writePreview(outputDir, item.name, rendered); err != nil {
			fatal(err)
		}
		fmt.Fprintf(&index, "<li><a href=\"%s.html\">%s</a> <small>&mdash; %s</small></li>", html.EscapeString(item.name), html.EscapeString(strings.ReplaceAll(item.name, "_", " ")), html.EscapeString(rendered.Subject))
	}
	index.WriteString("</ul></main></body></html>\n")
	if err := os.WriteFile(filepath.Join(outputDir, "index.html"), []byte(index.String()), 0o644); err != nil {
		fatal(fmt.Errorf("write preview index: %w", err))
	}

	fmt.Printf("Rendered %d email previews to %s\n", len(previews), outputDir)
	fmt.Printf("Open %s in a browser.\n", filepath.Join(outputDir, "index.html"))
}

func writePreview(outputDir, name string, rendered email.Rendered) error {
	if err := os.WriteFile(filepath.Join(outputDir, name+".html"), []byte(rendered.HTML), 0o644); err != nil {
		return fmt.Errorf("write %s HTML preview: %w", name, err)
	}
	text := "Subject: " + rendered.Subject + "\n\n" + rendered.Text
	if err := os.WriteFile(filepath.Join(outputDir, name+".txt"), []byte(text), 0o644); err != nil {
		return fmt.Errorf("write %s text preview: %w", name, err)
	}
	return nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repository root containing go.mod")
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "email-preview:", err)
	os.Exit(1)
}
