package sutl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/emlynoregan/sutl-go"
)

func loopTransform() map[string]sutl.Value {
	return map[string]sutl.Value{
		"!": "^$.loop",
		"loop": map[string]sutl.Value{
			"!": "^$.loop",
		},
	}
}

func TestLimits(t *testing.T) {
	got, err := sutl.EvaluateLimited(context.Background(), nil, 1, nil, sutl.Limits{MaxSteps: 1})
	if err != nil || got != 1 {
		t.Fatalf("literal under budget: %#v %v", got, err)
	}

	_, err = sutl.EvaluateLimited(context.Background(), nil, map[string]sutl.Value{"a": 1}, nil, sutl.Limits{MaxSteps: 1})
	if reason(t, err) != "steps" {
		t.Fatalf("steps: %v", err)
	}

	_, err = sutl.EvaluateLimited(context.Background(), nil, map[string]sutl.Value{"a": 1}, nil, sutl.Limits{MaxDepth: 1})
	if reason(t, err) != "depth" {
		t.Fatalf("depth: %v", err)
	}

	got, err = sutl.EvaluateLimited(context.Background(), nil, map[string]sutl.Value{"a": 1}, nil, sutl.Limits{MaxSteps: 2, MaxDepth: 2})
	if err != nil || !same(got, map[string]sutl.Value{"a": 1}) {
		t.Fatalf("within budget: %#v %v", got, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = sutl.EvaluateLimited(ctx, nil, 1, nil, sutl.Limits{})
	if reason(t, err) != "cancelled" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}

	source := loopTransform()
	_, err = sutl.EvaluateLimited(context.Background(), source, source, nil, sutl.Limits{MaxDepth: 8})
	if reason(t, err) != "depth" {
		t.Fatalf("loop: %v", err)
	}

	program, err := sutl.CompileLimited("^$.name", nil, sutl.Limits{MaxSteps: 100_000, MaxDepth: 256})
	if err != nil {
		t.Fatal(err)
	}
	got, err = program.RunContext(context.Background(), map[string]sutl.Value{"name": "Ada"})
	if err != nil || got != "Ada" {
		t.Fatalf("compiled path: %#v %v", got, err)
	}

	if _, err := sutl.CompileLimited(1, nil, sutl.Limits{MaxSteps: -1}); err == nil {
		t.Fatal("expected negative limits to be rejected")
	}
}

func reason(t *testing.T, err error) string {
	t.Helper()
	var limit *sutl.LimitError
	if !errors.As(err, &limit) {
		t.Fatalf("expected LimitError, got %v", err)
	}
	return limit.Reason
}
