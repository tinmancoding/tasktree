package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/bootstrap"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

type fakeBootstrapRunner struct {
	called bool
	steps  []domain.BootstrapStep
	err    error
}

func (f *fakeBootstrapRunner) Run(ctx context.Context, root string, steps []domain.BootstrapStep, opts bootstrap.Options) error {
	f.called = true
	f.steps = steps
	return f.err
}

func applyServiceWithBootstrap(t *testing.T, steps []domain.BootstrapStep, runner app.BootstrapRunner) (app.ApplyService, string) {
	t.Helper()
	root := createTasktree(t)
	store := metadata.NewStore()
	spec, err := store.Load(root)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	spec.Spec.Bootstrap = steps
	if err := store.Save(root, spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	svc := app.NewApplyService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())
	return svc.WithBootstrapRunner(runner), root
}

func TestApplyRunsBootstrapAfterSources(t *testing.T) {
	steps := []domain.BootstrapStep{{Name: "deps", Run: "echo hi"}}
	runner := &fakeBootstrapRunner{}
	svc, root := applyServiceWithBootstrap(t, steps, runner)

	result, err := svc.Run(context.Background(), root, app.ApplyOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !runner.called {
		t.Fatal("bootstrap runner was not invoked")
	}
	if !result.BootstrapRan {
		t.Fatal("BootstrapRan should be true")
	}
	if len(runner.steps) != 1 || runner.steps[0].Name != "deps" {
		t.Fatalf("wrong steps passed: %+v", runner.steps)
	}
}

func TestApplySkipBootstrap(t *testing.T) {
	steps := []domain.BootstrapStep{{Name: "deps", Run: "echo hi"}}
	runner := &fakeBootstrapRunner{}
	svc, root := applyServiceWithBootstrap(t, steps, runner)

	result, err := svc.Run(context.Background(), root, app.ApplyOptions{SkipBootstrap: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runner.called {
		t.Fatal("bootstrap should be skipped")
	}
	if result.BootstrapRan {
		t.Fatal("BootstrapRan should be false")
	}
}

func TestApplyNoBootstrapSteps(t *testing.T) {
	runner := &fakeBootstrapRunner{}
	svc, root := applyServiceWithBootstrap(t, nil, runner)

	if _, err := svc.Run(context.Background(), root, app.ApplyOptions{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runner.called {
		t.Fatal("bootstrap should not run with no steps")
	}
}

func TestApplyDryRunBuildsPlanWithoutRunning(t *testing.T) {
	steps := []domain.BootstrapStep{{Name: "deps", Run: "echo hi"}}
	runner := &fakeBootstrapRunner{}
	svc, root := applyServiceWithBootstrap(t, steps, runner)

	result, err := svc.Run(context.Background(), root, app.ApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runner.called {
		t.Fatal("dry-run must not execute bootstrap")
	}
	if len(result.BootstrapPlan) != 1 || result.BootstrapPlan[0].Name != "deps" {
		t.Fatalf("plan not populated: %+v", result.BootstrapPlan)
	}
}

func TestApplyBootstrapValidationGate(t *testing.T) {
	steps := []domain.BootstrapStep{{Name: "deps", Run: "echo hi"}, {Name: "deps", Run: "echo dup"}}
	runner := &fakeBootstrapRunner{}
	svc, root := applyServiceWithBootstrap(t, steps, runner)

	_, err := svc.Run(context.Background(), root, app.ApplyOptions{})
	var dup domain.DuplicateBootstrapNameError
	if !errors.As(err, &dup) {
		t.Fatalf("expected DuplicateBootstrapNameError, got %v", err)
	}
	if runner.called {
		t.Fatal("runner should not be called when validation fails")
	}
}

func TestApplyBootstrapErrorAbortsRun(t *testing.T) {
	steps := []domain.BootstrapStep{{Name: "deps", Run: "echo hi"}}
	runner := &fakeBootstrapRunner{err: domain.StepFailedError{Name: "deps", ExitCode: 1}}
	svc, root := applyServiceWithBootstrap(t, steps, runner)

	_, err := svc.Run(context.Background(), root, app.ApplyOptions{})
	var sfe domain.StepFailedError
	if !errors.As(err, &sfe) {
		t.Fatalf("expected StepFailedError, got %v", err)
	}
}
