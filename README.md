# tastecheck

`tastecheck` scores Go functions against *tastes*, which are code-quality properties such as "errors are wrapped with
context" or "goroutines have a controlled lifetime", using an LLM evaluator.

It is built to act as a quality gate for coding agents. Every score is normalized to `[0, 1]`, pass/fail is decided per
function against a threshold, and the result comes back both as an exit code and as a JSON report. Each taste has a
severity: a `fail` taste below its threshold fails the check, while a `warning` taste is only reported. An
implementation loop can therefore run until `tastecheck check` exits with `0`.

Tastes are grouped into *rulesets*. The builtin rulesets are `uber-go`, derived from
the [Uber Go Style Guide](https://github.com/uber-go/guide), and `google-go-guide`, `google-go-decisions`, and
`google-go-best-practices`, derived from the [Google Go Style Guide](https://google.github.io/styleguide/go/). Every
builtin ruleset is used unless the config file disables it. A config file disables builtin rulesets, adds ruleset files,
overrides individual tastes, and sets evaluator options.

The default evaluator backend is the [TypeSafe AI](https://typesafe.ai) System One API with the `jev` model, called
through [typesafeai-go](https://github.com/chez-shanpu/typesafeai-go). Backends are pluggable
(see [Architecture](#architecture)).

## Installation

Download a prebuilt binary for Linux or macOS (amd64 / arm64) from
[GitHub Releases](https://github.com/chez-shanpu/tastecheck/releases), or build it with Go 1.27 or later:

```sh
go install github.com/chez-shanpu/tastecheck@latest
```

## Usage

```sh
export TYPESAFE_API_KEY=...

# Evaluate all functions under the current module with .tastecheck.yaml,
# or with every builtin ruleset when that file does not exist.
tastecheck check ./...

# JSON report for agents.
tastecheck check ./internal/... --output json

# Validate the config file and its rulesets (no API call).
tastecheck validate -c .tastecheck.yaml

# List builtin rulesets and their taste IDs, or print one as YAML.
tastecheck ruleset list
tastecheck ruleset show uber-go
```

A path argument is a `.go` file, a directory (non-recursive), or a directory followed by `/...` (recursive). When
expanding directories, `vendor`, `testdata`, and directories starting with `.` or `_` are skipped. `_test.go` files are
skipped unless `--include-tests` is set. Generated files (`Code generated ... DO NOT EDIT.`) are always skipped.

| Flag               | Default                        | Description                                                                                                                                            |
|--------------------|--------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| `-c, --config`     | `.tastecheck.yaml`             | Config file. If the default file does not exist, every builtin ruleset is used. An explicitly given file must exist                                    |
| `-o, --output`     | `text`                         | `text` or `json`                                                                                                                                       |
| `--backend`        | `typesafeai`                   | Evaluator backend                                                                                                                                      |
| `--model`          | backend default (`jev-latest`) | Evaluator model                                                                                                                                        |
| `--concurrency`    | `4`                            | Functions evaluated in parallel                                                                                                                        |
| `--max-retries`    | `3`                            | Retries on rate limiting, overload, or network errors (exponential backoff). `0` disables                                                              |
| `--min-confidence` | `0`                            | Ignore a finding below its threshold when the evaluator's confidence is below this value, within `[0, 1]`. `0` disables. See [Confidence](#confidence) |
| `--include-tests`  | `false`                        | Include `_test.go` files                                                                                                                               |
| `-v, --verbose`    | `false`                        | Also list passing functions in text output                                                                                                             |

Evaluator flags given explicitly take precedence over the config file, which takes precedence over the flag defaults.

### Exit codes

| Code | Meaning                                                                                                                                                                             |
|------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `0`  | Every function meets every `fail` taste threshold, except findings ignored for low confidence. Functions below a `warning` taste threshold may still be reported                    |
| `1`  | At least one function is below a `fail` taste threshold, or the check could not be completed (invalid config, no functions found in the given paths, API failure, parse error, ...) |

The output tells the two cases of `1` apart. When the code is below a threshold, the report is written to stdout: the
text output ends with a `FAIL:` summary line, and the JSON output has `"passed": false`. When the check could not be
completed, nothing is written to stdout and `Error: ...` is written to stderr. A loop should improve the code in the
former case and treat the latter as a failure of the tool itself, not as a verdict on the code.

## Config file

[`.tastecheck.yaml.example`](.tastecheck.yaml.example) is a commented example to copy to `.tastecheck.yaml`.

```yaml
# .tastecheck.yaml
version: 1
rulesets: # every builtin ruleset is used unless disabled here
  - builtin: uber-go         # list a builtin only to override or disable it
    overrides: # keyed by taste ID within this ruleset
      goroutine-lifecycle:
        disabled: true
      error-wrap:
        threshold: 0.8
      reduce-nesting:
        severity: fail       # fail or warning
  - builtin: google-go-guide
    disabled: true           # exclude this builtin ruleset
  - path: ./rules/team.yaml  # relative to the config file
    name: team               # optional; defaults to the file name without extension
    overrides:
      error-wrap: # a different taste from uber-go/error-wrap
        threshold: 0.9
evaluator: # all optional
  backend: typesafeai
  model: jev-latest
  concurrency: 4
  max_retries: 3
  min_confidence: 0          # 0 disables; see Confidence
```

- Every builtin ruleset is selected, including ones added in later versions, unless its entry has `disabled: true`.
  `disabled` is only allowed for builtin rulesets; remove a `path` entry to stop using it. Overrides under a disabled
  builtin are ignored.
- Builtin rulesets are evaluated in name order, followed by `path` rulesets in the order they are listed.
- Each ruleset has a name: the builtin name, or `name` / the file name for `path` rulesets. Names must be unique within
  the config.
- A taste is identified as `<ruleset>/<id>` (e.g. `uber-go/error-wrap`), so rulesets may define tastes with the same ID.
  Overrides apply only to the ruleset they are written under.
- Overriding a taste ID that the ruleset does not define is an error, which catches typos. Leaving no taste enabled is
  also an error.
- Unknown fields are rejected.

## Builtin rulesets

### `uber-go`

Tastes adapted from the [Uber Go Style Guide](https://github.com/uber-go/guide). Only rules that can be judged from a
single function body are included; rules about type declarations, naming, imports, and formatting are left to linters.
Every taste uses three levels (clear violation, minor issue, compliant) and a threshold of `0.7`, so a minor issue falls
below the threshold. Rules whose violations lead to bugs have severity `fail`, and readability rules have severity
`warning`.

| ID                    | Guide section                      | Severity  | Checks that the function ...                                                     |
|-----------------------|------------------------------------|-----------|----------------------------------------------------------------------------------|
| `error-wrap`          | Error Wrapping                     | `fail`    | returns errors as-is or with succinct `%w` context, without "failed to" prefixes |
| `error-handle-once`   | Handle Errors Once                 | `fail`    | does not both log and return the same error                                      |
| `no-panic-or-exit`    | Don't Panic, Exit in Main          | `fail`    | returns errors instead of panicking, and exits only from `main`                  |
| `goroutine-lifecycle` | Don't fire-and-forget goroutines   | `fail`    | starts only goroutines that can be stopped and waited for                        |
| `defer-cleanup`       | Defer to Clean Up                  | `fail`    | releases locks and resources it owns with `defer`                                |
| `reduce-nesting`      | Reduce Nesting, Unnecessary Else   | `warning` | handles special cases early and avoids unnecessary `else`                        |
| `type-assertion`      | Handle Type Assertion Failures     | `fail`    | uses the comma-ok form for type assertions                                       |
| `container-copy`      | Copy Slices and Maps at Boundaries | `fail`    | copies slices and maps it stores from, or exposes to, callers                    |
| `time-types`          | Use "time" to handle time          | `warning` | uses `time.Time` / `time.Duration` instead of bare numbers                       |
| `naked-parameters`    | Avoid Naked Parameters             | `warning` | does not pass unexplained `bool` or numeric literals                             |

Use `overrides.<id>.severity` in the config file to change the severity of a taste.

A function that contains nothing a rule applies to (for example, no goroutines) is rated compliant for that rule. The
exceptions allowed by the guide, such as returning an error unchanged when there is no context to add, or leaving
cleanup to the caller that owns a returned resource, are written into the taste descriptions. Run
`tastecheck ruleset show uber-go` to see the full definitions.

The Uber Go Style Guide is © Uber Technologies, Inc., licensed under the Apache License 2.0.

### Google Go Style Guide rulesets

Three rulesets adapt the [Google Go Style Guide](https://google.github.io/styleguide/go/), one per document. They follow
the same conventions as `uber-go`: rules are judged from a single function, every taste uses three levels and a
threshold of `0.7`, a function that contains nothing a rule applies to is rated compliant, and rules whose violations
lead to bugs have severity `fail` while readability rules have severity `warning`. Rules that gofmt, go vet, or
staticcheck enforce mechanically, and rules that need more than one function to judge (package names, imports, interface
ownership, package size), are left out. Tastes named `test-*` apply only to test functions and test helpers, which are
checked with `--include-tests` or when a `_test.go` file is given explicitly.

Like every builtin ruleset, they are used by default. To use only some of them, disable the others:

```yaml
rulesets:
  - builtin: google-go-guide
    disabled: true
```

#### `google-go-guide`

Tastes adapted from the [Guide](https://google.github.io/styleguide/go/guide), which describes the style principles. The
principles are subjective, so every taste has severity `warning`.

| ID                  | Guide section       | Severity  | Checks that the function ...                                           |
|---------------------|---------------------|-----------|------------------------------------------------------------------------|
| `clarity`           | Clarity             | `warning` | can be understood by reading it top to bottom                          |
| `comment-why`       | Clarity             | `warning` | explains non-obvious reasons in comments without restating the code    |
| `simplicity`        | Simplicity          | `warning` | avoids unnecessary abstraction and clever code                         |
| `least-mechanism`   | Least mechanism     | `warning` | uses core constructs and the standard library before heavier machinery |
| `concision`         | Concision           | `warning` | avoids repetitive code and calls out subtle deviations from idioms     |
| `hidden-details`    | Maintainability     | `warning` | does not hide critical details such as `=` versus `:=` in a condition  |
| `local-consistency` | Consistency, Naming | `warning` | uses the same name and approach for the same concept                   |

#### `google-go-decisions`

Tastes adapted from the [Style Decisions](https://google.github.io/styleguide/go/decisions).

| ID                      | Decision section                           | Severity  | Checks that the function ...                                                        |
|-------------------------|--------------------------------------------|-----------|-------------------------------------------------------------------------------------|
| `variable-names`        | Variable names, Repetition                 | `warning` | uses names sized to their scope, without type or context words                      |
| `getters`               | Getters                                    | `warning` | is not named with a `Get` prefix                                                    |
| `named-results`         | Named result parameters                    | `warning` | names results only when it helps, and uses naked returns only when small            |
| `doc-comments`          | Doc comments, Comment sentences            | `warning` | has a doc comment in full sentences starting with its name when exported            |
| `handle-errors`         | Handle errors                              | `fail`    | does not discard errors without a comment explaining why                            |
| `returning-errors`      | Returning errors                           | `fail`    | returns failures as a trailing `error` of interface type                            |
| `in-band-errors`        | In-band errors                             | `fail`    | reports failure with an extra `ok` or `error` result instead of values like `-1`    |
| `indent-error-flow`     | Indent error flow                          | `warning` | handles errors first and keeps the normal path out of `else`                        |
| `struct-literals`       | Literal formatting                         | `warning` | omits irrelevant zero-value fields and names fields where helpful                   |
| `nil-slices`            | Nil slices                                 | `warning` | declares empty slices as nil and checks emptiness with `len`                        |
| `line-breaks`           | Indentation confusion, Function formatting | `warning` | does not wrap signatures, conditions, or long strings                               |
| `copying`               | Copying                                    | `fail`    | does not copy values such as `bytes.Buffer` whose methods use pointer receivers     |
| `dont-panic`            | Don't panic, Must functions                | `fail`    | returns errors instead of panicking, and uses `Must` helpers only at initialization |
| `goroutine-lifetimes`   | Goroutine lifetimes                        | `fail`    | makes it evident when the goroutines it starts exit                                 |
| `synchronous-functions` | Synchronous functions                      | `warning` | returns results directly instead of asynchronously                                  |
| `pass-values`           | Pass values                                | `warning` | does not take pointers to small values just to save bytes                           |
| `use-percent-q`         | Use %q                                     | `warning` | quotes strings with `%q`                                                            |
| `context-propagation`   | Contexts                                   | `fail`    | passes on its caller's context instead of creating `context.Background()`           |
| `context-first`         | Contexts, Custom contexts                  | `warning` | takes `context.Context` as its first parameter                                      |
| `crypto-rand`           | crypto/rand                                | `fail`    | generates secrets with `crypto/rand`                                                |
| `return-concrete-types` | Interfaces                                 | `warning` | returns concrete types rather than interfaces                                       |
| `test-failure-messages` | Useful test failures                       | `warning` | writes failure messages as `F(in) = got, want want`                                 |
| `test-keep-going`       | Keep going                                 | `warning` | reports mismatches with `t.Error` rather than `t.Fatal`                             |
| `test-comparisons`      | Full structure comparisons                 | `warning` | compares whole values with `cmp` instead of `reflect.DeepEqual` or field by field   |
| `test-error-semantics`  | Test error semantics                       | `warning` | does not check errors by their message text                                         |
| `test-helpers`          | Test helpers, Assertion libraries          | `warning` | calls `t.Helper()` and is not an assertion helper                                   |
| `test-table-driven`     | Table-driven tests, Subtests               | `warning` | identifies rows by their inputs and keeps the loop body uniform                     |

#### `google-go-best-practices`

Tastes adapted from the [Best Practices](https://google.github.io/styleguide/go/best-practices).

| ID                        | Best practice section                         | Severity  | Checks that the function ...                                                            |
|---------------------------|-----------------------------------------------|-----------|-----------------------------------------------------------------------------------------|
| `function-names`          | Function and method names                     | `warning` | has a name that does not repeat its receiver, parameters, or result types               |
| `shadowing`               | Shadowing                                     | `fail`    | does not shadow variables such as `ctx` or `err` in an inner scope by mistake           |
| `error-structure`         | Error structure                               | `fail`    | distinguishes errors with `errors.Is` / `errors.As` rather than by their text           |
| `error-annotation`        | Adding information to errors, Placement of %w | `warning` | adds non-redundant context, with `%w` at the end                                        |
| `error-logging`           | Logging errors                                | `warning` | does not log errors that it also returns                                                |
| `panic-and-recover`       | Program checks and panics, When to panic      | `fail`    | does not swallow panics with `recover` or let internal panics escape                    |
| `var-declarations`        | Variable declarations                         | `warning` | uses `:=` for values, `var` for zero values, and size hints only when the size is known |
| `channel-direction`       | Channel direction                             | `warning` | specifies channel directions                                                            |
| `function-arguments`      | Function argument lists                       | `warning` | does not take long or easily swapped parameter lists                                    |
| `documentation`           | Documentation conventions                     | `warning` | documents cleanup and non-obvious behavior without restating the obvious                |
| `signal-boosting`         | Signal boosting                               | `warning` | comments on lines that look like an idiom but differ, such as `err == nil`              |
| `string-concatenation`    | String concatenation                          | `warning` | builds strings with `+`, `fmt.Sprintf`, or `strings.Builder` as appropriate             |
| `global-state`            | Global state                                  | `fail`    | does not depend on package-level mutable state                                          |
| `test-validation-in-test` | Leave testing to the Test function            | `warning` | keeps validation in the `Test` function instead of assertion helpers                    |
| `test-fatal-usage`        | t.Error vs. t.Fatal                           | `warning` | uses `t.Fatal` only for setup or inside subtests                                        |
| `test-fatal-goroutine`    | Don't call t.Fatal from separate goroutines   | `fail`    | does not call `t.Fatal` from goroutines it starts                                       |
| `test-helper-errors`      | Error handling in test helpers                | `warning` | fails setup helpers with a descriptive `t.Fatal` after `t.Helper()`                     |
| `test-table-field-names`  | Use field names in struct literals            | `warning` | uses field names in table test cases                                                    |
| `test-setup-scope`        | Keep setup code scoped to specific tests      | `warning` | runs expensive setup only in the tests that need it                                     |

The Google Go Style Guide is by Google, licensed under [CC BY 3.0](https://creativecommons.org/licenses/by/3.0/). The
tastes summarize and adapt its guidance; they are not the original text.

## Ruleset files

A ruleset file defines tastes, and is referenced from the config file with `path:`. `tastecheck ruleset show <name>`
prints a builtin ruleset in this format, which is a convenient starting point.

```yaml
version: 1
tastes:
  - id: small-functions
    description: |
      Evaluate whether this Go function is small enough to understand at a glance. ...
    levels: # WORST to BEST, 2 to 10 levels
      - The function is long and does many things.
      - The function is somewhat long but readable.
      - The function is short and focused.
    threshold: 0.7     # pass when normalized score >= threshold
    severity: fail     # fail (default) or warning
```

| Field         | Description                                                                                                                                                                                                                                   |
|---------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `id`          | Unique identifier within the ruleset                                                                                                                                                                                                          |
| `description` | What the evaluator should judge                                                                                                                                                                                                               |
| `levels`      | Required. 2 to 10 levels listed from worst to best. The score is normalized so that the worst level is `0` and the best is `1`. For a yes/no taste, use two levels such as `[no, yes]`; the score is then the probability of the better level |
| `threshold`   | Required. Minimum normalized score in `[0, 1]`                                                                                                                                                                                                |
| `severity`    | `fail` (default) or `warning`. A function below the threshold of a `fail` taste fails the check (exit code `1`). A `warning` taste is reported without failing the check                                                                      |

Unknown fields are rejected.

## JSON report

```json
{
  "passed": false,
  "summary": {
    "functions": 2,
    "failed": 1,
    "warnings": 0,
    "low_confidence": 0,
    "mean": 0.75,
    "by_taste": {
      "uber-go/error-wrap": 0.75
    }
  },
  "results": [
    {
      "file": "internal/store/store.go",
      "line": 30,
      "function": "Load",
      "ruleset": "uber-go",
      "taste": "error-wrap",
      "score": 0.5,
      "threshold": 0.7,
      "severity": "fail",
      "passed": false,
      "confidence": 0.9,
      "label": "The function mostly follows the rule but has a minor issue, such as one verbose or inconsistent error message.",
      "improve_to": "The function follows the rule, or it does not propagate any error so the rule does not apply."
    }
  ]
}
```

- `passed` in a result tells whether the score meets the threshold, regardless of severity. The top-level `passed` is
  `false` only when a `fail` result has not passed. `summary.failed` counts the `fail` results that have not passed, and
  `summary.warnings` counts the `warning` results that have not passed.
- `label` is the level nearest to the score, and `improve_to` is the next better level. They give an agent a concrete
  target to aim for, because the evaluator does not return a rationale.
- `confidence` is the evaluator's certainty in `[0, 1]`. It is omitted when the backend does not report one. `0` is a
  reported value, not a missing one: see [Confidence](#confidence).
- `low_confidence` is `true` on a result below its threshold that is ignored because its confidence is below
  `--min-confidence`. Such a result counts in neither `summary.failed` nor `summary.warnings`, and does not affect the
  top-level `passed`. `summary.low_confidence` counts them. It is omitted when `false`.
- `mean` and `by_taste` are provided for reference only. Pass/fail depends solely on per-function thresholds. `by_taste`
  is keyed by `<ruleset>/<id>`.

## Confidence

The `typesafeai` backend reports a confidence with every score. It is high when the evaluator's probabilities
concentrate on one level, and it drops toward `0` when they are split between levels, such as between a violation and
compliance. A confidence of `0` therefore means that the evaluator could not decide, not that no confidence was
reported. Such scores tend to fall near the middle of the range, close to the threshold.

`--min-confidence` (or `evaluator.min_confidence`) ignores results below their threshold whose confidence is below the
given value. They stay in the JSON report with `"low_confidence": true`, and the text output lists them as `SKIP` with
`-v`. Results from a backend that reports no confidence are never ignored. Raising the value removes more borderline
results, including real violations the evaluator was unsure about, so it is disabled by default.

## Architecture

```
cmd/                          cobra commands (check, validate, ruleset)
internal/config               config file loading and ruleset resolution
internal/ruleset              builtin rulesets embedded in the binary
internal/taste                ruleset file loading and validation (provider-neutral schema)
internal/extract              Go function extraction (go/parser)
internal/evaluator            Evaluator interface and backend registry
internal/evaluator/typesafeai TypeSafe AI (jev) backend
internal/check                Parallel evaluation, retries, pass/fail decision
internal/report               text / JSON output
```

To add a backend, implement `evaluator.Evaluator`, call `evaluator.Register(name, factory)` from the package's `init`,
and import the package from `cmd`. Signal retryable failures by wrapping them with `evaluator.ErrTransient`.

To add a builtin ruleset, put a ruleset file under `internal/ruleset/builtin/`. Its file name without the extension
becomes the ruleset name.

## Development

Go and goreleaser are pinned in `aqua.yaml` and can be installed with [aqua](https://aquaproj.github.io/) (`aqua i`).
gofumpt, goimports, and staticcheck are Go tool dependencies in `go.mod` and need no separate installation.

```sh
make fmt        # gofumpt and goimports
make check      # vet, gofumpt/goimports diff, staticcheck, go test -race, goreleaser check
make build      # bin/tastecheck
make test-e2e   # e2e tests with a mock evaluator (no API calls)
make check-all  # check, build, and test-e2e (run in CI)
```

## License

Apache License 2.0
