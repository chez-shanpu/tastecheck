// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package cmd

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/chez-shanpu/tastecheck/internal/config"
)

const (
	ConfigFlag      = "config"
	ConfigShortFlag = "c"
	defaultConfig   = ".tastecheck.yaml"
)

func addConfigFlag(cmd *cobra.Command, path *string) {
	cmd.Flags().StringVarP(path, ConfigFlag, ConfigShortFlag, defaultConfig,
		"Path to the config file (every builtin ruleset is used when the default file does not exist)")
}

// loadConfig loads the config file at path. When the path was not given
// explicitly and the default file does not exist, it falls back to the
// default configuration and says so on stderr.
func loadConfig(cmd *cobra.Command, path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	// Note: err == nil, not !=. The success path returns first so that the
	// rest of the function handles only the fallback for a missing default
	// file.
	if err == nil {
		return cfg, nil
	}
	if cmd.Flags().Changed(ConfigFlag) || !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "no %s found; using every builtin ruleset\n", path)
	return config.Default(), nil
}
