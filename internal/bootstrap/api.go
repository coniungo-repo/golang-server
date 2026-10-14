package bootstrap

import (
	"context"

	"github.com/coniungo-repo/golang-server.git/internal/platform/config"
	"github.com/coniungo-repo/golang-server.git/internal/platform/httpserver"
)

func RunAPI(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	deps, err := NewDependencies(ctx, cfg)
	if err != nil {
		return err
	}

	defer deps.close()

	return httpserver.Run(ctx, cfg.Server, newRouter(deps))
}
