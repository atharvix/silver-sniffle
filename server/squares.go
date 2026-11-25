package main

import (
	"sync"
	"time"
)

// squares is this server's short-lived copy of map squares read from Redis. In a packed room hundreds of
// reports a second share one read per square instead of each reading the whole room again. Writes made
// here update the copy at once; another server's writes show up when the copy is re-read (at most `life`).
type squares struct {
	life time.Duration
	mu   sync.Mutex
	seen map[cell]square
}

type square struct {
	read  time.Time
	fixes map[string]fix
}

func newSquares(life time.Duration) *squares {
	return &squares{life: life, seen: map[cell]square{}}
}

// get adds the fresh positions of every copied square to out and returns the squares it doesn't have.
func (s *squares) get(cells []cell, now time.Time, out map[string]fix) (missing []cell) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range cells {
		sq, ok := s.seen[c]
		if !ok || time.Since(sq.read) >= s.life {
			missing = append(missing, c)
			continue
		}
		for u, f := range sq.fixes {
			if f.fresh(now) {
				out[u] = f
			}
		}
	}
	return missing
}

// put stores a square just read from Redis. The map becomes the copy's own.
func (s *squares) put(c cell, fixes map[string]fix) {
	s.mu.Lock()
	s.seen[c] = square{time.Now(), fixes}
	s.mu.Unlock()
}

// move updates a copied square when this server places or removes someone (f nil: removed).
func (s *squares) move(c cell, uid string, f *fix) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sq, ok := s.seen[c]; ok {
		if f == nil {
			delete(sq.fixes, uid)
		} else {
			sq.fixes[uid] = *f
		}
	}
}

// prune drops copies too old to use. sweep calls it.
func (s *squares) prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c, sq := range s.seen {
		if time.Since(sq.read) >= s.life {
			delete(s.seen, c)
		}
	}
}
