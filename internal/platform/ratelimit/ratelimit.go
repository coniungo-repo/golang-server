// Package ratelimit limits request rates in memory with GCRA. Counts are per process, so N instances allow up to N times the limit in total.
package ratelimit

import (
	"context"
	"hash/maphash"
	"net/netip"
	"sync"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
)

const shardCount = 64

type window struct {
	interval time.Duration
	period   time.Duration
}

type shard struct {
	mu   sync.Mutex
	tats map[string][]time.Time
}

// Limiter allows each key Limit requests in any Period of every window, with bursts up to Limit. A nil Limiter or one without windows allows everything.
type Limiter struct {
	clock   clock.Clock
	windows []window
	seed    maphash.Seed
	shards  [shardCount]shard
}

func New(c clock.Clock, windows []config.RateWindow) *Limiter {
	l := &Limiter{clock: c, seed: maphash.MakeSeed()}
	for _, w := range windows {
		l.windows = append(l.windows, window{interval: w.Period / time.Duration(w.Limit), period: w.Period})
	}
	for i := range l.shards {
		l.shards[i].tats = map[string][]time.Time{}
	}
	return l
}

// Allow counts one request for key. When it refuses, nothing is counted and wait is how long until the next request would be allowed.
func (l *Limiter) Allow(key string) (ok bool, wait time.Duration) {
	if l == nil || len(l.windows) == 0 {
		return true, 0
	}
	now := l.clock.Now()
	s := &l.shards[maphash.String(l.seed, key)%shardCount]
	s.mu.Lock()
	defer s.mu.Unlock()

	tats, found := s.tats[key]
	if !found {
		tats = make([]time.Time, len(l.windows))
	}
	for i, w := range l.windows {
		next := later(tats[i], now).Add(w.interval)
		wait = max(wait, next.Sub(now)-w.period)
	}
	if wait > 0 {
		return false, wait
	}
	for i, w := range l.windows {
		tats[i] = later(tats[i], now).Add(w.interval)
	}
	s.tats[key] = tats
	return true, 0
}

// Sweep forgets keys that have fully recovered, which Allow treats the same as unseen keys.
func (l *Limiter) Sweep() int {
	if l == nil {
		return 0
	}
	now := l.clock.Now()
	removed := 0
	for i := range l.shards {
		s := &l.shards[i]
		s.mu.Lock()
		for key, tats := range s.tats {
			if recovered(tats, now) {
				delete(s.tats, key)
				removed++
			}
		}
		s.mu.Unlock()
	}
	return removed
}

// RunSweeper calls Sweep every interval until ctx is done.
func (l *Limiter) RunSweeper(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.Sweep()
		}
	}
}

// ClientKey groups IPv6 clients by /64, the smallest block usually given to one subscriber, so rotating addresses within it does not reset the limit.
func ClientKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is6() {
		return ip
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func recovered(tats []time.Time, now time.Time) bool {
	for _, t := range tats {
		if t.After(now) {
			return false
		}
	}
	return true
}
