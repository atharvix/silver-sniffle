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
)

// Admin sign-in: only listed addresses, an app code never opens the panel, sessions end
// on logout or when the address leaves ADMIN_EMAILS.
func TestAdminAuth(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	email := "boss-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com"
	a := &App{store: s, hub: newHub(s), cfg: Config{AdminEmails: []string{email}}}
	ok := a.requireAdmin(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	call := func(h http.HandlerFunc, method, body, tok string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/admin/x", strings.NewReader(body))
		if tok != "" {
			req.Header.Set("X-Admin-Token", tok)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	if rec := call(ok, "GET", "", "anything"); rec.Code != 401 {
		t.Fatalf("no session: %d", rec.Code)
	}
	if rec := call(a.handleAdminAuthStart, "POST", `{"email":"nobody@example.com"}`, ""); rec.Code != 200 {
		t.Fatalf("unlisted start: %d", rec.Code) // same answer, no code
	}
	appCode, _, _ := reserveCode(email, time.Now()) // an app sign-in code for the same address
	if rec := call(a.handleAdminAuthVerify, "POST", `{"email":"`+email+`","code":"`+appCode+`"}`, ""); rec.Code != 401 {
		t.Fatalf("app code opened the panel: %d", rec.Code)
	}
	code, _, _ := reserveCode("admin:"+email, time.Now())
	rec := call(a.handleAdminAuthVerify, "POST", `{"email":"`+strings.ToUpper(email)+`","code":"`+code+`"}`, "")
	var out struct{ Token string }
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.Token == "" {
		t.Fatalf("verify: %d %s", rec.Code, rec.Body)
	}
	if rec := call(a.handleAdminAuthVerify, "POST", `{"email":"`+email+`","code":"`+code+`"}`, ""); rec.Code != 401 {
		t.Fatalf("code reused: %d", rec.Code)
	}
	var stored int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM admin_sessions WHERE token_hash = $1`, out.Token).Scan(&stored)
	if stored != 0 {
		t.Fatal("token stored in plaintext")
	}
	if rec := call(ok, "GET", "", out.Token); rec.Code != 204 {
		t.Fatalf("session: %d", rec.Code)
	}
	a.cfg.AdminEmails = nil
	a.cfg.AdminToken = "x"
	if rec := call(ok, "GET", "", out.Token); rec.Code != 401 {
		t.Fatalf("removed admin still in: %d", rec.Code)
	}
	a.cfg.AdminEmails = []string{email}
	call(a.handleAdminAuthLogout, "POST", "", out.Token)
	if rec := call(ok, "GET", "", out.Token); rec.Code != 401 {
		t.Fatalf("after logout: %d", rec.Code)
	}
}
