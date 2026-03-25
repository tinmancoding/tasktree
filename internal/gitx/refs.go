package gitx

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinmancoding/tasktree/internal/domain"
)

func (c Client) CloneBare(ctx context.Context, repoURL, destPath string) error {
	_, _, err := c.run(ctx, "clone", "--bare", repoURL, destPath)
	return err
}

func (c Client) FetchAllPrune(ctx context.Context, repoPath string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "fetch", "origin", "--prune", "+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*")
	return err
}

func (c Client) Clone(ctx context.Context, repoPath, destPath string) error {
	_, _, err := c.run(ctx, "clone", repoPath, destPath)
	return err
}

func (c Client) RemoteSetURL(ctx context.Context, repoPath, remoteName, repoURL string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "remote", "set-url", remoteName, repoURL)
	return err
}

func (c Client) DefaultBranch(ctx context.Context, repoPath string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	branch := trimOutput(stdout)
	branch = strings.TrimPrefix(branch, "origin/")
	return branch, nil
}

func (c Client) Checkout(ctx context.Context, repoPath, ref string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "checkout", ref)
	return err
}

func (c Client) CreateBranch(ctx context.Context, repoPath, branch string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "checkout", "-b", branch)
	return err
}

func (c Client) ValidateBranchName(ctx context.Context, branch string) error {
	_, _, err := c.run(ctx, "check-ref-format", "--branch", branch)
	if err == nil {
		return nil
	}
	if cmdErr, ok := err.(CommandError); ok && cmdErr.ExitCode != 0 {
		return domain.InvalidBranchNameError{Name: branch}
	}
	return err
}

func (c Client) BranchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	_, _, err := c.run(ctx, "-C", repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
	if err == nil {
		return true, nil
	}
	if cmdErr, ok := err.(CommandError); ok && cmdErr.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

func (c Client) CommitSHA(ctx context.Context, repoPath string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return trimOutput(stdout), nil
}

func (c Client) CurrentFullRef(ctx context.Context, repoPath string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "symbolic-ref", "-q", "HEAD")
	if err == nil {
		return trimOutput(stdout), nil
	}
	if cmdErr, ok := err.(CommandError); ok && cmdErr.ExitCode == 1 {
		return "", nil
	}
	return "", err
}

func (c Client) ResolveFullRef(ctx context.Context, repoPath, ref string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "rev-parse", "--symbolic-full-name", ref)
	if err != nil {
		return "", err
	}
	resolved := trimOutput(stdout)
	if strings.HasPrefix(resolved, "refs/") {
		return resolved, nil
	}
	return "", nil
}

func (c Client) ResolveCommit(ctx context.Context, repoPath, ref string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "rev-parse", fmt.Sprintf("%s^{commit}", ref))
	if err != nil {
		return "", err
	}
	return trimOutput(stdout), nil
}
