// Command crowd is a load test: n simulated phones open Kinjo at the same moment in one room, then keep
// reporting. It measures how long until everyone sees everyone (up to the 100-card deck cap) and what it cost.
//
//	go run ./cmd/crowd -api http://localhost:8001 -n 500 -for 60s
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"kinjo/server/internal/sim"
)

const deckCap = 100 // the server's maxDeck

var (
	api     = flag.String("api", "http://localhost:8001", "server base URL")
	n       = flag.Int("n", 100, "phones in the room")
	runFor  = flag.Duration("for", 60*time.Second, "how long the room stays")
	every   = flag.Duration("report", 2*time.Second, "position interval per phone (2 s = walking around)")
	spreadM = flag.Float64("spread", 15, "room size in metres: phones stand anywhere in this square")
	lat0    = flag.Float64("lat", 28.7, "room centre")
	lng0    = flag.Float64("lng", 77.3, "room centre")
)

type stats struct {
	firstErr                                                 sync.Map // step -> first error, to say why
	signupFail, cardFail, dialFail, dropped, messages, bytes atomic.Int64
	mu                                                       sync.Mutex
	fullAt, signupMs, cardMs                                 []time.Duration
}

func main() {
	flag.Parse()
	target := min(*n-1, deckCap)
	var st stats
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), *runFor)
	defer cancel()

	var wg sync.WaitGroup
	for i := range *n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			phone(ctx, i, start, target, &st)
		}()
	}
	wg.Wait()

	slices.Sort(st.fullAt)
	slices.Sort(st.signupMs)
	slices.Sort(st.cardMs)
	fmt.Printf("phones %d, room %.0f m, reports every %s, ran %s\n", *n, *spreadM, *every, *runFor)
	fmt.Printf("sign-up: %d failed, p50 %s, p95 %s\n", st.signupFail.Load(), pct(st.signupMs, 50), pct(st.signupMs, 95))
	fmt.Printf("save card: %d failed, p50 %s, p95 %s\n", st.cardFail.Load(), pct(st.cardMs, 50), pct(st.cardMs, 95))
	fmt.Printf("connect: %d failed, %d dropped while running\n", st.dialFail.Load(), st.dropped.Load())
	fmt.Printf("saw the full deck (%d cards): %d of %d phones; p50 %s, p90 %s, slowest %s (from the moment all opened)\n",
		target, len(st.fullAt), *n, pct(st.fullAt, 50), pct(st.fullAt, 90), pct(st.fullAt, 100))
	fmt.Printf("server sent %d messages, %.1f MB in total (%.0f KB per phone)\n",
		st.messages.Load(), float64(st.bytes.Load())/1e6, float64(st.bytes.Load())/1e3/float64(*n))
	st.firstErr.Range(func(step, err any) bool {
		fmt.Printf("first %s error: %v\n", step, err)
		return true
	})
}

func phone(ctx context.Context, i int, start time.Time, target int, st *stats) {
	t := time.Now()
	p, err := sim.SignUp(ctx, *api)
	if err != nil {
		st.signupFail.Add(1)
		st.firstErr.LoadOrStore("sign-up", err)
		return
	}
	st.add(&st.signupMs, time.Since(t))
	t = time.Now()
	if err := p.SaveCard(ctx, fmt.Sprintf("Crowd %d", i), "Load testing Kinjo", "Everyone in this room"); err != nil {
		st.cardFail.Add(1)
		st.firstErr.LoadOrStore("save card", err)
		return
	}
	st.add(&st.cardMs, time.Since(t))
	if err := p.Connect(ctx); err != nil {
		st.dialFail.Add(1)
		st.firstErr.LoadOrStore("connect", err)
		return
	}
	defer func() {
		msgs, bytes := p.Traffic()
		st.messages.Add(msgs)
		st.bytes.Add(bytes)
		p.Drop()
	}()

	// Stand somewhere in the room, shifting a little, like people do.
	r := rand.New(rand.NewSource(int64(i)))
	lat := *lat0 + (r.Float64()-0.5)**spreadM/111_195
	lng := *lng0 + (r.Float64()-0.5)**spreadM/(111_195*math.Cos(*lat0*math.Pi/180))
	report := time.NewTicker(*every + time.Duration(r.Intn(400))*time.Millisecond)
	defer report.Stop()
	look := time.NewTicker(50 * time.Millisecond)
	defer look.Stop()
	full := false
	for {
		if err := p.Report(ctx, lat+(r.Float64()-0.5)*2e-6, lng, 8, false); err != nil {
			if ctx.Err() == nil {
				st.dropped.Add(1)
			}
			return
		}
	wait:
		for {
			select {
			case <-report.C:
				break wait
			case <-look.C:
				if !full && len(p.Sees()) >= target {
					full = true
					st.add(&st.fullAt, time.Since(start))
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

func (st *stats) add(list *[]time.Duration, d time.Duration) {
	st.mu.Lock()
	*list = append(*list, d)
	st.mu.Unlock()
}

func pct(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[min(len(sorted)-1, len(sorted)*p/100)].Round(time.Millisecond)
}
