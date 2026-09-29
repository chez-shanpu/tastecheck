// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package ruleset

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/chez-shanpu/tastecheck/internal/taste"
)

func TestBuiltinRulesetsAreValid(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("Names() returned no builtin rulesets")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tastes, err := LoadBuiltin(name)
			if err != nil {
				t.Fatalf("LoadBuiltin(%q) error: %v", name, err)
			}
			if len(tastes) == 0 {
				t.Fatalf("LoadBuiltin(%q) returned no tastes", name)
			}
			src, err := Source(name)
			if err != nil {
				t.Fatalf("Source(%q) error: %v", name, err)
			}
			missing, err := tastesWithoutSeverity(src)
			if err != nil {
				t.Fatalf("decode Source(%q): %v", name, err)
			}
			if len(missing) > 0 {
				t.Errorf("Source(%q) tastes without explicit severity = %v, want none", name, missing)
			}
			for _, tt := range tastes {
				// The best level must state that the rule may not apply, so
				// that unrelated functions are not failed.
				if best := tt.Levels[len(tt.Levels)-1]; !strings.Contains(best, "does not apply") {
					t.Errorf("%s/%s: best level %q does not cover the not-applicable case", name, tt.ID, best)
				}
			}
		})
	}
}

// tastesWithoutSeverity returns the IDs of the tastes in the ruleset source
// src that do not set severity. Builtin rulesets set it on every taste, so
// that the choice is deliberate rather than the default.
func tastesWithoutSeverity(src []byte) ([]string, error) {
	var raw struct {
		Tastes []struct {
			ID       string `yaml:"id"`
			Severity string `yaml:"severity"`
		} `yaml:"tastes"`
	}
	if err := yaml.NewDecoder(bytes.NewReader(src)).Decode(&raw); err != nil {
		return nil, err
	}
	var ids []string
	for _, rt := range raw.Tastes {
		if rt.Severity == "" {
			ids = append(ids, rt.ID)
		}
	}
	return ids, nil
}

func TestUberGo(t *testing.T) {
	tastes, err := LoadBuiltin("uber-go")
	if err != nil {
		t.Fatalf(`LoadBuiltin("uber-go") error: %v`, err)
	}

	var ids []string
	severities := make(map[string]taste.Severity, len(tastes))
	for _, tt := range tastes {
		ids = append(ids, tt.ID)
		severities[tt.ID] = tt.Severity
	}
	want := []string{
		"error-wrap",
		"error-handle-once",
		"no-panic-or-exit",
		"goroutine-lifecycle",
		"defer-cleanup",
		"reduce-nesting",
		"type-assertion",
		"container-copy",
		"time-types",
		"naked-parameters",
	}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf(`LoadBuiltin("uber-go") taste IDs = %v, want %v`, ids, want)
	}

	wantSeverities := map[string]taste.Severity{
		"error-wrap":          taste.SeverityFail,
		"error-handle-once":   taste.SeverityFail,
		"no-panic-or-exit":    taste.SeverityFail,
		"goroutine-lifecycle": taste.SeverityFail,
		"defer-cleanup":       taste.SeverityFail,
		"reduce-nesting":      taste.SeverityWarning,
		"type-assertion":      taste.SeverityFail,
		"container-copy":      taste.SeverityFail,
		"time-types":          taste.SeverityWarning,
		"naked-parameters":    taste.SeverityWarning,
	}
	if !reflect.DeepEqual(severities, wantSeverities) {
		t.Errorf(`LoadBuiltin("uber-go") severities = %v, want %v`, severities, wantSeverities)
	}
}

func TestUnknownRuleset(t *testing.T) {
	const wantMsg = `unknown builtin ruleset "nope"`
	_, err := LoadBuiltin("nope")
	switch {
	case err == nil:
		t.Errorf(`LoadBuiltin("nope") error = nil, want error containing %q`, wantMsg)
	case !strings.Contains(err.Error(), wantMsg):
		t.Errorf(`LoadBuiltin("nope") error = %q, want containing %q`, err, wantMsg)
	}

	if _, err := Source("nope"); err == nil {
		t.Error(`Source("nope") error = nil, want non-nil`)
	}
}

func TestNamesIsACopy(t *testing.T) {
	want := Names()
	Names()[0] = "mutated"
	if got := Names(); !slices.Equal(got, want) {
		t.Errorf("Names() = %v after mutating a previously returned slice, want %v", got, want)
	}
}

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tastes.yaml")
	src := "version: 1\ntastes:\n  - {id: srp, description: d, levels: [no, yes], threshold: 0.5}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile(%q) error: %v", path, err)
	}
	want := []taste.Taste{{ID: "srp", Description: "d", Levels: []string{"no", "yes"}, Threshold: 0.5, Severity: taste.DefaultSeverity}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadFile(%q) = %+v, want %+v", path, got, want)
	}
}

func TestLoadFileMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := LoadFile(path); err == nil {
		t.Errorf("LoadFile(%q) error = nil, want non-nil", path)
	}
}
