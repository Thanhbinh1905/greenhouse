# Review corpus revision 5

The corpus contains 166 cases, including a fixed 59-case hold-out split with 44 real-source cases. This change builds the evaluation instrument. It does not establish a measured quality improvement.

Corpus digest: `ed7b98b0f75457fb748cf7d327121cfed60332eea16d839183d905366a8a6df8`.
Revision 4 digest: `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`.
The loader computes the digest from the stored corpus, including its source inventory and notices.

## Counts

| Source | Tune | Hold-out | Total |
|---|---:|---:|---:|
| Local history and mutations | 33 | 27 | 60 |
| Public history | 43 | 17 | 60 |
| Synthetic, including the original 16 | 31 | 15 | 46 |
| Total | 107 | 59 | 166 |

| Primary stratum | Cases |
|---|---:|
| Reversed bug fix | 35 |
| Injected defect | 42 |
| Clean real change | 45 |
| Security-sensitive | 13 |
| Refactor-only | 11 |
| Cross-file | 10 |
| Judgment | 10 |

Strata also act as tags. Counting each case once per trait gives 26 security-sensitive cases, 5 tagged large changes and 20 tagged tiny changes. Computing inserted plus deleted lines from the stored base and head trees gives 7 changes over 1,000 lines and 69 nonempty changes under 10 lines. The count uses line-level longest common subsequences, rather than total file lengths.

| Changed-file language | Cases |
|---|---:|
| Go | 74 |
| Python | 47 |
| TypeScript | 22 |
| Rust | 22 |

Go is 44.6% of all cases. One retained prose case has no code language. Languages are inferred from changed file extensions; unchanged callers do not inflate the counts. Categories are 94 bug, 2 policy, 2 noise, 58 clean and 10 judgment.

## Collection and labeling

The original 16 cases retain every existing field, expected range and tree byte. Metadata identifies them as handcrafted synthetic cases. The revision is 5; the schema version remains 1.

Local cases use exact commits from this repository. Fifteen reverse specific fixes; 25 inject defects into historical source; five inject a defect into a large historical change; ten construct behavior-preserving refactors of historical functions; five retain reference changes. Each derivation distinguishes upstream changes from constructed mutations or refactors. The large cases retain the relevant multi-file changes and tests so their size reflects a real change. They are review snapshots, not complete repository checkouts.

Public cases use exact commits from three repositories. Twenty reverse fixes and forty retain reference changes. Collection inspected the pinned patches and surrounding declarations. Fix reversals retain supporting fields and signatures where needed to keep the defective path reachable. Expected ranges point into the resulting head excerpts. Full notices were checked at every selected commit and identical notices were deduplicated inside the corpus root.

| Public repository | Cases | License | Notice |
|---|---:|---|---|
| pallets/werkzeug | 26 | BSD-3-Clause | `LICENSE-werkzeug-3b49dcee4105.txt` |
| colinhacks/zod | 12 | MIT | `LICENSE-zod-3f1189b28e38.txt` |
| rust-itertools/itertools | 22 | MIT option | `LICENSE-itertools-7576269ea71f.txt` |

[The source inventory](../../../internal/eval/testdata/corpus/SOURCES.md) lists every public case, commit link, license and file. Per-case derivations record excerpt coordinates and transformations. Excerpts retain changed declarations and their supporting class or implementation headers. Unrelated declarations and imports are omitted. The excerpts are not standalone upstream packages.

Thirty new synthetic cases cover ten unchanged-caller contracts, ten security boundaries and ten judgment decisions. Python callers exercise serialization, time, money, result shape, retry and security contracts. Judgment cases cover deadlines, negative caching, replica failover, notification concurrency, logging, eviction, page caps, jitter, compression and polling. Their contracts leave the disputed decision open; a matching concern is acceptable, not required.

## Split and scoring

[HOLDOUT.md](HOLDOUT.md) freezes all 59 held-out IDs. For real sources, assignment uses the first eight hexadecimal digits of SHA-256 of `repo + ":" + commit`, modulo three: zero selects hold-out. Cases from the same source commit stay together. The original synthetic cases use their original sorted index modulo three; new synthetic cases use a deterministic case-ID hash. Assignment occurred before tuning. No prompt or model tuning was performed.

