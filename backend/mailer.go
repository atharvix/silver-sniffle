package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// sendMailMIME sends a multipart/alternative (HTML + plain text) email, or logs it when
// SMTP isn't configured (so flows are testable without a mail server).
func (a *App) sendMailMIME(toEmail, subject, textBody, htmlBody string) error {
	if !a.cfg.smtpConfigured() {
		log.Printf("SMTP not configured; would email %s — %q:\n[TEXT]\n%s\n[HTML len %d]", toEmail, subject, textBody, len(htmlBody))
		return nil
	}

	bBytes := make([]byte, 12)
	_, _ = rand.Read(bBytes)
	boundary := "=_kinjo_" + hex.EncodeToString(bBytes)

	var msg strings.Builder
	msg.WriteString("From: " + a.cfg.SMTPFrom + "\r\n")
	msg.WriteString("To: " + toEmail + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")

	if htmlBody != "" {
		msg.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")

		// Text part
		msg.WriteString("--" + boundary + "\r\n")
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		msg.WriteString(textBody + "\r\n\r\n")

		// HTML part
		msg.WriteString("--" + boundary + "\r\n")
		msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		msg.WriteString(htmlBody + "\r\n\r\n")

		msg.WriteString("--" + boundary + "--\r\n")
	} else {
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		msg.WriteString(textBody + "\r\n")
	}

	var auth smtp.Auth
	if a.cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", a.cfg.SMTPUser, a.cfg.SMTPPass, a.cfg.SMTPHost)
	}
	addr := a.cfg.SMTPHost + ":" + a.cfg.SMTPPort

	d := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if a.cfg.SMTPPort == "465" { // implicit TLS: Utho blocks outbound 25/587
		conn, err = tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: a.cfg.SMTPHost})
	} else {
		conn, err = d.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	client, err := smtp.NewClient(conn, a.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if a.cfg.SMTPPort != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: a.cfg.SMTPHost}); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
	}
	if err := client.Mail(a.cfg.SMTPFrom); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(toEmail); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg.String())); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return client.Quit()
}

// sendMail wraps sendMailMIME with text only for backwards compatibility.
func (a *App) sendMail(toEmail, subject, text string) error {
	return a.sendMailMIME(toEmail, subject, text, "")
}

func (a *App) sendVerificationLink(toEmail, link string) error {
	subject := "Verify your email for Kinjo"
	text := "Confirm your email for Kinjo by opening this link:\r\n\r\n" + link +
		"\r\n\r\nIf you didn't request this, you can ignore this email."

	htmlBody := emailShell("Confirm your email address", `
		<p style="margin:0 0 16px 0;font-size:15px;line-height:1.6;color:#3f3f46;">Please confirm your email address for Kinjo by tapping the button below:</p>
		<div style="margin:28px 0;text-align:center;">
			<a href="`+html.EscapeString(link)+`" class="btn-main" style="display:inline-block;padding:13px 28px;background:#09090b;color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;border-radius:9999px;letter-spacing:0.01em;">Verify Email Address</a>
		</div>
		<p style="margin:16px 0 0 0;font-size:13px;line-height:1.5;color:#71717a;">Or copy and paste this link into your browser:<br><a href="`+html.EscapeString(link)+`" style="color:#09090b;word-break:break-all;">`+html.EscapeString(link)+`</a></p>
		<p style="margin:16px 0 0 0;font-size:13px;line-height:1.5;color:#a1a1aa;">If you didn't request this verification, you can safely ignore this email.</p>
	`)

	return a.sendMailMIME(toEmail, subject, text, htmlBody)
}

