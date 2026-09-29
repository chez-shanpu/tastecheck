// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package check evaluates extracted functions against tastes and decides
// whether the code passes.
//
// Code passes only when every function meets the threshold of every taste
// whose severity is fail. A function below the threshold of a warning taste
// is reported but does not fail the code, and neither does a finding below
// its threshold whose confidence is below the minimum confidence. Mean scores
// are reported for reference and never affect the outcome.
package check

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/chez-shanpu/tastecheck/internal/evaluator"
	"github.com/chez-shanpu/tastecheck/internal/extract"
	"github.com/chez-shanpu/tastecheck/internal/taste"
)

// Default settings. DefaultConcurrency and DefaultBackoff are used when the
// corresponding Config field is zero; DefaultMaxRetries is a suggested value
// for callers, since zero retries is a valid setting.
const (
	DefaultConcurrency = 4
	DefaultMaxRetries  = 3
	DefaultBackoff     = time.Second
)

// Config configures Run.
type Config struct {
	Evaluator evaluator.Evaluator
	Tastes    []taste.Taste
	Functions []extract.Function
	// Concurrency limits the number of functions evaluated in parallel.
	Concurrency int
	// MaxRetries is the number of retries for transient evaluation errors.
	// Zero or a negative value disables retries.
	MaxRetries int
	// Backoff is the base delay of the exponential backoff between retries.
	Backoff time.Duration
	// MinConfidence is the confidence below which a finding under its
	// threshold is ignored. Zero disables it. Findings without a reported
	// confidence are never ignored.
	MinConfidence float64
}

func (cfg *Config) setDefaults() {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = DefaultConcurrency
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = DefaultBackoff
	}
}

// evaluate calls the evaluator, retrying transient errors with exponential
// backoff and jitter.
func (cfg *Config) evaluate(ctx context.Context, s evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
	for attempt := 0; ; attempt++ {
		verdicts, err := cfg.Evaluator.Evaluate(ctx, s, cs)
		if err == nil {
			return verdicts, nil
		}
		if !errors.Is(err, evaluator.ErrTransient) || attempt >= cfg.MaxRetries {
			return nil, err
		}

		if serr := sleep(ctx, jitter(cfg.Backoff<<attempt)); serr != nil {
			return nil, fmt.Errorf("%w (last error: %w)", serr, err)
		}
	}
}

// Finding is the outcome of one taste for one function. Passed reports
// whether Score meets Threshold regardless of Severity, so a warning below
// its threshold is a finding that has not passed.
//
// LowConfidence reports that the finding has not passed but is ignored
// because its confidence is below Config.MinConfidence.
type Finding struct {
	File      string
	Line      int
	Function  string
	Ruleset   string
	TasteID   string
	Score     float64
	Threshold float64
	Severity  taste.Severity
	Passed    bool
	// Confidence is nil when the evaluator does not report one.
	Confidence    *float64
	LowConfidence bool
	Label         string
	NextLabel     string
}

// Summary aggregates findings.
type Summary struct {
	Functions int
	// Failed is the number of fail-severity findings below their threshold.
	Failed int
	// Warnings is the number of warning-severity findings below their
	// threshold.
	Warnings int
	// LowConfidence is the number of findings below their threshold that are
	// ignored because of their low confidence. They are not counted in Failed
	// or Warnings.
	LowConfidence int
	// Mean is the unweighted mean score of all findings.
	Mean float64
	// MeanByTaste is the mean score per taste key ("<ruleset>/<id>").
	MeanByTaste map[string]float64
}

// Result is the outcome of Run.
type Result struct {
	Passed bool
	Summary
	// Findings are ordered by file, line, and taste definition order.
	Findings []Finding
}

// Run evaluates every function against every taste. Any evaluation error
// fails the whole run, since an unevaluated function must not count as passing.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	cfg.setDefaults()
	criteria := toCriteria(cfg.Tastes)

	// Each goroutine writes only to its own index, so no locking is needed.
	perFunc := make([][]evaluator.Verdict, len(cfg.Functions))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Concurrency)
	for i, fn := range cfg.Functions {
		g.Go(func() error {
			verdicts, err := cfg.evaluate(gctx, subjectOf(fn), criteria)
			if err != nil {
				return err
			}
			perFunc[i] = verdicts
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return buildResult(cfg.Functions, cfg.Tastes, perFunc, cfg.MinConfidence)
}

// jitter returns a random duration in [d/2, 3d/2).
func jitter(d time.Duration) time.Duration {
	return d/2 + rand.N(d)
}

// sleep waits for d, returning early with ctx.Err() if ctx is done first.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func toCriteria(tastes []taste.Taste) []evaluator.Criterion {
	cs := make([]evaluator.Criterion, 0, len(tastes))
	for _, t := range tastes {
		cs = append(cs, evaluator.Criterion{
			ID:          t.Key(),
			Description: t.Description,
			Levels:      t.Levels,
		})
	}
	return cs
}

func subjectOf(fn extract.Function) evaluator.Subject {
	return evaluator.Subject{
		ID:   fmt.Sprintf("%s:%d %s", fn.File, fn.StartLine, fn.Name),
		Code: fn.Source,
	}
}

func buildResult(funcs []extract.Function, tastes []taste.Taste, perFunc [][]evaluator.Verdict, minConfidence float64) (*Result, error) {
	res := &Result{
		Passed:      true,
		Functions:   len(funcs),
		MeanByTaste: make(map[string]float64, len(tastes)),
	}

	var total float64
	for i, fn := range funcs {
		byID := make(map[string]evaluator.Verdict, len(perFunc[i]))
		for _, v := range perFunc[i] {
			byID[v.CriterionID] = v
		}
		for _, t := range tastes {
			v, ok := byID[t.Key()]
			if !ok {
				return nil, fmt.Errorf("%s:%d %s: evaluator returned no verdict for taste %q", fn.File, fn.StartLine, fn.Name, t.Key())
			}
			f := Finding{
				File:       fn.File,
				Line:       fn.StartLine,
				Function:   fn.Name,
				Ruleset:    t.Ruleset,
				TasteID:    t.ID,
				Score:      v.Score,
				Threshold:  t.Threshold,
				Severity:   t.Severity,
				Passed:     v.Score >= t.Threshold,
				Confidence: v.Confidence,
				Label:      v.Label,
				NextLabel:  v.NextLabel,
			}
			f.LowConfidence = !f.Passed && f.Confidence != nil && *f.Confidence < minConfidence
			// Any severity other than warning fails, so that an unset
			// severity never lets a violation through.
			switch {
			case f.Passed:
			case f.LowConfidence:
				res.LowConfidence++
			case f.Severity == taste.SeverityWarning:
				res.Warnings++
			default:
				res.Passed = false
				res.Failed++
			}
			res.Findings = append(res.Findings, f)
			res.MeanByTaste[t.Key()] += v.Score
			total += v.Score
		}
	}

	if len(funcs) > 0 {
		for id := range res.MeanByTaste {
			res.MeanByTaste[id] /= float64(len(funcs))
		}
		res.Mean = total / float64(len(res.Findings))
	}
	return res, nil
}
