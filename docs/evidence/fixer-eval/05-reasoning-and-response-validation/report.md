# Fixer evaluation: Reject fixed without edits; preserve catalog reasoning

- Fixer: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0; the review is seeded from each case
- Greenhouse: `f2814950295a4f1a917303916c87cc70efcbd419`, fixer corpus v1 revision 1 `18abdc4ea64b9b609bc405f0e0a458e8f25f0970c95b16a693c0e25a1ee9aaa2`
- Started 2026-11-09T07:07:36Z, finished 2026-11-09T07:08:54Z, parallel 4

Bar (at least 75% of cases fixed, no regression, no scope violation): **met**

| Metric | Value |
|---|---|
| Fixed | 90% (9/10) |
| Hidden acceptance tests passing | 10/10 |
| Regressions (cases / failing tests) | 0 / 0 |
| Scope violations (cases / edits) | 0 / 0 |
| Correct resolution responses | 9/10 |
| Cost | $0.0130 |
| Fixer latency total / mean / max | 143.0s / 14.3s / 17.5s |

| Case | Repair status | Response | Acceptance | Regressions | Scope violations | Fixed | Cost | Latency |
|---|---|---|---|---|---|---|---|---|
| dispute-guarded-division | fixer-failed | fixed | 1/1 | none | none | false | $0.0013 | 17.5s |
| fix-case-insensitive | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0010 | 12.3s |
| fix-dropped-decode-error | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0015 | 13.7s |
| fix-empty-average | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0011 | 14.8s |
| fix-needless-comments | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0016 | 13.6s |
| fix-nil-map-write | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0011 | 12.6s |
| fix-offbyone-window | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0015 | 14.0s |
| fix-pagination-bounds | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0010 | 14.6s |
| fix-swallowed-write-error | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0016 | 12.9s |
| fix-unlock-missing | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0012 | 16.8s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the fixer on stdin, `argv.json` the exact arguments, and `events.jsonl` the fixer's JSON event stream. `report.json` holds each repair diff, check result, resolution response and hidden test outcome.
