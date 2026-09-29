// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package evaluator defines the provider-neutral interface for scoring code
// against tastes. Concrete backends live in sub-packages and register
// themselves with Register from their init functions.
package evaluator

import (
	"context"
	"errors"
)

// ErrTransient marks an evaluation failure that may succeed on retry,
// such as rate limiting, provider overload, or a network error.
// Backends wrap such errors so that callers can detect them with errors.Is.
var ErrTransient = errors.New("transient evaluation error")

// Subject is a piece of code to evaluate.
type Subject struct {
	// ID identifies the subject in logs and errors, e.g. "a.go:42 (*T).Handle".
	ID   string
	Code string
}

// Criterion is a single property the subject is evaluated against.
type Criterion struct {
	ID          string
	Description string
	// Levels are the possible outcomes, ordered from worst to best.
	Levels []string
}

// Verdict is the evaluation result of one criterion.
type Verdict struct {
	CriterionID string
	// Score is normalized to [0, 1], where 1 is best.
	Score float64
	// Confidence is the backend's certainty in [0, 1], or nil when the backend
	// does not report one. Zero is a reported value meaning the backend could
	// not decide between levels.
	Confidence *float64
	// Label describes the most likely outcome.
	Label string
	// NextLabel describes the next better outcome to aim for; empty when
	// the subject already reached the best one.
	NextLabel string
}

// Evaluator scores a subject against criteria.
type Evaluator interface {
	// Evaluate returns one Verdict per criterion. Implementations may batch
	// all criteria into a single request or evaluate them one by one.
	Evaluate(ctx context.Context, s Subject, cs []Criterion) ([]Verdict, error)
}
