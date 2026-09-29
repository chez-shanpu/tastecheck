// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package cmd

import (
	"fmt"
	"math"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/chez-shanpu/tastecheck/internal/check"
	"github.com/chez-shanpu/tastecheck/internal/config"
	"github.com/chez-shanpu/tastecheck/internal/evaluator"
	_ "github.com/chez-shanpu/tastecheck/internal/evaluator/mock"
	"github.com/chez-shanpu/tastecheck/internal/evaluator/typesafeai"
	"github.com/chez-shanpu/tastecheck/internal/extract"
	"github.com/chez-shanpu/tastecheck/internal/report"
)

const (
	backendFlag       = "backend"
	modelFlag         = "model"
	concurrencyFlag   = "concurrency"
	maxRetriesFlag    = "max-retries"
	minConfidenceFlag = "min-confidence"
)

type checkOptions struct {
	config        string
	output        string
	backend       string
	model         string
	concurrency   int
	maxRetries    int
	minConfidence float64
	includeTests  bool
	verbose       bool
}

func init() {
	rootCmd.AddCommand(newCheckCmd())
}

func newCheckCmd() *cobra.Command {
	var opts checkOptions
	cmd := &cobra.Command{
		Use:   "check [paths...]",
		Short: "Evaluate Go functions against tastes",
		Long: `Evaluate every Go function in the given paths against the tastes of the
rulesets selected in the config file, and report the scores.

A path is a .go file, a directory, or a directory followed by "/..." (recursive).
Defaults to "./..." when no path is given.

Evaluator flags take precedence over the config file.

A taste with severity "warning" is reported when a function is below its
threshold, but does not fail the check.

Exit codes:
  0  every function meets every fail-severity taste threshold (warnings may
     be reported)
  1  at least one function is below a fail-severity taste threshold, or the
     check could not be completed (invalid config, no functions found, API
     failure, parse error, ...). The former prints the report ending with a
     "FAIL:" summary (or "passed": false in JSON) to stdout; the latter prints
     "Error: ..." to stderr.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{"./..."}
			}
			return runCheck(cmd, args, opts)
		},
	}

	addConfigFlag(cmd, &opts.config)
	f := cmd.Flags()
	f.StringVarP(&opts.output, "output", "o", string(report.FormatText), fmt.Sprintf("Output format %v", report.Formats))
	addEvaluatorFlags(f, &opts)
	f.BoolVar(&opts.includeTests, "include-tests", false, "Include _test.go files when expanding directories")
	f.BoolVarP(&opts.verbose, "verbose", "v", false, "Also list passing functions in text output")
	return cmd
}

// addEvaluatorFlags registers the evaluator flags. Their defaults are the
// defaults used when neither the flag nor the config file sets a value.
func addEvaluatorFlags(f *pflag.FlagSet, opts *checkOptions) {
	f.StringVar(&opts.backend, backendFlag, typesafeai.BackendName, fmt.Sprintf("Evaluator backend %v", evaluator.Backends()))
	f.StringVar(&opts.model, modelFlag, "", "Evaluator model (default: backend default)")
	f.IntVar(&opts.concurrency, concurrencyFlag, check.DefaultConcurrency, "Number of functions evaluated in parallel")
	f.IntVar(&opts.maxRetries, maxRetriesFlag, check.DefaultMaxRetries, "Retries for transient evaluator errors (0 disables)")
	f.Float64Var(&opts.minConfidence, minConfidenceFlag, 0, "Ignore findings below their threshold whose evaluator confidence is below this value (0 disables)")
}

func runCheck(cmd *cobra.Command, paths []string, opts checkOptions) error {
	format, err := report.ParseFormat(opts.output)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(cmd, opts.config)
	if err != nil {
		return err
	}
	tastes, err := cfg.Tastes()
	if err != nil {
		return err
	}
	funcs, err := extract.Extract(paths, extract.Options{IncludeTests: opts.includeTests})
	if err != nil {
		return err
	}
	// Treat an empty target as a tool error rather than a pass, so that a
	// mistyped path cannot end an agent loop as if the code were fine.
	if len(funcs) == 0 {
		return fmt.Errorf("no functions to check found in %v", paths)
	}

	// Precedence: flags given on the command line > config file > flag
	// defaults. Every field of ec is set because flagValues sets them all.
	flags := cmd.Flags()
	flagValues := config.EvaluatorConfig{Backend: &opts.backend, Model: &opts.model, Concurrency: &opts.concurrency, MaxRetries: &opts.maxRetries, MinConfidence: &opts.minConfidence}
	given := config.EvaluatorConfig{
		Backend:       ifChanged(flags, backendFlag, &opts.backend),
		Model:         ifChanged(flags, modelFlag, &opts.model),
		Concurrency:   ifChanged(flags, concurrencyFlag, &opts.concurrency),
		MaxRetries:    ifChanged(flags, maxRetriesFlag, &opts.maxRetries),
		MinConfidence: ifChanged(flags, minConfidenceFlag, &opts.minConfidence),
	}
	ec := flagValues.Merge(cfg.Evaluator).Merge(given)
	// The config file is validated on load, but the flag is not. A value
	// above 1 would ignore every finding below its threshold.
	if m := *ec.MinConfidence; math.IsNaN(m) || m < 0 || m > 1 {
		return fmt.Errorf("--%s must be within [0, 1], got %v", minConfidenceFlag, m)
	}

	ev, err := evaluator.New(*ec.Backend, evaluator.Options{Model: *ec.Model})
	if err != nil {
		return err
	}

	res, err := check.Run(cmd.Context(), check.Config{
		Evaluator:     ev,
		Tastes:        tastes,
		Functions:     funcs,
		Concurrency:   *ec.Concurrency,
		MaxRetries:    *ec.MaxRetries,
		MinConfidence: *ec.MinConfidence,
	})
	if err != nil {
		return err
	}

	if err := report.Write(cmd.OutOrStdout(), res, format, opts.verbose); err != nil {
		return err
	}
	if !res.Passed {
		return ErrTasteViolation
	}
	return nil
}

// ifChanged returns p when the named flag was given on the command line, and
// nil otherwise.
func ifChanged[T any](flags *pflag.FlagSet, name string, p *T) *T {
	if !flags.Changed(name) {
		return nil
	}
	return p
}
