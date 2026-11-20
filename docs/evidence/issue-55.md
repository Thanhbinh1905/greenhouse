# Issue 55 verification

The two failures reproduce on main before these changes.

- `native_start_check.py` read the run while it was still `unconfigured`, before the asynchronous review request set `review_used`. Its loop only waited when the status was already `running`, so it asserted `clean-review` against an early snapshot. This was a script bug. The script now waits for review admission and then for the run to settle.
- `tui_check.py` lost its daemon connection during Run. The actual socket path was `/home/thanhbinh/.posse/scratch/greenhouse/t161/gh-tui-7bk_ybfi/cancel-56/state/socket`, 85 bytes including `/socket`, below the 107-byte limit. The watcher received `refused: storage commit in progress; retry the read request` while a state write was underway. It treated that temporary refusal as daemon loss, so Run stopped before the next request. This was a product bug. The watcher now retries that refusal without clearing its stream or marking the connection lost.
- The cancel/detach fixture used two findings for the same file and line. Greenhouse deduplicates findings by changed code, so the fixture's second finding was removed and the expected count now matches the product.

The TUI fixture's daemon socket path was 85 bytes, below the 107-byte `sun_path` limit. Socket path length did not cause the disconnect.

## E2E checks

Commands follow `docs/development.md`, with `TMPDIR=/home/thanhbinh/.posse/scratch/greenhouse/t161`.

- `python3 tools/terminal_check.py "$TMPDIR/greenhouse" docs/evidence` - PASS: resolution responses, snapshot and confinement, PTY configuration and trust flows, transcript logs, quota pause and rerun, repair and handback, full pipeline, decisions, final-fix, accepted risk and AXI parity.
- `python3 tools/native_start_check.py "$TMPDIR/greenhouse" docs/evidence/native` - PASS at 56, 80 and 100 columns.
- `python3 tools/recovery_check.py "$TMPDIR/greenhouse" docs/evidence/recovery` - PASS: recovery before and after adopt, discard and new-run.
- `python3 tools/events_check.py "$TMPDIR/greenhouse" docs/evidence/events` - PASS at 56, 80 and 100 columns: review updates arrive without input; host-run mutations are refused.
- `python3 tools/tui_check.py "$TMPDIR/greenhouse" docs/evidence/tui` - PASS at 56, 80 and 100 columns, including cancel, detach and reattach.
- `python3 tools/notices.py "$TMPDIR/greenhouse" --check` - PASS: notices cover 21 linked modules.

## Go checks

- `go test ./...` - PASS.
- `go test -race ./...` - PASS.
- `go vet ./...` - PASS.
- `test -z "$(gofmt -l .)"` - PASS.
- `go build -o "$TMPDIR/greenhouse" ./cmd/greenhouse` - PASS.
- `golangci-lint run ./...` - PASS: 0 issues.
