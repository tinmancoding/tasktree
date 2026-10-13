package app

import (
	"context"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/gitx"
)

func networkTimeout(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		return 0, ValidationError{"sync.timeout must be a positive duration (for example 60s)"}
	}
	return timeout, nil
}
func (a App) implicitNetworkContext(ctx context.Context) (context.Context, error) {
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return ctx, err
	}
	timeout, err := networkTimeout(cfg.Sync.Timeout, 10*time.Second)
	if err != nil {
		return ctx, err
	}
	return gitx.WithNetworkTimeout(ctx, timeout), nil
}
