# Agents

Greenhouse runs reviewers and fixers through the native Pi, Claude Code or Codex CLI. The native CLI owns authentication, model discovery and provider routing.

## Discovery and Prepare

Greenhouse discovers `pi`, `claude` and `codex` on PATH and ignores relative PATH entries. Prepare binds the selected executable. Model and agent choices stay ordinary Greenhouse preferences.

Prepare resolves a Git branch or commit to an immutable full commit ID. It records the candidate, the intent and the committed diff. A settings change does not rewrite old evidence.

Review needs current rules and a matching prepared agent configuration. An explicit model must match the model the agent reports. An empty selection records the reported identity of the native default.

## Agent backends

Each review and repair starts a fresh session without the author's conversation.

| Backend | Session setup |
| --- | --- |
| Pi | Ephemeral JSON mode. Extensions, MCP, skills, context files, prompt templates, themes and project approval are disabled. The native `PI_CODING_AGENT_DIR` stays available for login and routing; inherited session variables are removed. |
| Claude | Safe mode, explicit read tools for review, strict MCP configuration and no session persistence. Native user settings keep authentication and routing. An unexpected tool refuses the result. |
| Codex | Native user configuration stays. Hooks, plugins, shell tools, web search and author context are disabled, and the session is ephemeral. Greenhouse asks `codex mcp list --json` for server names and disables each server explicitly; an unreadable server list refuses preparation. Review uses the read-only sandbox; repair uses workspace-write. |

