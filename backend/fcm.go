package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"golang.org/x/oauth2/google"
)

const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// FCM sends background push via the Firebase Cloud Messaging HTTP v1 API.
// It's the push transport only — no Firebase Auth/Firestore involved.
type FCM struct {
	projectID string
	creds     *google.Credentials
	http      *http.Client
}

// newFCM builds a sender from a service-account key, or returns (nil, nil) when
// FCM isn't configured so the rest of the app runs without background push.
func newFCM(ctx context.Context, cfg Config) (*FCM, error) {
	raw, err := cfg.fcmCredentials()
	if err != nil {
		return nil, fmt.Errorf("fcm credentials: %w", err)
	}
	if raw == nil {
		return nil, nil
	}
	creds, err := google.CredentialsFromJSON(ctx, raw, fcmScope)
	if err != nil {
		return nil, fmt.Errorf("fcm credentials: %w", err)
	}
	projectID := cfg.FCMProjectID
	if projectID == "" {
		projectID = creds.ProjectID
	}
	if projectID == "" {
		return nil, fmt.Errorf("fcm: project id not set and not in credentials")
	}
	return &FCM{projectID: projectID, creds: creds, http: &http.Client{Timeout: 10 * time.Second}}, nil
}

// send pushes the notification to each token and returns the tokens FCM reports
// as permanently invalid (unregistered), so the caller can prune them.
func (f *FCM) send(ctx context.Context, tokens []string, title, body string, data map[string]string) []string {
	if f == nil || len(tokens) == 0 {
		return nil
	}
	tok, err := f.creds.TokenSource.Token()
	if err != nil {
		log.Printf("fcm: oauth token: %v", err)
		return nil
	}
	endpoint := "https://fcm.googleapis.com/v1/projects/" + f.projectID + "/messages:send"

	var invalid []string
	for _, t := range tokens {
		msg := map[string]any{
			"token":        t,
			"notification": map[string]string{"title": title, "body": body},
			"android": map[string]any{
				"priority": "HIGH",
				"notification": map[string]any{
					"channel_id":              "fcm_default_channel",
					"default_sound":           true,
					"default_vibrate_timings": true,
				},
			},
		}
		if len(data) > 0 {
			msg["data"] = data
		}
		payload, _ := json.Marshal(map[string]any{"message": msg})

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := f.http.Do(req)
		if err != nil {
			log.Printf("fcm: send: %v", err)
			continue
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusOK:
			// delivered to FCM
		case resp.StatusCode == http.StatusNotFound,
			bytes.Contains(respBody, []byte("UNREGISTERED")),
			bytes.Contains(respBody, []byte("INVALID_ARGUMENT")):
			invalid = append(invalid, t) // dead token — prune it
		default:
			log.Printf("fcm: send %d: %s", resp.StatusCode, string(respBody))
		}
	}
	return invalid
}
