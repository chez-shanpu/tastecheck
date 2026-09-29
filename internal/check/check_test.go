// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package check

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chez-shanpu/tastecheck/internal/evaluator"
	"github.com/chez-shanpu/tastecheck/internal/evaluator/mock"
	"github.com/chez-shanpu/tastecheck/internal/extract"
	"github.com/chez-shanpu/tastecheck/internal/taste"
)

var (
	srp  = taste.Taste{ID: "srp", Description: "d", Levels: []string{"bad", "good"}, Threshold: 0.7}
	yes  = taste.Taste{ID: "yes", Description: "d", Levels: []string{"no", "yes"}, Threshold: 0.5}
	warn = taste.Taste{ID: "warn", Description: "d", Levels: []string{"no", "yes"}, Threshold: 0.7, Severity: taste.SeverityWarning}
	fnA  = extract.Function{File: "a.go", StartLine: 1, Name: "A", Source: "func A() {}"}
	fnB  = extract.Function{File: "a.go", StartLine: 5, Name: "B", Source: "func B() {}"}
	fnC  = extract.Function{File: "b.go", StartLine: 3, Name: "C", Source: "func C() {}"}
	fast = time.Millisecond
)

// scoresByCode returns a mock func that scores every criterion by subject code.
func scoresByCode(scores map[string]float64) func(evaluator.Subject, []evaluator.Criterion) ([]evaluator.Verdict, error) {
	return func(s evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
		vs := make([]evaluator.Verdict, 0, len(cs))
		for _, c := range cs {
			vs = append(vs, evaluator.Verdict{CriterionID: c.ID, Score: scores[s.Code], Label: "l-" + c.ID})
		}
		return vs, nil
	}
}

func TestRunJudgement(t *testing.T) {
	tests := []struct {
		name         string
		tastes       []taste.Taste
		funcs        []extract.Function
		scores       map[string]float64
		wantPassed   bool
		wantFailed   int
		wantWarnings int
		wantMean     float64
	}{
		{
			name:       "all functions meet threshold",
			tastes:     []taste.Taste{srp},
			funcs:      []extract.Function{fnA, fnB},
			scores:     map[string]float64{fnA.Source: 0.7, fnB.Source: 0.9},
			wantPassed: true,
			wantMean:   0.8,
		},
		{
			name:       "one function below threshold fails even with high mean",
			tastes:     []taste.Taste{srp},
			funcs:      []extract.Function{fnA, fnB, fnC},
			scores:     map[string]float64{fnA.Source: 1, fnB.Source: 1, fnC.Source: 0.4},
			wantPassed: false,
			wantFailed: 1,
			wantMean:   0.8,
		},
		{
			name:       "thresholds are per taste",
			tastes:     []taste.Taste{srp, yes},
			funcs:      []extract.Function{fnA},
			scores:     map[string]float64{fnA.Source: 0.6},
			wantPassed: false,
			wantFailed: 1,
			wantMean:   0.6,
		},
		{
			name:         "warning below threshold does not fail",
			tastes:       []taste.Taste{warn},
			funcs:        []extract.Function{fnA, fnB},
			scores:       map[string]float64{fnA.Source: 0.4, fnB.Source: 1},
			wantPassed:   true,
			wantWarnings: 1,
			wantMean:     0.7,
		},
		{
			name:         "fail and warning are counted separately",
			tastes:       []taste.Taste{srp, warn},
			funcs:        []extract.Function{fnA},
			scores:       map[string]float64{fnA.Source: 0.4},
			wantPassed:   false,
			wantFailed:   1,
			wantWarnings: 1,
			wantMean:     0.4,
		},
		{
			name:       "no functions passes",
			tastes:     []taste.Taste{srp},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Run(t.Context(), Config{
				Evaluator: &mock.Evaluator{Func: scoresByCode(tt.scores)},
				Tastes:    tt.tastes,
				Functions: tt.funcs,
			})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if res.Passed != tt.wantPassed || res.Failed != tt.wantFailed || res.Warnings != tt.wantWarnings {
				t.Errorf("Run() passed=%v failed=%d warnings=%d, want passed=%v failed=%d warnings=%d",
					res.Passed, res.Failed, res.Warnings, tt.wantPassed, tt.wantFailed, tt.wantWarnings)
			}
			if math.Abs(res.Mean-tt.wantMean) > 1e-9 {
				t.Errorf("Run() mean = %v, want %v", res.Mean, tt.wantMean)
			}
			if res.Functions != len(tt.funcs) {
				t.Errorf("Run() functions = %d, want %d", res.Functions, len(tt.funcs))
			}
		})
	}
}

