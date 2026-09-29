// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package ruleset loads rulesets, either builtin ones embedded in the binary
// or ones read from a file.
//
// A ruleset is a taste definition file. Builtin rulesets live under builtin/
// and are named after their file name without the extension.
package ruleset

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/chez-shanpu/tastecheck/internal/taste"
)

const builtinExt = ".yaml"

var (
	//go:embed builtin/*.yaml
	_builtinFS embed.FS

	// _names is computed at program initialization; the directory is
	// embedded at build time, so failing to read it is a build defect.
	_names = mustNames()
)

func mustNames() []string {
	entries, err := fs.ReadDir(_builtinFS, "builtin")
	if err != nil {
		panic(fmt.Sprintf("read embedded rulesets: %v", err))
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), builtinExt); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Names returns the builtin ruleset names in sorted order.
func Names() []string {
	return slices.Clone(_names)
}

// Source returns the YAML source of the named builtin ruleset.
func Source(name string) ([]byte, error) {
	if !slices.Contains(_names, name) {
		return nil, fmt.Errorf("unknown builtin ruleset %q (available: %v)", name, _names)
	}
	return _builtinFS.ReadFile(path.Join("builtin", name+builtinExt))
}

// LoadBuiltin parses and validates the named builtin ruleset. It returns an
// error for a name that is not in Names. The returned tastes have no Ruleset;
// callers that resolve rulesets set it.
func LoadBuiltin(name string) ([]taste.Taste, error) {
	src, err := Source(name)
	if err != nil {
		return nil, err
	}
	tastes, err := taste.Parse(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("builtin ruleset %s: %w", name, err)
	}
	return tastes, nil
}

// LoadFile reads, parses and validates the ruleset file at filename. The
// returned tastes have no Ruleset; callers that resolve rulesets set it.
func LoadFile(filename string) ([]taste.Taste, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open taste file: %w", err)
	}
	defer f.Close()

	tastes, err := taste.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	return tastes, nil
}
