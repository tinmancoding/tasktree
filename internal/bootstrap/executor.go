// Package bootstrap executes tasktree bootstrap steps sequentially and fail-fast.
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

	"github.com/tinmancoding/tasktree/v2/internal/gitx"
)

const killGrace = 5 * time.Second

type Step struct {
	Name    string            `yaml:"name"`
	Run     string            `yaml:"run"`
	Workdir string            `yaml:"workdir,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

type StepFailedError struct {
	Name, Workdir string
	ExitCode      int
}

func (e StepFailedError) Error() string {
	return fmt.Sprintf("bootstrap step %q failed with exit code %d", e.Name, e.ExitCode)
}

type WorkdirError struct{ Name, Workdir string }

func (e WorkdirError) Error() string {
	return fmt.Sprintf("bootstrap step %q workdir %q must be an existing directory within the tasktree root", e.Name, e.Workdir)
}

func ResolveWorkdir(root string, step Step) (string, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Clean(filepath.Join(base, step.Workdir))
	if filepath.IsAbs(step.Workdir) {
		joined = filepath.Clean(step.Workdir)
	}
	rel, err := filepath.Rel(base, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return joined, WorkdirError{step.Name, step.Workdir}
	}
	st, err := os.Stat(joined)
	if err != nil || !st.IsDir() {
		return joined, WorkdirError{step.Name, step.Workdir}
	}
	return joined, nil
}

func Run(ctx context.Context, root string, steps []Step, w io.Writer) error {
	if w == nil {
		w = os.Stderr
	}
	seen := map[string]bool{}
	for _, step := range steps {
		if step.Name == "" || step.Run == "" { return fmt.Errorf("bootstrap steps require name and run") }
		if seen[step.Name] { return fmt.Errorf("bootstrap step names must be unique: %q", step.Name) }
		seen[step.Name] = true
	}
	for _, step := range steps {
		dir, err := ResolveWorkdir(root, step)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "==> [%s] %s\n", step.Name, step.Run)
		cmd := exec.Command("sh", "-c", step.Run)
		cmd.Dir, cmd.Stdout, cmd.Stderr = dir, w, w
		extra := map[string]string{"TASKTREE_ROOT": root}
		for k, v := range step.Env {
			extra[k] = v
		}
		cmd.Env = gitx.CleanEnv(extra)
		setProcGroup(cmd)
		if err = cmd.Start(); err != nil {
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
		err = cmd.Wait()
		close(done)
		if ctx.Err() != nil {
			return fmt.Errorf("bootstrap step %q: %w", step.Name, ctx.Err())
		}
		if err != nil {
			code := -1
			if e, ok := err.(*exec.ExitError); ok {
				code = e.ExitCode()
			}
			return StepFailedError{step.Name, step.Workdir, code}
		}
	}
	return nil
}