`run`, `judge` and `rescore` accept `-split tune|holdout|all`, defaulting to `all`. Explicit case IDs must belong to the requested split. Reports record the selection, counts by split and primary stratum, and separate allowed-finding counts. Split-specific judgment and rescoring write into a `tune/` or `holdout/` child of the input report directory, preserving the mixed report. Cost and latency summaries use the selected records.

A judgment case has allowed ranges with an origin, optional policy rule and rationale. Matching uses the existing location tolerance and maximum finding span. An eligible required finding takes precedence. An allowed finding earns no detection credit, adds no false positive or blocking count, and is excluded from the precision denominator. A required-detection adjudication on an allowed finding is rejected. Other adjudication rules and the evaluation bar are unchanged.

## Validation

All commands below passed on this branch. Heavy Go commands ran sequentially with the required CPU and build limits:

```sh
taskset -c 10,11 nice -n 15 env GOMAXPROCS=2 GOFLAGS=-p=1 go run ./cmd/eval check
taskset -c 10,11 nice -n 15 env GOMAXPROCS=2 GOFLAGS=-p=1 go test ./internal/eval/... -p 1 -parallel 1 -count=1
taskset -c 10,11 nice -n 15 env GOMAXPROCS=2 GOFLAGS=-p=1 go vet ./internal/eval/...
gofmt -l internal/eval/*.go
git diff --check
python3 -B internal/eval/validate_fixtures.py
```

The full eval suite passed. Formatting and vet emitted no output. The deterministic `check` printed the counts above and the revision 5 digest. It enforces source minima, real-source strata, hold-out counts, source-commit grouping, language mix, actual change sizes, cross-file structure and judgment ranges. Tests mutate a valid corpus to prove rejection of count, split, license, language and size violations. Strict loader tests cover missing or invalid metadata, public provenance, manifest identity and range constraints. CLI tests prove hold-out filtering for rescoring and judging; the fake judge fails if an excluded tune finding reaches it.

The fixture script verified 40 synthetic Python base/head paths: every base satisfies its caller and every defective head fails. It also executed the public negative-float conversion and indentation-unwind methods: both bases pass and both reversed heads reproduce the defect. All 20 TypeScript judgment modules loaded with Node 22 type stripping. An additional task-scratch Go contract program extracted the ten refactored functions and verified both versions against path, grant, schema, protocol and selection invariants:

```sh
taskset -c 10,11 nice -n 15 env GOMAXPROCS=2 GOFLAGS=-p=1 go run /home/thanhbinh/.posse/scratch/greenhouse/t92/refactor-contracts.go
```

Red-before evidence: the metadata test first failed because `source` was an unknown field; split tests first failed because the selection helpers did not exist. The allowed-finding test first returned `Allowed:0`, two false positives and one blocking finding. The new coverage tests initially failed to compile without `CheckCoverage`. These tests passed after implementation. A baseline comparison separately verified all original 16 labels, ranges and tree files unchanged.

No production code, configuration, release files or root licensing files changed. No model call, paid evaluation, baseline evaluation, credential use, push or PR occurred. Eval checks and fixture validation used stored inputs without network access. Public Git collection used network access only during collection; the three task-owned clones were removed afterward.

## Exclusions and limits

All task minima are met. The broader audit targets remain future work: there is no Java coverage, no independent human labeling panel and no 90-day follow-up audit of reference changes. Clean labels mean no required defect was identified during collection; they do not certify that upstream changes contain no defects.

Public coverage is concentrated in three repositories. Source-commit grouping prevents exact-commit leakage, but neighboring historical commits and related functions can still resemble cases in the other split. Future benchmark interpretation must account for that dependence. The synthetic contracts are small and cannot substitute for real production workloads.

Full upstream suites and every historical local application were not rebuilt or executed. Most real-source labels rely on patch inspection, original regression-test context where present, and range validation. Only the two public methods and ten local refactors described above received extracted runtime checks. Python excerpts parsed, but omitted dependencies can prevent standalone execution; Rust and most public TypeScript excerpts were not compiled as packages. Location validation does not prove a semantic label correct.

Generated files and whole repositories were excluded. No unclear-license repository was included; the selected public sources supplied allowed notices. The license allowlist intentionally excludes other licenses. Corpus metadata and notices preserve attribution inside the permitted paths.

The expanded corpus has not been run against a reviewer model. Precision, recall, stability, cost and quality improvement remain unmeasured. Later tuning must use `tune`; evaluation of a candidate must keep the frozen hold-out split untouched.
