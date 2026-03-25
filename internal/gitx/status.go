package gitx

import (
	"context"
	"strings"
)

func (c Client) CurrentBranch(ctx context.Context, repoPath string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		return trimOutput(stdout), nil
	}
	if cmdErr, ok := err.(CommandError); ok && cmdErr.ExitCode == 1 {
		return "", nil
	}
	return "", err
}

func (c Client) HeadDescription(ctx context.Context, repoPath string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "describe", "--tags", "--exact-match", "HEAD")
	if err == nil {
		return trimOutput(stdout), nil
	}
	if cmdErr, ok := err.(CommandError); ok && cmdErr.ExitCode == 128 {
		sha, err := c.CommitSHA(ctx, repoPath)
		if err != nil {
			return "", err
		}
		if len(sha) > 12 {
			sha = sha[:12]
		}
		return sha, nil
	}
	return "", err
}

func (c Client) IsDirty(ctx context.Context, repoPath string) (bool, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(stdout) != "", nil
}
