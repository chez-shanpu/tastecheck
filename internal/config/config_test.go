// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chez-shanpu/tastecheck/internal/ruleset"
	"github.com/chez-shanpu/tastecheck/internal/taste"
)

// teamRuleset shares the error-wrap ID with the builtin uber-go ruleset.
const teamRuleset = `version: 1
tastes:
  - {id: error-wrap, description: team rule, levels: [no, yes], threshold: 0.5}
  - {id: small, description: small functions, levels: [no, yes], threshold: 0.6}
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// keys returns the key of each resolved taste.
func keys(t *testing.T, c *Config) []string {
	t.Helper()
	tastes, err := c.Tastes()
	if err != nil {
		t.Fatalf("Tastes() error: %v", err)
	}
	out := make([]string, 0, len(tastes))
	for _, tt := range tastes {
		out = append(out, tt.Key())
	}
	return out
}

// builtinKeys returns the keys of the named builtin ruleset's tastes, except
// the given taste IDs.
func builtinKeys(t *testing.T, name string, except ...string) []string {
	t.Helper()
	tastes, err := ruleset.LoadBuiltin(name)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tt := range tastes {
		if !slices.Contains(except, tt.ID) {
			out = append(out, name+"/"+tt.ID)
		}
	}
	return out
}

// allBuiltinKeys returns the keys of every builtin ruleset's tastes, except
// the given taste IDs of uber-go.
func allBuiltinKeys(t *testing.T, uberGoExcept ...string) []string {
	t.Helper()
	var out []string
	for _, name := range ruleset.Names() {
		if name == "uber-go" {
			out = append(out, builtinKeys(t, name, uberGoExcept...)...)
		} else {
			out = append(out, builtinKeys(t, name)...)
		}
	}
	return out
}

// namesWith returns every builtin ruleset name except the excluded ones,
// followed by extra.
func namesWith(excluded []string, extra ...string) []string {
	var out []string
	for _, name := range ruleset.Names() {
		if !slices.Contains(excluded, name) {
			out = append(out, name)
		}
	}
	return append(out, extra...)
}

// disableAllBuiltins returns rulesets entries that disable every builtin.
func disableAllBuiltins() string {
	var b strings.Builder
	for _, name := range ruleset.Names() {
		fmt.Fprintf(&b, "  - {builtin: %s, disabled: true}\n", name)
	}
	return b.String()
}

func TestDefault(t *testing.T) {
	if got, want := keys(t, Default()), allBuiltinKeys(t); !reflect.DeepEqual(got, want) {
		t.Errorf("Default() tastes = %v, want %v", got, want)
	}
}

func TestLoadResolvesRulesets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rules/team.yaml", teamRuleset)
	path := writeFile(t, dir, ".tastecheck.yaml", `version: 1
rulesets:
  - builtin: uber-go
    overrides:
      goroutine-lifecycle: {disabled: true}
      error-wrap: {threshold: 0.8, severity: warning}
  - path: rules/team.yaml
    overrides:
      error-wrap: {threshold: 0.9}
      small: {disabled: true}
evaluator:
  backend: mock
  model: m
  concurrency: 2
  max_retries: 0
  min_confidence: 0.3
`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got, want := c.RulesetNames(), namesWith(nil, "team"); !reflect.DeepEqual(got, want) {
		t.Errorf("RulesetNames() = %v, want %v", got, want)
	}
	if e := c.Evaluator; e.Backend == nil || *e.Backend != "mock" || e.Model == nil || *e.Model != "m" ||
		e.Concurrency == nil || *e.Concurrency != 2 || e.MaxRetries == nil || *e.MaxRetries != 0 ||
		e.MinConfidence == nil || *e.MinConfidence != 0.3 {
		t.Errorf("Evaluator = %+v", c.Evaluator)
	}

	tastes, err := c.Tastes()
	if err != nil {
		t.Fatalf("Tastes() error: %v", err)
	}
	thresholds := make(map[string]float64, len(tastes))
	severities := make(map[string]taste.Severity, len(tastes))
	var got []string
	for _, tt := range tastes {
		got = append(got, tt.Key())
		thresholds[tt.Key()] = tt.Threshold
		severities[tt.Key()] = tt.Severity
	}
	want := append(allBuiltinKeys(t, "goroutine-lifecycle"), "team/error-wrap")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tastes = %v, want %v", got, want)
	}
	// The same ID in different rulesets is overridden independently.
	if thresholds["uber-go/error-wrap"] != 0.8 || thresholds["team/error-wrap"] != 0.9 {
		t.Errorf("thresholds = %v", thresholds)
	}
	if thresholds["uber-go/reduce-nesting"] != 0.7 {
		t.Errorf("non-overridden threshold = %v, want 0.7", thresholds["uber-go/reduce-nesting"])
	}
	if severities["uber-go/error-wrap"] != taste.SeverityWarning || severities["team/error-wrap"] != taste.SeverityFail {
		t.Errorf("severities = %v", severities)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNames []string
		wantErr   string
	}{
		{
			name:      "rulesets omitted selects every builtin",
			input:     "version: 1\n",
			wantNames: ruleset.Names(),
		},
		{
			name: "explicit path name",
			input: `version: 1
