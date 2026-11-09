# T81 follow-up verification

The resumed task closes two findings at product commit `f281495`.

- A selected finding answered `fixed` must have an edit to its file. Otherwise the repair ends as `fixer-failed`, retains the response for diagnosis, and creates no commit.
- Private Pi provider entries carry the trusted catalog reasoning flag. Review and repair executions and attempts record Pi's reported effective thinking level as `thinking`.

## Red before and green after

The regression tests exercise the executable through AXI with real Git and fixture backends. Against the preceding product code, `TestFixedResponseWithoutEditRefusesTheRepair` failed because the repair settled as `status:no-change` with a `fixed` response. `TestPiReviewerRunsWithTheCatalogThinkingSetting` failed because the private model entry contained `"reasoning":false` for a catalog model that supports thinking.

Both pass after the changes. `TestPiFixerRecordsTheEffectiveThinkingLevel` also verifies the fixer's private model entry and the effective level in both its repair and attempt records. The existing pipeline tests exercise review and repair completion through full pipelines. No network or model call is part of these tests.

The first full-suite run exposed four specialized fake Pi fixers that ignored `--list-models`. They executed their kill, delayed-write, or shared-writer behavior during model discovery. Those fixtures now answer the Pi catalog command before running the behavior under test. All four custody and cancellation tests pass together afterward. The earlier thinking-test draft also expected `committed`; the established one-pass status is `fixed-not-independently-rereviewed`, which the final assertion checks.

## Real product path

[Fixer run 05](fixer-eval/05-reasoning-and-response-validation/report.md) uses the real Pi backend and the default fixer on all ten cases. It fixes 9/10, with no regression or scope violation, for $0.0130 at the recorded token prices. Mean fixer latency is 14.3s. Every hidden acceptance test passes. The dispute case falsely answers `fixed` without an edit and now fails explicitly: `resolution response fixed ... without editing ratio/ratio.go`. Its repair head is empty. Response accuracy remains 9/10; the contradiction is visible as a failure.

[Review run 12](review-eval/12-reasoning-restored/report.md) repeats all sixteen cases with the default reviewer. Recall is 100% (13/13), high-severity recall is 100% (9/9), precision is 93% (13/14), and no clean case blocks. Mean reviewer latency is 14.4s. The sole false positive concerns a helper's doc comment, also seen in run 11. No measured review regression appears on this pass.

Pi reports `medium` thinking for every fixer and reviewer case. Each run records the exact configuration, product commit, delivered input, arguments, final messages, and timing. The private provider entry still has no prices, so the review report's zero cost is not a claim of free calls. Event captures retain token usage.

## Scope and limitations

The cumulative branch adds the ten-case fixer corpus and harness, adopted-review snapshot verification after restart, the t80 error-string correction, and the v1 status documentation. The follow-up changes preserve those results. Slop and comment checks apply only to branch changes; narrating comments were removed from the added harness functions.

The corpus has one dispute case. One reasoning-enabled pass cannot estimate model variance. The file-edit check detects contradictory responses, but only independent review and tests can establish whether an edit resolves a finding.

## Resource bounds and reused evidence

The host overloaded during overlapping unrestricted verification runs. Those runs stopped; three surviving events-PTY daemons received SIGTERM and exited. A process scan found no remaining t81 test-owned processes. The Lead's HTTP 400 `unsupported websocket request type: response.interrupt` was a harness control-path failure, separate from Greenhouse results.

Verification resumed with CPU affinity limited to logical CPUs 10 and 11, niceness 15, `GOMAXPROCS=2`, and `GOFLAGS=-p=1`. Go tests also use `-p 1 -parallel 1`. The fixer evaluation's hidden-test commands preserve `GOFLAGS`. Those commands and its product check pass the serialization flags. No product behavior changed, so the successful paid evaluations above are reused.

The small host-review recovery check passed twice under these limits. The [affinity capture](t81-affinity.json) records `go test`, `greenhouse.test`, an AXI client, and a daemon restricted to CPUs 10 and 11. The cleanup scan after the check found no remaining test-owned processes. The earlier non-race failure involved reading a fixture PID while empty and reconnecting to a killed listener under overload; the bounded checks passed without changes to that fixture.

The successful terminal, recovery, and events PTY checks at 56, 80, and 100 columns are reused. Their logs are in the task scratch directory as `terminal-followups.log`, `recovery-followups.log`, and `events-followups.log`. The PTY captures are in `pty-followups/`. The three daemon leaks came from the events check and were reported and stopped before bounded verification resumed.

## Final bounded checks

Each command below ran sequentially with the prefix `taskset -c 10,11 nice -n 15 env GOMAXPROCS=2 GOFLAGS='-p=1'`:

- `go test -p 1 -parallel 1 -timeout 30m ./...`: passed, Greenhouse 373.855s and eval 18.777s.
- `go test -race -p 1 -parallel 1 -timeout 30m ./...`: passed, Greenhouse 395.983s and eval 25.754s.
- `go vet ./...`: passed.
- `staticcheck ./...`: passed.
- `gofmt -l eval/fixer_score.go eval/fixer_live.go`: no output. The earlier whole-tree formatting check also passed.

`git diff --check` passes. The suite logs are `bounded-suite.log` and `bounded-race.log` in the task scratch directory. No timeout occurred. Process scans after both full suites found no remaining t81 test-owned processes. A live scan during the race suite confirmed that its test binary and daemon retained CPUs 10 and 11. No verification jobs ran concurrently after the resume. The only changes since the recorded live runs are documentation, comment removal, fixture catalog handling, and test-resource settings; product code is unchanged.
