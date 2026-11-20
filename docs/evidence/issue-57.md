# Issue 57: fixture daemon cleanup

## Cause

The reported trigger could not be reproduced on current main. Four temporary `tui_check.py` runs failed at injected steps, including after a review started, after a second fixture session opened, and during the reconnect scenario. None left a daemon under its fixture root. The original leak came from t161's in-progress branch, so its exact cause remains unknown.

## Cleanup

The PTY checks now scan `/proc` in their outermost `finally` blocks. They match the `greenhouse daemon` command and require `GREENHOUSE_HOME` to resolve below that check's fixture root. The sweep sends SIGTERM, waits, sends SIGKILL to survivors, and fails with each PID and home it had to clean up. The TUI check kills its tmux server before the sweep.

`tools/daemon_cleanup_test.py` proves two cases:

- An injected assertion after starting a fixture daemon still runs the `finally` sweep, stops the daemon, and preserves the assertion failure.
- A planted extra daemon is stopped, reported, and causes the check to fail.

A separate forced-failure run of a temporary `tui_check.py` copy failed after starting the review. It left no daemon under the Task scratch directory. `pgrep -af '^/home/thanhbinh/.posse/scratch/greenhouse/t162/greenhouse daemon$'` returned no matches, and `find "$TMPDIR" -path '*/state/pid' -print` returned no paths.

One check run exposed an existing race in `Fixture.settle()`: it returned while handback still reported `handback-in-progress`. The check now waits for handback to finish before asserting its result. The next run passed.

The planted-daemon test printed `daemon cleanup: terminating pid 1933092 GREENHOUSE_HOME=/home/thanhbinh/.posse/scratch/greenhouse/t162/tmpo5adlk_5/state`, then raised `fixture daemons required cleanup`. A follow-up `pgrep` found no Task-built fixture daemon.

## Verification

All checks ran with `TMPDIR=/home/thanhbinh/.posse/scratch/greenhouse/t162`:

| Check | Result |
| --- | --- |
| `GREENHOUSE_TEST_BINARY="$TMPDIR/greenhouse" python3 tools/daemon_cleanup_test.py` | pass: forced failure cleanup and planted daemon detection |
| `python3 tools/terminal_check.py "$TMPDIR/greenhouse" "$TMPDIR/evidence/terminal"` | pass |
| `python3 tools/native_start_check.py "$TMPDIR/greenhouse" "$TMPDIR/evidence/native_start"` | pass at 56, 80, and 100 columns |
| `python3 tools/recovery_check.py "$TMPDIR/greenhouse" "$TMPDIR/evidence/recovery"` | pass |
| `python3 tools/events_check.py "$TMPDIR/greenhouse" "$TMPDIR/evidence/events"` | pass at 56, 80, and 100 columns |
| `python3 tools/tui_check.py "$TMPDIR/greenhouse" "$TMPDIR/evidence/tui"` | pass |
| `python3 tools/notices.py "$TMPDIR/greenhouse" --check` | pass |
| `go test ./...` | pass |
| `go test -race ./...` | pass |
| `go vet ./...` | pass |
| `golangci-lint run ./...` | pass: 0 issues |
| `gofmt -l .` | no output |

## Rollback

Revert the Task commit. The cleanup only affects fixture daemons whose state home resolves under the check's private fixture root.
