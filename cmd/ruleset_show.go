// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/chez-shanpu/tastecheck/internal/ruleset"
)

func init() {
	rulesetCmd.AddCommand(newRulesetShowCmd())
}

func newRulesetShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Print the YAML definition of a builtin ruleset",
		Long: `Print the YAML definition of a builtin ruleset.

The output is a valid ruleset file, so it can be saved and customized, then
referenced from the config file with "path:".`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: ruleset.Names(),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := ruleset.Source(args[0])
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(src)
			return err
		},
	}
}
