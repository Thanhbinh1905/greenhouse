# Verification results

Final source checks:

| Command | Result |
|---|---|
| `go test ./...` | Pass: `greenhouse` 55.425s, `greenhouse/eval` cached |
| `go test -race ./...` | Pass: `greenhouse` 62.237s, `greenhouse/eval` 4.776s |
| `go vet ./...` | Pass, no diagnostics |
| `gofmt -l` on changed Go files | Pass, no filenames |
| `git diff --check` | Pass |
| `python3 tools/terminal_check.py <binary> <Task-scratch-output>` | Pass at 56, 80, and 100 columns |
| Credential-value scan | Pass; neither the GitHub token nor the provider key appears in the changed files or evidence |

[Unit output](go-test.log), [race output](go-race.log), [vet output](go-vet.log), and [PTY output](pty-check.log) preserve the results. These results are from Task t65, after the elided-values fix. Test executables and temporary state use Task scratch; Go's build cache remains shared.

[Red-before excerpts](regression-red.log) record the failures for word wrapping, long identifiers, displayed-ID selection, prompt spacing, column spacing, empty-diff rereview, and premature cancellation completion. Each was observed before its corresponding fix. [Elided-value red output](actionable-values-red.log) records the 56-column unit and PTY failures for the PR URL, remote URL, and candidate ref. The empty-diff executable regression now invokes two independent reviews and passes every configured local gate after the repair restores the target tree.

The cancellation checks include the existing executable rerun test and pre-commit cancellation test. [Three race-enabled repetitions](cancellation-regressions.log) pass without weakening their candidate, partial-edit, budget, or evidence assertions. The pre-commit seam now expects the run to remain `cancelling` until the daemon consumes final completion.

The [first race-suite failure](race-first-failure.log) records the auto-resume restart timeout. [Five repetitions after disabling child exit delay](restart-race-repetitions.log) pass. A subsequent full race run exposed the cancellation-state defect; the final full race run passes after that correction.

Live product checks are recorded in [the run report](README.md): full delivery through exact-head Actions, unchanged usage on reattachment, and one-pass repair with dirty-source refusal followed by safe handback. [The isolated HOME GitHub check](fresh-github.log) also confirms authenticated setup without copying credential files.

The retained repository is private. PRs #1 and #2 remain unmerged. The Greenhouse Task branch has only local commits. Manual human acceptance and the remaining SPEC.md work are listed in [the implementation plan](../../implementation-plan.md#unsupported-spec-behavior-in-this-slice).
