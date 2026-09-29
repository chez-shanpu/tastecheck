// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package mock provides a configurable evaluator.Evaluator for tests.
package mock

import (
	"context"
	"slices"
	"sync"

	"github.com/chez-shanpu/tastecheck/internal/evaluator"
)

// Evaluator is a mock implementation of evaluator.Evaluator.
type Evaluator struct {
	// Func computes the verdicts; when nil, every criterion gets Score.
	Func func(s evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error)
	// Score is the score returned for every criterion when Func is nil.
	Score float64

	mu    sync.Mutex
	calls []evaluator.Subject
}

// Evaluate implements evaluator.Evaluator. It records s for Calls and returns
// the result of Func, or one verdict with Score for each criterion when Func
// is nil. Verdicts with Score report no confidence.
func (m *Evaluator) Evaluate(_ context.Context, s evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
	m.record(s)

	if m.Func != nil {
		return m.Func(s, cs)
	}
	verdicts := make([]evaluator.Verdict, 0, len(cs))
	for _, c := range cs {
		verdicts = append(verdicts, evaluator.Verdict{CriterionID: c.ID, Score: m.Score})
	}
	return verdicts, nil
}

func (m *Evaluator) record(s evaluator.Subject) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, s)
}

// Calls returns the subjects passed to Evaluate so far.
func (m *Evaluator) Calls() []evaluator.Subject {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.calls)
}
