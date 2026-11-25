package main

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// Daily totals for the admin dashboard: counts only, never who and never where.
const (
	countSignups    = "signups"     // new accounts
	countCards      = "cards"       // cards saved (new or edited)
	countMatches    = "matches"     // new pairs within 30 m, by GPS
	countBLEMatches = "ble_matches" // new pairs found by Bluetooth
	countWakes      = "wakes"       // wake-ups queued for quiet phones
	countBlocks     = "blocks"
	countReports    = "reports"
	countHides      = "hides"
	countDeletes    = "deletes" // accounts deleted
)

var countNames = []string{countSignups, countCards, countMatches, countBLEMatches, countWakes, countBlocks, countReports, countHides, countDeletes}

func kCount(metric string) string { return "k:count:" + metric }

// count adds to a daily total. It sits in Redis until the next flush (every minute).
func (h *Hub) count(ctx context.Context, metric string, n int64) {
	if n == 0 {
		return
	}
	if err := h.rdb.IncrBy(ctx, kCount(metric), n).Err(); err != nil {
		slog.Warn("count", "metric", metric, "err", err)
	}
}

// flushCounts moves the counters into today's totals in Postgres. GETSET takes and resets each one in a single
// step, so several servers can flush at once; if saving fails, the taken counts go back into Redis.
func flushCounts(ctx context.Context, rdb redis.UniversalClient, store *Store) error {
	taken := map[string]int64{}
	for _, m := range countNames {
		n, err := rdb.GetSet(ctx, kCount(m), 0).Int64()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if n > 0 {
			taken[m] = n
		}
	}
	if err := store.addCounts(ctx, taken); err != nil {
		for m, n := range taken {
			rdb.IncrBy(ctx, kCount(m), n)
		}
		return err
	}
	return nil
}

// liveCounts counts phones connected to a server and phones findable right now, across every Redis node.
func (h *Hub) liveCounts(ctx context.Context) (online, located int64) {
	var on, loc atomic.Int64
	count := func(c redis.UniversalClient) {
		for _, p := range []struct {
			pattern string
			n       *atomic.Int64
		}{{"k:online:*", &on}, {"k:pos:*", &loc}} {
			iter := c.Scan(ctx, 0, p.pattern, 1000).Iterator()
			for iter.Next(ctx) {
				p.n.Add(1)
			}
		}
	}
	if cluster, ok := h.rdb.(*redis.ClusterClient); ok {
		_ = cluster.ForEachMaster(ctx, func(ctx context.Context, c *redis.Client) error {
			count(c)
			return nil
		})
	} else {
		count(h.rdb)
	}
	return on.Load(), loc.Load()
}
