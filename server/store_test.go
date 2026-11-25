package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Runs against a real Postgres only when KINJO_TEST_DSN points at a throwaway database.
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.db.Close)
	for _, table := range []string{"daily_counts", "app_days", "device_health", "reports", "blocks", "devices", "profiles", "sessions", "users"} {
		if _, err := s.db.Exec(ctx, "delete from "+table); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestAdminStatsAgainstPostgres(t *testing.T) {
	s := testStore(t)
	uid, _, err := s.newAccount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = s.saveHealth(ctx, uid, Health{InstallID: "i1", Platform: "android", Maker: "Xiaomi", Model: "M", OS: "Android 14", App: "0.1",
		BgRestricted: true, LocationAlways: true, LastExit: "low memory", KillerApps: "Clean Master,Greenify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.addCounts(ctx, map[string]int64{countMatches: 3, countSignups: 1}); err != nil {
		t.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	app := &App{store: s, hub: newHub(rdb), adminToken: "0123456789abcdefghijklmnop"}
	req := httptest.NewRequest(http.MethodGet, "/admin/stats?days=7", nil)
	res := httptest.NewRecorder()
	app.adminStats(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("no password must look like a missing page, got %d", res.Code)
	}
	req.Header.Set("X-Admin-Token", app.adminToken)
	res = httptest.NewRecorder()
	app.adminStats(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("got %d: %s", res.Code, res.Body)
	}
	var out struct {
		Totals struct{ Accounts, Active7 int }
		Daily  []map[string]any
		Health struct {
			Makers     []map[string]any `json:"makers"`
			KillerApps []map[string]any `json:"killer_apps"`
		}
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Totals.Accounts != 1 || out.Totals.Active7 != 1 {
		t.Fatalf("totals: %+v", out.Totals)
	}
	if len(out.Daily) != 1 || out.Daily[0]["matches"] != float64(3) {
		t.Fatalf("daily: %v", out.Daily)
	}
	if len(out.Health.Makers) != 1 || out.Health.Makers[0]["bg_restricted_pct"] != float64(100) {
		t.Fatalf("makers: %v", out.Health.Makers)
	}
	if len(out.Health.KillerApps) != 2 {
		t.Fatalf("cleaner apps: %v", out.Health.KillerApps)
	}
}
