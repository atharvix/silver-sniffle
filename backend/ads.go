package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Feed ad pacing: one sponsored card every [min,max] organic cards. The client
// jitters within this range and never places two ads back-to-back.
const (
	adFreqMin = 10
	adFreqMax = 15
)

// Ad is a sponsored brand card. JSON field names match what the feed expects.
type Ad struct {
	ID              int64     `json:"id"`
	BrandName       string    `json:"brandName"`
	Logo            string    `json:"logo"`
	Description     string    `json:"description"`
	CampaignMessage string    `json:"campaignMessage"`
	Category        string    `json:"category"`
	Location        string    `json:"location"`
	Image           string    `json:"image"`
	CTALabel        string    `json:"ctaLabel"`
	DestinationURL  string    `json:"destinationUrl"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"createdAt"`
}

// AdWithStats adds aggregate event counts for the admin list.
type AdWithStats struct {
	Ad
	Impressions int `json:"impressions"`
	Clicks      int `json:"clicks"`
	CTAClicks   int `json:"ctaClicks"`
}

var adEventTypes = map[string]bool{"impression": true, "click": true, "cta_click": true}

/* ---------------- store ---------------- */

func (s *Store) adCreate(ctx context.Context, a Ad) (int64, error) {
	if a.CTALabel == "" {
		a.CTALabel = "Learn More"
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ads (brand_name, logo, description, campaign_message, category,
		                 location, image, cta_label, destination_url, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		a.BrandName, a.Logo, a.Description, a.CampaignMessage, a.Category,
		a.Location, a.Image, a.CTALabel, a.DestinationURL, a.Active).Scan(&id)
	return id, err
}

func (s *Store) adsActive(ctx context.Context) ([]Ad, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, brand_name, logo, description, campaign_message, category,
		       location, image, cta_label, destination_url, active, created_at
		FROM ads WHERE active ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAds(rows)
}

func (s *Store) adsWithStats(ctx context.Context) ([]AdWithStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.brand_name, a.logo, a.description, a.campaign_message, a.category,
		       a.location, a.image, a.cta_label, a.destination_url, a.active, a.created_at,
		       COALESCE(SUM((e.event='impression')::int),0),
		       COALESCE(SUM((e.event='click')::int),0),
		       COALESCE(SUM((e.event='cta_click')::int),0)
		FROM ads a LEFT JOIN ad_events e ON e.ad_id = a.id
		GROUP BY a.id ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdWithStats{}
	for rows.Next() {
		var a AdWithStats
		if err := rows.Scan(&a.ID, &a.BrandName, &a.Logo, &a.Description, &a.CampaignMessage,
			&a.Category, &a.Location, &a.Image, &a.CTALabel, &a.DestinationURL, &a.Active, &a.CreatedAt,
			&a.Impressions, &a.Clicks, &a.CTAClicks); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanAds(rows pgx.Rows) ([]Ad, error) {
	out := []Ad{}
	for rows.Next() {
		var a Ad
		if err := rows.Scan(&a.ID, &a.BrandName, &a.Logo, &a.Description, &a.CampaignMessage,
			&a.Category, &a.Location, &a.Image, &a.CTALabel, &a.DestinationURL, &a.Active, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) adSetActive(ctx context.Context, id int64, active bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE ads SET active = $2 WHERE id = $1`, id, active)
	return err
}

func (s *Store) adDelete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ads WHERE id = $1`, id)
	return err
}

func (s *Store) adEvent(ctx context.Context, adID int64, uid, event string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO ad_events (ad_id, uid, event) VALUES ($1,$2,$3)`, adID, uid, event)
	return err
}

/* ---------------- app handlers ---------------- */

// handleGetAds returns active sponsored cards plus the pacing config the feed
// uses to interleave them.
func (a *App) handleGetAds(w http.ResponseWriter, r *http.Request, uid string) {
	ads, err := a.store.adsActive(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("lookup failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ads":          ads,
		"frequencyMin": adFreqMin,
		"frequencyMax": adFreqMax,
	})
}

// handleAdEvent records an ad impression/click/cta_click, kept separate from
// profile analytics. Best-effort: never fails the client.
func (a *App) handleAdEvent(w http.ResponseWriter, r *http.Request, uid string) {
	var in struct {
		AdID  int64  `json:"adId"`
		Event string `json:"event"`
	}
	if err := decodeJSON(r, &in); err != nil || in.AdID == 0 || !adEventTypes[in.Event] {
		writeJSON(w, http.StatusBadRequest, errBody("adId and valid event required"))
		return
	}
	_ = a.store.adEvent(r.Context(), in.AdID, uid, in.Event)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

/* ---------------- admin handlers ---------------- */

func (a *App) handleAdminAds(w http.ResponseWriter, r *http.Request) {
	ads, err := a.store.adsWithStats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("lookup failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ads": ads})
}

func (a *App) handleAdminCreateAd(w http.ResponseWriter, r *http.Request) {
	var in Ad
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.BrandName) == "" {
		writeJSON(w, http.StatusBadRequest, errBody("brandName required"))
		return
	}
	in.Active = true
	id, err := a.store.adCreate(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("create failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (a *App) handleAdminToggleAd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     int64 `json:"id"`
		Active bool  `json:"active"`
	}
	if err := decodeJSON(r, &in); err != nil || in.ID == 0 {
		writeJSON(w, http.StatusBadRequest, errBody("id required"))
		return
	}
	if err := a.store.adSetActive(r.Context(), in.ID, in.Active); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("update failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminDeleteAd(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id == 0 {
		writeJSON(w, http.StatusBadRequest, errBody("id required"))
		return
	}
	if err := a.store.adDelete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("delete failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