rulesets:
  - {path: a/rules.yaml, name: team}
  - {path: b/rules.yaml}
`,
			wantNames: namesWith(nil, "team", "rules"),
		},
		{
			name:      "builtin listed only for overrides is selected once",
			input:     "version: 1\nrulesets:\n  - builtin: uber-go\n    overrides:\n      error-wrap: {threshold: 0.8}\n",
			wantNames: ruleset.Names(),
		},
		{
			name:      "disabled builtin",
			input:     "version: 1\nrulesets:\n  - {builtin: uber-go, disabled: true}\n",
			wantNames: namesWith([]string{"uber-go"}),
		},
		{
			name:      "disabled builtin with overrides",
			input:     "version: 1\nrulesets:\n  - builtin: uber-go\n    disabled: true\n    overrides:\n      no-such-taste: {disabled: true}\n",
			wantNames: namesWith([]string{"uber-go"}),
		},
		{
			name:      "every builtin disabled",
			input:     "version: 1\nrulesets:\n" + disableAllBuiltins() + "  - {path: a/rules.yaml}\n",
			wantNames: []string{"rules"},
		},
		{name: "empty", input: "", wantErr: "config file is empty"},
		{name: "unsupported version", input: "version: 2\n", wantErr: "unsupported version 2"},
		{name: "unknown field", input: "version: 1\nruleset: []\n", wantErr: "field ruleset not found"},
		{
			name:    "unknown override field",
			input:   "version: 1\nrulesets:\n  - builtin: uber-go\n    overrides:\n      error-wrap: {enabled: false}\n",
			wantErr: "field enabled not found",
		},
		{name: "neither builtin nor path", input: "version: 1\nrulesets:\n  - {name: x}\n", wantErr: "one of builtin or path is required"},
		{
			name:    "both builtin and path",
			input:   "version: 1\nrulesets:\n  - {builtin: uber-go, path: a.yaml}\n",
			wantErr: "mutually exclusive",
		},
		{
			name:    "name on builtin",
			input:   "version: 1\nrulesets:\n  - {builtin: uber-go, name: x}\n",
			wantErr: "name is only allowed for path rulesets",
		},
		{
			name:    "disabled on path",
			input:   "version: 1\nrulesets:\n  - {path: a.yaml, disabled: true}\n",
			wantErr: "disabled is only allowed for builtin rulesets",
		},
		{
			name:    "unknown builtin",
			input:   "version: 1\nrulesets:\n  - builtin: nope\n",
			wantErr: `unknown builtin ruleset "nope"`,
		},
		{
			name:    "duplicate builtin",
			input:   "version: 1\nrulesets:\n  - {builtin: uber-go}\n  - {builtin: uber-go}\n",
			wantErr: `duplicate ruleset name "uber-go"`,
		},
		{
			name:    "duplicate path name",
			input:   "version: 1\nrulesets:\n  - {path: a/rules.yaml}\n  - {path: b/rules.yaml}\n",
			wantErr: `duplicate ruleset name "rules"`,
		},
		{
			name:    "path file name reserved by builtin",
			input:   "version: 1\nrulesets:\n  - {path: a/uber-go.yaml}\n",
			wantErr: `ruleset name "uber-go" is reserved for a builtin ruleset`,
		},
		{
			name:    "path name reserved by disabled builtin",
			input:   "version: 1\nrulesets:\n  - {builtin: uber-go, disabled: true}\n  - {path: a.yaml, name: uber-go}\n",
			wantErr: `ruleset name "uber-go" is reserved for a builtin ruleset`,
		},
		{
			name:    "override threshold out of range",
			input:   "version: 1\nrulesets:\n  - builtin: uber-go\n    overrides:\n      error-wrap: {threshold: 2}\n",
			wantErr: "overrides.error-wrap.threshold must be within [0, 1], got 2",
		},
		{
			name:    "override threshold NaN",
			input:   "version: 1\nrulesets:\n  - builtin: uber-go\n    overrides:\n      error-wrap: {threshold: .nan}\n",
			wantErr: "overrides.error-wrap.threshold must be within [0, 1], got NaN",
		},
		{
			name:    "override unknown severity",
			input:   "version: 1\nrulesets:\n  - builtin: uber-go\n    overrides:\n      error-wrap: {severity: info}\n",
			wantErr: `overrides.error-wrap: severity must be one of [fail warning], got "info"`,
		},
		{
			name:    "negative concurrency",
			input:   "version: 1\nevaluator: {concurrency: -1}\n",
			wantErr: "evaluator.concurrency must not be negative",
		},
		{
			name:    "negative max retries",
			input:   "version: 1\nevaluator: {max_retries: -1}\n",
			wantErr: "evaluator.max_retries must not be negative",
		},
		{
			name:    "min confidence above one",
			input:   "version: 1\nevaluator: {min_confidence: 1.5}\n",
			wantErr: "evaluator.min_confidence must be within [0, 1], got 1.5",
		},
		{
			name:    "negative min confidence",
			input:   "version: 1\nevaluator: {min_confidence: -0.1}\n",
			wantErr: "evaluator.min_confidence must be within [0, 1], got -0.1",
		},
		{
			name:    "NaN min confidence",
			input:   "version: 1\nevaluator: {min_confidence: .nan}\n",
			wantErr: "evaluator.min_confidence must be within [0, 1], got NaN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse(strings.NewReader(tt.input), ".")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got := c.RulesetNames(); !reflect.DeepEqual(got, tt.wantNames) {
				t.Errorf("RulesetNames() = %v, want %v", got, tt.wantNames)
			}
		})
	}
}

func TestTastesErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "team.yaml", teamRuleset)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "override for taste of another ruleset",
			input:   "version: 1\nrulesets:\n  - path: team.yaml\n    overrides:\n      reduce-nesting: {disabled: true}\n",
			wantErr: `ruleset team: override for unknown taste "reduce-nesting"`,
		},
		{
			name:    "missing path",
			input:   "version: 1\nrulesets:\n  - path: missing.yaml\n",
			wantErr: "open taste file",
		},
		{
			name: "all tastes disabled",
			input: "version: 1\nrulesets:\n" + disableAllBuiltins() +
				"  - path: team.yaml\n    overrides:\n      error-wrap: {disabled: true}\n      small: {disabled: true}\n",
			wantErr: "no tastes enabled",
		},
		{
			name:    "every builtin disabled and no path",
			input:   "version: 1\nrulesets:\n" + disableAllBuiltins(),
			wantErr: "no tastes enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse(strings.NewReader(tt.input), dir)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if _, err := c.Tastes(); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Tastes() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("Load() expected error")
	}
}

func TestEvaluatorConfigMerge(t *testing.T) {
	zero, eight := 0, 8
	empty, mock, x := "", "mock", "x"
	noConfidence, halfConfidence := 0.0, 0.5
	fromConfig := EvaluatorConfig{Backend: &mock, Model: &mock, Concurrency: &eight, MaxRetries: &zero, MinConfidence: &halfConfidence}

	tests := []struct {
		name string
		c, o EvaluatorConfig
		want EvaluatorConfig
	}{
		{
			name: "both unset",
		},
		{
			name: "unset fields keep c, including zero values",
			c:    fromConfig,
			want: fromConfig,
		},
		{
			name: "set fields in o take precedence",
			c:    fromConfig,
			o:    EvaluatorConfig{Backend: &x, MaxRetries: &eight},
			want: EvaluatorConfig{Backend: &x, Model: &mock, Concurrency: &eight, MaxRetries: &eight, MinConfidence: &halfConfidence},
		},
		{
			name: "zero values in o take precedence",
			c:    fromConfig,
			o:    EvaluatorConfig{Model: &empty, Concurrency: &zero, MinConfidence: &noConfidence},
			want: EvaluatorConfig{Backend: &mock, Model: &empty, Concurrency: &zero, MaxRetries: &zero, MinConfidence: &noConfidence},
		},
		{
			name: "set fields in o fill unset fields in c",
			o:    EvaluatorConfig{Backend: &x},
			want: EvaluatorConfig{Backend: &x},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Merge(tt.o); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Merge() = %s, want %s", formatEvaluatorConfig(got), formatEvaluatorConfig(tt.want))
			}
		})
	}
}

// formatEvaluatorConfig formats c with the pointed-to values.
func formatEvaluatorConfig(c EvaluatorConfig) string {
	return fmt.Sprintf("{Backend:%s Model:%s Concurrency:%s MaxRetries:%s MinConfidence:%s}",
		deref(c.Backend), deref(c.Model), deref(c.Concurrency), deref(c.MaxRetries), deref(c.MinConfidence))
}

func deref[T any](p *T) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprint(*p)
}
