package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/observability"
)

type SMTPService struct {
	host        string
	port        int
	username    string
	password    string
	senderEmail string
	senderName  string
	encryption  string // "tls" (STARTTLS), "ssl" (direct TLS), "none"
	logger      *slog.Logger
	metrics     *observability.Metrics
}

func NewSMTPService(
	host string,
	port int,
	username string,
	password string,
	senderEmail string,
	senderName string,
	encryption string,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *SMTPService {
	if port <= 0 {
		port = 587
	}
	if senderName == "" {
		senderName = "Kinjo"
	}
	if senderEmail == "" {
		senderEmail = "hello@kinjo.world"
	}
	if encryption == "" {
		if port == 465 {
			encryption = "ssl"
		} else {
			encryption = "tls"
		}
	}

	return &SMTPService{
		host:        strings.TrimSpace(host),
		port:        port,
		username:    strings.TrimSpace(username),
		password:    password,
		senderEmail: strings.TrimSpace(senderEmail),
		senderName:  strings.TrimSpace(senderName),
		encryption:  strings.ToLower(strings.TrimSpace(encryption)),
		logger:      logger,
		metrics:     metrics,
	}
}

func (s *SMTPService) IsConfigured() bool {
	return s.host != "" && s.senderEmail != ""
}

func (s *SMTPService) SendOTP(ctx context.Context, toEmail, otp string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("smtp is not configured")
	}

	subject := fmt.Sprintf("%s is your Kinjo verification code", otp)
	htmlBody := s.buildOTPHtml(otp)

	return s.sendMailWithRetry(ctx, toEmail, subject, htmlBody, "send_otp")
}

func (s *SMTPService) SendWelcome(ctx context.Context, toEmail, name, about string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("smtp is not configured")
	}

	safeFirstName := strings.Split(strings.TrimSpace(name), " ")[0]
	subject := fmt.Sprintf("Welcome to Kinjo, %s 🎉", safeFirstName)
	htmlBody := s.buildWelcomeHtml(toEmail, name, about)

	return s.sendMailWithRetry(ctx, toEmail, subject, htmlBody, "send_welcome")
}

func (s *SMTPService) sendMailWithRetry(ctx context.Context, toEmail, subject, htmlBody, operation string) error {
	var lastErr error
	maxRetries := 2

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt*300) * time.Millisecond):
			}
		}

		start := time.Now()
		err := s.sendMail(ctx, toEmail, subject, htmlBody)
		duration := time.Since(start)

		if err == nil {
			if s.metrics != nil {
				s.metrics.ExternalAPIDuration.WithLabelValues("smtp", operation, "200").Observe(duration.Seconds())
			}
			return nil
		}

		lastErr = err
		if s.metrics != nil {
			s.metrics.ExternalAPIDuration.WithLabelValues("smtp", operation, "error").Observe(duration.Seconds())
		}
		s.logger.WarnContext(ctx, "smtp send attempt failed",
			slog.Int("attempt", attempt+1),
			slog.String("to", toEmail),
			slog.String("operation", operation),
			slog.String("error", err.Error()),
		)
	}

	return fmt.Errorf("failed to send email via smtp after retries: %w", lastErr)
}

func (s *SMTPService) sendMail(ctx context.Context, toEmail, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	// Build MIME message with UTF-8 support
	header := make(map[string]string)
	header["From"] = fmt.Sprintf("%s <%s>", s.senderName, s.senderEmail)
	header["To"] = toEmail
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=UTF-8"
	header["Date"] = time.Now().Format(time.RFC1123Z)

	var message strings.Builder
	for k, v := range header {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n")
	message.WriteString(htmlBody)

	msgBytes := []byte(message.String())

	var auth smtp.Auth
	if s.username != "" && s.password != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	tlsConfig := &tls.Config{
		ServerName: s.host,
	}

	// Direct SSL/TLS connection (typical for port 465)
	if s.encryption == "ssl" || s.port == 465 {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to dial tls to smtp server: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return fmt.Errorf("failed to create smtp client: %w", err)
		}
		defer client.Close()

		if auth != nil {
			if ok, _ := client.Extension("AUTH"); ok {
				if err := client.Auth(auth); err != nil {
					return fmt.Errorf("smtp auth failed: %w", err)
				}
			}
		}

		if err := client.Mail(s.senderEmail); err != nil {
			return fmt.Errorf("smtp MAIL FROM failed: %w", err)
		}
		if err := client.Rcpt(toEmail); err != nil {
			return fmt.Errorf("smtp RCPT TO failed: %w", err)
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp DATA command failed: %w", err)
		}
		if _, err := w.Write(msgBytes); err != nil {
			_ = w.Close()
			return fmt.Errorf("smtp write message failed: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("smtp close data writer failed: %w", err)
		}

		return client.Quit()
	}

	// STARTTLS or plaintext connection (typical for port 587 / 25)
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to smtp server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("failed to create smtp client: %w", err)
	}
	defer client.Close()

	// Use STARTTLS if supported and not explicitly set to "none"
	if s.encryption != "none" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("smtp STARTTLS failed: %w", err)
			}
		}
	}

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth failed: %w", err)
			}
		}
	}

	if err := client.Mail(s.senderEmail); err != nil {
		return fmt.Errorf("smtp MAIL FROM failed: %w", err)
	}
	if err := client.Rcpt(toEmail); err != nil {
		return fmt.Errorf("smtp RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA command failed: %w", err)
	}
	if _, err := w.Write(msgBytes); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write message failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data writer failed: %w", err)
	}

	return client.Quit()
}

func (s *SMTPService) buildOTPHtml(otp string) string {
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

func (s *SMTPService) buildWelcomeHtml(toEmail, name, about string) string {
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
