package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Admin dashboard queries: aggregates and one person's detail. Read-only, except
// revokeSessions (admin "sign out everywhere").

const today = `(now() AT TIME ZONE '` + usageTZ + `')::date`

// providerSQL: how an account first signed in, from its uid prefix (li_, g_, em_).
const providerSQL = `CASE WHEN u.id LIKE 'li\_%' THEN 'linkedin' WHEN u.id LIKE 'g\_%' THEN 'google' ELSE 'email' END`

type GrowthDay struct {
	Day      string `json:"day"`
	Signups  int    `json:"signups"`
	LinkedIn int    `json:"linkedin"`
	Google   int    `json:"google"`
	Email    int    `json:"email"`
	Active   int    `json:"active"` // people who opened the app that day
	Opens    int    `json:"opens"`
}

type Growth struct {
	Days      []GrowthDay    `json:"days"`
	DAU       int            `json:"dau"`
	WAU       int            `json:"wau"`
	MAU       int            `json:"mau"`
	Providers map[string]int `json:"providers"` // all accounts, by first sign-in method
	Platforms map[string]int `json:"platforms"` // people active in 30 days, by the platform of their latest open
	Devices   map[string]int `json:"devices"`   // push-registered installs by platform
	Funnel    struct {
		Users    int `json:"users"`
		Named    int `json:"named"`
		Photo    int `json:"photo"`
		Complete int `json:"complete"`
		Verified int `json:"verified"`
		Active7  int `json:"active7"`
	} `json:"funnel"`
	Retention struct {
		D1       int `json:"d1"` // opened the day after signing up
		D1Cohort int `json:"d1Cohort"`
		D7       int `json:"d7"` // opened on any of days 7–13 after signing up
		D7Cohort int `json:"d7Cohort"`
	} `json:"retention"`
	Notifications struct {
		Total  int `json:"total"`
		Unread int `json:"unread"`
		Sent7d int `json:"sent7d"`
	} `json:"notifications"`
}

