package gitx

import (
	"context"
	"os"
	"os/exec"
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

// A core.sshCommand set by includeIf "hasconfig:remote.*.url:..." must be
// respected on clone, before any remote exists in a repository.
func TestNetworkRespectsHasconfigSSHCommandOnClone(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	include := filepath.Join(dir, "org.gitconfig")
	if e := os.WriteFile(include, []byte("[core]\n\tsshCommand = org-ssh\n"), 0644); e != nil {
		t.Fatal(e)
	}
	global := filepath.Join(dir, "global.gitconfig")
	if e := os.WriteFile(global, []byte("[includeIf \"hasconfig:remote.*.url:git@example.com:Org/**\"]\n\tpath = "+include+"\n"), 0644); e != nil {
		t.Fatal(e)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// Real git answers the config probe; the transport prints the ssh override.
	binary := filepath.Join(dir, "git")
	script := "#!/bin/sh\ncase \"$*\" in *'config --get core.sshCommand') exec '" + realGit + "' \"$@\";; esac\nprintf '%s' \"$GIT_SSH_COMMAND\"\n"
	if e := os.WriteFile(binary, []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	old, present := os.LookupEnv("GIT_SSH_COMMAND")
	os.Unsetenv("GIT_SSH_COMMAND")
	t.Cleanup(func() {
		if present {
			os.Setenv("GIT_SSH_COMMAND", old)
		}
	})
	c := Client{config: &config{binary: binary}}
	dest := filepath.Join(dir, "dest.git")
	out, _, e := c.Run(context.Background(), "", nil, "clone", "--bare", "git@example.com:Org/repo.git", dest)
	if e != nil || out != "" {
		t.Fatalf("matching clone GIT_SSH_COMMAND=%q %v", out, e)
	}
	out, _, e = c.Run(context.Background(), "", nil, "clone", "--branch", "main", "git@example.com:Other/repo.git", dest)
	if e != nil || out != SSHStallCommand {
		t.Fatalf("non-matching clone GIT_SSH_COMMAND=%q %v", out, e)
	}
}
