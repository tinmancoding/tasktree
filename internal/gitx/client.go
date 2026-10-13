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
	"time"
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
	verboseMu     sync.Mutex
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
	return c.runInput(ctx, dir, env, nil, args...)
}

// RunInput supports atomic ref transactions without a shell or shared files.
func (c Client) RunInput(ctx context.Context, dir string, env map[string]string, input string, args ...string) (string, string, error) {
	return c.runInput(ctx, dir, env, strings.NewReader(input), args...)
}

// cwdExists reports whether the process working directory still exists. On
// some systems Getwd keeps returning the stale path of a deleted directory, so
// the path is also stat'ed.
func cwdExists() bool {
	wd, err := os.Getwd()
	if err != nil {
		return false
	}
	st, err := os.Stat(wd)
	return err == nil && st.IsDir()
}

func (c Client) runInput(ctx context.Context, dir string, env map[string]string, input io.Reader, args ...string) (string, string, error) {
	cfg := c.withConfig()
	execArgs := args
	if command := transportIndex(args); command >= 0 {
		if limit := wallClockLimit(ctx); limit > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, limit)
			defer cancel()
		}
		networkEnv := make(map[string]string, len(env)+2)
		for k, v := range env {
			networkEnv[k] = v
		}
		networkEnv["GIT_TERMINAL_PROMPT"] = "0"
		_, inherited := os.LookupEnv("GIT_SSH_COMMAND")
		_, supplied := env["GIT_SSH_COMMAND"]
		if !inherited && !supplied {
			// Respect repository/global SSH configuration, including an explicit
			// empty command. Otherwise disable interactive SSH authentication and
			// detect stalled connections without bounding total transfer time.
			if _, _, err := c.Run(ctx, dir, env, append(append([]string(nil), args[:command]...), "config", "--get", "core.sshCommand")...); err != nil {
				networkEnv["GIT_SSH_COMMAND"] = SSHStallCommand
			}
		}
		env = networkEnv
		execArgs = make([]string, 0, len(args)+4)
		execArgs = append(execArgs, args[:command]...)
		execArgs = append(execArgs, HTTPStallArgs...)
		execArgs = append(execArgs, args[command:]...)
	}
	if cfg.verboseWriter != nil {
		prefix := "git"
		if dir != "" {
			prefix = "git -C " + quoteArg(dir)
		}
		cfg.verboseMu.Lock()
		_, _ = fmt.Fprintf(cfg.verboseWriter, "%s %s\n", prefix, formatArgs(args))
		cfg.verboseMu.Unlock()
	}
	cmd := exec.CommandContext(ctx, cfg.binary, execArgs...)
	// A killed git can leave an SSH/helper child holding the pipes open.
	// Bound pipe draining as well as the lifetime of the direct process.
	cmd.WaitDelay = 250 * time.Millisecond
	if dir == "" {
		if !cwdExists() {
			// The caller's working directory was deleted (for example by closing
			// the checkout it was in). Every command here names its repository
			// with an absolute path, so any existing directory will do.
			dir = os.TempDir()
		}
	}
	cmd.Dir = dir
	cmd.Env = CleanEnv(env)
	cmd.Stdin = input
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
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

// Commit records a commit using the repository identity when both identity
// fields resolve. When they do not, it supplies the tasktree identity only for
// this invocation; it never writes to the repository's configuration.
func (c Client) commitIdentityArgs(ctx context.Context, dir string, env map[string]string, author, machine string) []string {
	name, _, nameErr := c.Run(ctx, dir, env, "config", "user.name")
	email, _, emailErr := c.Run(ctx, dir, env, "config", "user.email")
	args := []string{"-c", "commit.gpgsign=false"}
	if nameErr != nil || emailErr != nil || strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" {
		if author == "" {
			author = "tasktree"
		}
		if machine == "" {
			machine = "localhost"
		}
		args = append(args, "-c", "user.name="+author, "-c", "user.email="+author+"@"+machine)
	}
	return args
}
func (c Client) Commit(ctx context.Context, dir string, env map[string]string, author, machine, message string) (string, string, error) {
	args := c.commitIdentityArgs(ctx, dir, env, author, machine)
	args = append(args, "commit", "--no-verify", "-m", message)
	return c.Run(ctx, dir, env, args...)
}

// CommitTree creates a plumbing commit with the same identity fallback as Commit.
// commit-tree runs no hooks and commit.gpgsign is disabled for structural writes.
func (c Client) CommitTree(ctx context.Context, dir string, env map[string]string, author, machine, tree, parent, message string) (string, string, error) {
	parents := []string{}
	if parent != "" {
		parents = append(parents, parent)
	}
	return c.CommitTreeParents(ctx, dir, env, author, machine, tree, parents, message)
}

// CommitTreeParents uses the same identity path for two-parent sync merges.
func (c Client) CommitTreeParents(ctx context.Context, dir string, env map[string]string, author, machine, tree string, parents []string, message string) (string, string, error) {
	args := c.commitIdentityArgs(ctx, dir, env, author, machine)
	args = append(args, "commit-tree", tree, "-m", message)
	for _, parent := range parents {
		args = append(args, "-p", parent)
	}
	return c.Run(ctx, dir, env, args...)
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
