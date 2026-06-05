// Package bootstrap executes the ordered, idempotent setup steps declared in
// a workspace's spec.bootstrap. Steps run sequentially and fail-fast after all
// sources are materialized.
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
)

// killGrace is how long a cancelled/timed-out step's process group is given to
// exit after SIGTERM before it is force-killed with SIGKILL.
const killGrace = 5 * time.Second

// Options configures a bootstrap run.
type Options struct {
	// Stderr receives per-step headers and the live (passthrough) stdout/stderr
	// of each step's child process. If nil, os.Stderr is used.
	Stderr io.Writer
}

// PlanStep is a resolved, non-executed view of a bootstrap step, used for
// dry-run reporting.
type PlanStep struct {
	Name          string
	Run           string
	Workdir       string // as declared (relative); empty means workspace root
	ResolvedDir   string // absolute resolved working directory
	WorkdirExists bool
	WorkdirError  error // non-nil when the workdir escapes root or is invalid
}

// ResolveWorkdir resolves a step's workdir against the workspace root, enforces
// the within-root boundary, and reports whether the directory exists. It never
// executes anything; it is shared by the executor (hard error) and dry-run
// (warning).
func ResolveWorkdir(root string, step domain.BootstrapStep) (resolved string, exists bool, err error) {
	base := filepath.Clean(root)
	joined := base
	if step.Workdir != "" {
		joined = filepath.Clean(filepath.Join(base, step.Workdir))
	}

	// Enforce within-root: reject .. escapes and absolute paths outside root.
	rel, relErr := filepath.Rel(base, joined)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return joined, false, domain.WorkdirEscapesRootError{Name: step.Name, Workdir: step.Workdir}
	}

	info, statErr := os.Stat(joined)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return joined, false, nil
		}
		return joined, false, statErr
	}
	if !info.IsDir() {
		return joined, false, nil
	}
	return joined, true, nil
}

// Plan resolves every step for dry-run reporting without executing anything.
func Plan(root string, steps []domain.BootstrapStep) []PlanStep {
	plan := make([]PlanStep, 0, len(steps))
	for _, step := range steps {
		resolved, exists, err := ResolveWorkdir(root, step)
		plan = append(plan, PlanStep{
			Name:          step.Name,
			Run:           step.Run,
			Workdir:       step.Workdir,
			ResolvedDir:   resolved,
			WorkdirExists: exists,
			WorkdirError:  err,
		})
	}
	return plan
}

// Run executes the bootstrap steps in order against the workspace root. It is
// fail-fast: the first non-zero exit aborts and is returned as a
// StepFailedError. No rollback is performed.
func Run(ctx context.Context, root string, steps []domain.BootstrapStep, opts Options) error {
	w := opts.Stderr
	if w == nil {
		w = os.Stderr
	}

	for _, step := range steps {
		dir, exists, err := ResolveWorkdir(root, step)
		if err != nil {
			return err
		}
		if !exists {
			return domain.WorkdirNotFoundError{Name: step.Name, Workdir: step.Workdir, ResolvedDir: dir}
		}

		fmt.Fprintf(w, "==> [%s] %s\n", step.Name, step.Run)

		if err := runStep(ctx, root, dir, step, w); err != nil {
			return err
		}
	}
	return nil
}

func runStep(ctx context.Context, root, dir string, step domain.BootstrapStep, w io.Writer) error {
	// Plain exec.Command (not CommandContext): cancellation is handled manually
	// below so we can signal the whole process group, not just the leader.
	cmd := exec.Command("sh", "-c", step.Run)
	cmd.Dir = dir
	cmd.Env = mergeEnv(os.Environ(), root, step.Env)
	cmd.Stdout = w
	cmd.Stderr = w
	setProcGroup(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("bootstrap step %q: start: %w", step.Name, err)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killGroup(cmd, syscallSIGTERM)
			select {
			case <-done:
			case <-time.After(killGrace):
				killGroup(cmd, syscallSIGKILL)
			}
		case <-done:
		}
	}()

	waitErr := cmd.Wait()
	close(done)

	if ctx.Err() != nil {
		return fmt.Errorf("bootstrap step %q: %w", step.Name, ctx.Err())
	}
	if waitErr != nil {
		exitCode := -1
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return domain.StepFailedError{Name: step.Name, Workdir: step.Workdir, ExitCode: exitCode}
	}
	return nil
}

// mergeEnv builds the child environment: the inherited base, with TASKTREE_ROOT
// injected, and the step's env overlaid last so it wins on key collisions.
func mergeEnv(base []string, root string, stepEnv map[string]string) []string {
	overlay := map[string]string{"TASKTREE_ROOT": root}
	for k, v := range stepEnv {
		overlay[k] = v
	}

	out := make([]string, 0, len(base)+len(overlay))
	for _, kv := range base {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if _, override := overlay[key]; override {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range overlay {
		out = append(out, k+"="+v)
	}
	return out
}
