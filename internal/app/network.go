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

// bestEffortWindow is the wall clock for implicit best-effort calls (D59). A
// shorter configured sync.timeout wins.
const bestEffortWindow = 10 * time.Second

func (a App) implicitNetworkContext(ctx context.Context) (context.Context, error) {
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return ctx, err
	}
	timeout, err := networkTimeout(cfg.Sync.Timeout, bestEffortWindow)
	if err != nil {
		return ctx, err
	}
	if timeout > bestEffortWindow {
		timeout = bestEffortWindow
	}
	return gitx.BestEffort(ctx, timeout), nil
}

// explicitNetworkContext marks ctx as an explicit transfer: stall detection and
// no wall clock unless sync.timeout is set (D59).
func (a App) explicitNetworkContext(ctx context.Context) (context.Context, error) {
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return ctx, err
	}
	if cfg.Sync.Timeout == "" {
		return gitx.Explicit(ctx), nil
	}
	timeout, err := networkTimeout(cfg.Sync.Timeout, 0)
	if err != nil {
		return ctx, err
	}
	return gitx.ExplicitLimit(ctx, timeout), nil
}
