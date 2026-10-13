package gitx

import (
	"context"
	"time"
)

type networkTimeoutKey struct{}

// WithNetworkTimeout bounds each transport command, not the whole sync: slow
// unrelated repositories do not consume one another's fetch budgets.
func WithNetworkTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, networkTimeoutKey{}, timeout)
}
func transportIndex(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-C", "-c", "--git-dir", "--work-tree":
			i++
			continue
		case "fetch", "push", "clone", "ls-remote":
			return i
		}
		if len(arg) > 0 && arg[0] == '-' {
			continue
		}
		return -1
	}
	return -1
}
