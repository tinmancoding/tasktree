package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
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