// sendWelcomeEmail sends the normal welcome email for OAuth (LinkedIn & Google) users.
func (a *App) sendWelcomeEmail(toEmail, firstName, verifyLink string) error {
	greeting := "Hi"
	if firstName != "" {
		greeting = "Hi " + firstName
	}

	subject := "Welcome to Kinjo 👋"
	textConfirm := ""
	if verifyLink != "" {
		subject = "Welcome to Kinjo — confirm your email"
		textConfirm = "\r\nPlease confirm your email address (link valid for 24 hours):\r\n" + verifyLink + "\r\n"
	}

	text := greeting + ",\r\n\r\n" +
		"Welcome to Kinjo! Kinjo is built for founders, professionals, and creators to connect with people within 30 meters in the real world.\r\n" +
		textConfirm + "\r\n" +
		"Here is how to get the most out of Kinjo:\r\n" +
		"1. Complete your card: Upload a photo and tell people what you do and what you're looking for.\r\n" +
		"2. Stay discoverable: Set Location to 'Allow all the time' and Battery to 'Unrestricted' so you stay visible when your phone is in your pocket.\r\n" +
		"3. Find your people: When you're out at a coffee shop, tech hub, or event, open Kinjo to see who's near you.\r\n\r\n" +
		"Open Kinjo: https://kinjo.world\r\n\r\n" +
		"— The Kinjo Team"

	var confirmBlock string
	if verifyLink != "" {
		confirmBlock = `
		<div style="margin:24px 0;padding:20px;background:#f4f4f5;border-radius:12px;border:1px solid #e4e4e7;">
			<p style="margin:0 0 12px 0;font-size:14px;font-weight:600;color:#18181b;">Action Required: Confirm your email</p>
			<p style="margin:0 0 16px 0;font-size:13.5px;color:#52525b;line-height:1.5;">Please confirm your address to complete your account security.</p>
			<a href="` + html.EscapeString(verifyLink) + `" class="btn-main" style="display:inline-block;padding:11px 22px;background:#09090b;color:#ffffff;text-decoration:none;font-size:13px;font-weight:600;border-radius:8px;">Confirm Email</a>
		</div>`
	}

	htmlBody := emailShell("Welcome to Kinjo", `
		<p style="margin:0 0 16px 0;font-size:15px;line-height:1.6;color:#3f3f46;">`+html.EscapeString(greeting)+`,</p>
		<p style="margin:0 0 18px 0;font-size:15px;line-height:1.6;color:#3f3f46;">Welcome to <b>Kinjo</b>. Kinjo connects you with founders, creators, and professionals within 30&nbsp;meters in the physical world — no cold DMs, no connection requests, and no online small talk.</p>
		`+confirmBlock+`
		<div style="margin:28px 0 24px 0;border-top:1px solid #e4e4e7;padding-top:24px;">
			<p style="margin:0 0 14px 0;font-size:14px;font-weight:600;color:#18181b;">Three steps to get started:</p>
			<div style="margin-bottom:14px;">
				<p style="margin:0;font-size:14px;line-height:1.5;color:#3f3f46;"><b>1. Finish your card:</b> Upload a photo and share what you do and what you're looking for.</p>
			</div>
			<div style="margin-bottom:14px;">
				<p style="margin:0;font-size:14px;line-height:1.5;color:#3f3f46;"><b>2. Stay discoverable:</b> Keep Location set to <i>Allow all the time</i> and Battery to <i>Unrestricted</i> so Kinjo works smoothly in your pocket.</p>
			</div>
			<div>
				<p style="margin:0;font-size:14px;line-height:1.5;color:#3f3f46;"><b>3. Walk up &amp; connect:</b> Next time you're at a café, conference, or coworking space, see who's around and say hi.</p>
			</div>
		</div>
		<div style="margin:32px 0 16px 0;text-align:center;">
			<a href="https://kinjo.world" class="btn-main" style="display:inline-block;padding:13px 32px;background:#09090b;color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;border-radius:9999px;">Open Kinjo</a>
		</div>
		<p style="margin:24px 0 0 0;font-size:14px;color:#71717a;">— The Kinjo Team</p>
	`)

	return a.sendMailMIME(toEmail, subject, text, htmlBody)
}

// sendWelcomeWithOTPEmail sends the combined Welcome + OTP email for first-time email signups.
func (a *App) sendWelcomeWithOTPEmail(toEmail, code string) error {
	subject := "Welcome to Kinjo — your code is " + code

	text := "Welcome to Kinjo!\r\n\r\n" +
		"Your 6-digit sign-in code is: " + code + "\r\n\r\n" +
		"This code expires in 10 minutes.\r\n\r\n" +
		"Kinjo connects you with founders, creators, and professionals within 30 meters in the real world.\r\n" +
		"Once signed in, complete your card so people around you can discover what you do.\r\n\r\n" +
		"If you didn't request this code, you can safely ignore this email.\r\n\r\n" +
		"— The Kinjo Team"

	htmlBody := emailShell("Welcome to Kinjo", `
		<p style="margin:0 0 16px 0;font-size:15px;line-height:1.6;color:#3f3f46;">Welcome to <b>Kinjo</b>! You're one step away from connecting with professionals and collaborators within 30&nbsp;meters in the real world.</p>
		<p style="margin:0 0 8px 0;font-size:14.5px;color:#3f3f46;">Enter this verification code in the app to complete your sign-in:</p>
		<div class="otp-box" style="margin:24px 0;padding:22px 16px;background:#f4f4f5;border:1px solid #e4e4e7;border-radius:12px;text-align:center;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;font-size:36px;font-weight:700;letter-spacing:10px;color:#09090b;">
			`+html.EscapeString(code)+`
		</div>
		<p style="margin:0 0 20px 0;font-size:13px;line-height:1.5;color:#71717a;text-align:center;">This code is valid for <b>10 minutes</b>. Never share this code with anyone.</p>
		<div style="margin:24px 0 0 0;border-top:1px solid #e4e4e7;padding-top:20px;">
			<p style="margin:0 0 8px 0;font-size:13.5px;line-height:1.5;color:#52525b;">Once signed in, complete your card and turn on proximity to discover people around you in real time.</p>
			<p style="margin:12px 0 0 0;font-size:12.5px;line-height:1.5;color:#a1a1aa;">If you did not request this sign-in code, you can safely ignore this email: nobody can sign in without it.</p>
		</div>
		<p style="margin:24px 0 0 0;font-size:14px;color:#71717a;">— The Kinjo Team</p>
	`)

	return a.sendMailMIME(toEmail, subject, text, htmlBody)
}

