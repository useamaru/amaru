package scaffold

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeGitRunner records invocations and lets each test inject behaviors.
// On a "clone <url> <dir>" call, the registered handler is responsible for
// writing fixture content into <dir> so subsequent in-place steps see a real
// repo on disk.
type fakeGitRunner struct {
	t        *testing.T
	calls    [][]string
	handlers map[string]func(t *testing.T, dir string, args []string) (stdout, stderr []byte, err error)
}

func newFakeRunner(t *testing.T) *fakeGitRunner {
	return &fakeGitRunner{t: t, handlers: map[string]func(*testing.T, string, []string) ([]byte, []byte, error){}}
}

// On registers a handler keyed by the first git arg (e.g. "clone", "rev-parse").
// Multiple subsequent calls with the same key cycle through registered handlers.
func (f *fakeGitRunner) On(key string, h func(t *testing.T, dir string, args []string) ([]byte, []byte, error)) {
	f.handlers[key] = h
}

// OnSequence registers an ordered sequence of handlers for the same key.
// Used when the same git verb is invoked multiple times with different expected outputs.
type sequenceHandler struct {
	hs   []func(t *testing.T, dir string, args []string) ([]byte, []byte, error)
	next int
}

func (f *fakeGitRunner) OnSequence(key string, hs ...func(t *testing.T, dir string, args []string) ([]byte, []byte, error)) {
	seq := &sequenceHandler{hs: hs}
	f.handlers[key] = func(t *testing.T, dir string, args []string) ([]byte, []byte, error) {
		if seq.next >= len(seq.hs) {
			t.Fatalf("git %v invoked more times than sequence handlers (%d)", args, len(seq.hs))
		}
		h := seq.hs[seq.next]
		seq.next++
		return h(t, dir, args)
	}
}

func (f *fakeGitRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	cp := append([]string{}, args...)
	f.calls = append(f.calls, cp)
	if len(args) == 0 {
		return nil, nil, errors.New("empty args")
	}
	if h, ok := f.handlers[args[0]]; ok {
		return h(f.t, dir, args)
	}
	return []byte{}, []byte{}, nil
}

func (f *fakeGitRunner) callsByKey(key string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if len(c) > 0 && c[0] == key {
			out = append(out, c)
		}
	}
	return out
}

// cloneHandlerWritingLegacy returns a clone handler that populates the target
// directory with a v1 (nested) registry layout.
func cloneHandlerWritingLegacy() func(t *testing.T, dir string, args []string) ([]byte, []byte, error) {
	return func(t *testing.T, _ string, args []string) ([]byte, []byte, error) {
		// args = ["clone", url, target]
		if len(args) < 3 {
			return nil, nil, fmt.Errorf("clone needs url+target, got %v", args)
		}
		target := args[2]
		writeLegacyRegistry(t, target)
		return nil, nil, nil
	}
}

// cloneHandlerWritingFlat populates the target with a v2 layout (already migrated).
func cloneHandlerWritingFlat() func(t *testing.T, dir string, args []string) ([]byte, []byte, error) {
	return func(t *testing.T, _ string, args []string) ([]byte, []byte, error) {
		if len(args) < 3 {
			return nil, nil, fmt.Errorf("clone needs url+target, got %v", args)
		}
		target := args[2]
		writeFlatRegistry(t, target)
		return nil, nil, nil
	}
}

func staticOK(stdout string) func(t *testing.T, dir string, args []string) ([]byte, []byte, error) {
	return func(_ *testing.T, _ string, _ []string) ([]byte, []byte, error) {
		return []byte(stdout), nil, nil
	}
}

func staticErr(msg string) func(t *testing.T, dir string, args []string) ([]byte, []byte, error) {
	return func(_ *testing.T, _ string, _ []string) ([]byte, []byte, error) {
		return nil, []byte(msg), errors.New(msg)
	}
}

// ---- Tests ----------------------------------------------------------------

