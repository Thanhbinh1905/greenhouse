# Development

## Verification

Set `TMPDIR` to an isolated scratch directory, then run:

```sh
go test ./...
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
go build -o "$TMPDIR/greenhouse" ./cmd/greenhouse
python3 tools/terminal_check.py "$TMPDIR/greenhouse" docs/evidence
python3 tools/native_start_check.py "$TMPDIR/greenhouse" docs/evidence/native
python3 tools/recovery_check.py "$TMPDIR/greenhouse" docs/evidence/recovery
python3 tools/events_check.py "$TMPDIR/greenhouse" docs/evidence/events
python3 tools/tui_check.py "$TMPDIR/greenhouse" docs/evidence/tui
python3 tools/notices.py "$TMPDIR/greenhouse" --check
GREENHOUSE_TEST_BINARY="$TMPDIR/greenhouse" python3 tools/daemon_cleanup_test.py
```

Integration fixtures use private state homes and temporary Git repositories, and build the executable with the race detector. Logs are under the selected evidence directory. Each PTY check scans `/proc` for daemons whose `GREENHOUSE_HOME` is under its fixture roots. The final sweep sends SIGTERM, then SIGKILL to survivors, and fails if it finds any fixture daemon. `daemon_cleanup_test.py` tests the sweep after a forced failure and with a planted daemon.

Restart fixtures seed synthetic usage and interrupted-writer state only while their daemon is stopped. They are not review evidence.

After a dependency change, regenerate the notices with `python3 tools/notices.py "$TMPDIR/greenhouse"`.

### PTY checks

`terminal_check.py`, `native_start_check.py`, `recovery_check.py` and `events_check.py` set `TERM=dumb`, so they drive the line client.

`terminal_check.py` drives real 56-, 80- and 100-column PTYs through lifecycle and settings flows and these scenarios:

- configuration migration, repository trust, per-value sources and a repository Save
- the transcript log
- a backend quota pause with its explanation, auto-resume cancellation, and an agent switch and rerun
- selected repair, check results, dirty-handback refusal and safe handback
- full-pipeline repair to CI readiness, a failed gate with diagnostics and a repaired-unreviewed stage
- grouped ask and cap decisions answered with `d`, final-fix and accepted risk
- resolution responses, the review snapshot and confinement
- AXI parity

Pass `responses` as a third argument to run only the resolution-response flow; add `before` after it to record an older binary's behavior.

`events_check.py` opens the terminal in 56-, 80- and 100-column PTYs. It starts a review over AXI without a keypress and requires the terminal to redraw through `running` to the findings. It then follows a host-owned run and requires its terminal mutation to refuse. Pass `before` as a third argument to record an older binary's behavior instead.

`recovery_check.py` kills the daemon right after a selected repair commits. It captures the recovery screen before and after `adopt` at 56 columns, `discard` at 80 and `new-run` at 100.

### TUI check

`tui_check.py` drives the interactive screen in an isolated tmux server at 56, 80 and 100 columns by 24 rows. It needs `tmux`. It uses fake native agents and synthetic agent sessions in a private `HOME`, and covers:

- a missing agent
- one Run with collected intent and history isolation
- the no-match and ambiguity questions
- intent editing with Esc
- a moved head
- progress, resize while running, cancel versus detach, and reattach
- findings selection, repair and handback
- details and help
- a Settings draft with Apply and Save
- a decision
- daemon loss and resync
- `NO_COLOR`
- the line client under piped stdin and `TERM=dumb`

## Review evaluation

`cmd/eval/` is a development command that measures independent review with a real model. It is not part of the `greenhouse` executable, and `go test ./...` never calls a model.

### Corpus

The corpus in [`internal/eval/testdata/corpus`](../internal/eval/testdata/corpus) holds small Git changes with known findings: planted correctness bugs, `unslop` and `no-comments` violations, out-of-scope noise, clean changes, and judgment cases.

- Each `case.json` records the change intent, the selected rules, and every expected finding by file, candidate line range, origin, rule, severity and acceptable priorities.
- Each expected finding has an `id` and a `defect` description.
- Each case has a `split` (`tune` or `holdout`), a `stratum` and `source` metadata. `run`, `judge` and `rescore` take `-split tune|holdout|all` (default `all`).
- A judgment case lists `allowed` ranges. A finding in an allowed range earns no credit and is not a false positive.
- `noise` lists lines that must not be flagged.
- `corpus.json` lists the cases and a revision number. Reports also record a digest of every corpus byte.

