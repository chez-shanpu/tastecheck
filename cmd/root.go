// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version information. These are set via ldflags during build.
	version = "dev"
	commit  = "none"
)

// ErrTasteViolation is returned when at least one function is below the
// threshold of a fail-severity taste.
var ErrTasteViolation = errors.New("taste threshold not met")

var rootCmd = &cobra.Command{
	Use:           "tastecheck",
	Short:         "Score Go functions against taste definitions with an LLM evaluator",
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:       version,
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("tastecheck version %s (commit: %s)\n", version, commit))
}

// Execute runs the root command. main.main maps the returned error to an
// exit code, so that the program exits only from main.
func Execute() error {
	return rootCmd.Execute()
}
