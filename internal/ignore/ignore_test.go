// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package ignore

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNewErrors(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr string
	}{
		{name: "empty", pattern: "", wantErr: "must not be empty"},
		{name: "absolute", pattern: "/abs/path", wantErr: "must be a relative path"},
		{name: "parent", pattern: "../other", wantErr: "must not refer outside"},
		{name: "parent after clean", pattern: "a/../../other", wantErr: "must not refer outside"},
		{name: "bad pattern", pattern: "a/[", wantErr: "syntax error in pattern"},
		{name: "bad pattern after clean", pattern: `a\/`, wantErr: "syntax error in pattern"},
		{name: "base directory", pattern: ".", wantErr: "must not refer to its base directory itself"},
		{name: "base directory with slash", pattern: "./", wantErr: "must not refer to its base directory itself"},
		{name: "base directory after clean", pattern: "a/..", wantErr: "must not refer to its base directory itself"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(".", []string{"ok", tt.pattern})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("New() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name     string
		patterns []string
		file     string
		want     bool
	}{
		{name: "exact file", patterns: []string{"cmd/gen.go"}, file: "cmd/gen.go", want: true},
		{name: "directory prefix", patterns: []string{"internal/legacy"}, file: "internal/legacy/sub/x.go", want: true},
		{name: "directory itself", patterns: []string{"internal/legacy"}, file: "internal/legacy", want: true},
		{name: "cleaned pattern", patterns: []string{"./internal/legacy/"}, file: "internal/legacy/x.go", want: true},
		{name: "glob per element", patterns: []string{"internal/*/mock"}, file: "internal/evaluator/mock/mock.go", want: true},
		{name: "glob file name", patterns: []string{"cmd/gen_*.go"}, file: "cmd/gen_types.go", want: true},
		{name: "partial element does not match", patterns: []string{"internal/leg"}, file: "internal/legacy/x.go", want: false},
		{name: "anchored to base directory", patterns: []string{"mock"}, file: "internal/mock/m.go", want: false},
		{name: "star does not cross slash", patterns: []string{"*.go"}, file: "sub/x.go", want: false},
		{name: "no patterns", patterns: nil, file: "x.go", want: false},
		{name: "outside base directory", patterns: []string{"*"}, file: "../outside.go", want: false},
		{name: "base directory itself never matches", patterns: []string{"*", ".*", "?*"}, file: ".", want: false},
		{name: "hidden file", patterns: []string{".*"}, file: ".hidden/x.go", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(base, tt.patterns)
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			if got := m.Match(filepath.Join(base, filepath.FromSlash(tt.file))); got != tt.want {
				t.Errorf("Match(%q) with %v = %v, want %v", tt.file, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestMatchRelativeBaseDir(t *testing.T) {
	t.Chdir(t.TempDir())

	m, err := New("conf", []string{"legacy"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	for file, want := range map[string]bool{
		"conf/legacy/x.go": true,
		"./conf/legacy":    true,
		"legacy/x.go":      false,
		"conf/x.go":        false,
	} {
		if got := m.Match(file); got != want {
			t.Errorf("Match(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestAny(t *testing.T) {
	a, err := New(".", []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(".", []string{"b"})
	if err != nil {
		t.Fatal(err)
	}

	match := Any(nil, a, b)
	for file, want := range map[string]bool{"a/x.go": true, "b/x.go": true, "c/x.go": false} {
		if got := match(file); got != want {
			t.Errorf("Any()(%q) = %v, want %v", file, got, want)
		}
	}

	if Any()("a/x.go") {
		t.Error("Any() with no matchers = true, want false")
	}
}
