package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/database"
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
	default:
		fmt.Println("Usage: kinjo-admin [list | find <email> | tokens]")
	}
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

		fmt.Fprintf(w, "%s...\t%s\t%s\t%s\n",
			tokenHash[:8], email, expiresAt.Format("15:04:05"), createdAt.Format("15:04:05"),
		)
	}
	w.Flush()
}
