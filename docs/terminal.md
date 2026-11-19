# Terminal

Run `greenhouse` with no arguments inside a committed Git worktree. On a terminal it opens the interactive screen. When stdin or stdout is not a terminal, or `TERM` is unset or `dumb`, it opens the line client.

## Run a review

From a feature branch with committed changes, run `greenhouse`. The screen shows the branch, the agent it will use, the suggested Git target and one highlighted action, **Run review**. Press Enter.

Run collects the intent, saves it with the target, loads the selected rules and starts one independent review. Each step is a separate controller action. A refused or failed step stops Run and says why; Run never retries it.

Dirty files are excluded from review.

Opening, resizing, navigating and detaching only attach and follow events. Nothing runs and no budget is used.

### Where the intent comes from

Run reads the intent from your own agent history: Claude Code, Codex and Pi sessions that started inside this worktree and mention the changed files.

- Run keeps your requests from that session word for word, redacts secrets, drops tool output, and labels the result with the agent, session and time.
- The intent is never a summary and never comes from the diff.
- When no session matches, Run asks what the change should do. When several sessions match equally, it asks which one.
- When the requests exceed the 8192-byte intent limit, Run asks for a shorter intent. It never cuts them.

To skip collection, Tab to the Intent row and type the intent before Run. Tab to Agent or Target to change them. Changing the agent changes the reviewer only and keeps the fixer settings.

## Interactive screen

Keys act at once, without Enter.

| Key | Action |
| --- | --- |
| Enter | Run the highlighted action. Only this key starts work, and only the action it names. |
| ↑ ↓ Tab | Move between rows and choices. On the home screen, Tab reaches the Agent, Intent, Target and Rules rows to edit them. |
| Esc | Go back one step. During Run, stop after the current step. |
| `a` | All actions the controller allows now, such as Load rules, Start review, Rerun, Rerun from start, New run, Trust repository override, Migrate configuration and Cancel automatic resume. |
| `s` | Settings draft. |
| `f` | Findings: Enter shows the full finding, Space selects it for repair, `r` asks to repair the selection. |
| `d` | Answer the open decision. |
| `i` | Details: run ID, tokens, full commits, digests, sources, rules and criteria (`c`), and the transcript log (`o`). |
| `x` | Cancel running work. Press `x` again to confirm. During Run, `x` stops Run at once. |
| `r` | Recheck installed agents and reconnect. |
| `o` | The transcript log. |
| `?` | Help: keys, and every action with whether it is available now. |
| `q`, Ctrl-C | Detach. Work keeps running in the daemon; run `greenhouse` again to reattach. |

Inside a text field, typed letters go to the field. `q` and `x` neither detach nor cancel there.

Some actions need confirmation:

- New run, Rerun from start, Trust and Migrate need their word typed: `new`, `rerun`, `trust` or `migrate`.
- A recovery needs a reason and its option name typed.
- A decision needs a reason.

The screen does not offer an action the controller does not allow. Help lists it as not available.

The default screens hide run IDs, tokens, full commits and digests. Details and AXI show them.

### Following a run

The screen follows the run through [events](axi.md#events). It redraws when the daemon commits a change, without a keypress.

If the daemon stops, the header shows `RECONNECTING`. The last state stays on screen, and actions stay disabled until fresh state arrives. The screen never starts work or restarts a daemon to follow events. `r` starts the daemon when you ask.

Progress shows the stage, agent and elapsed time. `≥` marks a time measured since you attached rather than since the stage started.

You can follow a run owned by a [host](embedding.md) but not change it.

The screen adapts to the terminal size and needs at least 40x12. `NO_COLOR` removes color; every status also has a symbol and words.

## Line client

The line client reads one command per line. Press Enter after each command.

| Key | Action |
| --- | --- |
| `i` | Refresh inspection. |
| `c` | Toggle criteria details. |
| `l` | Resolve (load) rules. |
| `s` | Open Settings. |
| `n` | Request New run. |
| `p` | Prepare intent and an exact target commit. |
| `v` | Dispatch one review. |
| `f` | Request repair of the finding IDs you enter. |
| `h` | Request handback. |
| `g` | Start or continue the full pipeline. |
| `m` | Authorize the fixer for a target conflict. |
| `d` | Answer the open decision. |
| `e` | Answer the open recovery. |
| `x` | Cancel a review, fixer, check or pipeline step. |
| `r` | Rerun a paused, failed or cancelled stage. |
| `a` | Cancel a scheduled automatic resume. |
| `b` | Start a linked rerun from start. |
| `t` | Trust the repository override. |
| `u` | Migrate older configuration files. |
| `o` | Show the transcript log. |
| `q` | Detach. |

Ctrl-C also disconnects without cancelling the daemon. The screen shows `Events: live` or `Events: reconnecting (daemon unavailable)`.

Single-letter keys in the other docs name line-client commands. On the interactive screen, the same actions are in the actions menu (`a`).

## Settings

Settings edits are drafts until you select Apply or Save.

- Apply adds an immutable run-local snapshot to the current run.
- Save changes user defaults or the trusted repository override without changing the current run. In the line client, Save uses `d` for user defaults and `r` for the repository override. Use Settings twice to do both.
- A draft survives leaving Settings until you Apply, Save or reset it.

Settings lists each current value with its source: `built-in default`, `per-user`, `trusted repository override` or `run-local`. It marks drafted values with `*`.

The terminal edits these settings:

- mode and preset
- the combined review and repair limit
- the reviewer backend and model, and the fixer backend and model
- rule selection
- the auto-resume switch

Each role uses `pi`, `claude` or `codex`; see [agent backends](agents.md#agent-backends). An empty model uses the native default. Explicit Pi models use provider-qualified IDs.

Executables come from absolute PATH directories. Repository overrides cannot name executables; see [repository overrides](configuration.md#repository-overrides-and-trust).

Rule selection does not load bytes. Explicit resolution snapshots the content. Dispatch retains the exact emitted criteria and committed diff.

The terminal wraps prose at word boundaries and abbreviates tokens wider than the screen. AXI retains their full values.
