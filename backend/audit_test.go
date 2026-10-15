package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// K2: a database outage must answer 503 (retry), never 401 (app deletes its token).
func TestRequireAuthDBDown(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://x@127.0.0.1:1/x?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := &App{store: &Store{pool: pool}}
	h := a.requireAuth(func(http.ResponseWriter, *http.Request, string) { t.Fatal("handler ran") })

	for _, tc := range []struct {
		auth string
		want int
	}{{"", http.StatusUnauthorized}, {"Bearer some-token", http.StatusServiceUnavailable}} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		h(rec, req)
		if rec.Code != tc.want {
			t.Errorf("auth %q: got %d, want %d", tc.auth, rec.Code, tc.want)
		}
	}
}

// K3: expired cache entries (which hold photos) must be evicted.
func TestProfileCachePurge(t *testing.T) {
	pc := newProfileCache(nil)
	pc.items["old"] = profEntry{at: time.Now().Add(-profileTTL - time.Second)}
	pc.items["new"] = profEntry{at: time.Now()}
	pc.purge()
	if _, ok := pc.items["old"]; ok {
		t.Error("expired entry kept")
	}
	if _, ok := pc.items["new"]; !ok {
		t.Error("fresh entry evicted")
	}
}

// K5: only inline raster images (and legacy LinkedIn CDN links) are accepted.
func TestValidPhoto(t *testing.T) {
	for s, want := range map[string]bool{
		"data:image/jpeg;base64,AAAA":                true,
		"data:image/webp;base64,AAAA":                true,
		"https://media.licdn.com/dms/image/x":        false, // LinkedIn photos are no longer used
		"https://tracker.example/pixel.png":          false, // IP/location tracking
		"data:image/svg+xml;base64,PHN2Zz4=":         false, // can carry script
		"data:image/jpeg,raw-not-base64":             false,
		"":                                           false,
		"javascript:alert(1)":                        false,
		"https://media.licdn.com.evil.example/x.png": false,
	} {
		if got := validPhoto(s); got != want {
			t.Errorf("validPhoto(%q) = %v, want %v", s, got, want)
		}
	}
}

