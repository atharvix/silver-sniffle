package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// openBatch batches app opens: the app posts one on every start and resume, and one INSERT
// each was most of the database's writes. They queue (one per person, latest platform) and go
// in one statement a second; the answer to the app doesn't wait for the database.
type openBatch struct {
	mu      sync.Mutex
	pending map[string]string // uid -> platform
	done    chan struct{}     // closed once the last write on shutdown is done
}

func newOpenBatch() *openBatch {
	return &openBatch{pending: map[string]string{}, done: make(chan struct{})}
}

func (b *openBatch) add(uid, platform string) {
	b.mu.Lock()
	b.pending[uid] = platform
	b.mu.Unlock()
}

func (b *openBatch) take() map[string]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.pending
	b.pending = map[string]string{}
	return out
}

// run writes the queue every second, and once more on shutdown.
func (b *openBatch) run(ctx context.Context, s *Store) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	defer close(b.done)
	write := func() {
		batch := b.take()
		if len(batch) == 0 {
			return
		}
		wctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.recordOpens(wctx, batch); err != nil {
			log.Printf("app opens (%d): %v", len(batch), err)
			b.mu.Lock() // put them back unless newer ones came: the next tick retries
			for uid, p := range batch {
				if _, ok := b.pending[uid]; !ok {
					b.pending[uid] = p
				}
			}
			b.mu.Unlock()
		}
	}
	for {
		select {
		case <-ctx.Done():
			write()
			return
		case <-t.C:
			write()
		}
	}
}
