package clock

import "time"

type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

type system struct{}

func (system) Now() time.Time                  { return time.Now() }
func (system) Since(t time.Time) time.Duration { return time.Since(t) }

func New() Clock {
	return system{}
}

type Mock struct {
	t time.Time
}

func (m *Mock) Now() time.Time {
	return m.t
}

func (m *Mock) Since(t time.Time) time.Duration {
	return m.t.Sub(t)
}

func (m *Mock) Set(t time.Time) {
	m.t = t
}

func (m *Mock) Add(d time.Duration) {
	m.t = m.t.Add(d)
}

func NewMock(t time.Time) *Mock {
	return &Mock{t: t}
}
