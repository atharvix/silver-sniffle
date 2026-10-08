package main

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"
)

// notifyUser stores a notification in the user's feed and delivers it. This is
// the single entry point for "send a custom notification to a user".
func (a *App) notifyUser(uid, title, body string, data any) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var raw json.RawMessage
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			raw = b
		}
	}
	id, err := a.store.insertNotification(ctx, uid, title, body, raw)
	if err != nil {
		log.Printf("notify %s: %v", uid, err)
		return
	}
	a.deliver(notifTarget{id: id, uid: uid}, title, body, raw)
}

// deliver pushes a stored notification live over the WebSocket (if the user is
// connected) and via FCM to their devices, marking it sent only once FCM accepts it.
func (a *App) deliver(t notifTarget, title, body string, raw json.RawMessage) {
	a.hub.pushNotification(t.uid, map[string]any{
		"type":         "notif",
		"notification": Notification{ID: t.id, Title: title, Body: body, Data: raw, CreatedAt: time.Now()},
	})
	if a.fcm == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tokens, err := a.store.pushTokensForUser(ctx, t.uid)
	if err != nil || len(tokens) == 0 {
		return
	}
	sent, invalid := a.fcm.send(ctx, tokens, title, body,
		map[string]string{"notifId": strconv.FormatInt(t.id, 10)})
	for _, tok := range invalid {
		_ = a.store.deleteDevice(ctx, t.uid, tok)
	}
	// Only once FCM accepted it; otherwise the next device registration retries.
	if sent > 0 {
		_ = a.store.markNotificationSent(ctx, t.id)
	}
}

// welcomeNewUser greets a brand-new account: an in-app notification plus a
// welcome email (if we have an address and SMTP is configured).
func (a *App) welcomeNewUser(uid, email, name string, verified bool) {
	first := name
	if i := strings.IndexByte(name, ' '); i > 0 {
		first = name[:i]
	}
	a.notifyUser(uid, "Welcome to Kinjo 👋",
		"You're all set. Finish your card so people within 30 m can find you.", nil)
	if email == "" {
		return
	}
	var link string
	if !verified {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		link, err = a.verificationLink(ctx, uid, email)
		cancel()
		if err != nil {
			log.Printf("verification link for %s: %v", uid, err) // still send the welcome
		}
	}
	if err := a.sendWelcomeEmail(email, first, link); err != nil {
		log.Printf("welcome email to %s: %v", email, err)
	}
}

// wakeUsers sends each user's phones a silent "wake" push, which restarts the
// location service if the phone's battery manager stopped it.
func (a *App) wakeUsers(uids []string) {
	if a.fcm == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, uid := range uids {
		tokens, err := a.store.pushTokensForUser(ctx, uid)
		if err != nil || len(tokens) == 0 {
			continue
		}
		for _, t := range a.fcm.sendData(ctx, tokens, map[string]string{"kind": "wake"}) {
			_ = a.store.deleteDevice(ctx, uid, t)
		}
	}
}
