package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GET /admin/export?what=people|downloads|cities&format=csv|xlsx (+ the People filters:
// q, filter, city, platform). Admin only. Never positions, photos, push tokens or device ids.
func (a *App) handleAdminExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	what, format := q.Get("what"), q.Get("format")
	if format != "xlsx" {
		format = "csv"
	}
	var head []string
	var rows [][]any
	switch what {
	case "people":
		ps, _, err := a.queryPeople(ctx, q, 0, 0)
		if err != nil {
			log.Printf("export people: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, errBody("export failed"))
			return
		}
		head = []string{"Name", "Email", "Email verified", "Role", "Looking for", "Photo", "Card complete", "Signed up with",
			"City", "Country", "Joined", "Last open", "Platform", "Events", "Hidden", "Reminders", "Online now", "uid"}
		for _, p := range ps {
			last := ""
			if p.LastOpen != nil {
				last = *p.LastOpen
			}
			rows = append(rows, []any{p.Name, p.Email, p.Verified, p.Role, p.Look, p.HasPhoto, p.Complete, p.Provider,
				p.City, p.Country, p.CreatedAt.In(usageLoc()).Format("2006-01-02 15:04"), last, p.Platform, p.Events, p.Hidden, p.Reminders, p.Online, p.UID})
		}
	case "downloads":
		res, err := a.store.pool.Query(ctx, `
			SELECT i.first_at, i.last_at, i.platform, i.model, i.os, i.version, i.uid IS NOT NULL, coalesce(p.name, ''), coalesce(u.city, '')
			FROM installs i LEFT JOIN users u ON u.id = i.uid LEFT JOIN profiles p ON p.uid = i.uid
			WHERE ($1 = '' OR i.platform = $1) ORDER BY i.first_at DESC`, q.Get("platform"))
		if err == nil {
			head = []string{"Installed", "Last open", "Platform", "Device", "OS", "App version", "Signed in", "Name", "City"}
			for res.Next() {
				var first, last time.Time
				var pf, model, os, ver, name, city string
				var signed bool
				if err = res.Scan(&first, &last, &pf, &model, &os, &ver, &signed, &name, &city); err != nil {
					break
				}
				rows = append(rows, []any{first.In(usageLoc()).Format("2006-01-02 15:04"), last.In(usageLoc()).Format("2006-01-02 15:04"),
					pf, model, os, ver, signed, name, city})
			}
			res.Close()
		}
		if err != nil {
			log.Printf("export downloads: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, errBody("export failed"))
			return
		}
	case "cities":
		res, err := a.store.pool.Query(ctx, `
			SELECT u.city, coalesce(u.country, ''), count(*),
			  count(*) FILTER (WHERE EXISTS (SELECT 1 FROM app_opens o WHERE o.uid = u.id AND o.day > `+today+` - 7)),
			  count(*) FILTER (WHERE u.created_at > now() - interval '7 days')
			FROM users u WHERE u.city <> '' GROUP BY 1, 2 ORDER BY 3 DESC`)
		if err == nil {
			head = []string{"City", "Country", "People", "Active this week", "New this week"}
			for res.Next() {
				var city, country string
				var n, act, nw int
				if err = res.Scan(&city, &country, &n, &act, &nw); err != nil {
					break
				}
				rows = append(rows, []any{city, country, n, act, nw})
			}
			res.Close()
		}
		if err != nil {
			log.Printf("export cities: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, errBody("export failed"))
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, errBody("what must be people, downloads or cities"))
		return
	}
	log.Printf("admin export %s %s: %d rows", what, format, len(rows))
	name := "kinjo-" + what + "-" + time.Now().In(usageLoc()).Format("2006-01-02") + "." + format
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if format == "xlsx" {
		b, err := xlsx(what, head, rows)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody("export failed"))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		_, _ = w.Write(b)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	_, _ = w.Write([]byte("\ufeff")) // so Excel reads UTF-8
	cw := csv.NewWriter(w)
	_ = cw.Write(head)
	for _, row := range rows {
		rec := make([]string, len(row))
		for i, v := range row {
			rec[i] = csvSafe(cell(v))
		}
		_ = cw.Write(rec)
	}
	cw.Flush()
}

func usageLoc() *time.Location {
	if l, err := time.LoadLocation(usageTZ); err == nil {
		return l
	}
	return time.UTC
}

func cell(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case int:
		return strconv.Itoa(x)
	case string:
		return x
	}
	return fmt.Sprint(v)
}

// csvSafe stops formula injection: a name like "=HYPERLINK(...)" opens as text in Excel.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// xlsx writes a one-sheet workbook. Text cells are inline strings (never formulas), so
// what people typed can't run in Excel. Header row bold, frozen.
func xlsx(sheet string, head []string, rows [][]any) ([]byte, error) {
	esc := func(s string) string {
		s = strings.Map(func(r rune) rune { // characters XML 1.0 can't carry
			if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r != 0xFFFE && r != 0xFFFF {
				return r
			}
			return -1
		}, s)
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	col := func(i int) string {
		s := ""
		for i++; i > 0; i = (i - 1) / 26 {
			s = string(rune('A'+(i-1)%26)) + s
		}
		return s
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	sb.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><sheetData>`)
	writeRow := func(n int, vals []any, style string) {
		fmt.Fprintf(&sb, `<row r="%d">`, n)
		for i, v := range vals {
			ref := col(i) + strconv.Itoa(n)
			if x, ok := v.(int); ok {
				fmt.Fprintf(&sb, `<c r="%s"%s><v>%d</v></c>`, ref, style, x)
				continue
			}
			fmt.Fprintf(&sb, `<c r="%s" t="inlineStr"%s><is><t xml:space="preserve">%s</t></is></c>`, ref, style, esc(cell(v)))
		}
		sb.WriteString(`</row>`)
	}
	h := make([]any, len(head))
	for i, x := range head {
		h[i] = x
	}
	writeRow(1, h, ` s="1"`)
	for i, r := range rows {
		writeRow(i+2, r, "")
	}
	sb.WriteString(`</sheetData></worksheet>`)

	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + esc(sheet) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs></styleSheet>`},
		{"xl/worksheets/sheet1.xml", sb.String()},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		fw, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
