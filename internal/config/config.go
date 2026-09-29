// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package config loads the tastecheck configuration file, which selects the
// rulesets to evaluate, overrides individual tastes per ruleset, and sets
// evaluator options.
package config

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/chez-shanpu/tastecheck/internal/ruleset"
	"github.com/chez-shanpu/tastecheck/internal/taste"
)

// SupportedVersion is the only configuration schema version understood.
const SupportedVersion = 1

// Config is a validated tastecheck configuration.
type Config struct {
	Rulesets  []RulesetRef
	Evaluator EvaluatorConfig
	// baseDir is the directory that relative ruleset paths resolve against.
	baseDir string
}

// RulesetNames returns the names of the selected rulesets in order.
func (c *Config) RulesetNames() []string {
	names := make([]string, 0, len(c.Rulesets))
	for _, ref := range c.Rulesets {
		names = append(names, ref.RulesetName())
	}
	return names
}

// Tastes loads the selected rulesets and returns their enabled tastes with
// overrides applied. Each taste carries its ruleset name, so tastes with the
// same ID in different rulesets stay distinct.
func (c *Config) Tastes() ([]taste.Taste, error) {
	var (
		tastes []taste.Taste
		errs   []error
	)
	for _, ref := range c.Rulesets {
		resolved, err := c.resolve(ref)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tastes = append(tastes, resolved...)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if len(tastes) == 0 {
		return nil, errors.New("no tastes enabled: every ruleset or taste is disabled")
	}
	return tastes, nil
}

func (c *Config) resolve(ref RulesetRef) ([]taste.Taste, error) {
	name := ref.RulesetName()
	loaded, err := c.load(ref)
	if err != nil {
		return nil, err
	}

	known := make(map[string]bool, len(loaded))
	for _, t := range loaded {
		known[t.ID] = true
	}
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(ref.Overrides)) {
		if !known[id] {
			errs = append(errs, fmt.Errorf("ruleset %s: override for unknown taste %q", name, id))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	tastes := make([]taste.Taste, 0, len(loaded))
	for _, t := range loaded {
		o := ref.Overrides[t.ID]
		if o.Disabled {
			continue
		}
		if o.Threshold != nil {
			t.Threshold = *o.Threshold
		}
		if o.Severity != "" {
			// Validated by RulesetRef.validate.
			t.Severity = taste.Severity(o.Severity)
		}
		t.Ruleset = name
		tastes = append(tastes, t)
	}
	return tastes, nil
}

func (c *Config) load(ref RulesetRef) ([]taste.Taste, error) {
	if ref.Builtin != "" {
		return ruleset.LoadBuiltin(ref.Builtin)
	}
	path := ref.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.baseDir, path)
	}
	return ruleset.LoadFile(path)
}

// RulesetRef selects one ruleset: either a builtin by name or a file by path.
type RulesetRef struct {
	Builtin string `yaml:"builtin"`
	Path    string `yaml:"path"`
	// Name overrides the ruleset name for path rulesets; it defaults to the
	// file name without its extension.
	Name string `yaml:"name"`
	// Disabled excludes a builtin ruleset, which is otherwise always selected.
	Disabled bool `yaml:"disabled"`
	// Overrides adjusts tastes of this ruleset, keyed by taste ID.
	Overrides map[string]TasteOverride `yaml:"overrides"`
}

func (ref *RulesetRef) validate() error {
	var errs []error
	switch {
	case ref.Builtin == "" && ref.Path == "":
		errs = append(errs, errors.New("one of builtin or path is required"))
	case ref.Builtin != "" && ref.Path != "":
		errs = append(errs, errors.New("builtin and path are mutually exclusive"))
	case ref.Builtin != "" && ref.Name != "":
		errs = append(errs, errors.New("name is only allowed for path rulesets"))
	case ref.Path != "" && ref.Disabled:
		errs = append(errs, errors.New("disabled is only allowed for builtin rulesets"))
	case ref.Builtin != "" && !slices.Contains(ruleset.Names(), ref.Builtin):
		errs = append(errs, fmt.Errorf("unknown builtin ruleset %q (available: %v)", ref.Builtin, ruleset.Names()))
	}
	for _, id := range slices.Sorted(maps.Keys(ref.Overrides)) {
		o := ref.Overrides[id]
		if t := o.Threshold; t != nil && (math.IsNaN(*t) || *t < 0 || *t > 1) {
			errs = append(errs, fmt.Errorf("overrides.%s.threshold must be within [0, 1], got %v", id, *t))
		}
		if o.Severity != "" {
			if _, err := taste.ParseSeverity(o.Severity); err != nil {
				errs = append(errs, fmt.Errorf("overrides.%s: %w", id, err))
			}
		}
	}
	return errors.Join(errs...)
}

