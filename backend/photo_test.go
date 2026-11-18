package main

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

// Cards come without the photo but still count as complete; the photo is read on its own.
func TestCardsWithoutPhotos(t *testing.T) {
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
	uid, _, err := s.emailUser(ctx, "card-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer s.deleteAccount(ctx, uid)
	photo := "data:image/jpeg;base64,/9j/4AAQ"
	if err := s.saveProfile(ctx, uid, Profile{Name: "A", Role: "B", Look: "C", Photo: photo}); err != nil {
		t.Fatal(err)
	}
	got, err := s.getCards(ctx, []string{uid, "nobody"})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := got[uid]
	if !ok || c.Photo != "" || !c.HasPhoto || !c.complete() || len(got) != 1 {
		t.Fatalf("card: %+v %v (of %d)", c, ok, len(got))
	}
	if p, err := s.getPhoto(ctx, uid); err != nil || p != photo {
		t.Fatalf("photo: %q %v", p, err)
	}
	if p, err := s.getPhoto(ctx, "nobody"); err != nil || p != "" {
		t.Fatalf("no profile: %q %v", p, err)
	}
}
