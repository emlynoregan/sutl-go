package sutl

import (
	"context"
	"fmt"
)

// Limits bound one evaluation. Zero means that bound is unset.
type Limits struct {
	MaxSteps int
	MaxDepth int
}

func (l Limits) active() bool {
	return l.MaxSteps > 0 || l.MaxDepth > 0
}

func (l Limits) valid() error {
	if l.MaxSteps < 0 || l.MaxDepth < 0 {
		return fmt.Errorf("sutl: limits must be >= 0")
	}
	return nil
}

// LimitError is returned when evaluation exceeds a limit or the host cancels.
// Reason is "steps", "depth", or "cancelled". A cancelled error unwraps to the
// context error.
type LimitError struct {
	Reason string
	Err    error
}

func (e *LimitError) Error() string {
	return "sutl: evaluation stopped: " + e.Reason
}

func (e *LimitError) Unwrap() error {
	return e.Err
}

type budget struct {
	steps    int
	depth    int
	maxSteps int
	maxDepth int
	ctx      context.Context
}

type limitSignal struct {
	reason string
	err    error
}

func (r *Runner) charge() bool {
	b := r.budget
	if b == nil {
		return false
	}
	b.steps++
	if b.maxSteps > 0 && b.steps > b.maxSteps {
		panic(limitSignal{reason: "steps"})
	}
	if b.ctx != nil {
		if err := b.ctx.Err(); err != nil {
			panic(limitSignal{reason: "cancelled", err: err})
		}
	}
	if b.maxDepth > 0 && b.depth >= b.maxDepth {
		panic(limitSignal{reason: "depth"})
	}
	b.depth++
	return true
}

func (r *Runner) release() {
	if r.budget != nil {
		r.budget.depth--
	}
}

// CompileLimited compiles transform and applies limits on every run.
func CompileLimited(transform Value, library Library, limits Limits) (*Program, error) {
	if err := limits.valid(); err != nil {
		return nil, err
	}
	program := Compile(transform, library)
	program.limits = limits
	return program, nil
}

// RunContext runs the compiled transform. Limits set by CompileLimited are
// enforced here, and ctx is checked at every step. A context that cannot be
// cancelled and a program with no limits use the compiled function directly.
func (p *Program) RunContext(ctx context.Context, source Value) (Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !p.limits.active() && ctx.Done() == nil {
		return p.fn(source, source), nil
	}
	return evaluateLimited(ctx, source, p.root, p.library, p.limits)
}

// EvaluateLimited runs a transform with limits and cancellation.
func EvaluateLimited(ctx context.Context, source, transform Value, library Library, limits Limits) (Value, error) {
	if err := limits.valid(); err != nil {
		return nil, err
	}
	return evaluateLimited(ctx, source, transform, library, limits)
}

func evaluateLimited(ctx context.Context, source, transform Value, library Library, limits Limits) (value Value, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runner := NewRunner()
	if limits.active() || ctx.Done() != nil {
		runner.budget = &budget{
			maxSteps: limits.MaxSteps,
			maxDepth: limits.MaxDepth,
			ctx:      ctx,
		}
	}
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		signal, ok := recovered.(limitSignal)
		if !ok {
			panic(recovered)
		}
		value = nil
		err = &LimitError{Reason: signal.reason, Err: signal.err}
	}()
	value = runner.Evaluate(source, transform, library)
	return value, nil
}
