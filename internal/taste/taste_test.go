// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package taste

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Taste
		wantErr string
	}{
		{
			name: "score taste",
			input: `
version: 1
tastes:
  - id: srp
    description: single responsibility
    levels: [bad, ok, good]
    threshold: 0.7
`,
			want: []Taste{{
				ID:          "srp",
				Description: "single responsibility",
				Levels:      []string{"bad", "ok", "good"},
				Threshold:   0.7,
				Severity:    SeverityFail,
			}},
		},
		{
			name: "two levels, zero threshold",
			input: `
version: 1
tastes:
  - id: srp
    description: d
    levels: [no, yes]
    threshold: 0
`,
			want: []Taste{{ID: "srp", Description: "d", Levels: []string{"no", "yes"}, Severity: SeverityFail}},
		},
		{
			name: "explicit severities",
			input: `
version: 1
tastes:
  - {id: a, description: d, levels: [no, yes], threshold: 0.5, severity: fail}
  - {id: b, description: d, levels: [no, yes], threshold: 0.5, severity: warning}
`,
			want: []Taste{
				{ID: "a", Description: "d", Levels: []string{"no", "yes"}, Threshold: 0.5, Severity: SeverityFail},
				{ID: "b", Description: "d", Levels: []string{"no", "yes"}, Threshold: 0.5, Severity: SeverityWarning},
			},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: "empty",
		},
		{
			name: "unknown field",
			input: `
version: 1
tastes:
  - id: srp
    description: d
    levels: [a, b]
    threshold: 0.5
    weight: 1.0
`,
			wantErr: "field weight not found",
		},
		{
			name: "unsupported version",
			input: `
version: 2
tastes:
  - {id: srp, description: d, levels: [no, yes], threshold: 0.5}
`,
			wantErr: "unsupported version 2",
		},
		{
			name:    "no tastes",
			input:   "version: 1\n",
			wantErr: "no tastes defined",
		},
		{
			name: "duplicate id",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [no, yes], threshold: 0.5}
  - {id: srp, description: d, levels: [no, yes], threshold: 0.5}
`,
			wantErr: `duplicate id "srp"`,
		},
		{
			name: "missing required fields",
			input: `
version: 1
tastes:
  - {levels: [no, yes]}
`,
			wantErr: "id is required",
		},
		{
			name: "missing threshold",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [no, yes]}
`,
			wantErr: "threshold is required",
		},
		{
			name: "threshold out of range",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [no, yes], threshold: 1.5}
`,
			wantErr: "threshold must be within [0, 1]",
		},
		{
			name: "threshold NaN",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [no, yes], threshold: .nan}
`,
			wantErr: "threshold must be within [0, 1], got NaN",
		},
		{
			name: "too few levels",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [a], threshold: 0.5}
`,
			wantErr: "needs 2 to 10 levels, got 1",
		},
		{
			name: "too many levels",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [a, b, c, d, e, f, g, h, i, j, k], threshold: 0.5}
`,
			wantErr: "needs 2 to 10 levels, got 11",
		},
		{
			name: "empty level",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [a, ""], threshold: 0.5}
`,
			wantErr: "levels[1] is empty",
		},
		{
			name: "type is no longer a field",
			input: `
version: 1
tastes:
  - {id: srp, description: d, type: score, levels: [a, b], threshold: 0.5}
`,
			wantErr: "field type not found",
		},
		{
			name: "unknown severity",
			input: `
version: 1
tastes:
  - {id: srp, description: d, levels: [no, yes], threshold: 0.5, severity: error}
`,
			wantErr: `srp: severity must be one of [fail warning], got "error"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.input))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestKey(t *testing.T) {
	tests := []struct {
		taste Taste
		want  string
	}{
		{taste: Taste{ID: "srp"}, want: "srp"},
		{taste: Taste{Ruleset: "uber-go", ID: "error-wrap"}, want: "uber-go/error-wrap"},
	}
	for _, tt := range tests {
		if got := tt.taste.Key(); got != tt.want {
			t.Errorf("Taste{Ruleset: %q, ID: %q}.Key() = %q, want %q", tt.taste.Ruleset, tt.taste.ID, got, tt.want)
		}
	}
}
