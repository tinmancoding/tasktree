package gitx

import (
	"context"
	"fmt"
	"time"
)

// Network policy (D59). Transport commands (fetch, push, clone, ls-remote)
// never prompt and always use stall detection; the policy only decides whether
// a wall-clock limit also applies.
//
//   - Explicit: a user-requested transfer. No wall clock by default, because a
//     healthy large transfer must not be killed. An optional limit may be given.
//   - BestEffort: an implicit call whose failure is tolerated. A wall clock is
//     mandatory.
//
// Call sites choose one. A context with no policy behaves as Explicit without a
// limit, which is what purely local pushes (to the local hub) rely on.

// SSHStallCommand is the ssh command used when the user configured none.
const SSHStallCommand = "ssh -o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=4"

// HTTPStallArgs abort an HTTP transfer slower than 1000 B/s for 60s.
var HTTPStallArgs = []string{"-c", "http.lowSpeedLimit=1000", "-c", "http.lowSpeedTime=60"}

type networkTimeoutKey struct{}

// Explicit marks transport commands run with ctx as explicit transfers: stall
// detection only, no wall-clock limit.
func Explicit(ctx context.Context) context.Context {
	return context.WithValue(ctx, networkTimeoutKey{}, time.Duration(0))
}

// ExplicitLimit is Explicit with an optional per-command wall clock (for
// example sync.timeout). A limit <= 0 means no limit.
func ExplicitLimit(ctx context.Context, limit time.Duration) context.Context {
	if limit < 0 {
		limit = 0
	}
	return context.WithValue(ctx, networkTimeoutKey{}, limit)
}

// BestEffort bounds each transport command run with ctx by d (not the whole
// operation), so slow unrelated repositories do not consume one another's
// budgets. d must be positive.
func BestEffort(ctx context.Context, d time.Duration) context.Context {
	if d <= 0 {
		d = 10 * time.Second
	}
	return context.WithValue(ctx, networkTimeoutKey{}, d)
}

func wallClockLimit(ctx context.Context) time.Duration {
	limit, _ := ctx.Value(networkTimeoutKey{}).(time.Duration)
	return limit
}

// sshCommandProbe builds the git arguments that read the effective
// core.sshCommand for the transport command at args[command]. Each positional
// argument after the command is offered as a remote URL so that
// includeIf "hasconfig:remote.*.url:..." sections match the target before any
// remote exists (for example on clone). Arguments that are not URLs, such as a
// branch name or destination path, cannot match a URL pattern.
func sshCommandProbe(args []string, command int) []string {
	probe := append([]string(nil), args[:command]...)
	n := 0
	for _, arg := range args[command+1:] {
		if arg == "" || arg[0] == '-' {
			continue
		}
		probe = append(probe, "-c", fmt.Sprintf("remote.tasktree-probe-%d.url=%s", n, arg))
		n++
	}
	return append(probe, "config", "--get", "core.sshCommand")
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
