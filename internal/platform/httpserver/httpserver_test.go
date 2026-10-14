package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
)

func TestRunShutsDownGracefully(t *testing.T) {
	cfg := config.Server{
		Host:            "127.0.0.1",
		Port:            0,
		ShutdownTimeout: time.Second,
		DrainDelay:      10 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	var drained atomic.Bool

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), func() { drained.Store(true) })
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
	if !drained.Load() {
		t.Fatal("beforeShutdown was not called")
	}
}

func TestRunReportsListenError(t *testing.T) {
	cfg := config.Server{Host: "256.0.0.1", Port: 1, ShutdownTimeout: time.Second}
	if err := Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), nil); err == nil {
		t.Fatal("expected listen error")
	}
}
