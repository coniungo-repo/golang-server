// Package clock is an injectable time source, so code that depends on the time (rate limits, session ages, sign-in windows) can be tested with a Mock it moves by hand.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

type system struct{}

func (system) Now() time.Time                  { return time.Now() }
func (system) Since(t time.Time) time.Duration { return time.Since(t) }

func New() Clock { return system{} }

// Mock is a manually advanced Clock for tests. It is safe for concurrent use.
type Mock struct {
	mu sync.Mutex
	t  time.Time
}

func NewMock(t time.Time) *Mock { return &Mock{t: t} }

func (m *Mock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.t
}

func (m *Mock) Since(t time.Time) time.Duration { return m.Now().Sub(t) }

func (m *Mock) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.t = t
}

func (m *Mock) Add(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.t = m.t.Add(d)
}
