// Command scenarios plays out what real people do with Kinjo against a running server and checks the result:
// meeting, walking past, a busy café, hiding, editing, blocking, losing signal, reconnecting, deleting, bad data.
//
//	go run ./cmd/scenarios -api http://localhost:8001
//
//lint:file-ignore ST1005 messages start with people's names and are printed for humans
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"kinjo/server/internal/sim"
)

var api = flag.String("api", "http://localhost:8001", "server base URL")

// Each scenario happens at its own spot, ~1 km from the others, so they never meet.
type spot struct{ lat, lng float64 }

func (s spot) north(m float64) spot { return spot{s.lat + m/111_195, s.lng} }

func place(i int) spot { return spot{28.9 + float64(i)*0.01, 77.4} }

type scenario struct {
	name string
	run  func(ctx context.Context, at spot) (string, error)
}

func main() {
	flag.Parse()
	scenarios := []scenario{
		{"Two friends meet (20 m apart)", twoFriendsMeet},
		{"Strangers 80 m apart never see each other", tooFar},
		{"Someone walks past and keeps going", walksPast},
		{"Busy café: 20 people arrive at once", busyCafe},
		{"Someone hides, then shows again", hidesAndShows},
		{"Someone edits their card", editsCard},
		{"Someone blocks a person", blocks},
		{"Two people open Kinjo at the same second", sameSecond},
		{"Resting phone: a signal blip, then gone for good", stillPhoneDrops},
		{"Phone loses signal and comes back", reconnects},
		{"Someone deletes their account", deletesAccount},
		{"Garbage and impossible data don't hurt anyone", badData},
		{"Phone killed while walking disappears", movingPhoneDies},
	}
	failed := 0
	for i, s := range scenarios {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		start := time.Now()
		detail, err := s.run(ctx, place(i))
		cancel()
		status := "PASS"
		if err != nil {
			status, detail, failed = "FAIL", err.Error(), failed+1
		}
		fmt.Printf("%-4s %-55s %6.1fs  %s\n", status, s.name, time.Since(start).Seconds(), detail)
	}
	fmt.Printf("\n%d of %d scenarios passed\n", len(scenarios)-failed, len(scenarios))
	if failed > 0 {
		os.Exit(1)
	}
}

// person signs up, saves a card, connects and stands at a spot.
func person(ctx context.Context, name string, at spot) (*sim.Phone, error) {
	p, err := sim.SignUp(ctx, *api)
	if err == nil {
		err = p.SaveCard(ctx, name, "Testing real life", "Seeing if Kinjo works")
	}
	if err == nil {
		err = p.Connect(ctx)
	}
	if err == nil {
		err = p.Report(ctx, at.lat, at.lng, 5, false)
	}
	if err != nil {
		return nil, fmt.Errorf("%s couldn't get started: %w", name, err)
	}
	return p, nil
}

