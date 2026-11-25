package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// postHealth stores the phone's background-health snapshot (sent on app open and service restarts, at most hourly).
func (a *App) postHealth(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	var h Health
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&h); err != nil || !validHealth(h) {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := a.store.saveHealth(r.Context(), uid, h); err != nil {
		unavailable(w, "save health", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validHealth(h Health) bool {
	short := func(s string, max int) bool { return utf8.RuneCountInString(s) <= max }
	return h.InstallID != "" && short(h.InstallID, 64) && (h.Platform == "android" || h.Platform == "ios") &&
		short(h.Maker, 40) && short(h.Model, 60) && short(h.OS, 30) && short(h.App, 20) &&
		short(h.LastExit, 40) && short(h.KillerApps, 300) && h.StandbyBucket >= 0 && h.StandbyBucket <= 100
}

// isAdmin checks X-Admin-Token against ADMIN_TOKEN. With no token configured the admin API doesn't exist.
func (a *App) isAdmin(r *http.Request) bool {
	given := r.Header.Get("X-Admin-Token")
	return len(a.adminToken) >= 24 && subtle.ConstantTimeCompare([]byte(given), []byte(a.adminToken)) == 1
}

type row = map[string]any

// adminStats is everything the admin dashboard shows: daily totals, active people, live counts and phone health.
// Totals and percentages only: no person, position or place.
func (a *App) adminStats(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		http.NotFound(w, r)
		return
	}
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || days < 1 || days > 90 {
		days = 30
	}
	ctx := r.Context()
	out := row{"days": days}

	// Daily totals: counters plus active people per day.
	daily := map[string]row{}
	dayRow := func(d time.Time) row {
		key := d.Format("2006-01-02")
		if daily[key] == nil {
			daily[key] = row{"day": key}
		}
		return daily[key]
	}
	err = a.each(ctx, `select day, metric, count from daily_counts where day > `+today+` - $1::int`, []any{days}, func(rows pgx.Rows) error {
		var d time.Time
		var metric string
		var n int64
		if err := rows.Scan(&d, &metric, &n); err != nil {
			return err
		}
		dayRow(d)[metric] = n
		return nil
	})
	if err == nil {
		err = a.each(ctx, `select day, count(*) from app_days where day > `+today+` - $1::int group by day`, []any{days}, func(rows pgx.Rows) error {
			var d time.Time
			var n int64
			if err := rows.Scan(&d, &n); err != nil {
				return err
			}
			dayRow(d)["active"] = n
			return nil
		})
	}
	if err != nil {
		unavailable(w, "admin daily", err)
		return
	}
	dailyList := make([]row, 0, len(daily))
	for _, v := range daily {
		dailyList = append(dailyList, v)
	}
	out["daily"] = dailyList

	totals := row{}
	var accounts, cards, hidden, active7, active30, reports30 int64
	err = a.store.db.QueryRow(ctx, `select
		(select count(*) from users), (select count(*) from profiles), (select count(*) from users where hidden),
		(select count(distinct user_id) from app_days where day > `+today+` - 7),
		(select count(distinct user_id) from app_days where day > `+today+` - 30),
		(select count(*) from reports where created_at > now() - interval '30 days')`).
		Scan(&accounts, &cards, &hidden, &active7, &active30, &reports30)
	if err != nil {
		unavailable(w, "admin totals", err)
		return
	}
	totals["accounts"], totals["cards"], totals["hidden"] = accounts, cards, hidden
	totals["active7"], totals["active30"], totals["reports30"] = active7, active30, reports30
	out["totals"] = totals

	online, located := a.hub.liveCounts(ctx)
	out["live"] = row{"connected": online, "findable": located}

	health := row{}
	queries := map[string]string{
		// Per maker: how many phones, and the share of each setting that keeps Kinjo alive.
		"makers": `select maker, count(*),
			round(100.0 * avg(battery_free::int)), round(100.0 * avg(bg_restricted::int)),
			round(100.0 * avg(location_always::int)), round(100.0 * avg(bluetooth_on::int)),
			round(100.0 * avg(notifications_on::int)), round(100.0 * avg(power_save::int))
			from device_health where updated_at > now() - interval '30 days' group by maker order by count(*) desc limit 30`,
		"exits":        `select last_exit, count(*) from device_health where updated_at > now() - interval '30 days' group by 1 order by 2 desc`,
		"killer_apps":  `select cleaner, count(*) from device_health, unnest(string_to_array(nullif(killer_apps, ''), ',')) as cleaner where updated_at > now() - interval '30 days' group by 1 order by 2 desc`,
		"app_versions": `select app, count(*) from device_health where updated_at > now() - interval '30 days' group by 1 order by 2 desc`,
		"os_versions":  `select os, count(*) from device_health where updated_at > now() - interval '30 days' group by 1 order by 2 desc`,
		"standby":      `select standby_bucket::text, count(*) from device_health where updated_at > now() - interval '30 days' group by 1 order by 1`,
	}
	for name, q := range queries {
		var list []row
		err := a.each(ctx, q, nil, func(rows pgx.Rows) error {
			vals, err := rows.Values()
			if err != nil {
				return err
			}
			if name == "makers" {
				list = append(list, row{"maker": vals[0], "phones": vals[1], "battery_free_pct": vals[2], "bg_restricted_pct": vals[3],
					"location_always_pct": vals[4], "bluetooth_on_pct": vals[5], "notifications_on_pct": vals[6], "power_save_pct": vals[7]})
			} else {
				list = append(list, row{"name": vals[0], "phones": vals[1]})
			}
			return nil
		})
		if err != nil {
			unavailable(w, "admin health", err)
			return
		}
		health[name] = list
	}
	out["health"] = health
	writeJSON(w, http.StatusOK, out)
}

func (a *App) each(ctx context.Context, q string, args []any, fn func(pgx.Rows) error) error {
	rows, err := a.store.db.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