func TestRunFindings(t *testing.T) {
	res, err := Run(t.Context(), Config{
		Evaluator:   &mock.Evaluator{Func: scoresByCode(map[string]float64{fnA.Source: 0.5, fnB.Source: 1, fnC.Source: 0.8})},
		Tastes:      []taste.Taste{srp, yes},
		Functions:   []extract.Function{fnA, fnB, fnC},
		Concurrency: 3,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	var got []string
	for _, f := range res.Findings {
		got = append(got, fmt.Sprintf("%s:%d %s %s %v %v", f.File, f.Line, f.Function, f.TasteID, f.Score, f.Passed))
	}
	want := []string{
		"a.go:1 A srp 0.5 false",
		"a.go:1 A yes 0.5 true",
		"a.go:5 B srp 1 true",
		"a.go:5 B yes 1 true",
		"b.go:3 C srp 0.8 true",
		"b.go:3 C yes 0.8 true",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings =\n%v\nwant\n%v", got, want)
	}
	if len(res.MeanByTaste) != 2 {
		t.Errorf("MeanByTaste = %v, want 2 entries", res.MeanByTaste)
	}
	for id, mean := range res.MeanByTaste {
		if want := (0.5 + 1 + 0.8) / 3; math.Abs(mean-want) > 1e-9 {
			t.Errorf("MeanByTaste[%s] = %v, want %v", id, mean, want)
		}
	}
	if f := res.Findings[0]; f.Threshold != 0.7 || f.Label != "l-srp" {
		t.Errorf("finding[0] = %+v", f)
	}
}

func TestRunMinConfidence(t *testing.T) {
	tests := []struct {
		name              string
		taste             taste.Taste
		score             float64
		confidence        *float64
		minConfidence     float64
		wantPassed        bool
		wantFailed        int
		wantWarnings      int
		wantLowConfidence int
	}{
		{
			name:              "fail below min confidence is ignored",
			taste:             srp,
			score:             0.5,
			confidence:        new(0.2),
			minConfidence:     0.3,
			wantPassed:        true,
			wantLowConfidence: 1,
		},
		{
			name:              "zero confidence is below min confidence",
			taste:             srp,
			score:             0.5,
			confidence:        new(0.0),
			minConfidence:     0.3,
			wantPassed:        true,
			wantLowConfidence: 1,
		},
		{
			name:          "fail at min confidence counts",
			taste:         srp,
			score:         0.5,
			confidence:    new(0.3),
			minConfidence: 0.3,
			wantFailed:    1,
		},
		{
			name:          "unreported confidence is never ignored",
			taste:         srp,
			score:         0.5,
			minConfidence: 0.3,
			wantFailed:    1,
		},
		{
			name:       "zero min confidence disables it",
			taste:      srp,
			score:      0.5,
			confidence: new(0.0),
			wantFailed: 1,
		},
		{
			name:              "warning below min confidence is ignored",
			taste:             warn,
			score:             0.5,
			confidence:        new(0.2),
			minConfidence:     0.3,
			wantPassed:        true,
			wantLowConfidence: 1,
		},
		{
			name:          "passing finding is not marked",
			taste:         srp,
			score:         0.9,
			confidence:    new(0.2),
			minConfidence: 0.3,
			wantPassed:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mock.Evaluator{Func: func(_ evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
				return []evaluator.Verdict{{CriterionID: cs[0].ID, Score: tt.score, Confidence: tt.confidence}}, nil
			}}
			res, err := Run(t.Context(), Config{
				Evaluator:     m,
				Tastes:        []taste.Taste{tt.taste},
				Functions:     []extract.Function{fnA},
				MinConfidence: tt.minConfidence,
			})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if res.Passed != tt.wantPassed || res.Failed != tt.wantFailed || res.Warnings != tt.wantWarnings || res.LowConfidence != tt.wantLowConfidence {
				t.Errorf("Run() passed=%v failed=%d warnings=%d lowConfidence=%d, want passed=%v failed=%d warnings=%d lowConfidence=%d",
					res.Passed, res.Failed, res.Warnings, res.LowConfidence, tt.wantPassed, tt.wantFailed, tt.wantWarnings, tt.wantLowConfidence)
			}
			if got, want := res.Findings[0].LowConfidence, tt.wantLowConfidence == 1; got != want {
				t.Errorf("Findings[0].LowConfidence = %v, want %v", got, want)
			}
		})
	}
}

func TestRunSubject(t *testing.T) {
	m := &mock.Evaluator{Score: 1}
	if _, err := Run(t.Context(), Config{Evaluator: m, Tastes: []taste.Taste{srp}, Functions: []extract.Function{fnA}}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	want := []evaluator.Subject{{ID: "a.go:1 A", Code: fnA.Source}}
	if got := m.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("subjects = %+v, want %+v", got, want)
	}
}

func TestRunRetry(t *testing.T) {
	transient := fmt.Errorf("%w: rate limited", evaluator.ErrTransient)
	permanent := errors.New("unauthorized")

	tests := []struct {
		name       string
		failures   int
		failWith   error
		maxRetries int
		wantErr    error
		wantCalls  int32
	}{
		{name: "transient error is retried", failures: 2, failWith: transient, maxRetries: 3, wantCalls: 3},
		{name: "retries are exhausted", failures: 10, failWith: transient, maxRetries: 2, wantErr: evaluator.ErrTransient, wantCalls: 3},
		{name: "zero disables retries", failures: 1, failWith: transient, maxRetries: 0, wantErr: evaluator.ErrTransient, wantCalls: 1},
		{name: "negative disables retries", failures: 1, failWith: transient, maxRetries: -1, wantErr: evaluator.ErrTransient, wantCalls: 1},
		{name: "permanent error is not retried", failures: 1, failWith: permanent, maxRetries: 3, wantErr: permanent, wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			m := &mock.Evaluator{Func: func(s evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
				if calls.Add(1) <= int32(tt.failures) {
					return nil, tt.failWith
				}
				return []evaluator.Verdict{{CriterionID: "srp", Score: 1}}, nil
			}}

			_, err := Run(t.Context(), Config{
				Evaluator:  m,
				Tastes:     []taste.Taste{srp},
				Functions:  []extract.Function{fnA},
				MaxRetries: tt.maxRetries,
				Backoff:    fast,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("calls = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestRunRetryCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	m := &mock.Evaluator{Func: func(evaluator.Subject, []evaluator.Criterion) ([]evaluator.Verdict, error) {
		cancel()
		return nil, evaluator.ErrTransient
	}}

	_, err := Run(ctx, Config{Evaluator: m, Tastes: []taste.Taste{srp}, Functions: []extract.Function{fnA}, MaxRetries: 3, Backoff: time.Hour})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunMissingVerdict(t *testing.T) {
	m := &mock.Evaluator{Func: func(evaluator.Subject, []evaluator.Criterion) ([]evaluator.Verdict, error) {
		return []evaluator.Verdict{{CriterionID: "srp", Score: 1}}, nil
	}}
	_, err := Run(t.Context(), Config{Evaluator: m, Tastes: []taste.Taste{srp, yes}, Functions: []extract.Function{fnA}})
	if err == nil || !strings.Contains(err.Error(), `no verdict for taste "yes"`) {
		t.Errorf("Run() error = %v, want missing verdict error", err)
	}
}

func TestToCriteria(t *testing.T) {
	scoped := taste.Taste{Ruleset: "team", ID: "n", Description: "d", Levels: []string{"no", "yes"}}
	got := toCriteria([]taste.Taste{srp, scoped})
	want := []evaluator.Criterion{
		{ID: "srp", Description: "d", Levels: []string{"bad", "good"}},
		{ID: "team/n", Description: "d", Levels: []string{"no", "yes"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toCriteria() = %+v, want %+v", got, want)
	}
}

func TestRunSameIDInDifferentRulesets(t *testing.T) {
	a := taste.Taste{Ruleset: "uber-go", ID: "error-wrap", Description: "d", Levels: []string{"no", "yes"}, Threshold: 0.5}
	b := taste.Taste{Ruleset: "team", ID: "error-wrap", Description: "d", Levels: []string{"no", "yes"}, Threshold: 0.9}
	m := &mock.Evaluator{Func: func(_ evaluator.Subject, cs []evaluator.Criterion) ([]evaluator.Verdict, error) {
		if len(cs) != 2 || cs[0].ID != "uber-go/error-wrap" || cs[1].ID != "team/error-wrap" {
			return nil, fmt.Errorf("unexpected criteria: %+v", cs)
		}
		return []evaluator.Verdict{{CriterionID: "uber-go/error-wrap", Score: 0.6}, {CriterionID: "team/error-wrap", Score: 0.8}}, nil
	}}

	res, err := Run(t.Context(), Config{Evaluator: m, Tastes: []taste.Taste{a, b}, Functions: []extract.Function{fnA}})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	var got []string
	for _, f := range res.Findings {
		got = append(got, fmt.Sprintf("%s/%s %v %v", f.Ruleset, f.TasteID, f.Score, f.Passed))
	}
	want := []string{"uber-go/error-wrap 0.6 true", "team/error-wrap 0.8 false"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
	wantMeans := map[string]float64{"uber-go/error-wrap": 0.6, "team/error-wrap": 0.8}
	if !reflect.DeepEqual(res.MeanByTaste, wantMeans) {
		t.Errorf("MeanByTaste = %v, want %v", res.MeanByTaste, wantMeans)
	}
}
