package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/storage"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("/var/www/silver-sniffle/backend/backend-go/.env")

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger := slog.Default()
	db, err := database.New(ctx, cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	cmd := "list"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "list":
		listUsers(ctx, db)
	case "find":
		if len(os.Args) < 3 {
			fmt.Println("Usage: kinjo-admin find <email>")
			os.Exit(1)
		}
		findUser(ctx, db, os.Args[2])
	case "tokens":
		listTokens(ctx, db)
	case "migrate-photos":
		migratePhotos(cfg, db)
	default:
		fmt.Println("Usage: kinjo-admin [list | find <email> | tokens | migrate-photos]")
	}
}

// migratePhotos moves profile and face-scan photos that were stored inline as
// base64 data URLs onto real storage, leaving only a URL in the row.
//
// Inline rows are why a single discovery response used to weigh ~160KB: the
// whole image travelled inside the JSON, could never be cached, and was re-sent
// on every poll. Only rows still holding a data: URL are touched, so running
// this twice is a no-op.
func migratePhotos(cfg *config.Config, db *database.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	store, err := storage.NewFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT email, photo_url, face_scan_photo_url
		FROM profiles
		WHERE photo_url LIKE 'data:%' OR face_scan_photo_url LIKE 'data:%';
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query failed: %v\n", err)
		os.Exit(1)
	}

	type legacyRow struct{ email, photo, face string }
	var pending []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.email, &r.photo, &r.face); err != nil {
			continue
		}
		pending = append(pending, r)
	}
	rows.Close()

	if len(pending) == 0 {
		fmt.Println("No profiles store photos inline as base64. Nothing to migrate.")
		return
	}

	migrated := 0
	for _, r := range pending {
		if strings.HasPrefix(r.photo, "data:") {
			if url, err := storage.ProcessImage(ctx, store, r.photo, cfg.MaxPhotoBytes); err == nil && url != "" {
				if _, err := db.Pool.Exec(ctx, `UPDATE profiles SET photo_url = $2 WHERE email = $1`, r.email, url); err == nil {
					migrated++
					fmt.Printf("  photo_url       %s -> %s\n", r.email, url)
				}
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "  photo_url       %s: %v\n", r.email, err)
			}
		}

		if strings.HasPrefix(r.face, "data:") {
			if url, err := storage.ProcessImage(ctx, store, r.face, cfg.MaxPhotoBytes); err == nil && url != "" {
				if _, err := db.Pool.Exec(ctx, `UPDATE profiles SET face_scan_photo_url = $2 WHERE email = $1`, r.email, url); err == nil {
					migrated++
					fmt.Printf("  face_scan_photo %s -> %s\n", r.email, url)
				}
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "  face_scan_photo %s: %v\n", r.email, err)
			}
		}
	}

	fmt.Printf("\nMigrated %d inline photo(s) across %d profile(s).\n", migrated, len(pending))
}

func listUsers(ctx context.Context, db *database.DB) {
	rows, err := db.Pool.Query(ctx, `
		SELECT email, name, email_verified, face_verified_at, created_at, last_seen_at
		FROM profiles
		ORDER BY created_at DESC;
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query failed: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tNAME\tEMAIL_VERIFIED\tFACE_VERIFIED\tJOINED")
	fmt.Fprintln(w, "-----\t----\t--------------\t-------------\t------")

	count := 0
	for rows.Next() {
		var email, name string
		var emailVerified bool
		var faceVerifiedAt *time.Time
		var createdAt time.Time
		var lastSeenAt *time.Time

		if err := rows.Scan(&email, &name, &emailVerified, &faceVerifiedAt, &createdAt, &lastSeenAt); err != nil {
			continue
		}

		faceStatus := "No"
		if faceVerifiedAt != nil {
			faceStatus = "Yes"
		}

		fmt.Fprintf(w, "%s\t%s\t%v\t%s\t%s\n",
			email, name, emailVerified, faceStatus, createdAt.Format("2006-01-02 15:04"),
		)
		count++
	}
	w.Flush()

	fmt.Printf("\nTotal Users: %d\n", count)
}

func findUser(ctx context.Context, db *database.DB, targetEmail string) {
	var email, name, bio, photoURL string
	var emailVerified bool
	var faceVerifiedAt *time.Time
	var createdAt, updatedAt time.Time

	err := db.Pool.QueryRow(ctx, `
		SELECT email, name, bio, photo_url, email_verified, face_verified_at, created_at, updated_at
		FROM profiles
		WHERE email = $1;
	`, targetEmail).Scan(&email, &name, &bio, &photoURL, &emailVerified, &faceVerifiedAt, &createdAt, &updatedAt)
	if err != nil {
		fmt.Printf("User with email '%s' not found.\n", targetEmail)
		return
	}

	fmt.Println("================ USER DETAILS ================")
	fmt.Printf("Email:          %s\n", email)
	fmt.Printf("Name:           %s\n", name)
	fmt.Printf("Bio:            %s\n", bio)
	fmt.Printf("Email Verified: %v\n", emailVerified)
	fmt.Printf("Face Verified:  %v\n", faceVerifiedAt != nil)
	fmt.Printf("Photo URL:      %s\n", photoURL)
	fmt.Printf("Joined:         %s\n", createdAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Last Updated:   %s\n", updatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println("==============================================")
}

func listTokens(ctx context.Context, db *database.DB) {
	rows, err := db.Pool.Query(ctx, `
		SELECT token_hash, email, expires_at, created_at
		FROM verification_tokens
		ORDER BY created_at DESC
		LIMIT 20;
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query failed: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TOKEN_HASH (FIRST 8)\tEMAIL\tEXPIRES\tCREATED")
	fmt.Fprintln(w, "--------------------\t-----\t-------\t-------")

	for rows.Next() {
		var tokenHash, email string
		var expiresAt, createdAt time.Time

		if err := rows.Scan(&tokenHash, &email, &expiresAt, &createdAt); err != nil {
			continue
		}

		shortHash := tokenHash
		if len(shortHash) > 8 {
			shortHash = shortHash[:8]
		}

		fmt.Fprintf(w, "%s...\t%s\t%s\t%s\n",
			shortHash, email, expiresAt.Format("15:04:05"), createdAt.Format("15:04:05"),
		)
	}
	w.Flush()
}