The controller verifies the snapshot after review and checks the repair scope before committing. Child processes have bounded lifetimes. Cancellation keeps the existing writer custody rules. Raw output is kept only in the redacted [transcript log](state.md#transcript-logs).

## Review snapshot

Each review admission records `run.review.snapshot`:

- the candidate commit and its Git tree ID
- the number of entries and bytes
- `digest`: the SHA-256 of `git ls-tree -r -l --full-tree` for that commit

The digest is part of the review input, so the input digest binds it. The snapshot holds at most 100,000 entries and 512 MiB.

Before launch, Greenhouse writes every blob of that listing from the object store into a private directory, without Git attributes or filters. It makes the files and directories read-only, and the reviewer runs there.

After the reviewer exits, Greenhouse compares every file, mode and symlink with the listing. A missing, extra or changed path makes the review `inconclusive`, and `snapshot.check` records `modified: <path>`. Otherwise `snapshot.check` is `unchanged`.

When a restart adopts an interrupted review, Greenhouse lists the candidate tree from the object store again and compares it with the bound digest. It then checks the reviewer's copy if that copy survived. The check reads `verified after restart: ...` or `failed: ...`, and a failed check makes the review `inconclusive`. It reads `unverified` only when the candidate tree is no longer available.

## OS confinement

On Linux, reviewers and fixers run under Landlock. Greenhouse probes the kernel's Landlock ABI and records the policy in `run.review.confinement` or in the repair's `confinement`. It applies the policy in the agent process before exec.

- The whole file system stays readable and executable.
- Pi may create and remove authentication lock directories. Native credential files stay read-only.
- Claude Code and Codex get no write access to their real config directories or credentials. Greenhouse records these roles as `degraded`, because the CLIs may need writable session state. Config, hooks, skills, plugins, MCP settings and credentials stay read-only.
- No reviewer can write the snapshot, the repository or any other path.
- A fixer may also write its private candidate worktree and a private scratch directory, which becomes its `TMPDIR`. The repository's Git metadata stays read-only.
- From ABI 4 on, listening sockets are refused. Outbound TCP is unrestricted, because native agents own provider routing. Greenhouse records this as degraded network confinement; the file system write restrictions still apply.
- `backends.json` can add writable directories per role, for example for a wrapper that keeps its own logs: `"writable": {"reviewer": ["/abs/dir"], "fixer": ["/abs/dir"]}`. Paths must be absolute and cannot be `/`.

The record lists the mechanism, the ABI, the writable directories (`writable`), the authentication lock directories and the network policy.

When Landlock is unavailable, the record's `status` is `degraded`, the network is `unrestricted` and `reason` says why. The agent then runs without OS confinement. A Landlock ABI below 4 confines writes but not the network, and is also `degraded`.

The terminal prints each record as `Confinement: reviewer landlock enforced (ABI 6)` or `Confinement: reviewer DEGRADED: <reason>`, followed by the writable paths and the network rule.

If the policy cannot be applied, the agent does not start. The review becomes `inconclusive`, or the repair `fixer-failed`.

## Backend pause, resume and rerun

Greenhouse classifies a failed backend run from the CLI's exit code and output:

| Reason | Claude | Codex | Pi |
|---|---|---|---|
| `quota` | `api_error: usage_limit_reached` or status 429; the reset comes from the `rate_limit_event` `resetsAt` | "You've hit your usage limit", "Quota exceeded" or a final 429; the reset comes from "try again at ..." | `429:` error message |
| `auth` | "Not logged in" or status 401/403 | 401 Unauthorized | "No API key found" or `401:`/`403:` |
| `unavailable` | status 5xx or a connection failure | 5xx, connection failure, or three "waiting for network" retries | `5xx:` or "Connection error." |

Any other failure stays an ordinary `inconclusive` review or `fixer-failed` repair. The recorded outputs are in [`testdata/backends/`](../testdata/backends/).

### Timeouts and cancellation

A review that reaches the configured [`executor_timeout`](axi.md#executor-timeout) (45 minutes by default) reports `status: "cancelled"`, `reason: "timeout"` and a diagnostic that names the limit. An explicit cancellation reports `reason: "cancelled"`. The reason is kept in `review.reason`, `host_result.reason` and `attempts[].reason`, including for full-pipeline reviews.

### Pauses

A `quota`, `auth` or `unavailable` outcome parks only the affected stage as `paused: backend <reason>`.

- Completed stages, verdicts, findings, candidate commits and counters stay as they are.
- The attempt does not consume a review or repair round: the round taken at admission is returned.
- `run.attempts` records every backend attempt with its backend, model, outcome, reason, cost and tokens.

Before the controller resets a paused repair's private candidate to its last committed candidate, it saves the fixer's edits since the repair base in the repair record. The saved edits include committed and uncommitted changes and the contents of new untracked files.

- `partial` holds the diff, cut at 64 KiB.
- `partial_digest` and `partial_bytes` identify the full diff.
- The attempt in `run.attempts` carries the same `partial`.

### Resume

`resume` (line client `r`) continues at the paused stage.

- A paused review runs again in a fresh session with the same bound input.
- A paused repair runs again from the restored candidate.

Greenhouse never reuses a partial transcript.

### Rerun

`rerun` (line client `r`) works whenever the current stage is paused, failed or cancelled. Cancelling a stage with `x` is an abort, and the run stays rerunnable.

Rerun starts again at that stage with the current settings and keeps every earlier stage and its evidence. It never resets budgets: a rerun review or repair counts as a new round.

A rerun of a stopped one-pass repair first checks:

- the budget
- the trusted fixer
- that its writer stopped
- that its private candidate is still at the base commit

It then saves the candidate's edits the same way as a paused repair and resets the candidate. The stopped repair, with `partial`, `partial_digest` and `partial_bytes`, stays in `run.prior_repairs`.

A refused rerun changes nothing. Rerun is refused while an execution of the stage is live or its outcome is unknown.

### Rerun from start

`rerun-from-start` (line client `b`, confirmed by typing `rerun`) creates a linked New run with the same settings, intent and target. It marks the old run `restarted` and keeps its evidence and candidate branch intact.

Both rerun actions are token-bound and replay their receipts.

### Automatic resume

Settings has an `auto_resume` switch: `on`, `off`, or empty for the default. The default is on in `auto` and `yolo`. `ask` never resumes automatically.

When the switch is on:

1. A configured fallback for the paused role is preferred over waiting. On a quota pause, Greenhouse switches the role to the next entry of `fallback` in `backends.json` and resumes at once. The switch is recorded with the `auto` actor, or the YOLO actor.
2. Otherwise, a quota pause with a known reset resumes 5 seconds after the reset, with the same backend and model.
3. A quota or unavailable pause without a reset retries with exponential backoff from `base_seconds` up to `max_seconds`, at most `max_attempts` times. The defaults are 60, 3600 and 3. After that the stage stays paused.
4. An auth pause always waits for the User.

The daemon stores the schedule in the run (`run.pause.auto_resume_at`) and reschedules it after a restart. It records the resume as an `auto` event.

Any manual action on the run, including Apply, cancels a pending automatic resume. `cancel-auto-resume` (line client `a`) cancels it alone.

The paused screen and the AXI `explanation` field show the same sentence, for example: "Codex quota exhausted; it resets at 14:30. Auto mode will resume at 14:30 with the same agent. Press r to rerun now, a to cancel auto-resume, s to change agent." AXI `actions` lists `resume`, `rerun`, `rerun-from-start` and `cancel-auto-resume` when they apply.

### Switching agents

1. While a run is paused, change the reviewer or fixer backend and model in Settings.
2. Apply to this run, Save as the default for later runs, or both.
3. Press `r`.

Each change is recorded as a `backend-switch` event with the old and new backend and model, the actor and the reason. Reviews and repairs from the old backend keep their recorded backend and model.

A new reviewer changes the review stage's inputs, so the review and every later gate rerun. A new fixer changes no gate input.

In a one-pass run, resolve rules again (`l`) after Apply, because rule snapshots bind the applied settings.
