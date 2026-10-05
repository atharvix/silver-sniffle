package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
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

	if a.cfg.SMTPPort == "465" {
		tlsConfig := &tls.Config{
			ServerName: a.cfg.SMTPHost,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, a.cfg.SMTPHost)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()

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

	if err := smtp.SendMail(addr, auth, a.cfg.SMTPFrom, []string{toEmail}, []byte(msg)); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}

func (a *App) sendVerificationLink(toEmail, link string) error {
	return a.sendMail(toEmail, "Verify your email for Kinjo",
		"Confirm your email for Kinjo by opening this link:\r\n\r\n"+link+
			"\r\n\r\nIf you didn't request this, you can ignore this email.")
}

func (a *App) sendWelcomeEmail(toEmail, firstName string) error {
	greeting := "Hi"
	if firstName != "" {
		greeting = "Hi " + firstName
	}
	return a.sendMail(toEmail, "Welcome to Kinjo",
		greeting+",\r\n\r\nWelcome to Kinjo! Finish setting up your card so the people "+
			"around you can see what you do and what you're looking for.\r\n\r\n— The Kinjo team")
}