func TestMigrateRemote_HappyPath_SideBranch(t *testing.T) {
	runner := newFakeRunner(t)
	runner.On("clone", cloneHandlerWritingLegacy())
	// rev-parse is called multiple times — sequence them: HEAD, --abbrev-ref HEAD, HEAD (after commit), origin/main
	runner.OnSequence("rev-parse",
		staticOK("abc123def4567890"),       // initial HEAD
		staticOK("main"),                   // current branch (default branch)
		staticOK("def456abc7890123"),       // commit SHA after migration commit
		staticOK("abc123def4567890"),       // origin/main tip (matches clonedHead → ancestry OK)
	)
	runner.On("checkout", staticOK(""))
	runner.On("add", staticOK(""))
	runner.On("status", staticOK(" M something")) // non-empty so the "internal error" guard passes
	runner.On("commit", staticOK(""))
	runner.On("fetch", staticOK(""))
	runner.On("push", staticOK(""))

	res, err := MigrateRemote(context.Background(), "github:acme/skills",
		RemoteMigrateOptions{Runner: runner})
	if err != nil {
		t.Fatalf("MigrateRemote error: %v", err)
	}
	if !res.Pushed {
		t.Error("expected Pushed=true")
	}
	if res.PushedToDefault {
		t.Error("expected PushedToDefault=false (default is side branch)")
	}
	if res.Branch != DefaultMigrationBranch {
		t.Errorf("Branch = %q, want %q", res.Branch, DefaultMigrationBranch)
	}
	if res.InPlaceResult == nil || res.InPlaceResult.Status != MigrationStatusCompleted {
		t.Errorf("InPlaceResult = %+v", res.InPlaceResult)
	}

	// Verify the call sequence: clone → rev-parse HEAD → rev-parse abbrev-ref → checkout -b → add → status → commit → rev-parse HEAD → fetch → rev-parse origin/main → push.
	wantSeq := []string{"clone", "rev-parse", "rev-parse", "checkout", "add", "status", "commit", "rev-parse", "fetch", "rev-parse", "push"}
	if len(runner.calls) != len(wantSeq) {
		t.Fatalf("call count = %d, want %d. Calls: %v", len(runner.calls), len(wantSeq), runner.calls)
	}
	for i, k := range wantSeq {
		if runner.calls[i][0] != k {
			t.Errorf("call[%d] = %q, want %q (full: %v)", i, runner.calls[i][0], k, runner.calls[i])
		}
	}
	// And the push targeted the side branch.
	pushCalls := runner.callsByKey("push")
	if len(pushCalls) != 1 {
		t.Fatalf("expected 1 push, got %d", len(pushCalls))
	}
	push := pushCalls[0]
	if push[len(push)-1] != DefaultMigrationBranch {
		t.Errorf("push target = %q, want %q", push[len(push)-1], DefaultMigrationBranch)
	}
}

