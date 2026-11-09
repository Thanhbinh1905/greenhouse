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
