package health

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/respond"
	"github.com/coniungo-repo/golang-server/internal/platform/route"
)

const pingTimeout = 2 * time.Second

type Pinger interface {
	Ping(ctx context.Context) error
}

// Probe serves liveness and readiness. Readiness fails once Drain is called, so load balancers stop routing before shutdown.
type Probe struct {
	db       Pinger
	draining atomic.Bool
}

func New(db Pinger) *Probe {
	return &Probe{db: db}
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

	ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
	defer cancel()

	if err := p.db.Ping(ctx); err != nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "Database is unreachable")
		return
	}
	respond.JSON(w, r, http.StatusOK, status{Status: "ok"})
}

type status struct {
	Status string `json:"status"`
}
