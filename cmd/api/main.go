// Command api runs the HTTP API: it loads configuration, connects its dependencies and serves until SIGINT or SIGTERM, then drains and shuts down. The work happens in internal/bootstrap.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/coniungo-repo/golang-server/internal/bootstrap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()

	if err := bootstrap.RunAPI(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}
