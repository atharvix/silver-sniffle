package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/observability"
)

type BrevoService struct {
	apiKey      string
	senderEmail string
	client      *http.Client
	logger      *slog.Logger
	metrics     *observability.Metrics
}

func NewBrevoService(apiKey, senderEmail string, logger *slog.Logger, metrics *observability.Metrics) *BrevoService {
	return &BrevoService{
		apiKey:      apiKey,
		senderEmail: senderEmail,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:  logger,
		metrics: metrics,
	}
}

func (s *BrevoService) IsConfigured() bool {
	return s.apiKey != "" && s.senderEmail != ""
}

type brevoRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoSender struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type brevoEmailPayload struct {
	Sender      brevoSender      `json:"sender"`
	To          []brevoRecipient `json:"to"`
	Subject     string           `json:"subject"`
	HTMLContent string           `json:"htmlContent"`
}

func (s *BrevoService) SendOTP(ctx context.Context, toEmail, otp string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("brevo is not configured")
	}

	htmlBody := s.buildOTPHtml(otp)
	payload := brevoEmailPayload{
		Sender:      brevoSender{Email: s.senderEmail, Name: "Kinjo"},
		To:          []brevoRecipient{{Email: toEmail}},
		Subject:     fmt.Sprintf("%s is your Kinjo verification code", otp),
		HTMLContent: htmlBody,
	}

	return s.sendWithRetry(ctx, payload, "send_otp")
}

func (s *BrevoService) SendWelcome(ctx context.Context, toEmail, name, about string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("brevo is not configured")
	}

	htmlBody := s.buildWelcomeHtml(toEmail, name, about)
	payload := brevoEmailPayload{
		Sender:      brevoSender{Email: s.senderEmail, Name: "Kinjo"},
		To:          []brevoRecipient{{Email: toEmail, Name: name}},
		Subject:     fmt.Sprintf("Welcome to Kinjo, %s 🎉", strings.Split(name, " ")[0]),
		HTMLContent: htmlBody,
	}

	return s.sendWithRetry(ctx, payload, "send_welcome")
}

func (s *BrevoService) sendWithRetry(ctx context.Context, payload brevoEmailPayload, operation string) error {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	var lastErr error
	maxRetries := 2

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt*200) * time.Millisecond):
			}
		}

		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("api-key", s.apiKey)

		resp, err := s.client.Do(req)
		duration := time.Since(start)

		if err != nil {
			lastErr = err
			if s.metrics != nil {
				s.metrics.ExternalAPIDuration.WithLabelValues("brevo", operation, "error").Observe(duration.Seconds())
			}
			continue
		}
		resp.Body.Close()

		if s.metrics != nil {
			s.metrics.ExternalAPIDuration.WithLabelValues("brevo", operation, fmt.Sprintf("%d", resp.StatusCode)).Observe(duration.Seconds())
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("brevo responded with status %d", resp.StatusCode)
		// Don't retry 4xx errors
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}

	return fmt.Errorf("failed after retries: %w", lastErr)
}

func (s *BrevoService) buildOTPHtml(otp string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800;900&display=swap" rel="stylesheet">
</head>
<body style="margin:0;padding:0;background:#0e0b08;font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background:#0e0b08;padding:40px 16px;">
    <tr><td align="center">
      <table width="100%%" style="max-width:480px;">
        <tr><td style="padding-bottom:32px;text-align:center;">
          <span style="font-size:26px;font-weight:900;color:#ffffff;letter-spacing:-1px;">Kinjo</span>
        </td></tr>
        <tr><td style="background:#141210;border:1px solid rgba(255,255,255,0.1);border-radius:14px;padding:40px 36px;">
          <p style="margin:0 0 8px;font-size:22px;font-weight:800;color:#ffffff;letter-spacing:-0.4px;">
            Your verification code
          </p>
          <p style="margin:0 0 28px;font-size:14px;color:rgba(255,255,255,0.5);line-height:1.55;">
            Enter this code to verify your email. It expires in 10&nbsp;minutes.
          </p>
          <div style="background:#ffffff;border-radius:14px;padding:24px;text-align:center;margin-bottom:28px;">
            <span style="font-size:42px;font-weight:900;color:#111111;letter-spacing:14px;">%s</span>
          </div>
          <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.3);line-height:1.5;">
            If you didn&rsquo;t request this, you can safely ignore this email.
          </p>
        </td></tr>
        <tr><td style="padding-top:24px;text-align:center;border-top:1px solid rgba(255,255,255,0.08);margin-top:24px;">
          <p style="margin:0;font-size:12px;color:rgba(255,255,255,0.25);">Kinjo &middot; Discover people around you</p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, html.EscapeString(otp))
}

