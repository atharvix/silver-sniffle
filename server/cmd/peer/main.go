// Command peer is a fake Kinjo user for device tests: it signs up, uploads a card and
// stands at a given spot, printing the names it sees whenever they change.
//
//	go run ./cmd/peer -lat 28.6139 -lng 77.2090 -name Asha
package main

import (
	"context"
	"flag"
	"log"
	"slices"
	"time"

	"kinjo/server/internal/sim"
)

func main() {
	api := flag.String("api", "http://localhost:8001", "server base URL")
	lat := flag.Float64("lat", 0, "latitude to stand at")
	lng := flag.Float64("lng", 0, "longitude to stand at")
	name := flag.String("name", "Test Peer", "card name")
	flag.Parse()
	if *lat == 0 && *lng == 0 {
		log.Fatal("give -lat and -lng")
	}
	ctx := context.Background()

	p, err := sim.SignUp(ctx, *api)
	must(err)
	log.Printf("signed up as %s", p.UID)
	must(p.SaveCard(ctx, *name, "Testing Kinjo nearby", "Someone to check this works"))
	must(p.Connect(ctx))
	var seen []string
	for {
		must(p.Report(ctx, *lat, *lng, 5, false))
		if now := p.Sees(); !slices.Equal(now, seen) {
			seen = now
			log.Printf("sees: %v", seen)
		}
		time.Sleep(2 * time.Second)
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
