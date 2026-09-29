// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

// Package extract collects function declarations from Go source files.
package extract

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Function is a single function or method declaration with its source text.
type Function struct {
	File string
	// StartLine is the line of the func keyword; EndLine is the line of the closing brace.
	StartLine int
	EndLine   int
	// Name is "Func" for functions and "(T).Method" or "(*T).Method" for methods.
	Name string
	// Source is the declaration source, including its doc comment.
	Source string
}

// Options controls which files are collected.
type Options struct {
	// IncludeTests includes _test.go files found by expanding directories.
	IncludeTests bool
}

// Extract returns the functions declared in the Go files matched by patterns,
// sorted by file and line.
//
// A pattern is a .go file, a directory (its .go files, non-recursive), or a
// directory followed by "/..." (recursive). Directory expansion skips vendor,
// testdata, and directories starting with "." or "_", as the go tool does.
// Files marked as generated ("Code generated ... DO NOT EDIT.") are always skipped.
func Extract(patterns []string, opts Options) ([]Function, error) {
	files, err := resolveFiles(patterns, opts)
	if err != nil {
		return nil, err
	}

	var funcs []Function
	for _, file := range files {
		found, err := extractFile(file)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, found...)
	}
	return funcs, nil
}

func resolveFiles(patterns []string, opts Options) ([]string, error) {
	var files []string
	for _, p := range patterns {
		matched, err := resolvePattern(p, opts)
		if err != nil {
			return nil, err
		}
		files = append(files, matched...)
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

func resolvePattern(pattern string, opts Options) ([]string, error) {
	if root, ok := strings.CutSuffix(pattern, "/..."); ok {
		return walkDir(cmp.Or(root, "."), opts)
	}

	info, err := os.Stat(pattern)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", pattern, err)
	}
	if !info.IsDir() {
		if filepath.Ext(pattern) != ".go" {
			return nil, fmt.Errorf("resolve %s: not a .go file", pattern)
		}
		return []string{filepath.Clean(pattern)}, nil
	}

	entries, err := os.ReadDir(pattern)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", pattern, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && isTargetFile(e.Name(), opts) {
			files = append(files, filepath.Join(pattern, e.Name()))
		}
	}
	return files, nil
}

func walkDir(root string, opts Options) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && isSkippedDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isTargetFile(d.Name(), opts) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, nil
}

func isSkippedDir(name string) bool {
	return name == "vendor" || name == "testdata" ||
		strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func isTargetFile(name string, opts Options) bool {
	if filepath.Ext(name) != ".go" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return false
	}
	return opts.IncludeTests || !strings.HasSuffix(name, "_test.go")
}

func extractFile(path string) ([]Function, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}

	var funcs []Function
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// Source includes the doc comment so that the evaluator can judge
		// it, while StartLine stays at the func keyword, which is where
		// editors and compilers report the function.
		start := fn.Pos()
		if fn.Doc != nil {
			start = fn.Doc.Pos()
		}
		funcs = append(funcs, Function{
			File:      path,
			StartLine: fset.Position(fn.Pos()).Line,
			EndLine:   fset.Position(fn.End()).Line,
			Name:      funcName(fn),
			Source:    string(src[fset.Position(start).Offset:fset.Position(fn.End()).Offset]),
		})
	}
	return funcs, nil
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return fmt.Sprintf("(%s).%s", types.ExprString(fn.Recv.List[0].Type), fn.Name.Name)
}
