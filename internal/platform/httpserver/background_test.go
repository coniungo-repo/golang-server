package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

func TestServeBackground(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := ServeBackground(ctx, "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}), log)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get("http://" + addr.String() + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("got %q", body)
	}

	if _, err := ServeBackground(ctx, "not-an-address", http.NotFoundHandler(), log); err == nil {
		t.Fatal("a bad address must fail startup")
	}
}
