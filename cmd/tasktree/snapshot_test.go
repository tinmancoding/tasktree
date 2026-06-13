package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/testutil"
)

func TestCLISnapshotRestoreFlow(t *testing.T) {
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	workspace := t.TempDir()
	tasktreeRoot := filepath.Join(workspace, "feature")
	t.Setenv("HOME", t.TempDir())

	testutil.RunTasktree(t, workspace, "init", tasktreeRoot)
	testutil.RunTasktree(t, tasktreeRoot, "add", remoteURL, "--branch", "main", "--name", "app")
	testutil.RunTasktree(t, tasktreeRoot, "apply")

	// Dirty edit.
	appPath := filepath.Join(tasktreeRoot, "app")
	if err := os.WriteFile(filepath.Join(appPath, "README.md"), []byte("seed\ncli dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// snapshot -o <file>
	snapPath := filepath.Join(workspace, "snap.tar.gz")
	out := testutil.RunTasktree(t, tasktreeRoot, "snapshot", "-o", snapPath)
	if !strings.Contains(out, "Wrote snapshot to") {
		t.Fatalf("unexpected snapshot output: %q", out)
	}
	if _, err := os.Stat(snapPath); err != nil {
		t.Fatalf("snapshot file not written: %v", err)
	}

	// restore <file> --into <dir>
	restoreInto := filepath.Join(workspace, "restored")
	rout := testutil.RunTasktree(t, workspace, "restore", snapPath, "--into", restoreInto, "--skip-bootstrap")
	if !strings.Contains(rout, "Restored") {
		t.Fatalf("unexpected restore output: %q", rout)
	}
	b, err := os.ReadFile(filepath.Join(restoreInto, "app", "README.md"))
	if err != nil || string(b) != "seed\ncli dirty\n" {
		t.Fatalf("dirty edit not restored: %v / %q", err, string(b))
	}
}

func TestCLISnapshotRestoreStdinStdout(t *testing.T) {
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	workspace := t.TempDir()
	tasktreeRoot := filepath.Join(workspace, "feature")
	t.Setenv("HOME", t.TempDir())

	testutil.RunTasktree(t, workspace, "init", tasktreeRoot)
	testutil.RunTasktree(t, tasktreeRoot, "add", remoteURL, "--branch", "main", "--name", "app")
	testutil.RunTasktree(t, tasktreeRoot, "apply")

	bin := testutil.BuildBinary(t)

	// snapshot -o -  (stdout) -> file
	snapPath := filepath.Join(workspace, "piped.tar.gz")
	snapFile, err := os.Create(snapPath)
	if err != nil {
		t.Fatal(err)
	}
	snapCmd := exec.Command(bin, "snapshot", "-o", "-")
	snapCmd.Dir = tasktreeRoot
	snapCmd.Env = os.Environ()
	snapCmd.Stdout = snapFile
	var snapErr strings.Builder
	snapCmd.Stderr = &snapErr
	if err := snapCmd.Run(); err != nil {
		t.Fatalf("snapshot -o - failed: %v\n%s", err, snapErr.String())
	}
	_ = snapFile.Close()

	// restore -  (stdin)
	restoreInto := filepath.Join(workspace, "restored")
	in, err := os.Open(snapPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	restoreCmd := exec.Command(bin, "restore", "-", "--into", restoreInto, "--skip-bootstrap")
	restoreCmd.Dir = workspace
	restoreCmd.Env = os.Environ()
	restoreCmd.Stdin = in
	var rout, rerr strings.Builder
	restoreCmd.Stdout = &rout
	restoreCmd.Stderr = &rerr
	if err := restoreCmd.Run(); err != nil {
		t.Fatalf("restore - failed: %v\n%s", err, rerr.String())
	}
	if !strings.Contains(rout.String(), "Restored") {
		t.Fatalf("unexpected restore output: %q", rout.String())
	}
	if _, err := os.Stat(filepath.Join(restoreInto, "app", "README.md")); err != nil {
		t.Fatalf("restore via stdin did not produce workspace: %v", err)
	}
}
