---
name: tastecheck
license: Apache-2.0
description: Score Go functions against code-quality "tastes" with the tastecheck CLI and iterate until the quality gate passes. Use when writing or modifying Go code in a repository that uses tastecheck (it has a .tastecheck.yaml or mentions tastecheck), when asked to run a taste or quality check on Go code, or when creating or tuning .tastecheck.yaml and custom ruleset files.
---

# tastecheck

`tastecheck` scores each Go function against *tastes*, code-quality properties such as "errors are wrapped with
context", using an LLM evaluator. Every score is normalized to `[0, 1]` and compared with the taste's threshold. A taste
with severity `fail` fails the check when a function is below its threshold; a `warning` taste is only reported.

Use it as a quality gate: change the code, run `tastecheck check`, fix what it reports, and repeat until it exits with
`0`.

## Prerequisites

- The `tastecheck` binary. If `command -v tastecheck` finds nothing, install it with
  `go install github.com/chez-shanpu/tastecheck@latest` (Go 1.27 or later), or download a binary from
  https://github.com/chez-shanpu/tastecheck/releases.
- `TYPESAFE_API_KEY` in the environment, for the default TypeSafe AI backend. It is optional only when `--base-url`
  points to a compatible API that does not need one. If the key is missing, ask the user; never guess or hard-code one.
- Every `check` run calls a paid API for every function in the given paths. Check only the code you changed.

## Quality gate loop

1. **Validate the config** (no API call):

   ```sh
   tastecheck validate
   ```

   Without `.tastecheck.yaml`, every builtin ruleset is used. Use `-c <file>` for another config file.

2. **Check the changed code** with the JSON report:

   ```sh
   tastecheck check ./internal/store/... --output json
   tastecheck check internal/store/store.go cmd/root.go --output json
   ```

   A path is a `.go` file, a directory (non-recursive), or a directory followed by `/...` (recursive). Prefer the files
   or packages you touched over `./...`. `_test.go` files are skipped unless `--include-tests` is set or a test file is
   given explicitly.

3. **Read the exit code and stdout together.** Exit code `1` has two meanings:

   | Exit code | stdout                          | Meaning                    | What to do                                                  |
   |-----------|---------------------------------|----------------------------|-------------------------------------------------------------|
   | `0`       | report, `"passed": true`        | The gate passed            | Stop. Mention any remaining warnings                        |
   | `1`       | report, `"passed": false`       | Code is below a threshold  | Improve the code (next step)                                |
   | `1`       | empty, `Error: ...` on stderr   | The check did not complete | Do not change the code. Fix the cause or report it to the user |

   Causes of `Error:` include an invalid config, no functions in the given paths, a missing API key, an API failure,
   and a Go parse error.

4. **Fix the findings.** In `results`, handle the entries with `"passed": false` and `"severity": "fail"` first; they
   are what fails the gate. Fix `warning` entries when they are in code you are changing anyway. For each finding:

   - `file`, `line`, `function` locate the function. `ruleset` and `taste` identify the rule (`<ruleset>/<taste>`).
   - `label` describes the level the function is at now, and `improve_to` describes the next better level. The
     evaluator gives no other rationale, so aim the change at `improve_to`.
   - For the full definition of a builtin taste, run `tastecheck ruleset show <ruleset>` and find the taste by `id`.
     `tastecheck ruleset list` lists builtin rulesets and their taste IDs.
   - `confidence` near `0` means the evaluator could not decide between levels. Such a score sits near the threshold
     and may flip between runs.
   - Entries with `"low_confidence": true` were ignored because of `--min-confidence`; they do not fail the gate.

5. **Re-run** the same `check` command and repeat until it exits with `0`.

   Stop and report to the user instead of looping when the same finding stays below its threshold after two or three
   honest attempts, when it flips between runs with low `confidence`, or when fixing it would conflict with the user's
   requirements or another taste. Include the finding (`file`, `function`, `taste`, `score`, `threshold`) and what you
   tried.

## Do not game the gate

Pass the gate by improving the code. Without the user's explicit consent, do not:

- lower a `threshold`, change a `severity` to `warning`, or set `disabled: true` in the config;
- add paths to `ignore` or pass `--ignore` for code you were asked to work on;
- raise `--min-confidence` / `evaluator.min_confidence`;
- rename, split, or restructure code only to hide it from the evaluator (for example, moving logic into a generated
  file, which is always skipped).

If a taste does not fit the project, explain why and propose the config change to the user.

## Configuring tastecheck

To create or edit `.tastecheck.yaml`, add ignore patterns, or write a custom ruleset file, read
[references/config.md](references/config.md). Always run `tastecheck validate` after changing the config.
