package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunFailFastAndWorkdirBoundary(t *testing.T) {
	root := t.TempDir()
	steps := []Step{{Name: "fail", Run: "exit 7"}, {Name: "later", Run: "touch later"}}
	var out bytes.Buffer
	err := Run(context.Background(), root, steps, &out)
	var failed StepFailedError
	if !errors.As(err, &failed) || failed.ExitCode != 7 {
		t.Fatalf("error=%v", err)
	}
	if _, err = os.Stat(filepath.Join(root, "later")); !os.IsNotExist(err) {
		t.Fatalf("later step ran: %v", err)
	}
	if err = Run(context.Background(), root, []Step{{Name: "escape", Run: "true", Workdir: ".."}}, &out); err == nil {
		t.Fatal("escaping workdir accepted")
	}
}

func TestRunRejectsDuplicateNames(t *testing.T) {
	err := Run(context.Background(), t.TempDir(), []Step{{Name: "same", Run: "true"}, {Name: "same", Run: "true"}}, nil)
	if err == nil {
		t.Fatal("duplicate names accepted")
	}
}

func TestRunCancellationKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are Unix-specific")
	}
	root := t.TempDir()
	pidFile := filepath.Join(root, "pid")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, root, []Step{{Name: "long", Run: "sleep 60 & echo $! > pid; wait"}}, nil) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("step did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("canceled step did not exit")
	}
}

func TestResolveWorkdirRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveWorkdir(root, Step{Name: "escape", Workdir: "escape"}); err == nil {
		t.Fatal("symlink escape accepted")
	}
}