func (s *Store) growth(ctx context.Context, days int) (Growth, error) {
	if days < 7 || days > 180 {
		days = 30
	}
	var g Growth
	rows, err := s.pool.Query(ctx, `
		WITH d AS (SELECT generate_series(`+today+` - ($1::int - 1), `+today+`, interval '1 day')::date AS day),
		su AS (SELECT (u.created_at AT TIME ZONE '`+usageTZ+`')::date AS day, count(*) n,
		              count(*) FILTER (WHERE u.id LIKE 'li\_%') li, count(*) FILTER (WHERE u.id LIKE 'g\_%') g,
		              count(*) FILTER (WHERE u.id NOT LIKE 'li\_%' AND u.id NOT LIKE 'g\_%') em
		       FROM users u WHERE u.created_at >= now() - make_interval(days => $1::int + 1) GROUP BY 1),
		ao AS (SELECT day, count(*) active, sum(opens) opens FROM app_opens WHERE day > `+today+` - $1::int GROUP BY day)
		SELECT to_char(d.day, 'YYYY-MM-DD'), coalesce(su.n, 0), coalesce(su.li, 0), coalesce(su.g, 0), coalesce(su.em, 0),
		       coalesce(ao.active, 0), coalesce(ao.opens, 0)
		FROM d LEFT JOIN su USING (day) LEFT JOIN ao USING (day) ORDER BY d.day`, days)
	if err != nil {
		return g, err
	}
	for rows.Next() {
		var d GrowthDay
		if err := rows.Scan(&d.Day, &d.Signups, &d.LinkedIn, &d.Google, &d.Email, &d.Active, &d.Opens); err != nil {
			rows.Close()
			return g, err
		}
		g.Days = append(g.Days, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return g, err
	}

	f, r, n := &g.Funnel, &g.Retention, &g.Notifications
	err = s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM app_opens WHERE day = `+today+`),
		  (SELECT count(DISTINCT uid) FROM app_opens WHERE day > `+today+` - 7),
		  (SELECT count(DISTINCT uid) FROM app_opens WHERE day > `+today+` - 30),
		  (SELECT count(*) FROM users),
		  (SELECT count(*) FROM profiles WHERE name <> ''),
		  (SELECT count(*) FROM profiles WHERE photo <> ''),
		  (SELECT count(*) FROM profiles WHERE name <> '' AND role <> '' AND look <> '' AND photo <> ''),
		  (SELECT count(*) FROM users WHERE email_verified),
		  (SELECT count(*) FROM users u WHERE u.created_at < now() - interval '2 days' AND u.created_at > now() - interval '32 days'),
		  (SELECT count(*) FROM users u WHERE u.created_at < now() - interval '2 days' AND u.created_at > now() - interval '32 days'
		     AND EXISTS (SELECT 1 FROM app_opens o WHERE o.uid = u.id AND o.day = (u.created_at AT TIME ZONE '`+usageTZ+`')::date + 1)),
		  (SELECT count(*) FROM users u WHERE u.created_at < now() - interval '14 days' AND u.created_at > now() - interval '74 days'),
		  (SELECT count(*) FROM users u WHERE u.created_at < now() - interval '14 days' AND u.created_at > now() - interval '74 days'
		     AND EXISTS (SELECT 1 FROM app_opens o WHERE o.uid = u.id
		       AND o.day BETWEEN (u.created_at AT TIME ZONE '`+usageTZ+`')::date + 7 AND (u.created_at AT TIME ZONE '`+usageTZ+`')::date + 13)),
		  (SELECT count(*) FROM notifications),
		  (SELECT count(*) FROM notifications WHERE read_at IS NULL),
		  (SELECT count(*) FROM notifications WHERE sent_at > now() - interval '7 days')`).
		Scan(&g.DAU, &g.WAU, &g.MAU, &f.Users, &f.Named, &f.Photo, &f.Complete, &f.Verified,
			&r.D1Cohort, &r.D1, &r.D7Cohort, &r.D7, &n.Total, &n.Unread, &n.Sent7d)
	if err != nil {
		return g, err
	}
	f.Active7 = g.WAU

	count := func(q string) (map[string]int, error) {
		out := map[string]int{}
		rows, err := s.pool.Query(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var v int
			if err := rows.Scan(&k, &v); err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, rows.Err()
	}
	if g.Providers, err = count(`SELECT ` + providerSQL + `, count(*) FROM users u GROUP BY 1`); err != nil {
		return g, err
	}
	if g.Platforms, err = count(`SELECT coalesce(nullif(platform, ''), 'unknown'), count(*) FROM (
		SELECT DISTINCT ON (uid) uid, platform FROM app_opens WHERE day > ` + today + ` - 30 ORDER BY uid, day DESC) x GROUP BY 1`); err != nil {
		return g, err
	}
	g.Devices, err = count(`SELECT platform, count(*) FROM devices WHERE platform <> 'ioslocation' GROUP BY 1`)
	return g, err
}

type AdminDevice struct {
	Platform string    `json:"platform"`
	DeviceID string    `json:"deviceId"`
	Updated  time.Time `json:"updatedAt"`
	Location bool      `json:"locationPush"` // an iPhone's location-push token, not a phone of its own
}

type AdminOpen struct {
	Day      string    `json:"day"`
	Opens    int       `json:"opens"`
	Platform string    `json:"platform"`
	First    time.Time `json:"firstAt"`
	Last     time.Time `json:"lastAt"`
}

type AdminNotif struct {
	Title   string     `json:"title"`
	Body    string     `json:"body"`
	Created time.Time  `json:"createdAt"`
	Sent    *time.Time `json:"sentAt"`
	Read    *time.Time `json:"readAt"`
}

type AdminUserDetail struct {
	UID         string        `json:"uid"`
	Email       string        `json:"email"`
	Verified    bool          `json:"verified"`
	VerifiedAt  *time.Time    `json:"verifiedAt"`
	CreatedAt   time.Time     `json:"createdAt"`
	Provider    string        `json:"provider"`
	LinkedIn    bool          `json:"linkedin"`
	Google      bool          `json:"google"`
	Name        string        `json:"name"`
	Role        string        `json:"role"`
	Look        string        `json:"look"`
	HasPhoto    bool          `json:"hasPhoto"`
	ProfileAt   *time.Time    `json:"profileAt"`
	Sessions    int           `json:"sessions"`
	LastSeen    *time.Time    `json:"lastSeen"`
	Devices     []AdminDevice `json:"devices"`
	Opens       []AdminOpen   `json:"opens"`
	NotifTotal  int           `json:"notifTotal"`
	NotifUnread int           `json:"notifUnread"`
	Notifs      []AdminNotif  `json:"notifs"`
}

var errNoUser = errors.New("no such user")

func (s *Store) userDetail(ctx context.Context, uid string) (AdminUserDetail, error) {
	d := AdminUserDetail{Devices: []AdminDevice{}, Opens: []AdminOpen{}, Notifs: []AdminNotif{}}
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.email_verified, u.email_verified_at, u.created_at, `+providerSQL+`,
		       u.linkedin_sub IS NOT NULL, u.google_sub IS NOT NULL,
		       coalesce(p.name, ''), coalesce(p.role, ''), coalesce(p.look, ''), coalesce(p.photo, '') <> '', p.updated_at,
		       (SELECT count(*) FROM sessions WHERE uid = u.id), (SELECT max(seen_at) FROM sessions WHERE uid = u.id),
		       (SELECT count(*) FROM notifications WHERE uid = u.id),
		       (SELECT count(*) FROM notifications WHERE uid = u.id AND read_at IS NULL)
		FROM users u LEFT JOIN profiles p ON p.uid = u.id WHERE u.id = $1`, uid).
		Scan(&d.UID, &d.Email, &d.Verified, &d.VerifiedAt, &d.CreatedAt, &d.Provider, &d.LinkedIn, &d.Google,
			&d.Name, &d.Role, &d.Look, &d.HasPhoto, &d.ProfileAt, &d.Sessions, &d.LastSeen, &d.NotifTotal, &d.NotifUnread)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, errNoUser
	}
	if err != nil {
		return d, err
	}
	// never the push token itself
	rows, err := s.pool.Query(ctx, `SELECT platform, coalesce(device_id, ''), updated_at FROM devices WHERE uid = $1 ORDER BY updated_at DESC`, uid)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v AdminDevice
		if err := rows.Scan(&v.Platform, &v.DeviceID, &v.Updated); err != nil {
			rows.Close()
			return d, err
		}
		v.Location = v.Platform == "ioslocation"
		d.Devices = append(d.Devices, v)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT to_char(day, 'YYYY-MM-DD'), opens, platform, first_at, last_at FROM app_opens WHERE uid = $1 ORDER BY day DESC LIMIT 30`, uid)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var o AdminOpen
		if err := rows.Scan(&o.Day, &o.Opens, &o.Platform, &o.First, &o.Last); err != nil {
			rows.Close()
			return d, err
		}
		d.Opens = append(d.Opens, o)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT title, body, created_at, sent_at, read_at FROM notifications WHERE uid = $1 ORDER BY created_at DESC LIMIT 10`, uid)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var n AdminNotif
		if err := rows.Scan(&n.Title, &n.Body, &n.Created, &n.Sent, &n.Read); err != nil {
			return d, err
		}
		d.Notifs = append(d.Notifs, n)
	}
	return d, rows.Err()
}

// revokeSessions signs a user out everywhere: sessions (cache first, like logout)
// and push devices, so their phones stop getting pushes and their services stop on the next 401.
func (s *Store) revokeSessions(ctx context.Context, uid string) (int64, error) {
	s.forgetSessions(uid)
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE uid = $1`, uid)
	s.forgetSessions(uid)
	if err != nil {
		return 0, err
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM devices WHERE uid = $1`, uid)
	return tag.RowsAffected(), err
}

// dbStats: connection pool and database size for the server-health view.
func (s *Store) dbStats(ctx context.Context) map[string]any {
	st := s.pool.Stat()
	out := map[string]any{
		"conns": st.TotalConns(), "idle": st.IdleConns(), "inUse": st.AcquiredConns(), "max": st.MaxConns(),
		"acquires": st.AcquireCount(), "waited": st.EmptyAcquireCount(),
		"avgAcquireMs": float64(st.AcquireDuration().Microseconds()) / 1000 / float64(max(st.AcquireCount(), 1)),
	}
	var size int64
	start := time.Now()
	if s.pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&size) == nil {
		out["sizeMB"] = float64(size) / 1e6
		out["pingMs"] = float64(time.Since(start).Microseconds()) / 1000
	}
	return out
}
