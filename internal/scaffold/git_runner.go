package scaffold

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitInvocationTimeout caps each git subprocess at this duration. Prevents a
// credential helper waiting on stdin (or a network stall) from hanging the
// migrate command indefinitely.
const GitInvocationTimeout = 60 * time.Second

// GitRunner abstracts shelling out to git so MigrateRemote is testable
// without a real git binary or network. The Run method receives a context
// (already wrapped with GitInvocationTimeout by the caller) and the git
// arguments; it returns stdout, stderr, and any error.
type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout, stderr []byte, err error)
}

// realGitRunner shells out to the system git binary.
type realGitRunner struct{}

// NewRealGitRunner returns a GitRunner that invokes `git` from PATH.
func NewRealGitRunner() GitRunner {
	return &realGitRunner{}
}

func (r *realGitRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("git %s timed out after %s (a credential prompt on stdin can cause this — configure non-interactive auth)",
			strings.Join(args, " "), GitInvocationTimeout)
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

// runGit is a thin convenience that wraps timeout enforcement so callers
// don't repeat it. Returns the trimmed stdout on success.
func runGit(ctx context.Context, runner GitRunner, dir string, args ...string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, GitInvocationTimeout)
	defer cancel()
	stdout, stderr, err := runner.Run(tctx, dir, args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(stderr)))
	}
	return strings.TrimSpace(string(stdout)), nil
}
