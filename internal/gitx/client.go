package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	binary string
}

func NewClient() Client {
	return Client{binary: "git"}
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

func (c Client) run(ctx context.Context, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, c.binary, args...)
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
