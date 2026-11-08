# Live standalone validation

Recorded on 2026-11-09 UTC. This record covers the standalone human terminal and AXI paths. The terminal interactions were scripted through real PTYs, using `greenhouse` without arguments. No person manually typed the recorded sessions. The scripted real-PTY sessions were accepted as proof for this Task. The [manual acceptance checklist](manual-acceptance.md) remains available for a later human run. The complete SPEC.md remains unfinished; see [the implementation plan](../../implementation-plan.md#unsupported-spec-behavior-in-this-slice).

## Versions and configuration

- Product base: `2e2e433`, with `live-source.patch` for the completed live flows and `source.patch` for final terminal captures. [Build identities](build.json) record their SHA-256 digests. Initial captures use the base binary.
- Go: `go1.26.7 linux/amd64`. Pi: `1.5.0`. Claude Code: `2.1.319`. Codex: `0.166.0`, installed but not invoked in these runs.
- Full reviewer: Claude Code, requested and reported `claude-sonnet-5-5`, selected through Settings. Full fixer: Pi, `openai/gpt-5.6-luna`.
- One-pass reviewer and fixer: Pi, requested and reported `openai/gpt-5.6-luna`.
- Full settings: `ask full 8`, rules `unslop,no-comments`, auto-resume `off`. One-pass uses built-in `auto one-pass 4` defaults; selected repair still requires authorization.
- Exact trusted configuration files are alongside this record. The OpenAI-compatible endpoint is `http://127.0.0.1:8317/v1`; the configuration names `OPENAI_API_KEY` without its value. Claude uses its existing CLI login. `GH_TOKEN` comes from the existing GitHub CLI login in the launch environment.
- The fixture directory contains the submitted source and `fixture/repaired/stats.py` from the final candidate.

The private repository [Thanhbinh1905/greenhouse-v1-validation-t59-20261109](https://github.com/Thanhbinh1905/greenhouse-v1-validation-t59-20261109) is retained for inspection. It contains only this validation fixture. Neither PR was merged. Greenhouse's own repository was not pushed.

AXI snapshots omit raw input bytes, repeated prompt arguments, and selected diffs. They retain findings, configuration, candidate identities, stage evidence, command digests, budgets, and decisions. The configuration files contain the exact gate argv. Full API snapshots remain in Task t59 scratch. No credential values were written to the evidence.

## Full delivery

Run `8281036a00c4cf49a236983480517711` starts from submitted head `a17ce7f24e9621f1f17df96be7584b54fcdad94b`, targeting `b5e7936ba587cf647feea00263511a111184b7ae`.

1. The change replaces `sum` with `math.fsum` and accidentally removes the empty-input guard. Settings selects Claude as reviewer and Pi as fixer. Prepare records the public behavior; Load rules snapshots both rule packs.
2. Pipeline invokes Claude once. It reports P1: `average([])` divides by zero. [The gate snapshot](full2-gate.json) and the 56-, 80-, and 100-column decision captures show the same finding and gate.
3. Detach and reattach retain the run ID, candidate, finding, and usage. [The reattached snapshot](full2-reattached.json) still shows one review and zero repairs. Attaching starts no work.
4. AXI answers the exact gate with `repair` and the selected full finding ID. The terminal displays the same resulting state. Pi repairs only `stats.py`; Greenhouse commits `2c6e7616ff282be4536a0c9f5c574c049acd2617`.
5. A fresh Claude invocation reviews that exact commit. The P1 is resolved. A P3 note about `fsum`'s numeric edge cases remains informational and visible; this is not a finding-free verdict.
6. The test gate runs `python3 -B -m unittest -v`. Lint parses each Python file with `ast.parse`. Docs checks the README's empty-input contract, and the reviewer supplies the documentation assessment. These are real gates for the fixture, not checks of Greenhouse itself.
7. Greenhouse pushes its candidate branch, creates [PR #2](https://github.com/Thanhbinh1905/greenhouse-v1-validation-t59-20261109/pull/2), and waits for the `test` check on the exact candidate SHA. [The final snapshot](full2-ci-ready.json) reports `ci-ready` and `repaired-pass`, with two reviews and one repair.

GitHub independently reports `test`, `completed`, `success`, head `2c6e7616ff282be4536a0c9f5c574c049acd2617`, app `github-actions`. The [Actions job](https://github.com/Thanhbinh1905/greenhouse-v1-validation-t59-20261109/actions/runs/42512174465/job/128827393531) matches that head. PR #2 is open with `merged: false`.

## One-pass and isolated HOME

An empty HOME under Task scratch installs with `GOBIN="$HOME/.local/bin" go install .`. Its private `reviewer.json`, `providers.json`, and `fixer.json` match the recorded configuration. The launch environment supplies the existing provider key; no credential file is copied into HOME. Go uses Posse's shared build and module caches.

The committed change includes the accurate summation implementation and an accuracy regression test. Bare `greenhouse`, Prepare, Load rules, and Review produce one P1 empty-input finding through live Pi. [The review snapshot](onepass-review-result.json) records the exact inputs and model result.

At 56 columns the terminal abbreviates the long finding ID. Entering that displayed ID authorizes exactly its finding. Pi restores the guard. Greenhouse runs all three unit tests and creates local commit `904df97550676f50a69dfc46057576460c698db9`. [The fixed snapshot](onepass-fixed.json) records one review, one repair, passing checks, and `fixed-not-independently-rereviewed`.

An unrelated untracked note makes handback refuse and survives unchanged. After removing that test note, explicit handback fast-forwards the source. [The final snapshot](onepass-handback.json) still reports the unreviewed result and the original review count. No second review, push, or PR occurs. [Source tests after handback](onepass-source-tests.log) pass.

## Defects and corrections

**Mid-word wrapping and overflowing prompts.** Before captures split `configuration`, `resolve-rules`, and `Settings` inside words; Settings printed its model catalog on one line. Screens and prompts now share sanitized word wrapping, preserve indentation and column spacing, and abbreviate tokens wider than the terminal. Full values remain in AXI. Displayed finding IDs resolve only to unique findings; ambiguous or duplicate selections refuse.

`TestScreenWrapsAtWordBoundaries` failed with `confi\nguration` instead of `configuration` on the next line. `TestWrapWordsKeepsIndentAndFitsIdentifiers` failed on a 128-character token. `TestDisplayedFindingIDSelectsOnlyUniqueFinding` failed because the displayed ID was unknown. Prompt and column-spacing regressions also failed before their corrections. All pass after the fixes.

**Repair restores the target tree.** The initial live Pi repair restored the baseline byte for byte. Rereview stopped with `review admission refused: empty committed diff`, before any independent invocation. [The original stopped snapshot](full-after-repair.json) records it. `TestFullPipelineRereviewsRepairThatRestoresTargetTree` reproduced that refusal, then passed after the fix. Greenhouse now sends the prior reviewed paths with their current committed contents and explicitly explains the empty diff to a fresh reviewer. It does not infer review success. The same parked live run resumed, invoked Claude again, and passed local gates and publication.

**Duplicate check configuration.** The first fixture workflow ran on both push and pull request. GitHub created two exact-head checks named `test`. Greenhouse correctly refused `duplicate required check: test`; [the snapshot](full-result.json) retains that refusal. This is a fixture configuration issue, not a product defect. The second run starts from a committed `pull_request`-only workflow in a separate clone. It reaches readiness without deleting checks or weakening identity checks.

**Cancelled repair becomes terminal before completion.** With the AXI exit delay removed, the race suite exposed a real bug in `TestRerunStoppedSelectedRepairKeepsEvidenceAndCounters/cancel`: immediate rerun refused `stale run/input/gate/token`. The progress handler published `cancelled` and changed the token before the final completion handler did so again. Clients could observe a terminal state while completion remained queued. The new regression failed with `status=cancelled` and a changed token. The progress handler now retains `cancelling`; only final completion publishes the terminal result and enables rerun. The existing pre-commit cancellation assertion now distinguishes the stopped executor from the daemon's pending completion.

**Race-suite restart timing.** The first `go test -race ./...` run failed `TestAutoResumeWithBackoffSurvivesDaemonRestart`: `run never reached [clean-review]: <nil>`. Go's race-enabled AXI child processes wait one second at exit by default. A two-second scheduled retry can therefore begin after the response snapshot but before the test stops the daemon. The test helper now appends `atexit_sleep_ms=0` to the child's `GORACE` settings. Production scheduling and recovery are unchanged. Five isolated repetitions passed before the adjustment, so the timing cause is an inference from the runtime behavior and the retry interval, not a consistently reproduced product fault. Final results are in `verification.md`.

**Dead code.** `runCaptured` returned stdout that its only caller discarded. The code now runs directly in `runBounded`; its output limits and exit handling are unchanged.

## Terminal evidence and checks

[PTY captures](pty/) retain the control sequences and typed commands; CR characters and trailing whitespace are removed. Original captures remain in Task scratch. Before: `before-56-settings`, `before-56-decision`, `full-dispatch-80`, and `full-authorize-100`. After: `full2-dispatch-56`, the three `full2-decision` captures, the three `full2-ci-ready` captures, the one-pass authorization and handback captures, and `final-ready` reattachments with the final binary.

`tools/terminal_check.py` drives real 56-, 80-, and 100-column sessions with deterministic backends. It covers settings, rules, findings, selected repair, checks, dirty and clean handback, pipeline stages, failures, decisions, final-fix, accepted risk, quota pause, agent switch, and AXI parity. Those deterministic backends test behavior; the separate live runs above establish provider and forge operation.

See [verification results](verification.md) for the regression failures and final Go and PTY checks.
