package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"time"
)

// sendMail sends a plain-text email, or logs it when SMTP isn't configured
// (so flows are testable without a mail server).
func (a *App) sendMail(toEmail, subject, text string) error {
	if !a.cfg.smtpConfigured() {
		log.Printf("SMTP not configured; would email %s — %q:\n%s", toEmail, subject, text)
		return nil
	}
	msg := "From: " + a.cfg.SMTPFrom + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		text + "\r\n"

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
	// One deadline for the whole conversation: a stuck mail server must not hang
	// this goroutine (or the HTTP request waiting on it) forever.
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
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return client.Quit()
}

func (a *App) sendVerificationLink(toEmail, link string) error {
	return a.sendMail(toEmail, "Verify your email for Kinjo",
		"Confirm your email for Kinjo by opening this link:\r\n\r\n"+link+
			"\r\n\r\nIf you didn't request this, you can ignore this email.")
}

// sendWelcomeEmail greets a new member and, in the same email, asks them to
// confirm their address (one email, not two).
func (a *App) sendWelcomeEmail(toEmail, firstName, verifyLink string) error {
	greeting := "Hi"
	if firstName != "" {
		greeting = "Hi " + firstName
	}
	subject, confirm, next := "Welcome to Kinjo", "", "Finish your card"
	if verifyLink != "" {
		subject = "Welcome to Kinjo — confirm your email"
		confirm = "Please confirm your email address (the link works for 24 hours):\r\n\r\n" + verifyLink + "\r\n\r\n"
		next = "Then finish your card"
	}
	return a.sendMail(toEmail, subject,
		greeting+",\r\n\r\nWelcome to Kinjo! "+confirm+next+" so the people "+
			"around you can see what you do and what you're looking for.\r\n\r\n— The Kinjo team")
}
