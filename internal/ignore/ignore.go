// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package ignore matches file paths against ignore patterns, which exclude
// files and directories from evaluation.
//
// A pattern is a slash-separated path relative to a base directory, in which
// each element may use the syntax of path.Match. A pattern matches a file when
// it matches the file's path relative to the base directory or any of that
// path's parent directories, so a pattern naming a directory ignores
// everything under it. Patterns are anchored to the base directory.
package ignore

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Matcher matches file paths against ignore patterns. A nil Matcher matches
// nothing.
type Matcher struct {
	// baseDir is the absolute directory that patterns are relative to.
	baseDir string
	// workDir is the absolute current directory when the Matcher was
	// created, against which relative files are resolved.
	workDir  string
	patterns []string
}

// New returns a Matcher for patterns relative to baseDir, which may itself be
// relative to the current directory. Patterns are cleaned, so "./a/" is
// equivalent to "a". It returns the errors of every invalid pattern (see
// Validate) joined.
func New(baseDir string, patterns []string) (*Matcher, error) {
	var errs []error
	cleaned := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if err := Validate(p); err != nil {
			errs = append(errs, err)
			continue
		}
		cleaned = append(cleaned, path.Clean(p))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	workDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current directory: %w", err)
	}
	return &Matcher{baseDir: absPath(workDir, baseDir), workDir: workDir, patterns: cleaned}, nil
}

// Match reports whether file, or one of its parent directories, matches a
// pattern. A relative file is resolved against the current directory at the
// time New was called. The base directory itself and files outside it never
// match. Paths are compared lexically; symbolic links are not resolved.
func (m *Matcher) Match(file string) bool {
	if m == nil || len(m.patterns) == 0 {
		return false
	}
	rel, err := filepath.Rel(m.baseDir, absPath(m.workDir, file))
	if err != nil {
		// Rel fails only when the file cannot be made relative to the base
		// directory (e.g. it is on another volume), so the file is outside
		// the base directory and is not ignored.
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}

	// Match the parent directories before the file itself, so that a
	// pattern naming a directory ignores everything under it.
	var prefix string
	for elem := range strings.SplitSeq(rel, "/") {
		prefix = path.Join(prefix, elem)
		if m.matchPath(prefix) {
			return true
		}
	}
	return false
}

// matchPath reports whether p, a slash-separated path relative to the base
// directory, matches a pattern itself.
func (m *Matcher) matchPath(p string) bool {
	for _, pattern := range m.patterns {
		// New validated every cleaned pattern, and path.Match returns an
		// error only for a malformed pattern.
		if ok, _ := path.Match(pattern, p); ok {
			return true
		}
	}
	return false
}

// Validate reports whether pattern is a valid ignore pattern: a non-empty,
// relative path pattern that names something below the base directory. The
// pattern is checked after cleaning, which is the form New matches with.
func Validate(pattern string) error {
	switch {
	case pattern == "":
		return errors.New("ignore pattern must not be empty")
	case path.IsAbs(pattern) || filepath.IsAbs(pattern):
		return fmt.Errorf("ignore pattern %q must be a relative path", pattern)
	}
	c := path.Clean(pattern)
	switch {
	case c == ".":
		// Match never ignores the base directory itself, so such a pattern
		// would silently match nothing.
		return fmt.Errorf("ignore pattern %q must not refer to its base directory itself", pattern)
	case c == ".." || strings.HasPrefix(c, "../"):
		return fmt.Errorf("ignore pattern %q must not refer outside its base directory", pattern)
	}
	// path.Match checks the syntax of the whole pattern even when the name
	// does not match, so matching against "" only validates it.
	if _, err := path.Match(c, ""); err != nil {
		return fmt.Errorf("ignore pattern %q: %w", pattern, err)
	}
	return nil
}

// Any returns a function that reports whether a file matches any of ms, in
// the form of extract.Options.Ignore. Nil matchers match nothing.
func Any(ms ...*Matcher) func(file string) bool {
	return func(file string) bool {
		for _, m := range ms {
			if m.Match(file) {
				return true
			}
		}
		return false
	}
}

// absPath returns p as an absolute path, resolving a relative p against
// workDir.
func absPath(workDir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(workDir, p)
}
