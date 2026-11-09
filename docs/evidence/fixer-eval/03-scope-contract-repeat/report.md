# Fixer evaluation: repeat of 02

- Fixer: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0; the review is seeded from each case
- Greenhouse: `969a12233224e3f729f3b9e6fe71e4c1ac2a710b`, fixer corpus v1 revision 1 `18abdc4ea64b9b609bc405f0e0a458e8f25f0970c95b16a693c0e25a1ee9aaa2`
- Started 2026-11-09T02:31:21Z, finished 2026-11-09T02:32:13Z, parallel 4

Bar (at least 75% of cases fixed, no regression, no scope violation): **met**

| Metric | Value |
|---|---|
| Fixed | 90% (9/10) |
| Hidden acceptance tests passing | 10/10 |
| Regressions (cases / failing tests) | 0 / 0 |
| Scope violations (cases / edits) | 0 / 0 |
| Correct resolution responses | 9/10 |
| Cost | $0.0129 |
| Fixer latency total / mean / max | 122.4s / 12.2s / 15.4s |

| Case | Repair status | Response | Acceptance | Regressions | Scope violations | Fixed | Cost | Latency |
|---|---|---|---|---|---|---|---|---|
| dispute-guarded-division | no-change | fixed | 1/1 | none | none | false | $0.0008 | 7.2s |
| fix-case-insensitive | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0012 | 7.9s |
| fix-dropped-decode-error | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0015 | 15.4s |
| fix-empty-average | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0015 | 11.4s |
| fix-needless-comments | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0012 | 14.5s |
| fix-nil-map-write | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0011 | 14.9s |
| fix-offbyone-window | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0015 | 10.4s |
| fix-pagination-bounds | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0011 | 13.9s |
| fix-swallowed-write-error | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0016 | 11.9s |
| fix-unlock-missing | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0013 | 14.9s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the fixer on stdin, `argv.json` the exact arguments, and `events.jsonl` the fixer's JSON event stream. `report.json` holds each repair diff, check result, resolution response and hidden test outcome.
