package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"=HYPERLINK(1)": "'=HYPERLINK(1)", "+1": "'+1", "@x": "'@x", "-2": "'-2", "Riya": "Riya", "": ""} {
		if got := csvSafe(in); got != want {
			t.Fatalf("csvSafe(%q) = %q", in, got)
		}
	}
}

// The workbook is a valid zip of well-formed XML, and typed text stays text (no formulas).
func TestXLSX(t *testing.T) {
	b, err := xlsx("people", []string{"Name", "Events"}, [][]any{{"=cmd|' /C calc'!A0 <b>&\x01", 3}, {"Riya", 0}})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var sheet string
	for _, f := range zr.File {
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		rc.Close()
		d := xml.NewDecoder(bytes.NewReader(body))
		for {
			if _, err := d.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
		}
		if f.Name == "xl/worksheets/sheet1.xml" {
			sheet = string(body)
		}
	}
	if strings.Contains(sheet, "<f>") || !strings.Contains(sheet, `<c r="B2"><v>3</v></c>`) || !strings.Contains(sheet, "&lt;b&gt;&amp;") {
		t.Fatalf("sheet: %s", sheet)
	}
}

func TestAdminExport(t *testing.T) {
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
	a := &App{store: s, hub: newHub(s), publicURL: "https://kinjo.test"}
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	uid, _, err := s.emailUser(ctx, "exp-"+stamp+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.deleteAccount(context.Background(), uid) })
	if err := s.saveProfile(ctx, uid, Profile{Name: "=Evil " + stamp, Role: "Builds", Look: "Friends", Photo: "data:image/jpeg;base64,/9j/"}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"csv", "xlsx"} {
		for _, what := range []string{"people", "downloads", "cities"} {
			rec := httptest.NewRecorder()
			a.handleAdminExport(rec, httptest.NewRequest("GET", "/admin/export?what="+what+"&format="+f+"&q="+stamp, nil))
			if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Disposition"), "kinjo-"+what) {
				t.Fatalf("%s %s: %d", what, f, rec.Code)
			}
			body := rec.Body.String()
			if what == "people" && f == "csv" && (!strings.Contains(body, "'=Evil "+stamp) || strings.Contains(body, "data:image")) {
				t.Fatalf("people csv: %s", body)
			}
		}
	}
	rec := httptest.NewRecorder()
	a.handleAdminExport(rec, httptest.NewRequest("GET", "/admin/export?what=sessions", nil))
	if rec.Code != 400 {
		t.Fatalf("unknown export: %d", rec.Code)
	}
}
