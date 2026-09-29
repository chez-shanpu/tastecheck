// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package evaluator

import (
	"context"
	"slices"
	"strings"
	"testing"
)

type stubEvaluator struct{ model string }

func (stubEvaluator) Evaluate(context.Context, Subject, []Criterion) ([]Verdict, error) {
	return nil, nil
}

func TestFactory(t *testing.T) {
	Register("stub", func(opts Options) (Evaluator, error) {
		return stubEvaluator{model: opts.Model}, nil
	})
	t.Cleanup(func() { delete(factories, "stub") })

	e, err := New("stub", Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := e.(stubEvaluator).model; got != "m" {
		t.Errorf("model = %q, want m", got)
	}
	if !slices.Contains(Backends(), "stub") {
		t.Errorf("Backends() = %v, want to contain stub", Backends())
	}

	if _, err := New("missing", Options{}); err == nil || !strings.Contains(err.Error(), `unknown evaluator backend "missing"`) {
		t.Errorf("New(missing) error = %v", err)
	}
}
