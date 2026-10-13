package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNetworkNoninteractiveAndSSHOverrides(t *testing.T) {
	// The fake transport is local only; inspect the exact child environment.
	binary := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\ncase \"$*\" in *config*) if [ -n \"$TEST_CORE_SSH\" ]; then echo \"$TEST_CORE_SSH\"; exit 0; fi; exit 1;; esac\nprintf '%s\\n%s\\n' \"$GIT_TERMINAL_PROMPT\" \"$GIT_SSH_COMMAND\"\n"
	if e := os.WriteFile(binary, []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	c := Client{config: &config{binary: binary}}
	// Clear any inherited override only within the test's process environment.
	old, present := os.LookupEnv("GIT_SSH_COMMAND")
	os.Unsetenv("GIT_SSH_COMMAND")
	t.Cleanup(func() {
		if present {
			os.Setenv("GIT_SSH_COMMAND", old)
		} else {
			os.Unsetenv("GIT_SSH_COMMAND")
		}
	})
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("TEST_CORE_SSH", "")
	out, _, e := c.Run(context.Background(), "", nil, "fetch", "origin")
	if e != nil || out != "0\n"+SSHStallCommand+"\n" {
		t.Fatalf("defaults=%q %v", out, e)
	}
	t.Setenv("GIT_SSH_COMMAND", "custom-ssh")
	out, _, e = c.Run(context.Background(), "", nil, "push", "origin")
	if e != nil || out != "0\ncustom-ssh\n" {
		t.Fatalf("override=%q %v", out, e)
	}
	os.Unsetenv("GIT_SSH_COMMAND")
	t.Setenv("TEST_CORE_SSH", "configured-ssh")
	out, _, e = c.Run(context.Background(), "", nil, "-C", "repo", "ls-remote", "origin")
	if e != nil || out != "0\n\n" {
		t.Fatalf("core.sshCommand replaced=%q %v", out, e)
	}
}
func TestNetworkConfiguredTimeout(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\ncase \"$*\" in *config*) exit 1;; esac\nsleep 2\n"
	if e := os.WriteFile(binary, []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	c := Client{config: &config{binary: binary}}
	start := time.Now()
	_, _, e := c.Run(BestEffort(context.Background(), 50*time.Millisecond), "", nil, "fetch", "origin")
	if e == nil || !strings.Contains(e.Error(), "deadline exceeded") || time.Since(start) > time.Second {
		t.Fatalf("deadline=%v took %s", e, time.Since(start))
	}
}
