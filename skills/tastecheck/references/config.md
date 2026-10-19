# Configuring tastecheck

Read this when creating or editing `.tastecheck.yaml`, adding ignore patterns, or writing a custom ruleset file. After
any change, run `tastecheck validate` (or `tastecheck validate -c <file>`); it checks the config and every ruleset
without calling the evaluator.

## Config file

`tastecheck check` and `tastecheck validate` read `.tastecheck.yaml` in the current directory, or the file given with
`-c, --config`. When the default file does not exist, every builtin ruleset is used with no overrides. An explicitly
given file must exist.

```yaml
# .tastecheck.yaml
version: 1                   # only 1 is supported
rulesets:                    # every builtin ruleset is used unless disabled here
  - builtin: uber-go         # list a builtin only to override or disable it
    overrides:               # keyed by taste ID within this ruleset
      goroutine-lifecycle:
        disabled: true
      error-wrap:
        threshold: 0.8       # within [0, 1]
      reduce-nesting:
        severity: fail       # fail or warning
  - builtin: google-go-guide
    disabled: true           # exclude this builtin ruleset
  - path: ./rules/team.yaml  # relative to the config file
    name: team               # optional; defaults to the file name without extension
    overrides:
      error-wrap:            # a different taste from uber-go/error-wrap
        threshold: 0.9
ignore:                      # relative to the config file
  - internal/legacy          # a directory and everything under it
  - "cmd/gen_*.go"
evaluator:                   # all optional; these are the defaults
  backend: typesafeai
  model: jev-latest
  concurrency: 4             # functions evaluated in parallel
  max_retries: 3             # retries for transient errors; 0 disables
  min_confidence: 0          # ignore below-threshold findings with lower confidence; 0 disables
```

The builtin rulesets are `uber-go`, `google-go-guide`, `google-go-decisions`, and `google-go-best-practices`. Run
`tastecheck ruleset list` for the current list and their taste IDs.

Rules that `validate` enforces:

- Unknown fields are rejected.
- `disabled` on a ruleset entry is only allowed for builtin rulesets. To stop using a `path` ruleset, remove its entry.
- Overriding a taste ID that the ruleset does not define is an error. Overrides apply only to the ruleset they are
  written under, and a taste is identified as `<ruleset>/<id>`.
- Ruleset names (the builtin name, or `name` / the file name for `path` rulesets) must be unique.
- At least one taste must remain enabled.

Command line flags given explicitly (`--backend`, `--model`, `--concurrency`, `--max-retries`, `--min-confidence`)
take precedence over `evaluator`, which takes precedence over the flag defaults. `--base-url` is a flag only.

## Ignore patterns

`ignore` in the config file and the repeatable `--ignore` flag exclude files and directories from evaluation.

- A pattern is a slash-separated relative path. Each element may use Go's `path.Match` syntax (`*`, `?`, `[...]`).
  `**` is not supported, and `*` does not match `/`.
- A pattern matches a file when it matches the file's path or one of its parent directories, so a directory pattern
  ignores everything under it. `internal/leg` does not match `internal/legacy/x.go`.
- Patterns are anchored to their base directory: the config file directory for `ignore`, the current directory for
  `--ignore`. `mock` matches only `./mock`, not `internal/mock`; use `internal/*/mock` for a fixed depth.
- `--ignore` adds to the config file's `ignore`.
- Empty, absolute, or malformed patterns, and patterns that refer to the base directory itself (`.`) or outside it
  (`../x`), are errors.

Independently of `ignore`, directory expansion always skips `vendor`, `testdata`, and directories starting with `.` or
`_`, and generated files (`Code generated ... DO NOT EDIT.`) are always skipped.

## Ruleset files

A custom ruleset file defines tastes and is referenced from the config file with `path:`. Start from a builtin ruleset:

```sh
tastecheck ruleset show uber-go > rules/team.yaml
```

Then edit it to this format:

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

| Field         | Description                                                                                                                      |
|---------------|----------------------------------------------------------------------------------------------------------------------------------|
| `id`          | Unique identifier within the ruleset                                                                                             |
| `description` | What the evaluator should judge                                                                                                  |
| `levels`      | Required. 2 to 10 levels from worst to best. The worst level scores `0` and the best `1`. For yes/no, use two levels like `[no, yes]` |
| `threshold`   | Required. Minimum normalized score in `[0, 1]`                                                                                   |
| `severity`    | `fail` (default) or `warning`                                                                                                    |

Unknown fields are rejected.

Guidelines for writing tastes, following the builtin rulesets:

- The evaluator sees one function at a time. Write tastes that can be judged from a single function body; leave
  naming across packages, imports, and formatting to linters.
- State in `description` that a function containing nothing the rule applies to is compliant, and list the exceptions
  the rule allows. Otherwise unrelated functions score low.
- Write each level as a concrete description of code at that level. `label` and `improve_to` in the report are these
  texts, so they are what an agent fixing the code will aim at.
- Three levels (clear violation, minor issue, compliant) with a threshold of `0.7` make a minor issue fail.
