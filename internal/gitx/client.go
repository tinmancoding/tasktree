package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
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

func (c Client) run(ctx context.Context, args ...string) (string, string, error) {
	cfg := c.withConfig()
	if cfg.verboseWriter != nil {
		_, _ = fmt.Fprintf(cfg.verboseWriter, "git %s\n", formatArgs(args))
	}
	cmd := exec.CommandContext(ctx, cfg.binary, args...)
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