func (s *BrevoService) buildWelcomeHtml(toEmail, name, about string) string {
	safeName := html.EscapeString(strings.TrimSpace(name))
	safeEmail := html.EscapeString(toEmail)
	safeFirstName := html.EscapeString(strings.Split(strings.TrimSpace(name), " ")[0])

	aboutParts := strings.Split(strings.TrimSpace(about), "\n")
	safeWhatYouDo := ""
	if len(aboutParts) > 0 {
		safeWhatYouDo = html.EscapeString(strings.TrimSpace(aboutParts[0]))
	}
	safeWhatLookingFor := ""
	if len(aboutParts) > 1 {
		safeWhatLookingFor = html.EscapeString(strings.TrimSpace(strings.Join(aboutParts[1:], " ")))
	}

	profileRows := ""
	if safeWhatYouDo != "" {
		profileRows += fmt.Sprintf(`<p style="margin:0 0 2px;font-size:11px;font-weight:600;color:rgba(0,0,0,0.4);letter-spacing:0.06em;text-transform:uppercase;">What you do</p><p style="margin:0 0 14px;font-size:14px;color:#333333;line-height:1.5;">%s</p>`, safeWhatYouDo)
	}
	if safeWhatLookingFor != "" {
		profileRows += fmt.Sprintf(`<p style="margin:0 0 2px;font-size:11px;font-weight:600;color:rgba(0,0,0,0.4);letter-spacing:0.06em;text-transform:uppercase;">What you&rsquo;re looking for</p><p style="margin:0 0 14px;font-size:14px;color:#444444;line-height:1.5;font-style:italic;">&ldquo;%s&rdquo;</p>`, safeWhatLookingFor)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800;900&display=swap" rel="stylesheet">
</head>
<body style="margin:0;padding:0;background:#0e0b08;font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background:#0e0b08;padding:40px 16px;">
    <tr><td align="center">
      <table width="100%%" style="max-width:480px;">
        <tr><td style="padding-bottom:32px;text-align:center;">
          <span style="font-size:26px;font-weight:900;color:#ffffff;letter-spacing:-1px;">Kinjo</span>
        </td></tr>
        <tr><td style="background:#141210;border:1px solid rgba(255,255,255,0.1);border-radius:14px;padding:40px 36px;">
          <p style="margin:0 0 6px;font-size:24px;font-weight:900;color:#ffffff;letter-spacing:-0.5px;">
            Welcome to Kinjo, %s &#x1F44B;
          </p>
          <p style="margin:0 0 28px;font-size:14px;color:rgba(255,255,255,0.5);line-height:1.55;">
            You&rsquo;re all set. People nearby can now discover you on Kinjo.
          </p>
          <table width="100%%" cellpadding="0" cellspacing="0" style="background:#ffffff;border-radius:14px;padding:20px 22px;margin-bottom:28px;">
            <tr><td>
              <p style="margin:0 0 2px;font-size:11px;font-weight:600;color:rgba(0,0,0,0.4);letter-spacing:0.06em;text-transform:uppercase;">Name</p>
              <p style="margin:0 0 14px;font-size:16px;font-weight:700;color:#111111;">%s</p>
              %s
              <p style="margin:0 0 2px;font-size:11px;font-weight:600;color:rgba(0,0,0,0.4);letter-spacing:0.06em;text-transform:uppercase;">Email</p>
              <p style="margin:0;font-size:13px;color:rgba(0,0,0,0.45);">%s</p>
            </td></tr>
          </table>
          <p style="margin:0 0 18px;font-size:15px;font-weight:700;color:#ffffff;">What happens next</p>
          <table width="100%%" cellpadding="0" cellspacing="0" style="margin-bottom:28px;">
            <tr><td style="padding-bottom:18px;">
              <p style="margin:0 0 3px;font-size:14px;font-weight:700;color:#ffffff;">📍 Discover people nearby</p>
              <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.45);line-height:1.55;">Kinjo surfaces interesting people in your area — no algorithm, no feed, just real proximity.</p>
            </td></tr>
            <tr><td style="padding-bottom:18px;">
              <p style="margin:0 0 3px;font-size:14px;font-weight:700;color:#ffffff;">💬 Start a real conversation</p>
              <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.45);line-height:1.55;">No follower counts, no likes — just a direct line to someone worth meeting.</p>
            </td></tr>
          </table>
          <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.25);line-height:1.5;border-top:1px solid rgba(255,255,255,0.08);padding-top:20px;">
            You&rsquo;re receiving this because you signed up for Kinjo.
          </p>
        </td></tr>
        <tr><td style="padding-top:24px;text-align:center;border-top:1px solid rgba(255,255,255,0.08);margin-top:4px;">
          <p style="margin:0;font-size:12px;color:rgba(255,255,255,0.25);">Kinjo &middot; Discover people around you</p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, safeFirstName, safeName, profileRows, safeEmail)
}
