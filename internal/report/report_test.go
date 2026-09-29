// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package report

import (
	"bytes"
	"testing"

	"github.com/chez-shanpu/tastecheck/internal/check"
	"github.com/chez-shanpu/tastecheck/internal/taste"
)

var result = &check.Result{
	Passed:        false,
	Functions:     2,
	Failed:        1,
	Warnings:      2,
	LowConfidence: 1,
	Mean:          0.5,
	MeanByTaste:   map[string]float64{"uber-go/errors": 0.5, "uber-go/nesting": 0.25, "uber-go/srp": 0.625},
	Findings: []check.Finding{
		{
			File: "a.go", Line: 3, Function: "Mixed", Ruleset: "uber-go", TasteID: "srp",
			Score: 0.25, Threshold: 0.7, Severity: taste.SeverityFail, Passed: false, Confidence: new(0.9),
			Label: "two responsibilities", NextLabel: "mostly focused",
		},
		{
			File: "a.go", Line: 3, Function: "Mixed", Ruleset: "uber-go", TasteID: "nesting",
			Score: 0, Threshold: 0.7, Severity: taste.SeverityWarning, Passed: false,
			Label: "deeply nested", NextLabel: "flat",
		},
		{
			File: "a.go", Line: 10, Function: "(*T).Clean", Ruleset: "uber-go", TasteID: "srp",
			Score: 1, Threshold: 0.7, Severity: taste.SeverityFail, Passed: true, Confidence: new(0.95),
			Label: "single responsibility",
		},
		{
			File: "a.go", Line: 10, Function: "(*T).Clean", Ruleset: "uber-go", TasteID: "nesting",
			Score: 0.5, Threshold: 0.7, Severity: taste.SeverityWarning, Passed: false,
			Label: "one nested block", NextLabel: "flat",
		},
		{
			File: "a.go", Line: 10, Function: "(*T).Clean", Ruleset: "uber-go", TasteID: "errors",
			Score: 0.5, Threshold: 0.7, Severity: taste.SeverityFail, Passed: false, LowConfidence: true, Confidence: new(0.0),
			Label: "one unwrapped error", NextLabel: "wrapped",
		},
	},
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, result, FormatJSON, false); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	want := `{
  "passed": false,
  "summary": {
    "functions": 2,
    "failed": 1,
    "warnings": 2,
    "low_confidence": 1,
    "mean": 0.5,
    "by_taste": {
      "uber-go/errors": 0.5,
      "uber-go/nesting": 0.25,
      "uber-go/srp": 0.625
    }
  },
  "results": [
    {
      "file": "a.go",
      "line": 3,
      "function": "Mixed",
      "ruleset": "uber-go",
      "taste": "srp",
      "score": 0.25,
      "threshold": 0.7,
      "severity": "fail",
      "passed": false,
      "confidence": 0.9,
      "label": "two responsibilities",
      "improve_to": "mostly focused"
    },
    {
      "file": "a.go",
      "line": 3,
      "function": "Mixed",
      "ruleset": "uber-go",
      "taste": "nesting",
      "score": 0,
      "threshold": 0.7,
      "severity": "warning",
      "passed": false,
      "label": "deeply nested",
      "improve_to": "flat"
    },
    {
      "file": "a.go",
      "line": 10,
      "function": "(*T).Clean",
      "ruleset": "uber-go",
      "taste": "srp",
      "score": 1,
      "threshold": 0.7,
      "severity": "fail",
      "passed": true,
      "confidence": 0.95,
      "label": "single responsibility"
    },
    {
      "file": "a.go",
      "line": 10,
      "function": "(*T).Clean",
      "ruleset": "uber-go",
      "taste": "nesting",
      "score": 0.5,
      "threshold": 0.7,
      "severity": "warning",
      "passed": false,
      "label": "one nested block",
      "improve_to": "flat"
    },
    {
      "file": "a.go",
      "line": 10,
      "function": "(*T).Clean",
      "ruleset": "uber-go",
      "taste": "errors",
      "score": 0.5,
      "threshold": 0.7,
      "severity": "fail",
      "passed": false,
      "low_confidence": true,
      "confidence": 0,
      "label": "one unwrapped error",
      "improve_to": "wrapped"
    }
  ]
}
`
	if got := buf.String(); got != want {
		t.Errorf("JSON output =\n%s\nwant\n%s", got, want)
	}
}

func TestWriteJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	empty := &check.Result{Passed: true, MeanByTaste: map[string]float64{}}
	if err := Write(&buf, empty, FormatJSON, false); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"results": []`)) {
		t.Errorf("empty results must be an array, got:\n%s", buf.String())
	}
}

func TestWriteText(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
		want    string
	}{
		{
			name: "failures only",
			want: `FAIL a.go:3 Mixed [uber-go/srp] score 0.25 (threshold 0.70, confidence 0.90)
    current: two responsibilities
    improve to: mostly focused
WARN a.go:3 Mixed [uber-go/nesting] score 0.00 (threshold 0.70)
    current: deeply nested
    improve to: flat
WARN a.go:10 (*T).Clean [uber-go/nesting] score 0.50 (threshold 0.70)
    current: one nested block
    improve to: flat

FAIL: 2 functions, 1 failed findings, 2 warnings, 1 low-confidence findings ignored, mean score 0.50
  uber-go/errors: mean 0.50
  uber-go/nesting: mean 0.25
  uber-go/srp: mean 0.62
`,
		},
		{
			name:    "verbose",
			verbose: true,
			want: `FAIL a.go:3 Mixed [uber-go/srp] score 0.25 (threshold 0.70, confidence 0.90)
    current: two responsibilities
    improve to: mostly focused
WARN a.go:3 Mixed [uber-go/nesting] score 0.00 (threshold 0.70)
    current: deeply nested
    improve to: flat
WARN a.go:10 (*T).Clean [uber-go/nesting] score 0.50 (threshold 0.70)
    current: one nested block
    improve to: flat
SKIP a.go:10 (*T).Clean [uber-go/errors] score 0.50 (threshold 0.70, confidence 0.00)
    current: one unwrapped error
    improve to: wrapped
PASS a.go:10 (*T).Clean [uber-go/srp] score 1.00 (threshold 0.70, confidence 0.95)
    current: single responsibility

FAIL: 2 functions, 1 failed findings, 2 warnings, 1 low-confidence findings ignored, mean score 0.50
  uber-go/errors: mean 0.50
  uber-go/nesting: mean 0.25
  uber-go/srp: mean 0.62
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(&buf, result, FormatText, tt.verbose); err != nil {
				t.Fatalf("Write() error: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("text output =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	if f, err := ParseFormat("json"); err != nil || f != FormatJSON {
		t.Errorf("ParseFormat(json) = %q, %v", f, err)
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Error("ParseFormat(xml) expected error")
	}
}
