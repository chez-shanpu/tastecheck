// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chez-shanpu/tastecheck/internal/ruleset"
)

func init() {
	rulesetCmd.AddCommand(newRulesetListCmd())
}

func newRulesetListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List builtin rulesets and their taste IDs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var b strings.Builder
			for _, name := range ruleset.Names() {
				tastes, err := ruleset.LoadBuiltin(name)
				if err != nil {
					return err
				}
				fmt.Fprintf(&b, "%s\n", name)
				for _, t := range tastes {
					fmt.Fprintf(&b, "  %s\n", t.ID)
				}
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}
