# Interactive terminal evidence (t87)

## Before

`before-80x24-scrollback.txt` is the first screen of the line client in a fresh repository with no agent installed, captured in an isolated tmux server at 80x24. It prints 33 lines, so the top scrolls off a 24-row terminal; `before-80x24-visible.txt` is what stays visible. It reports `Reviewer: pi /` although no agent is on `PATH`, lists two blockers in controller operation names and offers 22 keys.

## After

Each `walkthrough-*.txt` file is a real PTY session driven by `tools/tui_check.py`: every key sent, then the screen as tmux rendered it, framed at its width.

| File | Size | What it shows |
| --- | --- | --- |
| `walkthrough-no-agent-{56,80,100}.txt` | 56, 80, 100 x 24 | Missing agent reported on the first screen; Enter starts nothing; `q` quits with a notice. |
| `walkthrough-cancel-detach-56.txt` | 56 x 24 | One Run; no matching agent session, so Run asks once; an answer wider than the screen with `q` and `x` in it, deletion, and resize to 100 and back while editing; progress with stage, agent and time; resize to 100 and back while running; `x` asks again; `q` detaches and says the review keeps running. |
| `walkthrough-cancel-reattach-56.txt` | 56 x 24 | Reattach shows the running review with `≥` time; `x` `x` cancels; Rerun is offered. |
| `walkthrough-findings-repair-80.txt` | 80 x 24 | Tab to the Intent row, typing, Esc back, typed intent kept and used by Run over a collected one; findings list, full finding, selection, repair confirmation, handback, details and help. |
| `walkthrough-settings-decision-100.txt` | 100 x 24 | Settings draft marked with `*`, kept after Esc, Apply and Save with their scopes; Run pipeline with intent collected from a synthetic Claude Code session while another repository's session is ignored; cap decision with a typed reason; daemon stopped (`RECONNECTING`, only views in the actions menu); resync. |
| `walkthrough-ambiguous-100.txt` | 100 x 24 | Two sessions match equally; Run asks which one and uses the choice. |
| `walkthrough-moved-head-100.txt` | 100 x 24 | Head moved after opening; Run stops with the controller's refusal and sends nothing else. |
| `color-80.ansi`, `no-color-80.ansi` | 80 x 24 | Raw screen with escape sequences; the `NO_COLOR=1` capture has no SGR sequence. |
| `walkthrough-term-dumb-80.txt`, `line-mode-piped.txt` | 80 | `TERM=dumb` and piped stdin keep the line client. |

## Commands and results

All commands ran on Linux under `taskset -c 10,11 nice -n 15 env GOMAXPROCS=2 GOFLAGS='-p=1'`, one at a time, with `TMPDIR` set to the Task scratch directory.

| Command | Result |
| --- | --- |
| `gofmt -l .` | no output |
| `go vet ./...` | pass |
| `staticcheck ./...` | pass |
| `go test -p 1 -parallel 1 -timeout 1800s ./...` | pass |
| `go test -race -p 1 -parallel 1 -timeout 3000s ./...` | pass (`greenhouse` 448 s, `greenhouse/eval` 35 s) |
| `python3 tools/terminal_check.py`, `native_start_check.py`, `recovery_check.py`, `events_check.py` | pass; they now set `TERM=dumb` and drive the line client |
| `python3 tools/tui_check.py "$TMPDIR/greenhouse" docs/evidence/tui` | pass |
| `python3 tools/notices.py "$TMPDIR/greenhouse" --check` | pass: 21 linked modules |

## Dependencies and build version

`go.mod` now requires Go 1.24.2, the minimum of `github.com/charmbracelet/bubbletea` v1.3.10 and `github.com/charmbracelet/bubbles` v1.0.0. The executable links 21 modules: MIT, except `github.com/atotto/clipboard` and `golang.org/x/sys` (BSD-3-Clause). `THIRD_PARTY_NOTICES.md` carries every license text and `tools/notices.py --check` compares it with the built binary. The protocol, the run store and AXI are unchanged.

## Known limitations

- Elapsed time is measured by the client. After a reattach it counts from the attach and shows `≥`.
- Installed agents are detected from the client's `PATH`, as before; the daemon's `PATH` can differ.
- Intent collection reads only sessions whose working directory is inside the reviewed worktree, modified since 72 hours before the target commit. A session started in another worktree of the same repository is not found. Codex sessions are read from `~/.codex/sessions` rollout files, not from Codex's SQLite state.
- Collected requests are never cut. When they exceed the 8192-byte intent limit, Run asks for a shorter intent instead.
- A session matches by mentioning the changed file paths. It does not use a model, so it cannot weigh two equally matching sessions; Run asks instead.
- The collected intent is stored as the run's intent text with its source label. There is no separate protocol field for the source.
- Decision options show the controller's option names.
- Under `NO_COLOR` the screen also drops bold and reverse video; the `▸` marker shows focus.

## Cleanup

`tools/tui_check.py` stops every fixture daemon it starts and kills its tmux server. `tools/events_check.py` left its three fixture daemons running; they were verified by their `GREENHOUSE_HOME` under the Task scratch directory and stopped with SIGTERM. No other process was touched.

## Rollback

Revert the branch's commits. The line client is unchanged; before a revert, `TERM=dumb greenhouse` opens it on any terminal.
