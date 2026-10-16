package health

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/platform/route"
)

const pingTTL = time.Second

type Pinger interface {
	Ping(ctx context.Context) error
}

// Probe serves liveness and readiness. Readiness fails once Drain is called, so load balancers stop routing before shutdown. The database ping is shared for pingTTL, so however often /readyz is called it holds at most one pool connection.
type Probe struct {
	db          Pinger
	pingTimeout time.Duration
	now         func() time.Time
	draining    atomic.Bool

	mu       sync.Mutex
	pingedAt time.Time
	pingErr  error
}

// New returns a Probe whose database ping gives up after pingTimeout.
func New(db Pinger, pingTimeout time.Duration) *Probe {
	return &Probe{db: db, pingTimeout: pingTimeout, now: time.Now}
}

func (p *Probe) Register(mux route.Mux) {
	mux.Handle("GET /livez", http.HandlerFunc(p.live))
	mux.Handle("GET /readyz", http.HandlerFunc(p.ready))
}

func (p *Probe) Drain() {
	p.draining.Store(true)
}

func (p *Probe) live(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, r, http.StatusOK, status{Status: "ok"})
}

func (p *Probe) ready(w http.ResponseWriter, r *http.Request) {
	if p.draining.Load() {
		respond.Error(w, r, http.StatusServiceUnavailable, "Shutting down")
		return
	}

	if err := p.ping(r.Context()); err != nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "Database is unreachable")
		return
	}
	respond.JSON(w, r, http.StatusOK, status{Status: "ok"})
}

type status struct {
	Status string `json:"status"`
}

func (p *Probe) ping(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.pingedAt.IsZero() && p.now().Sub(p.pingedAt) < pingTTL {
		return p.pingErr
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.pingTimeout)
	defer cancel()
	p.pingErr = p.db.Ping(ctx)
	p.pingedAt = p.now()
	return p.pingErr
}