After a ground-truth change, `go run ./cmd/eval rescore docs/evidence/review-eval/*/` scores the recorded findings again from their adjudication records.

### Run

```sh
go run ./cmd/eval check
go run ./cmd/eval run -model openai/gpt-5.6-luna \
  -parallel 4 -label "what changed" -out docs/evidence/review-eval/NN-name
```

Add `-backend claude -model claude-sonnet-5-5` to review through the Claude backend. `-agent` names the `claude` executable (default: `claude` on `PATH`). The judge still runs on Pi with `-judge-model`.

For each case, `run` commits fixed base and head trees and drives Greenhouse through AXI. A PATH-installed recording shim forwards the emitted arguments and input to the native CLI. Configure evaluation routing in that CLI; Greenhouse creates no provider files. Runs are paid model calls.

### Output

`report.md` summarizes a run. `report.json` holds the configuration, every case record and the summary.

Each case directory holds:

- `input.json`: the exact stdin bytes
- `argv.json`
- `events.jsonl`: the backend's events without streaming deltas
- `stderr.log`
- `timing.json`

### Metrics

- A finding is eligible for an expected finding when file, origin and rule are equal, the line ranges overlap, and the finding spans at most 15 lines. Eligibility earns no credit by itself.
- `adjudications.json` holds one record per eligible finding: the finding index and digest, the expected defect ID, a verdict (`detects`, `different-defect` or `duplicate`), and a one-sentence rationale that quotes the finding. `run` produces it with a separate judge call per finding (`-judge-model`, default `-model`). `go run ./cmd/eval judge -model provider/id run-dir` produces it for saved runs. The judge model and prompt are recorded. An eligible finding without a record earns no credit.
- Recall is detected over expected findings. High-severity recall counts only planted `high` findings. Precision is matching findings over all findings except those in allowed ranges.
- Priority accuracy counts detections whose priority is in the expected list.
- A false positive detects and duplicates nothing. A `different-defect` record makes it a wrong-defect false positive. A blocking result is a P0 or P1 false positive. An inconclusive review on a clean case also blocks.
- The bar is a high-severity recall of at least 80% and no blocking result on a clean case.
- Inconclusive reviews score no findings. `model_answer` keeps the model's raw answer.
- Cost uses Pi's catalog pricing; a proxy or subscription may bill differently. Latency is the wall time of the Pi process.

### Results

[The recorded runs](evidence/review-eval/README.md) document each reviewer change, its measured effect and a model comparison.

On this corpus, `openai/gpt-5.6-luna` through Pi stays the recommended default reviewer. One run of `claude-sonnet-5-5` through the Claude backend matched or beat it on recall and precision at about 20 times the cost, so it is the recommended reviewer `fallback`. `xai` answered `402` (no credits), and `google`, `openrouter` and `meta` had no credentials.

### Real posse PR regression set

[`tools/posse_review_regression.json`](../tools/posse_review_regression.json) pins the ten exact PR heads, merge-base targets and expected outcomes from the t167 benchmark. `tools/posse_review_regression.py` runs the cases through the built Greenhouse AXI CLI with reviewer Pi, builtin `unslop` and `no-comments`, and one fresh state home per repetition. It records each exact-head host result as JSON. Runs make paid model calls.

Clone posse and fetch every pinned head and target commit before running; all worktrees, homes and result files must stay in private scratch:

```sh
git clone https://github.com/Thanhbinh1905/posse.git "$TMPDIR/posse"
python3 - "$TMPDIR/posse" <<'PY'
import json, subprocess, sys
from pathlib import Path
repo = Path(sys.argv[1])
cases = json.loads(Path("tools/posse_review_regression.json").read_text())
for sha in sorted({value for case in cases for value in (case["head"], case["target"])}):
    subprocess.run(["git", "-C", str(repo), "fetch", "--no-tags", "origin", sha], check=True)
PY
go build -o "$TMPDIR/greenhouse" ./cmd/greenhouse
python3 tools/posse_review_regression.py \
  --binary "$TMPDIR/greenhouse" --source "$TMPDIR/posse" \
  --out "$TMPDIR/posse-regression" --runs 2
```

The expected finding checks require the expected priority, path and overlapping candidate line range. PR 1146 checks specifically for the historical P1 at `scripts/preflight.sh:44-45`; a different finding elsewhere in that file is reported separately and does not satisfy or fail that location-specific check. A clean case requires a clean verdict; an inconclusive result is not clean.

