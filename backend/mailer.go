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
	"strconv"
	"strings"
	"time"
)

// sendMailMIME sends a multipart/alternative (HTML + plain text) email, or logs it when
// SMTP isn't configured (so flows are testable without a mail server).
func (a *App) sendMailMIME(toEmail, subject, textBody, htmlBody string) (err error) {
	if !a.cfg.smtpConfigured() {
		log.Printf("SMTP not configured; would email %s — %q:\n[TEXT]\n%s\n[HTML len %d]", toEmail, subject, textBody, len(htmlBody))
		return nil
	}
	defer func() { tel.mailed(err) }()

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

	htmlBody := emailShell("Confirm your email.", "One tap and your Kinjo account is confirmed.",
		mailP(`One tap and your account is confirmed. The link works for 24 hours.`)+
			mailButton(link, "Confirm email")+
			mailLink(link)+
			mailFine(`Didn’t ask for this? You can ignore this email.`))

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

	var confirm string
	open := mailButton("https://kinjo.world", "Open Kinjo")
	if verifyLink != "" {
		open = mailButtonLine("https://kinjo.world", "Open Kinjo") // one solid button per email
		confirm = mailButton(verifyLink, "Confirm email") +
			mailFine(`Confirming keeps your account yours. The link works for 24 hours.`)
	}

	htmlBody := emailShell("Welcome to Kinjo.", "Find your people within 30 m, in the real world.",
		mailP(html.EscapeString(greeting)+`,`)+
			mailP(`Kinjo shows you the founders, creators and professionals within <b style="color:#ffffff;font-weight:500;">30&nbsp;meters</b> of you. No cold DMs, no connection requests, no online small talk. Just the people in the room.`)+
			confirm+
			mailSteps([][2]string{
				{"Finish your card", "Add a photo and say what you do and what you’re looking for."},
				{"Stay discoverable", "Set Location to <i>Allow all the time</i> and Battery to <i>Unrestricted</i>, so Kinjo works from your pocket."},
				{"Walk up and say hi", "At a café, a conference or your coworking space, open Kinjo and see who’s around."},
			})+
			open+
			mailFine(`— The Kinjo team`))

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

	htmlBody := emailShell("Welcome to Kinjo.", "Your sign-in code is "+code+".",
		mailP(`You’re one step from the people within 30&nbsp;meters of you. Enter this code in the app:`)+
			mailCode(code)+
			mailP(`Once you’re in, finish your card so the people around you can see what you do.`)+
			mailFine(`Never share this code. Didn’t ask for it? Ignore this email: nobody can sign in without it.`))

	return a.sendMailMIME(toEmail, subject, text, htmlBody)
}

// sendOTPEmail sends a clean, direct security OTP code email (for resends and returning users) with NO welcome copy.
func (a *App) sendOTPEmail(toEmail, code string) error {
	subject := "Your Kinjo sign-in code: " + code

	text := "Your Kinjo sign-in code is: " + code + "\r\n\r\n" +
		"This code expires in 10 minutes.\r\n\r\n" +
		"If you didn't request this code, you can safely ignore this email: nobody can sign in without it.\r\n\r\n" +
		"— The Kinjo Team"

	htmlBody := emailShell("Your sign-in code.", "Your sign-in code is "+code+".",
		mailP(`Enter this code in Kinjo to sign in:`)+
			mailCode(code)+
			mailFine(`Never share this code. Didn’t ask for it? Ignore this email: nobody can sign in without it.`))

	return a.sendMailMIME(toEmail, subject, text, htmlBody)
}

/*
The emails wear the app's own look: near-black, DM Sans, light large headings, hairlines,

	square white buttons. Dark by design (color-scheme: dark), so clients that darken mail
	leave it alone. Tables and inline styles only: Gmail drops most <style>. Arguments are
	trusted HTML, except where escaped here.
*/
const (
	mailFont  = `'DM Sans','Helvetica Neue',Helvetica,Arial,sans-serif`
	mailText  = `#a3a3a3` // 7.6:1 on the background
	mailQuiet = `#8a8a8a` // 5.4:1
	mailRule  = `#262626`
)

func mailP(inner string) string {
	return `<p style="margin:0 0 18px;font-family:` + mailFont + `;font-size:16px;line-height:1.6;color:` + mailText + `;">` + inner + `</p>`
}

func mailFine(inner string) string {
	return `<p style="margin:28px 0 0;font-family:` + mailFont + `;font-size:13px;line-height:1.55;color:` + mailQuiet + `;">` + inner + `</p>`
}

func mailButton(href, label string) string {
	h := html.EscapeString(href)
	return `<table role="presentation" border="0" cellspacing="0" cellpadding="0" style="margin:30px 0 4px;"><tr>
<td bgcolor="#ffffff" style="background:#ffffff;">
<a href="` + h + `" style="display:inline-block;padding:17px 28px;font-family:` + mailFont + `;font-size:16px;line-height:1;color:#0a0a0a;text-decoration:none;">` + html.EscapeString(label) + `&nbsp;&nbsp;&rarr;</a>
</td></tr></table>`
}

