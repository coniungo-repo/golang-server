package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Policy caps attempts per value. Kind namespaces the fingerprints, so the same value under two policies never collides.
type Policy struct {
	Kind    string
	Windows []config.RateWindow
}

type Attempt struct {
	Policy Policy
	Value  string
}

const (
	kindDestination = "destination"
	kindClientIP    = "client_ip"
	kindSMSTotal    = "sms_total"
)

// Limiter counts sign-in code sends in Postgres, keyed by an HMAC so raw emails, phones and IPs are never stored.
type Limiter struct {
	db        *pgxpool.Pool
	secret    []byte
	clock     clock.Clock
	retention time.Duration
}

// NewLimiter keeps attempts for retention, which must cover the longest window of any policy used with it.
func NewLimiter(db *pgxpool.Pool, secret string, c clock.Clock, retention time.Duration) *Limiter {
	return &Limiter{db: db, secret: []byte(secret), clock: c, retention: retention}
}

// Allow records the attempts and returns true only if every one is under all of its windows. Nothing is recorded when it returns false.
func (l *Limiter) Allow(ctx context.Context, attempts ...Attempt) (bool, error) {
	type keyed struct {
		Attempt
		fingerprint []byte
	}
	ks := make([]keyed, len(attempts))
	for i, a := range attempts {
		ks[i] = keyed{a, l.fingerprint(a.Policy.Kind, a.Value)}
	}
	slices.SortFunc(ks, func(a, b keyed) int { return bytes.Compare(a.fingerprint, b.fingerprint) })

	now := l.clock.Now()
	allowed := true
	err := pgx.BeginFunc(ctx, l.db, func(tx pgx.Tx) error {
		q := store.New(tx)
		for _, k := range ks {
			if err := q.LockSignInFingerprint(ctx, lockKey(k.fingerprint)); err != nil {
				return err
			}
		}
		for _, k := range ks {
			for _, w := range k.Policy.Windows {
				n, err := q.CountSignInAttemptsSince(ctx, store.CountSignInAttemptsSinceParams{
					Kind: k.Policy.Kind, Fingerprint: k.fingerprint, Since: now.Add(-w.Period),
				})
				if err != nil {
					return err
				}
				if n >= w.Limit {
					allowed = false
					return nil
				}
			}
		}
		for _, k := range ks {
			if err := q.InsertSignInAttempt(ctx, store.InsertSignInAttemptParams{
				Kind: k.Policy.Kind, Fingerprint: k.fingerprint, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("auth: rate limit check: %w", err)
	}
	return allowed, nil
}

// Prune deletes attempts older than the longest window.
func (l *Limiter) Prune(ctx context.Context) (int64, error) {
	return store.New(l.db).DeleteSignInAttemptsBefore(ctx, l.clock.Now().Add(-l.retention))
}

// RunPruner calls Prune every interval until ctx is done.
func (l *Limiter) RunPruner(ctx context.Context, interval time.Duration, onError func(error)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := l.Prune(ctx); err != nil && ctx.Err() == nil {
				onError(err)
			}
		}
	}
}

func (l *Limiter) fingerprint(kind, value string) []byte {
	m := hmac.New(sha256.New, l.secret)
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(value))
	return m.Sum(nil)
}

func lockKey(fingerprint []byte) int64 {
	return int64(binary.BigEndian.Uint64(fingerprint[:8]))
}