// RulesetName returns the name that qualifies the ruleset's taste keys.
func (ref *RulesetRef) RulesetName() string {
	switch {
	case ref.Builtin != "":
		return ref.Builtin
	case ref.Name != "":
		return ref.Name
	default:
		base := filepath.Base(ref.Path)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
}

// TasteOverride adjusts one taste.
type TasteOverride struct {
	Disabled  bool     `yaml:"disabled"`
	Threshold *float64 `yaml:"threshold"`
	Severity  string   `yaml:"severity"`
}

// EvaluatorConfig holds evaluator settings. A nil field means "not set"; a
// non-nil field is set even when it points to a zero value.
type EvaluatorConfig struct {
	Backend     *string `yaml:"backend"`
	Model       *string `yaml:"model"`
	Concurrency *int    `yaml:"concurrency"`
	MaxRetries  *int    `yaml:"max_retries"`
	// MinConfidence is the confidence below which a finding under its
	// threshold is ignored; zero disables it.
	MinConfidence *float64 `yaml:"min_confidence"`
}

// Merge returns c with every field set in o taking precedence.
func (c EvaluatorConfig) Merge(o EvaluatorConfig) EvaluatorConfig {
	return EvaluatorConfig{
		Backend:       or(o.Backend, c.Backend),
		Model:         or(o.Model, c.Model),
		Concurrency:   or(o.Concurrency, c.Concurrency),
		MaxRetries:    or(o.MaxRetries, c.MaxRetries),
		MinConfidence: or(o.MinConfidence, c.MinConfidence),
	}
}

// or returns a when it is set, and b otherwise.
func or[T any](a, b *T) *T {
	if a != nil {
		return a
	}
	return b
}

type rawConfig struct {
	Version   int             `yaml:"version"`
	Rulesets  []RulesetRef    `yaml:"rulesets"`
	Evaluator EvaluatorConfig `yaml:"evaluator"`
}

func (raw *rawConfig) validate() error {
	var errs []error
	if raw.Version != SupportedVersion {
		errs = append(errs, fmt.Errorf("unsupported version %d (want %d)", raw.Version, SupportedVersion))
	}

	seen := make(map[string]bool, len(raw.Rulesets))
	for i, ref := range raw.Rulesets {
		if err := ref.validate(); err != nil {
			errs = append(errs, fmt.Errorf("rulesets[%d]: %w", i, err))
			continue
		}
		name := ref.RulesetName()
		// Builtin rulesets are selected even when not listed, so their names
		// are reserved for them.
		if ref.Path != "" && slices.Contains(ruleset.Names(), name) {
			errs = append(errs, fmt.Errorf("rulesets[%d]: ruleset name %q is reserved for a builtin ruleset (set a distinct name)", i, name))
			continue
		}
		if seen[name] {
			errs = append(errs, fmt.Errorf("rulesets[%d]: duplicate ruleset name %q (set a distinct name)", i, name))
		}
		seen[name] = true
	}

	if c := raw.Evaluator.Concurrency; c != nil && *c < 0 {
		errs = append(errs, fmt.Errorf("evaluator.concurrency must not be negative, got %d", *c))
	}
	if r := raw.Evaluator.MaxRetries; r != nil && *r < 0 {
		errs = append(errs, fmt.Errorf("evaluator.max_retries must not be negative, got %d", *r))
	}
	if m := raw.Evaluator.MinConfidence; m != nil && (math.IsNaN(*m) || *m < 0 || *m > 1) {
		errs = append(errs, fmt.Errorf("evaluator.min_confidence must be within [0, 1], got %v", *m))
	}
	return errors.Join(errs...)
}

// Default returns the configuration used when no configuration file exists:
// every builtin ruleset with no overrides.
func Default() *Config {
	return &Config{Rulesets: effectiveRulesets(nil)}
}

// effectiveRulesets returns the rulesets to evaluate: every builtin ruleset in
// name order, using the entry in refs when there is one and skipping disabled
// ones, followed by the path rulesets in refs in order. refs must be valid.
func effectiveRulesets(refs []RulesetRef) []RulesetRef {
	builtins := make(map[string]RulesetRef)
	var paths []RulesetRef
	for _, ref := range refs {
		if ref.Builtin != "" {
			builtins[ref.Builtin] = ref
		} else {
			paths = append(paths, ref)
		}
	}

	var out []RulesetRef
	for _, name := range ruleset.Names() {
		ref, ok := builtins[name]
		if !ok {
			ref = RulesetRef{Builtin: name}
		}
		if !ref.Disabled {
			out = append(out, ref)
		}
	}
	return append(out, paths...)
}

// Load reads and validates the configuration file at path.
// Relative ruleset paths are resolved against the file's directory.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	cfg, err := Parse(f, filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes and validates a configuration from r. Relative ruleset paths
// are resolved against baseDir. Unknown fields are rejected.
func Parse(r io.Reader, baseDir string) (*Config, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var raw rawConfig
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config file is empty")
		}
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := raw.validate(); err != nil {
		return nil, err
	}

	return &Config{
		Rulesets:  effectiveRulesets(raw.Rulesets),
		Evaluator: raw.Evaluator,
		baseDir:   baseDir,
	}, nil
}
