package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditExplicitProgressingTransportSurvivesTenSeconds(t *testing.T) {
	// A real local ext transport delays upload-pack while reporting progress.
	dir := t.TempDir()
	g := NewClient()
	if _, _, err := g.Run(context.Background(), "", nil, "init", "--bare", filepath.Join(dir, "remote.git")); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(dir, "slow-upload-pack")
	script := "#!/bin/sh\ni=0\nwhile [ $i -lt 11 ]; do echo progressing >&2; sleep 1; i=$((i+1)); done\nexec git upload-pack \"$1\"\n"
	if err := os.WriteFile(helper, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _, err := g.Run(Explicit(context.Background()), "", map[string]string{"GIT_ALLOW_PROTOCOL": "ext:file"}, "clone", "ext::"+helper+" "+filepath.Join(dir, "remote.git"), filepath.Join(dir, "clone"))
	if err != nil {
		t.Fatalf("explicit progressing clone killed after %s: %v", time.Since(start), err)
	}
	if time.Since(start) < 10*time.Second {
		t.Fatal("fixture did not exercise a slow transport")
	}
}

func TestAuditImplicitTransportStillTimesOut(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncase \"$*\" in *config*) exit 1;; esac\nsleep 20\n"), 0755); err != nil {
		t.Fatal(err)
	}
	c := Client{config: &config{binary: binary}}
	start := time.Now()
	_, _, err := c.Run(BestEffort(context.Background(), 10*time.Second), "", nil, "ls-remote", "origin")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") || time.Since(start) > 12*time.Second {
		t.Fatalf("implicit timeout: %v (%s)", err, time.Since(start))
	}
}

func TestAuditHTTPStallOptions(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncase \"$*\" in *config*) exit 1;; esac\nprintf '%s\\n' \"$*\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	c := Client{config: &config{binary: binary}}
	out, _, err := c.Run(Explicit(context.Background()), "", nil, "fetch", "origin")
	if err != nil || !strings.Contains(out, "-c http.lowSpeedLimit=1000 -c http.lowSpeedTime=60 fetch origin") {
		t.Fatalf("stall options: %q %v", out, err)
	}
}
