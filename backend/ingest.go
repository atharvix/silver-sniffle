package main

import (
	"context"
	"hash/fnv"
	"sync"
	"time"
)

// Latest-wins ingest for socket fixes: a socket's reader drops its fix into one of
// ingestShards slots (by uid) and goes back to reading, never waiting on the hub's lock.
// A newer fix from the same person overwrites one not yet applied, so under overload the
// server applies fewer but always current positions, and memory stays one slot per person.
// One worker applies them; the reporter's "mode" answer goes out after its fix is in.
const ingestShards = 16

type queued struct {
	c  *Client
	r  report
	at time.Time
}

type ingestShard struct {
	mu      sync.Mutex
	pending map[string]queued
}

type ingest struct {
	shards [ingestShards]ingestShard
	kick   chan struct{}
}

func newIngest() *ingest {
	in := &ingest{kick: make(chan struct{}, 1)}
	for i := range in.shards {
		in.shards[i].pending = map[string]queued{}
	}
	return in
}

func (in *ingest) shard(uid string) *ingestShard {
	f := fnv.New32a()
	_, _ = f.Write([]byte(uid))
	return &in.shards[f.Sum32()%ingestShards]
}

// enqueue a socket fix; an older one for the same person still waiting is replaced.
func (h *Hub) enqueue(c *Client, r report, at time.Time) {
	s := h.in.shard(c.uid)
	s.mu.Lock()
	if _, waiting := s.pending[c.uid]; waiting {
		h.stat.coalesced.Add(1)
	}
	s.pending[c.uid] = queued{c: c, r: r, at: at}
	s.mu.Unlock()
	select {
	case h.in.kick <- struct{}{}:
	default:
	}
}

// drop forgets a waiting fix (hide, logout): it must not land after the leave.
func (h *Hub) dropPending(uid string) {
	s := h.in.shard(uid)
	s.mu.Lock()
	delete(s.pending, uid)
	s.mu.Unlock()
}

// applyLoop is the one worker that applies queued fixes.
func (h *Hub) applyLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.in.kick:
		}
		for i := range h.in.shards {
			s := &h.in.shards[i]
			s.mu.Lock()
			batch := s.pending
			if len(batch) > 0 {
				s.pending = map[string]queued{}
			}
			s.mu.Unlock()
			for uid, q := range batch {
				h.addReport(uid, q.r, true, q.at)
				if q.c.reporter {
					mode, next, sleep := h.mode(uid, q.at)
					q.c.sendJSON(map[string]any{"type": "mode", "mode": mode, "next": next, "sleep": sleep})
				}
			}
		}
	}
}
