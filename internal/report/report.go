// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package report renders check results for humans (text) and agents (JSON).
package report

import (
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/chez-shanpu/tastecheck/internal/check"
	"github.com/chez-shanpu/tastecheck/internal/taste"
)

// Format is an output format.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Formats lists the supported output formats.
var Formats = []Format{FormatText, FormatJSON}

// ParseFormat validates s as a Format.
func ParseFormat(s string) (Format, error) {
	f := Format(s)
	if !slices.Contains(Formats, f) {
		return "", fmt.Errorf("unknown format %q (available: %v)", s, Formats)
	}
	return f, nil
}

// Write renders res to w in the given format. In text format, passing and
// low-confidence findings are listed only when verbose is set; JSON always has
// all findings.
func Write(w io.Writer, res *check.Result, format Format, verbose bool) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, res)
	case FormatText:
		return writeText(w, res, verbose)
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

type jsonReport struct {
	Passed  bool          `json:"passed"`
	Summary jsonSummary   `json:"summary"`
	Results []jsonFinding `json:"results"`
}

type jsonSummary struct {
	Functions     int                `json:"functions"`
	Failed        int                `json:"failed"`
	Warnings      int                `json:"warnings"`
	LowConfidence int                `json:"low_confidence"`
	Mean          float64            `json:"mean"`
	ByTaste       map[string]float64 `json:"by_taste"`
}

type jsonFinding struct {
	File          string   `json:"file"`
	Line          int      `json:"line"`
	Function      string   `json:"function"`
	Ruleset       string   `json:"ruleset,omitempty"`
	Taste         string   `json:"taste"`
	Score         float64  `json:"score"`
	Threshold     float64  `json:"threshold"`
	Severity      string   `json:"severity"`
	Passed        bool     `json:"passed"`
	LowConfidence bool     `json:"low_confidence,omitzero"`
	Confidence    *float64 `json:"confidence,omitzero"`
	Label         string   `json:"label,omitempty"`
	ImproveTo     string   `json:"improve_to,omitempty"`
}

func writeJSON(w io.Writer, res *check.Result) error {
	out := jsonReport{
		Passed: res.Passed,
		Summary: jsonSummary{
			Functions:     res.Functions,
			Failed:        res.Failed,
			Warnings:      res.Warnings,
			LowConfidence: res.LowConfidence,
			Mean:          res.Mean,
			ByTaste:       res.MeanByTaste,
		},
		Results: make([]jsonFinding, 0, len(res.Findings)),
	}
	for _, f := range res.Findings {
		out.Results = append(out.Results, jsonFinding{
			File:          f.File,
			Line:          f.Line,
			Function:      f.Function,
			Ruleset:       f.Ruleset,
			Taste:         f.TasteID,
			Score:         f.Score,
			Threshold:     f.Threshold,
			Severity:      string(severityOf(f)),
			Passed:        f.Passed,
			LowConfidence: f.LowConfidence,
			Confidence:    f.Confidence,
			Label:         f.Label,
			ImproveTo:     f.NextLabel,
		})
	}
	if err := json.MarshalWrite(w, out, json.Deterministic(true), jsontext.WithIndent("  ")); err != nil {
		return fmt.Errorf("write json report: %w", err)
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func writeText(w io.Writer, res *check.Result, verbose bool) error {
	var b strings.Builder

	// Failures first, then warnings, so that they are visible without
	// scrolling.
	order := []string{"FAIL", "WARN"}
	if verbose {
		order = append(order, "SKIP", "PASS")
	}
	for _, status := range order {
		for _, f := range res.Findings {
			if statusOf(f) == status {
				writeFinding(&b, status, f)
			}
		}
	}

	if len(res.Findings) > 0 {
		b.WriteString("\n")
	}
	status := "PASS"
	if !res.Passed {
		status = "FAIL"
	}
	fmt.Fprintf(&b, "%s: %d functions, %d failed findings, %d warnings", status, res.Functions, res.Failed, res.Warnings)
	if res.LowConfidence > 0 {
		fmt.Fprintf(&b, ", %d low-confidence findings ignored", res.LowConfidence)
	}
	fmt.Fprintf(&b, ", mean score %.2f\n", res.Mean)
	for _, id := range slices.Sorted(maps.Keys(res.MeanByTaste)) {
		fmt.Fprintf(&b, "  %s: mean %.2f\n", id, res.MeanByTaste[id])
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// statusOf returns the text status of f, matching how check.Run counts it.
func statusOf(f check.Finding) string {
	switch {
	case f.Passed:
		return "PASS"
	case f.LowConfidence:
		return "SKIP"
	case isWarning(f):
		return "WARN"
	default:
		return "FAIL"
	}
}

func writeFinding(b *strings.Builder, status string, f check.Finding) {
	fmt.Fprintf(b, "%s %s:%d %s [%s] score %.2f (threshold %.2f", status, f.File, f.Line, f.Function, tasteKey(f), f.Score, f.Threshold)
	if f.Confidence != nil {
		fmt.Fprintf(b, ", confidence %.2f", *f.Confidence)
	}
	b.WriteString(")\n")
	if f.Label != "" {
		fmt.Fprintf(b, "    current: %s\n", f.Label)
	}
	if !f.Passed && f.NextLabel != "" {
		fmt.Fprintf(b, "    improve to: %s\n", f.NextLabel)
	}
}

// severityOf returns the severity of f. An unset severity is reported as the
// default, matching how check.Run judges it.
func severityOf(f check.Finding) taste.Severity {
	return cmp.Or(f.Severity, taste.DefaultSeverity)
}

func isWarning(f check.Finding) bool {
	return f.Severity == taste.SeverityWarning
}

func tasteKey(f check.Finding) string {
	if f.Ruleset == "" {
		return f.TasteID
	}
	return f.Ruleset + "/" + f.TasteID
}