// A lookup that started before a card save must not cache the old card for profileTTL
// after invalidate. Needs a throwaway Postgres: KINJO_TEST_DSN=postgres://...
func TestProfileCacheIgnoresStaleFill(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid, _, err := s.emailUser(ctx, "cache-race-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer s.deleteAccount(ctx, uid)
	pc := newProfileCache(s)
	tx, err := s.pool.Begin(ctx) // hold the table so the lookup is still in flight when the save lands
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE profiles IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { pc.get(uid); close(done) }()
	time.Sleep(200 * time.Millisecond)
	pc.invalidate(uid) // what handleSaveProfile does after the save
	_ = tx.Rollback(ctx)
	<-done
	pc.mu.Lock()
	_, cached := pc.items[uid]
	pc.mu.Unlock()
	if cached {
		t.Fatal("a lookup overtaken by invalidate must not be cached")
	}
	if _, ok := pc.get(uid); ok {
		t.Fatal("no profile row: want ok=false")
	}
	if _, cached := pc.items[uid]; !cached {
		t.Fatal("an undisturbed lookup should be cached")
	}
}

// Bluetooth tokens survive a restart (a sleeping phone keeps advertising the one it
// has), a rotation keeps the previous one for its grace only, and hiding revokes them.
func TestBLETokensSurviveRestart(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid, _, err := s.emailUser(ctx, "ble-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer s.deleteAccount(ctx, uid)
	a := &App{store: s, hub: newHub(s, nil)}
	fetch := func() string {
		rec := httptest.NewRecorder()
		a.handleBLEToken(rec, httptest.NewRequest(http.MethodGet, "/api/ble-token", nil), uid)
		var out struct{ Token string }
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Token
	}
	first := fetch()
	restart := func() *Hub {
		h := newHub(s, nil)
		rows, err := s.loadBLE(ctx)
		if err != nil {
			t.Fatal(err)
		}
		h.loadBLE(rows, time.Now())
		return h
	}
	if h := restart(); h.bleOwner(first, time.Now()) != uid {
		t.Fatal("token forgotten across a restart")
	}
	a.hub.mu.Lock()
	a.hub.ble.issued[uid] = time.Now().Add(-bleTokenEvery - time.Second) // due for rotation
	a.hub.mu.Unlock()
	second := fetch()
	h := restart()
	if second == first || h.bleOwner(second, time.Now()) != uid || h.bleOwner(first, time.Now()) != uid {
		t.Fatal("after rotation both the new token and the previous one (grace) must resolve")
	}
	if h.bleOwner(first, time.Now().Add(bleTokenGrace+time.Second)) != "" {
		t.Fatal("the previous token must only last its grace after a restart")
	}
	a.hub.dropPresence(uid)
	if err := a.hub.forget(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if h := restart(); h.bleOwner(second, time.Now()) != "" {
		t.Fatal("hide must revoke saved tokens")
	}
}

// Logout stops pushes to that phone only; the account's other signed-in phone keeps them.
func TestLogoutStopsPushesToThatPhone(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid, _, err := s.emailUser(ctx, "push-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer s.deleteAccount(ctx, uid)
	a := &App{store: s, hub: newHub(s, nil)}
	call := func(h func(http.ResponseWriter, *http.Request, string), method, path, session, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+session)
		rec := httptest.NewRecorder()
		h(rec, req, uid)
		return rec.Code
	}
	for i, sess := range []string{"sess-phone1-" + uid, "sess-phone2-" + uid} {
		if err := s.createSession(ctx, sess, uid); err != nil {
			t.Fatal(err)
		}
		if c := call(a.handleRegisterDevice, "POST", "/api/device", sess, `{"pushToken":"fcm-`+strconv.Itoa(i+1)+`-`+uid+`"}`); c != http.StatusOK {
			t.Fatalf("register: %d", c)
		}
	}
	call(a.handleLogout, "POST", "/api/logout", "sess-phone1-"+uid, "")
	toks, err := s.pushTokensForUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != 1 || toks[0] != "fcm-2-"+uid {
		t.Fatalf("after phone 1 logs out only phone 2 may get pushes, got %v", toks)
	}
}

// App opens are counted per day (repeats within 30 s merge), the admin usage view
// lists them, and an admin broadcast reaches a connected user and is stored for everyone.
func TestUsageAndBroadcast(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid, _, err := s.emailUser(ctx, "usage-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer s.deleteAccount(ctx, uid)
	a := &App{store: s, hub: newHub(s, nil)}
	open := func() {
		rec := httptest.NewRecorder()
		a.handleOpen(rec, httptest.NewRequest("POST", "/api/open", strings.NewReader(`{"platform":"ios"}`)), uid)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("open: %d", rec.Code)
		}
	}
	open()
	open() // within 30 s: same open
	if _, err := s.pool.Exec(ctx, `UPDATE app_opens SET last_at = now() - interval '1 minute' WHERE uid = $1`, uid); err != nil {
		t.Fatal(err)
	}
	open() // a minute later: a second open
	rec := httptest.NewRecorder()
	a.handleAdminUsage(rec, httptest.NewRequest("GET", "/admin/usage", nil))
	var out struct {
		Users []UsageRow
		Days  []UsageDay
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	var mine *UsageRow
	for i := range out.Users {
		if out.Users[i].UID == uid {
			mine = &out.Users[i]
		}
	}
	if mine == nil || mine.Opens != 2 || mine.Platform != "ios" || len(out.Days) == 0 {
		t.Fatalf("want 2 opens on ios today, got %+v (days %v)", mine, out.Days)
	}

	c := newClient(uid)
	a.hub.attach(c)
	rec = httptest.NewRecorder()
	a.handleAdminNotify(rec, httptest.NewRequest("POST", "/admin/notify", strings.NewReader(`{"broadcast":true,"title":"Hello all","body":"test"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("broadcast: %d %s", rec.Code, rec.Body.String())
	}
	got := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline) && !got; time.Sleep(20 * time.Millisecond) {
		for len(c.send) > 0 {
			got = got || strings.Contains(string(<-c.send), "Hello all")
		}
	}
	if !got {
		t.Fatal("a connected user must get the broadcast live")
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE uid = $1 AND title = 'Hello all'`, uid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("broadcast must be stored for the user, got %d (%v)", n, err)
	}

	sess := "sess-dev-" + uid
	if err := s.createSession(ctx, sess, uid); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/device", strings.NewReader(`{"pushToken":"fcm-dev-`+uid+`","platform":"ios","deviceId":"install-123"}`))
	req.Header.Set("Authorization", "Bearer "+sess)
	rec = httptest.NewRecorder()
	a.handleRegisterDevice(rec, req, uid)
	var platform, devID string
	if err := s.pool.QueryRow(ctx, `SELECT platform, device_id FROM devices WHERE uid = $1`, uid).Scan(&platform, &devID); err != nil || platform != "ios" || devID != "install-123" {
		t.Fatalf("device row: platform %q id %q (%v)", platform, devID, err)
	}
}
