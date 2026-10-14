// Package trace records what the pipeline did for one preview, step by step, for the debug page. A context without
// a trace records nothing and costs nothing.
package trace

import (
	"context"
	"sync"
	"time"
)

// Kinds of step.
const (
	KindFetch    = "fetch"
	KindSource   = "source"
	KindProvider = "provider"
	KindWall     = "wall"
	KindEnricher = "enricher"
)

// Step is one thing the pipeline did: what, how long it took, what came of it.
type Step struct {
	Kind    string
	Name    string
	Detail  string
	Outcome string
	Took    time.Duration
}

// Candidate is one source's value for a field.
type Candidate struct {
	Source string
	Value  string
}

// Decision is how a field was filled: the sources asked in order, the one that won and why.
type Decision struct {
	Field      string
	Candidates []Candidate
	Winner     string
	Reason     string
}

// Trace collects the steps and decisions of one preview; the image proxy's enricher measures from several goroutines,
// so it is safe for concurrent use.
type Trace struct {
	mu        sync.Mutex
	steps     []Step
	decisions []Decision
}

type key struct{}

// With is a context whose pipeline records into the trace.
func With(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, key{}, t)
}

// From is the context's trace; nil, which records nothing, for none.
func From(ctx context.Context) *Trace {
	t, _ := ctx.Value(key{}).(*Trace)
	return t
}

// Step records a step.
func (t *Trace) Step(step Step) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps = append(t.steps, step)
}

// Decide records a field's decision.
func (t *Trace) Decide(decision Decision) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.decisions = append(t.decisions, decision)
}

// Steps are the steps recorded, in order.
func (t *Trace) Steps() []Step {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Step(nil), t.steps...)
}

// Decisions are the field decisions recorded, in order.
func (t *Trace) Decisions() []Decision {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Decision(nil), t.decisions...)
}