func TestMigrateRemote_PushToDefaultSkipsCheckout(t *testing.T) {
	runner := newFakeRunner(t)
	runner.On("clone", cloneHandlerWritingLegacy())
	runner.OnSequence("rev-parse",
		staticOK("abc123def4567890"),
		staticOK("main"),
		staticOK("def456"),
		staticOK("abc123def4567890"),
	)
	runner.On("add", staticOK(""))
	runner.On("status", staticOK(" M x"))
	runner.On("commit", staticOK(""))
	runner.On("fetch", staticOK(""))
	runner.On("push", staticOK(""))

	res, err := MigrateRemote(context.Background(), "github:acme/skills",
		RemoteMigrateOptions{Runner: runner, PushToDefault: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.PushedToDefault {
		t.Error("expected PushedToDefault=true")
	}
	if got := runner.callsByKey("checkout"); len(got) != 0 {
		t.Errorf("expected no checkout call with --push-to-default, got %v", got)
	}
	pushCalls := runner.callsByKey("push")
	if pushCalls[0][len(pushCalls[0])-1] != "main" {
		t.Errorf("push target = %q, want main", pushCalls[0][len(pushCalls[0])-1])
	}
}

func TestMigrateRemote_AlreadyMigratedSkipsPush(t *testing.T) {
	runner := newFakeRunner(t)
	runner.On("clone", cloneHandlerWritingFlat())
	runner.OnSequence("rev-parse",
		staticOK("abc123"),
		staticOK("main"),
	)
	runner.On("checkout", staticOK(""))

	res, err := MigrateRemote(context.Background(), "github:acme/skills",
		RemoteMigrateOptions{Runner: runner})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Pushed {
		t.Error("already-migrated: should not push")
	}
	if got := runner.callsByKey("push"); len(got) != 0 {
		t.Error("already-migrated: should not invoke git push")
	}
	if got := runner.callsByKey("commit"); len(got) != 0 {
		t.Error("already-migrated: should not invoke git commit")
	}
}

func TestMigrateRemote_RemoteMovedRefusesPush(t *testing.T) {
	runner := newFakeRunner(t)
	runner.On("clone", cloneHandlerWritingLegacy())
	runner.OnSequence("rev-parse",
		staticOK("abc123def4567890"), // initial HEAD
		staticOK("main"),
		staticOK("def456"),
		staticOK("999999999999"), // origin/main tip — moved!
	)
	runner.On("checkout", staticOK(""))
	runner.On("add", staticOK(""))
	runner.On("status", staticOK(" M x"))
	runner.On("commit", staticOK(""))
	runner.On("fetch", staticOK(""))

	_, err := MigrateRemote(context.Background(), "github:acme/skills",
		RemoteMigrateOptions{Runner: runner})
	if err == nil {
		t.Fatal("expected error for moved remote")
	}
	if !strings.Contains(err.Error(), "has moved since clone") {
		t.Errorf("error should mention remote moved: %v", err)
	}
	if got := runner.callsByKey("push"); len(got) != 0 {
		t.Error("must not push when remote moved")
	}
}

func TestMigrateRemote_CloneFailureNoFurtherCalls(t *testing.T) {
	runner := newFakeRunner(t)
	runner.On("clone", staticErr("fatal: repository not found"))

	_, err := MigrateRemote(context.Background(), "github:acme/missing",
		RemoteMigrateOptions{Runner: runner})
	if err == nil {
		t.Fatal("expected clone failure")
	}
	if !strings.Contains(err.Error(), "clone failed") {
		t.Errorf("error should mention clone: %v", err)
	}
	// Only the clone call should have been made.
	if len(runner.calls) != 1 {
		t.Errorf("expected 1 call (clone), got %d: %v", len(runner.calls), runner.calls)
	}
}

func TestMigrateRemote_DryRunDoesNotPush(t *testing.T) {
	runner := newFakeRunner(t)
	runner.On("clone", cloneHandlerWritingLegacy())
	runner.OnSequence("rev-parse",
		staticOK("abc123"),
		staticOK("main"),
	)
	runner.On("checkout", staticOK(""))
	runner.On("add", staticOK(""))
	runner.On("status", staticOK(" M x"))

	_, err := MigrateRemote(context.Background(), "github:acme/skills",
		RemoteMigrateOptions{Runner: runner, MigrateOptions: MigrateOptions{DryRun: true}})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if got := runner.callsByKey("commit"); len(got) != 0 {
		t.Error("dry-run: must not commit")
	}
	if got := runner.callsByKey("push"); len(got) != 0 {
		t.Error("dry-run: must not push")
	}
}

func TestMigrateRemote_InvalidURLRejectedEarly(t *testing.T) {
	runner := newFakeRunner(t)
	_, err := MigrateRemote(context.Background(), "not-a-real-url",
		RemoteMigrateOptions{Runner: runner})
	if err == nil {
		t.Fatal("expected URL parse error")
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected no git calls for invalid URL, got %v", runner.calls)
	}
}

func TestBuildCloneURL(t *testing.T) {
	tests := []struct {
		canonical, protocol, want string
	}{
		{"github:org/repo", "ssh", "git@github.com:org/repo.git"},
		{"github:org/repo", "", "git@github.com:org/repo.git"},
		{"github:org/repo", "https", "https://github.com/org/repo.git"},
		{"github:org/repo", "weird", "https://github.com/org/repo.git"},
	}
	for _, tt := range tests {
		got := buildCloneURL(tt.canonical, tt.protocol)
		if got != tt.want {
			t.Errorf("buildCloneURL(%q, %q) = %q, want %q", tt.canonical, tt.protocol, got, tt.want)
		}
	}
}
