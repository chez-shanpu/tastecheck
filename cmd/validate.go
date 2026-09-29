// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newValidateCmd())
}

func newValidateCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the config file and its rulesets without calling the evaluator",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd, configPath)
			if err != nil {
				return err
			}
			tastes, err := cfg.Tastes()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d tastes enabled from rulesets %v\n", len(tastes), cfg.RulesetNames())
			return nil
		},
	}
	addConfigFlag(cmd, &configPath)
	return cmd
}