// sendOTPEmail sends a clean, direct security OTP code email (for resends and returning users) with NO welcome copy.
func (a *App) sendOTPEmail(toEmail, code string) error {
	subject := "Your Kinjo sign-in code: " + code

	text := "Your Kinjo sign-in code is: " + code + "\r\n\r\n" +
		"This code expires in 10 minutes.\r\n\r\n" +
		"If you didn't request this code, you can safely ignore this email: nobody can sign in without it.\r\n\r\n" +
		"— The Kinjo Team"

	htmlBody := emailShell("Sign in to Kinjo", `
		<p style="margin:0 0 8px 0;font-size:14.5px;color:#3f3f46;">Enter this verification code in Kinjo to sign in to your account:</p>
		<div class="otp-box" style="margin:24px 0;padding:22px 16px;background:#f4f4f5;border:1px solid #e4e4e7;border-radius:12px;text-align:center;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;font-size:36px;font-weight:700;letter-spacing:10px;color:#09090b;">
			`+html.EscapeString(code)+`
		</div>
		<p style="margin:0 0 20px 0;font-size:13px;line-height:1.5;color:#71717a;text-align:center;">This code is valid for <b>10 minutes</b>.</p>
		<div style="margin:24px 0 0 0;border-top:1px solid #e4e4e7;padding-top:16px;">
			<p style="margin:0;font-size:12.5px;line-height:1.5;color:#a1a1aa;">For your security, never share this code with anyone. If you didn't request this code, no action is required.</p>
		</div>
	`)

	return a.sendMailMIME(toEmail, subject, text, htmlBody)
}

// emailShell wraps email content in an industry-standard, responsive HTML email shell.
func emailShell(heading, innerContent string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>` + html.EscapeString(heading) + `</title>
<style>
  :root { color-scheme: light dark; supported-color-schemes: light dark; }
  body { margin:0; padding:0; width:100% !important; -webkit-text-size-adjust:100%; -ms-text-size-adjust:100%; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif; background-color:#fafafa; color:#18181b; }
  .email-container { max-width:540px; margin:0 auto; padding:40px 20px; }
  .email-card { background:#ffffff; border:1px solid #e4e4e7; border-radius:16px; padding:36px 32px; box-shadow:0 1px 3px rgba(0,0,0,0.03); }
  @media (prefers-color-scheme: dark) {
    body { background-color:#09090b !important; color:#f4f4f5 !important; }
    .email-card { background:#121214 !important; border-color:#27272a !important; }
    h1, h2, b, strong { color:#fafafa !important; }
    p, li, span { color:#a1a1aa !important; }
    .otp-box { background:#18181b !important; border-color:#27272a !important; color:#ffffff !important; }
    .btn-main { background:#ffffff !important; color:#09090b !important; }
    .brand-mark { color:#ffffff !important; }
    .footer-sub { color:#71717a !important; }
  }
</style>
</head>
<body style="margin:0;padding:0;background-color:#fafafa;color:#18181b;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table width="100%" border="0" cellspacing="0" cellpadding="0" style="background-color:#fafafa;margin:0;padding:0;">
  <tr>
    <td align="center" style="padding:40px 16px;">
      <table class="email-container" width="100%" border="0" cellspacing="0" cellpadding="0" style="max-width:540px;margin:0 auto;">
        <tr>
          <td align="left" style="padding-bottom:24px;">
            <table border="0" cellspacing="0" cellpadding="0">
              <tr>
                <td style="font-size:19px;font-weight:800;letter-spacing:2px;text-transform:uppercase;color:#09090b;" class="brand-mark">
                  KINJO
                </td>
              </tr>
            </table>
          </td>
        </tr>
        <tr>
          <td>
            <div class="email-card" style="background:#ffffff;border:1px solid #e4e4e7;border-radius:16px;padding:36px 32px;">
              <h1 style="margin:0 0 20px 0;font-size:22px;font-weight:700;letter-spacing:-0.02em;color:#09090b;">` + html.EscapeString(heading) + `</h1>
              ` + innerContent + `
            </div>
          </td>
        </tr>
        <tr>
          <td align="center" style="padding-top:28px;">
            <p class="footer-sub" style="margin:0;font-size:12px;color:#a1a1aa;line-height:1.5;">
              KINJO &bull; Find your people within 30&nbsp;m<br>
              <a href="https://kinjo.world" style="color:#71717a;text-decoration:none;">kinjo.world</a> &bull; <a href="mailto:hello@kinjo.world" style="color:#71717a;text-decoration:none;">hello@kinjo.world</a>
            </p>
          </td>
        </tr>
      </table>
    </td>
  </tr>
</table>
</body>
</html>`
}