func mailButtonLine(href, label string) string {
	return `<table role="presentation" border="0" cellspacing="0" cellpadding="0" style="margin:30px 0 4px;"><tr>
<td style="border:1px solid #3d3d3d;">
<a href="` + html.EscapeString(href) + `" style="display:inline-block;padding:16px 27px;font-family:` + mailFont + `;font-size:16px;line-height:1;color:#ffffff;text-decoration:none;">` + html.EscapeString(label) + `&nbsp;&nbsp;&rarr;</a>
</td></tr></table>`
}

func mailLink(href string) string {
	h := html.EscapeString(href)
	return `<p style="margin:22px 0 0;font-family:` + mailFont + `;font-size:13px;line-height:1.55;color:` + mailQuiet + `;">Or open this link:<br><a href="` + h + `" style="color:#ffffff;text-decoration:underline;word-break:break-all;">` + h + `</a></p>`
}

func mailCode(code string) string {
	return `<table role="presentation" width="100%" border="0" cellspacing="0" cellpadding="0" style="margin:8px 0 26px;"><tr>
<td style="padding:26px 24px;border:1px solid ` + mailRule + `;">
<div style="font-family:` + mailFont + `;font-size:44px;line-height:1;font-weight:400;letter-spacing:12px;color:#ffffff;font-variant-numeric:tabular-nums;">` + html.EscapeString(code) + `</div>
<div style="margin-top:14px;font-family:` + mailFont + `;font-size:13px;line-height:1.4;color:` + mailQuiet + `;">Expires in 10 minutes</div>
</td></tr></table>`
}

// mailSteps: title, text (trusted HTML), one row each between hairlines.
func mailSteps(steps [][2]string) string {
	rows := ""
	for i, s := range steps {
		rows += `<tr>
<td valign="top" width="36" style="padding:18px 0;border-top:1px solid ` + mailRule + `;font-family:` + mailFont + `;font-size:13px;line-height:1.5;color:` + mailQuiet + `;">` + strconv.Itoa(i+1) + `</td>
<td valign="top" style="padding:18px 0;border-top:1px solid ` + mailRule + `;font-family:` + mailFont + `;font-size:15px;line-height:1.55;color:` + mailText + `;"><span style="color:#ffffff;font-weight:500;">` + s[0] + `</span><br>` + s[1] + `</td>
</tr>`
	}
	return `<table role="presentation" width="100%" border="0" cellspacing="0" cellpadding="0" style="margin:22px 0 0;border-bottom:1px solid ` + mailRule + `;">` + rows + `</table>`
}

// emailShell wraps the content: wordmark, heading, body, footer. preheader is the
// inbox preview line.
func emailShell(heading, preheader, inner string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="dark">
<meta name="supported-color-schemes" content="dark">
<title>` + html.EscapeString(heading) + `</title>
<link href="https://fonts.googleapis.com/css2?family=DM+Sans:wght@400;500;700&display=swap" rel="stylesheet">
<style>
  :root { color-scheme: dark; supported-color-schemes: dark; }
  body { margin:0; padding:0; -webkit-text-size-adjust:100%; }
  a { color:#ffffff; }
  @media (max-width:520px) { .k-pad { padding:36px 22px 40px !important; } .k-h1 { font-size:32px !important; } }
</style>
</head>
<body bgcolor="#0a0a0a" style="margin:0;padding:0;background:#0a0a0a;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:#0a0a0a;">` + html.EscapeString(preheader) + `&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;&#8199;&#847;</div>
<table role="presentation" width="100%" border="0" cellspacing="0" cellpadding="0" bgcolor="#0a0a0a" style="background:#0a0a0a;">
<tr><td align="center" class="k-pad" style="padding:48px 28px 56px;">
<table role="presentation" width="100%" border="0" cellspacing="0" cellpadding="0" style="max-width:500px;">
<tr><td style="padding-bottom:22px;border-bottom:1px solid ` + mailRule + `;font-family:` + mailFont + `;font-size:13px;font-weight:700;letter-spacing:3px;color:#ffffff;">KINJO</td></tr>
<tr><td style="padding-top:44px;">
<h1 class="k-h1" style="margin:0 0 22px;font-family:` + mailFont + `;font-size:38px;line-height:1.06;font-weight:400;letter-spacing:-1.5px;color:#ffffff;">` + html.EscapeString(heading) + `</h1>
` + inner + `
</td></tr>
<tr><td style="padding-top:48px;">
<table role="presentation" width="100%" border="0" cellspacing="0" cellpadding="0"><tr>
<td style="padding-top:22px;border-top:1px solid ` + mailRule + `;font-family:` + mailFont + `;font-size:12.5px;line-height:1.6;color:` + mailQuiet + `;">Find your people within 30&nbsp;m.<br>
<a href="https://kinjo.world" style="color:` + mailQuiet + `;text-decoration:none;">kinjo.world</a> &nbsp;·&nbsp; <a href="mailto:hello@kinjo.world" style="color:` + mailQuiet + `;text-decoration:none;">hello@kinjo.world</a></td>
</tr></table>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`
}
