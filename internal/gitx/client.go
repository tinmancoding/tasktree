package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type Client struct {
	config *config
}

func NewClient() Client {
	return Client{config: &config{binary: "git"}}
}

type config struct {
	binary        string
	verboseWriter io.Writer
}

type CommandError struct {
	Args     []string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (e CommandError) Error() string {
	parts := []string{fmt.Sprintf("git %s", strings.Join(e.Args, " "))}
	if e.Stderr != "" {
		parts = append(parts, strings.TrimSpace(e.Stderr))
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ": ")
}

func (e CommandError) Unwrap() error {
	return e.Err
}

func (c Client) withConfig() *config {
	if c.config == nil {
		c.config = &config{binary: "git"}
	}
	if c.config.binary == "" {
		c.config.binary = "git"
	}
	return c.config
}

func (c Client) SetVerboseWriter(w io.Writer) {
	cfg := c.withConfig()
	cfg.verboseWriter = w
}

func (c Client) WithDefaults() Client {
	c.withConfig()
	return c
}

// Run executes git with an optional working directory and per-invocation
// environment. Inherited repository-context variables are removed before env
// is applied; transport and authentication variables are preserved.
func (c Client) Run(ctx context.Context, dir string, env map[string]string, args ...string) (string, string, error) {
	cfg := c.withConfig()
	if cfg.verboseWriter != nil {
		prefix := "git"
		if dir != "" {
			prefix = "git -C " + quoteArg(dir)
		}
		_, _ = fmt.Fprintf(cfg.verboseWriter, "%s %s\n", prefix, formatArgs(args))
	}
	cmd := exec.CommandContext(ctx, cfg.binary, args...)
	cmd.Dir = dir
	cmd.Env = CleanEnv(env)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return stdout.String(), stderr.String(), CommandError{
			Args:     append([]string(nil), args...),
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      err,
		}
	}
	return stdout.String(), stderr.String(), nil
}

func (c Client) run(ctx context.Context, args ...string) (string, string, error) {
	return c.Run(ctx, "", nil, args...)
}

var localEnvOnce sync.Once
var localEnvNames map[string]bool

func repositoryEnvNames() map[string]bool {
	localEnvOnce.Do(func() {
		localEnvNames = map[string]bool{}
		// Keep this fallback aligned with Git's common repository-local variables.
		fallback := []string{"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_OBJECT_DIRECTORY", "GIT_DIR", "GIT_WORK_TREE", "GIT_IMPLICIT_WORK_TREE", "GIT_GRAFT_FILE", "GIT_INDEX_FILE", "GIT_NO_REPLACE_OBJECTS", "GIT_REPLACE_REF_BASE", "GIT_PREFIX", "GIT_INTERNAL_SUPER_PREFIX", "GIT_SHALLOW_FILE", "GIT_COMMON_DIR"}
		for _, n := range fallback {
			localEnvNames[n] = true
		}
		cmd := exec.Command("git", "rev-parse", "--local-env-vars")
		cmd.Env = os.Environ()
		if out, err := cmd.Output(); err == nil {
			localEnvNames = map[string]bool{}
			for _, n := range strings.Fields(string(out)) {
				localEnvNames[n] = true
			}
		}
	})
	return localEnvNames
}

// CleanEnv returns the process environment with repository-context GIT_* variables
// removed and the supplied overlay applied. Transport/authentication variables remain.
func CleanEnv(extra map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(extra))
	strip := repositoryEnvNames()
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if strip[name] {
			continue
		}
		if _, replaced := extra[name]; replaced {
			continue
		}
		env = append(env, item)
	}
	for name, value := range extra {
		env = append(env, name+"="+value)
	}
	return env
}

func trimOutput(stdout string) string {
	return strings.TrimSpace(stdout)
}

func formatArgs(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = quoteArg(arg)
	}
	return strings.Join(parts, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if strings.ContainsAny(arg, " \t\n\"'\\") {
		return strconv.Quote(arg)
	}
	return arg
}
