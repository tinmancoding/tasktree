package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
)

func requireSh(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
}

func TestRunSuccessOrderingAndOutput(t *testing.T) {
	requireSh(t)
	root := t.TempDir()
	var buf bytes.Buffer
	steps := []domain.BootstrapStep{
		{Name: "one", Run: "echo first"},
		{Name: "two", Run: "echo second"},
	}
	if err := Run(context.Background(), root, steps, Options{Stderr: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "==> [one] echo first") || !strings.Contains(out, "==> [two] echo second") {
		t.Fatalf("missing headers: %q", out)
	}
	if i, j := strings.Index(out, "first"), strings.Index(out, "second"); i < 0 || j < 0 || i > j {
		t.Fatalf("steps ran out of order: %q", out)
	}
}

func TestRunEnvOverlayAndTasktreeRoot(t *testing.T) {
	requireSh(t)
	root := t.TempDir()
	t.Setenv("BOOT_BASE", "base")
	var buf bytes.Buffer
	steps := []domain.BootstrapStep{
		{Name: "env", Run: "echo root=$TASKTREE_ROOT base=$BOOT_BASE over=$BOOT_BASE2", Env: map[string]string{"BOOT_BASE2": "win"}},
		{Name: "override", Run: "echo over=$BOOT_BASE", Env: map[string]string{"BOOT_BASE": "win"}},
	}
	if err := Run(context.Background(), root, steps, Options{Stderr: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "root="+root) {
		t.Fatalf("TASKTREE_ROOT not injected: %q", out)
	}
	if !strings.Contains(out, "base=base") {
		t.Fatalf("inherited env missing: %q", out)
	}
	if !strings.Contains(out, "over=win") {
		t.Fatalf("step env did not win: %q", out)
	}
}

func TestRunWorkdir(t *testing.T) {
	requireSh(t)
	root := t.TempDir()
	sub := filepath.Join(root, "api")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	steps := []domain.BootstrapStep{
		{Name: "pwd", Run: "pwd", Workdir: "api"},
	}
	if err := Run(context.Background(), root, steps, Options{Stderr: &buf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// macOS /tmp is a symlink to /private/tmp; compare resolved.
	want, _ := filepath.EvalSymlinks(sub)
	if !strings.Contains(buf.String(), want) && !strings.Contains(buf.String(), sub) {
		t.Fatalf("workdir not honored, want %q in %q", sub, buf.String())
	}
}

func TestRunWorkdirNotFound(t *testing.T) {
	requireSh(t)
	root := t.TempDir()
	err := Run(context.Background(), root, []domain.BootstrapStep{{Name: "x", Run: "echo hi", Workdir: "missing"}}, Options{Stderr: &bytes.Buffer{}})
	var e domain.WorkdirNotFoundError
	if !errors.As(err, &e) {
		t.Fatalf("expected WorkdirNotFoundError, got %v", err)
	}
}

func TestRunWorkdirEscape(t *testing.T) {
	requireSh(t)
	root := t.TempDir()
	err := Run(context.Background(), root, []domain.BootstrapStep{{Name: "x", Run: "echo hi", Workdir: "../escape"}}, Options{Stderr: &bytes.Buffer{}})
	var e domain.WorkdirEscapesRootError
	if !errors.As(err, &e) {
		t.Fatalf("expected WorkdirEscapesRootError, got %v", err)
	}
}

func TestRunFailFast(t *testing.T) {
	requireSh(t)
	root := t.TempDir()
	marker := filepath.Join(root, "third-ran")
	var buf bytes.Buffer
	steps := []domain.BootstrapStep{
		{Name: "ok", Run: "true"},
		{Name: "boom", Run: "exit 7"},
		{Name: "never", Run: "touch " + marker},
	}
	err := Run(context.Background(), root, steps, Options{Stderr: &buf})
	var sfe domain.StepFailedError
	if !errors.As(err, &sfe) {
		t.Fatalf("expected StepFailedError, got %v", err)
	}
	if sfe.Name != "boom" || sfe.ExitCode != 7 {
		t.Fatalf("wrong failure details: %+v", sfe)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatalf("step after failure should not have run")
	}
}

func TestRunCancelKillsChild(t *testing.T) {
	requireSh(t)
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := Run(ctx, root, []domain.BootstrapStep{{Name: "sleep", Run: "sleep 30"}}, Options{Stderr: &bytes.Buffer{}})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("cancellation too slow: %v", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestPlan(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := Plan(root, []domain.BootstrapStep{
		{Name: "a", Run: "x", Workdir: "api"},
		{Name: "b", Run: "y", Workdir: "missing"},
		{Name: "c", Run: "z", Workdir: "../escape"},
	})
	if len(plan) != 3 {
		t.Fatalf("expected 3 plan steps, got %d", len(plan))
	}
	if !plan[0].WorkdirExists {
		t.Fatalf("api should exist")
	}
	if plan[1].WorkdirExists || plan[1].WorkdirError != nil {
		t.Fatalf("missing should not exist, no error: %+v", plan[1])
	}
	if plan[2].WorkdirError == nil {
		t.Fatalf("escape should report error")
	}
}