func people(ctx context.Context, at spot, names ...string) ([]*sim.Phone, error) {
	var out []*sim.Phone
	for i, n := range names {
		p, err := person(ctx, n, at.north(float64(i)*3)) // standing a few metres apart
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func has(name string) func([]string) bool {
	return func(names []string) bool { return slices.Contains(names, name) }
}

func lacks(name string) func([]string) bool {
	return func(names []string) bool { return !slices.Contains(names, name) }
}

func twoFriendsMeet(ctx context.Context, at spot) (string, error) {
	a, err := person(ctx, "Asha", at)
	if err != nil {
		return "", err
	}
	b, err := person(ctx, "Ravi", at.north(20))
	if err != nil {
		return "", err
	}
	ta, okA := a.WaitUntil(has("Ravi"), 5*time.Second)
	tb, okB := b.WaitUntil(has("Asha"), 5*time.Second)
	if !okA || !okB {
		return "", fmt.Errorf("after 5 s: Asha sees %v, Ravi sees %v", a.Sees(), b.Sees())
	}
	return fmt.Sprintf("Asha saw Ravi after %s, Ravi saw Asha after %s", ta.Round(time.Millisecond), tb.Round(time.Millisecond)), nil
}

func tooFar(ctx context.Context, at spot) (string, error) {
	a, err := person(ctx, "Asha", at)
	if err != nil {
		return "", err
	}
	if _, err := person(ctx, "Stranger", at.north(80)); err != nil {
		return "", err
	}
	time.Sleep(3 * time.Second)
	if len(a.Sees()) != 0 {
		return "", fmt.Errorf("Asha sees %v at 80 m", a.Sees())
	}
	return "nobody shown after 3 s", nil
}

// Ravi walks from 150 m south, past Asha, to 150 m north, 5 m every 0.6 s (a brisk walk).
func walksPast(ctx context.Context, at spot) (string, error) {
	a, err := person(ctx, "Asha", at)
	if err != nil {
		return "", err
	}
	b, err := person(ctx, "Ravi", at.north(-150))
	if err != nil {
		return "", err
	}
	appearedAt, goneAt := math.NaN(), math.NaN()
	for m := -150.0; m <= 150; m += 5 {
		here := at.north(m)
		if err := b.Report(ctx, here.lat, here.lng, 5, false); err != nil {
			return "", err
		}
		time.Sleep(600 * time.Millisecond)
		seen := slices.Contains(a.Sees(), "Ravi")
		if seen && math.IsNaN(appearedAt) {
			appearedAt = m
		}
		if !seen && !math.IsNaN(appearedAt) && math.IsNaN(goneAt) {
			goneAt = m
		}
	}
	switch {
	case math.IsNaN(appearedAt):
		return "", errors.New("Asha never saw Ravi walk past")
	case math.Abs(appearedAt) > 45:
		return "", fmt.Errorf("Ravi appeared %.0f m away: too far", math.Abs(appearedAt))
	case math.IsNaN(goneAt):
		return "", errors.New("Ravi never disappeared after walking 150 m away")
	}
	return fmt.Sprintf("appeared %.0f m before reaching Asha, gone %.0f m after", math.Abs(appearedAt), goneAt), nil
}

func busyCafe(ctx context.Context, at spot) (string, error) {
	const n = 20
	phones := make([]*sim.Phone, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			phones[i], errs[i] = person(ctx, "Guest "+strconv.Itoa(i), at.north(float64(i%5)*2))
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return "", err
	}
	var slowest time.Duration
	for _, p := range phones {
		t, ok := p.WaitUntil(func(names []string) bool { return len(names) == n-1 }, 10*time.Second)
		if !ok {
			return "", fmt.Errorf("a guest sees %d of %d people after 10 s", len(p.Sees()), n-1)
		}
		slowest = max(slowest, t)
	}
	return fmt.Sprintf("everyone saw all %d others; last one within %s of checking", n-1, slowest.Round(time.Millisecond)), nil
}

func hidesAndShows(ctx context.Context, at spot) (string, error) {
	ps, err := people(ctx, at, "Asha", "Ravi", "Meera")
	if err != nil {
		return "", err
	}
	a, b, c := ps[0], ps[1], ps[2]
	if _, ok := a.WaitUntil(has("Ravi"), 5*time.Second); !ok {
		return "", errors.New("Asha never saw Ravi before he hid")
	}
	if err := b.SetVisible(ctx, false); err != nil {
		return "", err
	}
	t1, okA := a.WaitUntil(lacks("Ravi"), 3*time.Second)
	_, okC := c.WaitUntil(lacks("Ravi"), 3*time.Second)
	_, okB := b.WaitUntil(func(n []string) bool { return len(n) == 0 }, 3*time.Second)
	if !okA || !okC || !okB {
		return "", fmt.Errorf("after hiding: Asha %v, Meera %v, Ravi %v", a.Sees(), c.Sees(), b.Sees())
	}
	if err := b.SetVisible(ctx, true); err != nil {
		return "", err
	}
	here := at.north(3)
	if err := b.Report(ctx, here.lat, here.lng, 5, false); err != nil {
		return "", err
	}
	t2, ok := a.WaitUntil(has("Ravi"), 5*time.Second)
	if !ok {
		return "", errors.New("Ravi didn't come back after showing his profile")
	}
	return fmt.Sprintf("gone for everyone in %s, back in %s", t1.Round(time.Millisecond), t2.Round(time.Millisecond)), nil
}

func editsCard(ctx context.Context, at spot) (string, error) {
	ps, err := people(ctx, at, "Asha", "Ravi")
	if err != nil {
		return "", err
	}
	if _, ok := ps[0].WaitUntil(has("Ravi"), 5*time.Second); !ok {
		return "", errors.New("Asha never saw Ravi")
	}
	if err := ps[1].SaveCard(ctx, "Ravi K", "Now a designer", "Someone to build with"); err != nil {
		return "", err
	}
	t, ok := ps[0].WaitUntil(has("Ravi K"), 3*time.Second)
	if !ok {
		return "", fmt.Errorf("Asha still sees %v", ps[0].Sees())
	}
	return fmt.Sprintf("Asha saw the new card after %s", t.Round(time.Millisecond)), nil
}

func blocks(ctx context.Context, at spot) (string, error) {
	ps, err := people(ctx, at, "Asha", "Ravi", "Meera")
	if err != nil {
		return "", err
	}
	a, b, c := ps[0], ps[1], ps[2]
	if _, ok := c.WaitUntil(func(n []string) bool { return len(n) == 2 }, 5*time.Second); !ok {
		return "", errors.New("the three never all met")
	}
	if err := a.Block(ctx, b.UID); err != nil {
		return "", err
	}
	_, okA := a.WaitUntil(lacks("Ravi"), 3*time.Second)
	_, okB := b.WaitUntil(lacks("Asha"), 3*time.Second)
	if !okA || !okB {
		return "", fmt.Errorf("after the block: Asha %v, Ravi %v", a.Sees(), b.Sees())
	}
	time.Sleep(2500 * time.Millisecond) // let a few reports pass: they must not bring the pair back
	if slices.Contains(a.Sees(), "Ravi") || len(c.Sees()) != 2 {
		return "", fmt.Errorf("later: Asha %v, Meera %v", a.Sees(), c.Sees())
	}
	return "Asha and Ravi gone for each other; Meera still sees both", nil
}

func stillPhoneDrops(ctx context.Context, at spot) (string, error) {
	a, err := person(ctx, "Asha", at)
	if err != nil {
		return "", err
	}
	b, err := person(ctx, "Ravi", at.north(10))
	if err != nil {
		return "", err
	}
	here := at.north(10)
	if err := b.Report(ctx, here.lat, here.lng, 5, true); err != nil { // his phone says it isn't moving
		return "", err
	}
	if _, ok := a.WaitUntil(has("Ravi"), 5*time.Second); !ok {
		return "", errors.New("Asha never saw Ravi")
	}
	// A short blip (lift, tunnel): the app reconnects and resends its position. The card must not flicker.
	b.Drop()
	time.Sleep(3 * time.Second)
	if err := b.Connect(ctx); err != nil {
		return "", err
	}
	if err := b.Report(ctx, here.lat, here.lng, 5, true); err != nil {
		return "", err
	}
	for range 20 {
		if !slices.Contains(a.Sees(), "Ravi") {
			return "", errors.New("Ravi's card blinked out during a 3 s signal blip")
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Gone for good (phone off, out of signal): the card must leave soon, not linger.
	b.Drop()
	t, ok := a.WaitUntil(lacks("Ravi"), 35*time.Second)
	if !ok {
		return "", errors.New("Ravi still shown 35 s after his phone went silent for good")
	}
	return fmt.Sprintf("stayed through a 3 s blip; gone %s after going silent for good", t.Round(time.Second)), nil
}

func reconnects(ctx context.Context, at spot) (string, error) {
	ps, err := people(ctx, at, "Asha", "Ravi")
	if err != nil {
		return "", err
	}
	b := ps[1]
	if _, ok := b.WaitUntil(has("Asha"), 5*time.Second); !ok {
		return "", errors.New("Ravi never saw Asha")
	}
	b.Drop()
	if err := b.Connect(ctx); err != nil {
		return "", err
	}
	t, ok := b.WaitUntil(has("Asha"), 3*time.Second)
	if !ok {
		return "", errors.New("after reconnecting Ravi saw nobody")
	}
	return fmt.Sprintf("cards back %s after reconnecting", t.Round(time.Millisecond)), nil
}

func deletesAccount(ctx context.Context, at spot) (string, error) {
	ps, err := people(ctx, at, "Asha", "Ravi")
	if err != nil {
		return "", err
	}
	a, b := ps[0], ps[1]
	if _, ok := a.WaitUntil(has("Ravi"), 5*time.Second); !ok {
		return "", errors.New("Asha never saw Ravi")
	}
	if err := b.Delete(ctx); err != nil {
		return "", err
	}
	t, ok := a.WaitUntil(lacks("Ravi"), 3*time.Second)
	if !ok {
		return "", errors.New("Ravi still shown after deleting his account")
	}
	b.Drop()
	if b.Connect(ctx) == nil {
		return "", errors.New("a deleted account could still connect")
	}
	return fmt.Sprintf("gone in %s; the deleted account can't connect again", t.Round(time.Millisecond)), nil
}

func badData(ctx context.Context, at spot) (string, error) {
	ps, err := people(ctx, at, "Asha", "Ravi")
	if err != nil {
		return "", err
	}
	a, b := ps[0], ps[1]
	junk := [][]byte{[]byte("not json"), []byte(`{"type":"fix","lat":"x"}`), []byte(`{"type":"seen","seen":[{"t":"zz","rssi":0}]}`), []byte(`{}`)}
	for _, j := range junk {
		if err := b.Send(ctx, j); err != nil {
			return "", err
		}
	}
	far := at.north(5000)
	_ = b.Report(ctx, far.lat, far.lng, 500, false) // 500 m accuracy: must be ignored
	_ = b.Report(ctx, 200, 500, 5, false)           // impossible place: must be ignored
	here := at.north(3)
	if err := b.Report(ctx, here.lat, here.lng, 5, false); err != nil {
		return "", errors.New("the server dropped Ravi's connection over bad data")
	}
	if _, ok := a.WaitUntil(has("Ravi"), 5*time.Second); !ok {
		return "", fmt.Errorf("after the bad data Asha sees %v", a.Sees())
	}
	return "bad messages ignored; Ravi stayed connected and in place", nil
}

func movingPhoneDies(ctx context.Context, at spot) (string, error) {
	ps, err := people(ctx, at, "Asha", "Ravi")
	if err != nil {
		return "", err
	}
	a, b := ps[0], ps[1]
	if _, ok := a.WaitUntil(has("Ravi"), 5*time.Second); !ok {
		return "", errors.New("Asha never saw Ravi")
	}
	b.Drop() // killed mid-walk
	t, ok := a.WaitUntil(lacks("Ravi"), 35*time.Second)
	if !ok {
		return "", errors.New("Ravi still shown 35 s after his walking phone died")
	}
	return fmt.Sprintf("gone %s after the phone died", t.Round(time.Second)), nil
}

// Two people open Kinjo for the first time at the very same moment, standing together.
func sameSecond(ctx context.Context, at spot) (string, error) {
	var a, b *sim.Phone
	var errA, errB error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a, errA = person(ctx, "Asha", at) }()
	go func() { defer wg.Done(); b, errB = person(ctx, "Ravi", at.north(5)) }()
	wg.Wait()
	if err := errors.Join(errA, errB); err != nil {
		return "", err
	}
	if a.UID == b.UID {
		return "", errors.New("both phones got the same account")
	}
	_, okA := a.WaitUntil(has("Ravi"), 5*time.Second)
	_, okB := b.WaitUntil(has("Asha"), 5*time.Second)
	if !okA || !okB {
		return "", fmt.Errorf("Asha sees %v, Ravi sees %v", a.Sees(), b.Sees())
	}
	return "two separate accounts, and they saw each other", nil
}
