// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package taste parses and validates taste definitions written in YAML.
//
// A taste is a code-quality property (e.g. "each function has a single
// responsibility") that an evaluator scores on a normalized [0, 1] scale,
// where 1 is best. The schema is provider-neutral so that the evaluator
// backend can be swapped without touching rule files.
package taste

import (
	"errors"
	"fmt"
	"io"
	"math"
	"slices"

	"go.yaml.in/yaml/v3"
)

// SupportedVersion is the only schema version this package understands.
const SupportedVersion = 1

// Level count limits for tastes.
const (
	MinLevels = 2
	MaxLevels = 10
)

// Severity selects how a taste below its threshold affects the outcome.
type Severity string

const (
	// SeverityFail fails the check when the taste is below its threshold.
	SeverityFail Severity = "fail"
	// SeverityWarning reports the taste below its threshold without failing
	// the check.
	SeverityWarning Severity = "warning"
)

// DefaultSeverity is used when a taste does not set a severity.
const DefaultSeverity = SeverityFail

// Severities lists the supported severities.
var Severities = []Severity{SeverityFail, SeverityWarning}

// ParseSeverity validates s as a Severity.
func ParseSeverity(s string) (Severity, error) {
	sev := Severity(s)
	if !slices.Contains(Severities, sev) {
		return "", fmt.Errorf("severity must be one of %v, got %q", Severities, s)
	}
	return sev, nil
}

// Taste is a validated taste definition.
type Taste struct {
	// Ruleset is the name of the ruleset the taste belongs to. It is set by
	// whoever resolves rulesets, not by the taste file itself.
	Ruleset     string
	ID          string
	Description string
	// Levels are the possible outcomes, ordered from worst to best.
	// A yes/no taste has two levels.
	Levels []string
	// Threshold is the minimum normalized score in [0, 1] required to pass.
	Threshold float64
	// Severity decides whether a score below Threshold fails the check.
	Severity Severity
}

// Key identifies the taste uniquely across rulesets as "<ruleset>/<id>",
// or just the ID when the taste belongs to no ruleset.
func (t Taste) Key() string {
	if t.Ruleset == "" {
		return t.ID
	}
	return t.Ruleset + "/" + t.ID
}

// Parse decodes and validates taste definitions from r.
// Unknown fields are rejected so that typos in rule files fail loudly.
// Empty input is an error, and all validation errors are reported together
// as a joined error. The returned tastes have no Ruleset; callers that
// resolve rulesets set it.
func Parse(r io.Reader) ([]Taste, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var raw rawFile
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("taste file is empty")
		}
		return nil, fmt.Errorf("decode taste file: %w", err)
	}
	return raw.validate()
}

type rawFile struct {
	Version int        `yaml:"version"`
	Tastes  []rawTaste `yaml:"tastes"`
}

func (f *rawFile) validate() ([]Taste, error) {
	var errs []error
	if f.Version != SupportedVersion {
		errs = append(errs, fmt.Errorf("unsupported version %d (want %d)", f.Version, SupportedVersion))
	}
	if len(f.Tastes) == 0 {
		errs = append(errs, errors.New("no tastes defined"))
	}

	seen := make(map[string]bool, len(f.Tastes))
	tastes := make([]Taste, 0, len(f.Tastes))
	for i := range f.Tastes {
		rt := &f.Tastes[i]
		if rt.ID != "" {
			if seen[rt.ID] {
				errs = append(errs, fmt.Errorf("tastes[%d]: duplicate id %q", i, rt.ID))
			}
			seen[rt.ID] = true
		}
		t, err := rt.validate()
		if err != nil {
			errs = append(errs, fmt.Errorf("tastes[%d]: %w", i, err))
			continue
		}
		tastes = append(tastes, t)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return tastes, nil
}

type rawTaste struct {
	ID          string   `yaml:"id"`
	Description string   `yaml:"description"`
	Levels      []string `yaml:"levels"`
	Threshold   *float64 `yaml:"threshold"`
	Severity    string   `yaml:"severity"`
}

func (rt *rawTaste) validate() (Taste, error) {
	var errs []error
	if rt.ID == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if rt.Description == "" {
		errs = append(errs, errors.New("description is required"))
	}

	if n := len(rt.Levels); n < MinLevels || n > MaxLevels {
		errs = append(errs, fmt.Errorf("taste needs %d to %d levels, got %d", MinLevels, MaxLevels, n))
	}
	for i, l := range rt.Levels {
		if l == "" {
			errs = append(errs, fmt.Errorf("levels[%d] is empty", i))
		}
	}

	switch {
	case rt.Threshold == nil:
		errs = append(errs, errors.New("threshold is required"))
	case math.IsNaN(*rt.Threshold) || *rt.Threshold < 0 || *rt.Threshold > 1:
		errs = append(errs, fmt.Errorf("threshold must be within [0, 1], got %v", *rt.Threshold))
	}

	severity := DefaultSeverity
	if rt.Severity != "" {
		sev, err := ParseSeverity(rt.Severity)
		if err != nil {
			errs = append(errs, err)
		} else {
			severity = sev
		}
	}

	if err := errors.Join(errs...); err != nil {
		if rt.ID != "" {
			err = fmt.Errorf("%s: %w", rt.ID, err)
		}
		return Taste{}, err
	}

	return Taste{
		ID:          rt.ID,
		Description: rt.Description,
		Levels:      slices.Clone(rt.Levels),
		Threshold:   *rt.Threshold,
		Severity:    severity,
	}, nil
}