The first current-main baseline used `9b9ba69` (after t170), two runs per PR, Pi reviewer `openai/gpt-5.6-luna`, default three-minute review budget, and isolated host-mode homes. The 20 reviews cost $1.02781 total, with a median of $0.04738 per review. Results:

| PR | Expected | Run 1 | Run 2 | Match |
|---:|---|---|---|---:|
| 1131 | P1 at `plan_blobs.go:94` (#1129) | clean | P2 `queries.sql:370-373` | 0/2 |
| 1156 | P1 at `greenhouse_watch.go:159` (#1145) | clean | clean | 0/2 |
| 1168 | P1 at `validation_summary.go:72-93` (#1157) | P1 `review_pass_next_step.go:129-131`, P1 `review_verdict.go:191-192`, P2 `validation_summary.go:101-106` | P1 `validation_summary.go:105-110` | 0/2 |
| 1146 | no P1 at `preflight.sh:44-45` | clean | P1 `preflight.sh:68-71` | 2/2 at the specified range |
| 1152 | clean | P1 `review_rounds.go:175-176` | clean | 1/2 |
| 1159 | clean | P1 `watch_runtime.go:1261-1266` | clean | 1/2 |
| 1164 | clean | clean | clean | 2/2 |
| 1165 | clean | P3 `ci-test-results.md:3` | clean | 1/2 |
| 1167 | clean | inconclusive (`review_budget`) | inconclusive (`review_budget`) | 0/2 |
| 1158 | clean | P1 `watch_runtime.go:1096-1104` | P2 `watch_runtime.go:1096-1104` | 0/2 |

The three intended defects were not detected at their expected locations. PR 1168's finding at `validation_summary.go:105-110` overlaps neither the expected lines nor the recorded t167 defect, so it receives no credit. The specified 1146 false P1 was absent in both runs; the separate P1 at lines 68-71 is outside that expected range. Five of twelve runs on the six clean cases returned clean, and only PR 1164 was clean in both runs. PR 1167 was inconclusive twice.

There was no prompt or criteria change: this baseline did not improve recall, and the six clean cases did not stay clean across both runs. The regression set and baseline are added without changing reviewer behavior.

## Fixer evaluation

`go run ./cmd/eval fix-run` measures selected-finding repair with a real model through the product's one-pass repair path. Like the review evaluation, it is development-only, and `go test ./...` never calls a model.

### Cases

[`internal/eval/testdata/fixer`](../internal/eval/testdata/fixer) holds nine defect cases and one dispute case. Each case records base and head trees, findings, selected findings, the expected response (`fixed` or `disputed`) and the allowed edit ranges.

- Defect cases include a reference repair in `fix/`.
- Every case has hidden tests in `hidden/`. Hidden `TestAccept` tests check the selected finding. `TestRegress` tests and the candidate's own tests check behavior that must survive.
- The fixer never sees the hidden tests.

`go run ./cmd/eval fix-check`, also run by `go test ./internal/eval`, proves two things: every acceptance test of a fixed case fails on the head while the other tests pass, and the reference repair passes every test inside the scope.

### Run

```sh
go run ./cmd/eval fix-check
go run ./cmd/eval fix-run -parallel 4 \
  -label "what changed" -out docs/evidence/fixer-eval/NN-name
```

For each case, `fix-run` seeds a fixed review, records the native fixer invocation, runs trusted checks, and evaluates the resulting commit with hidden tests. The native CLI owns login and routing.

### Metrics

- A case is fixed when the response matches the expected one, every acceptance test passes, no other test fails, and the repair has no scope violation.
- A regression is a failing `TestRegress` or candidate test.
- A scope violation is a changed line outside the case's scope ranges, an insertion that borders none, an added or deleted file, or an edit to another path that Greenhouse refused. Unselected findings lie outside the scope ranges.
- Resolution-response accuracy counts cases whose responses match the expected ones.
- Cost uses Pi's reported cost, or the reported tokens at `-price` when Pi reports none. Latency is the wall time of the fixer process.
- The bar is at least 75% of cases fixed, with no regression and no scope violation on any case.

### Results

[The recorded runs](evidence/fixer-eval/README.md) show the default fixer meeting the bar at 90% with no regression or scope violation. A `fixed` response without an edit to the finding's file now fails the repair. The latest run uses Pi's `medium` thinking level; the remaining dispute case fails.
