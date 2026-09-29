// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package cmd

import (
	"github.com/spf13/cobra"
)

// rulesetCmd is a package-level variable so that its subcommands can add
// themselves to it.
var rulesetCmd = &cobra.Command{
	Use:   "ruleset",
	Short: "Inspect builtin rulesets",
}

func init() {
	rootCmd.AddCommand(rulesetCmd)
}
