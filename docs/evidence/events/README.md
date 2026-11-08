# Live events and host-mode captures

`tools/events_check.py` drives the bare terminal in real PTYs at 56, 80 and 100 columns, with an isolated HOME, real Git and a fake Pi reviewer.

- `after-<width>-events.log`: the terminal opens with `Events: live`. A review is started over AXI and no key is pressed; the terminal redraws to `running` and then to `outstanding-findings` with the fixture finding.
- `after-<width>-host.log`: a fake host attaches with `host`, prepares and runs a review under grant `round-1` over AXI. The terminal follows the run to `Host: fake-host task-7 | grant round-1 | findings | current true`, and `v` is refused with `run is owned by host fake-host task-7`.
- `before-<width>-events.log`: the `main` binary before this change, same steps. The screen stays at `intent | unconfigured` with no redraw until a key is pressed. The same binary's AXI client refuses the host request with `json: unknown field "host"`, so there is no host capture.
- `menu-<width>-after.log`: a real terminal screen from the updated executable at the listed width, showing every menu entry intact. At 56 columns, `[s] Settings` wraps as one complete entry; at 80 and 100 columns, it remains on the first menu row.

Commands:

```sh
go build -o "$TMPDIR/greenhouse" .
python3 tools/events_check.py "$TMPDIR/greenhouse" docs/evidence/events
python3 tools/events_check.py "$TMPDIR/greenhouse-main" docs/evidence/events before
```
