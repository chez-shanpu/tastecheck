//go:build e2e

// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build binary
	root := filepath.Join("..")
	abs, err := filepath.Abs(filepath.Join(root, "bin", "tastecheck"))
	if err != nil {
		panic(err)
	}
	binaryPath = abs

	// The e2e tag adds the "mock" evaluator backend to the binary.
	cmd := exec.Command("go", "build", "-tags", "e2e", "-o", binaryPath, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("failed to build binary: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

// runBinary runs the binary without TYPESAFE_API_KEY so that no API call is made.
func runBinary(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = ".."
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TYPESAFE_API_KEY=") {
			env = append(env, kv)
		}
	}
	cmd.Env = env

	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode(), out.String(), errOut.String()
	}
	if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return 0, out.String(), errOut.String()
}

// builtinRulesets returns the builtin ruleset names in sorted order and the
// number of tastes of each, read from the ruleset files so that adding a
// builtin ruleset does not require updating the tests.
func builtinRulesets(t *testing.T) (names []string, tastes map[string]int) {
	t.Helper()
	dir := filepath.Join("..", "internal", "ruleset", "builtin")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	tastes = make(map[string]int, len(entries))
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok {
			continue
		}
		names = append(names, name)
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.Lines(string(src)) {
			if strings.HasPrefix(line, "  - id:") {
				tastes[name]++
			}
		}
	}
	return names, tastes
}

func TestValidateWithoutConfig(t *testing.T) {
	code, stdout, stderr := runBinary(t, "validate")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	names, tastes := builtinRulesets(t)
	total := 0
	for _, n := range tastes {
		total += n
	}
	if want := fmt.Sprintf("%d tastes enabled from rulesets %v", total, names); !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want containing %q", stdout, want)
	}
	if !strings.Contains(stderr, "using every builtin ruleset") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestValidateConfig(t *testing.T) {
	code, stdout, stderr := runBinary(t, "validate", "-c", "test/testdata/config.yaml")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	// The config disables one uber-go taste and the google-go-guide ruleset.
	names, tastes := builtinRulesets(t)
	names = slices.DeleteFunc(names, func(n string) bool { return n == "google-go-guide" })
	total := -1
	for _, n := range names {
		total += tastes[n]
	}
	if want := fmt.Sprintf("%d tastes enabled from rulesets %v", total, names); !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want containing %q", stdout, want)
	}
}

func TestValidateBrokenConfig(t *testing.T) {
	code, _, stderr := runBinary(t, "validate", "-c", "test/testdata/broken.yaml")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, `override for unknown taste "no-such-taste"`) {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestValidateMissingConfig(t *testing.T) {
	code, _, stderr := runBinary(t, "validate", "-c", "test/testdata/missing.yaml")
	if code != 1 || !strings.Contains(stderr, "open config") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestRulesetList(t *testing.T) {
	code, stdout, stderr := runBinary(t, "ruleset", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "uber-go\n  error-wrap\n") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRulesetShow(t *testing.T) {
	code, stdout, stderr := runBinary(t, "ruleset", "show", "uber-go")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	want, err := os.ReadFile(filepath.Join("..", "internal", "ruleset", "builtin", "uber-go.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if stdout != string(want) {
		t.Errorf("output differs from the builtin ruleset file")
	}

	code, _, stderr = runBinary(t, "ruleset", "show", "nope")
	if code != 1 || !strings.Contains(stderr, "unknown builtin ruleset") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckExitCodes(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFile(t, dir, "config.yaml", "version: 1\nrulesets:\n  - builtin: uber-go\nevaluator:\n  backend: mock\n")
	nopeCfg := writeFile(t, dir, "nope.yaml", "version: 1\nevaluator:\n  backend: nope\n")
	good := writeFile(t, dir, "good.go", "package p\n\nfunc Good() {}\n")
	bad := writeFile(t, dir, "bad.go", "package p\n\nfunc Bad() {}\n")
	empty := t.TempDir()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "pass", args: []string{"check", good, "-c", cfg}, wantCode: 0, wantStdout: "PASS"},
		{name: "violation", args: []string{"check", good, bad, "-c", cfg}, wantCode: 1, wantStdout: "FAIL"},
		{name: "json output", args: []string{"check", good, bad, "-c", cfg, "-o", "json"}, wantCode: 1, wantStdout: `"passed": false`},
		{name: "unknown format", args: []string{"check", good, "-c", cfg, "-o", "xml"}, wantCode: 1, wantStderr: `unknown format "xml"`},
		{name: "explicit missing config", args: []string{"check", good, "-c", filepath.Join(dir, "none.yaml")}, wantCode: 1, wantStderr: "open config"},
		{name: "missing path", args: []string{"check", filepath.Join(dir, "none.go"), "-c", cfg}, wantCode: 1, wantStderr: "resolve"},
		{name: "no functions", args: []string{"check", empty, "-c", cfg}, wantCode: 1, wantStderr: "no functions to check found"},
		{name: "flag overrides config backend", args: []string{"check", good, "-c", cfg, "--backend", "nope"}, wantCode: 1, wantStderr: `unknown evaluator backend "nope"`},
		{name: "config overrides default backend", args: []string{"check", good, "-c", nopeCfg}, wantCode: 1, wantStderr: `unknown evaluator backend "nope"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runBinary(t, tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, tt.wantCode, stderr)
			}
			if !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want containing %q", stdout, tt.wantStdout)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stderr = %q, want containing %q", stderr, tt.wantStderr)
			}
			// Both a violation and a tool error exit with 1, so the output
			// must tell them apart.
			if tt.wantStderr != "" {
				if stdout != "" || !strings.HasPrefix(stderr, "Error: ") {
					t.Errorf("tool error: stdout = %q, stderr = %q, want empty stdout and stderr starting with \"Error: \"", stdout, stderr)
				}
			} else if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestCheckWithoutAPIKey(t *testing.T) {
	code, _, stderr := runBinary(t, "check", "test/testdata/sample", "-c", "test/testdata/config.yaml")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "TYPESAFE_API_KEY is not set") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestVersion(t *testing.T) {
	code, stdout, _ := runBinary(t, "--version")
	if code != 0 || !strings.HasPrefix(stdout, "tastecheck version ") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}
