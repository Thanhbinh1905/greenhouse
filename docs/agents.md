# Agents

Greenhouse runs reviewers and fixers through the native Pi, Claude Code or Codex CLI. The native CLI owns authentication, model discovery and provider routing.

Install and sign in to the agent with that CLI's own instructions. Greenhouse uses the agent's login, provider routing and default model. When authentication is missing, Greenhouse reports the native login action. It never runs a login or changes credential files.

## Discovery and Prepare

Greenhouse discovers `pi`, `claude` and `codex` on PATH and ignores relative PATH entries. The interactive launch then keeps only signed-in agents: Claude must report `loggedIn` from `claude auth status`, Codex must pass `codex login status`, and Pi must list at least one model with `pi --list-models`. Each probe has a short timeout and runs offline-safe. Prepare binds the selected executable. Model and agent choices stay ordinary Greenhouse preferences.

An auto-started daemon inherits the environment of the client that starts it. Put the agent executables on that client's PATH. Later clients cannot change the running daemon's PATH; stop the daemon after its work settles and start it from a client with the required PATH.

Prepare resolves a Git branch or commit to an immutable full commit ID. It records the candidate, the intent and the committed diff. A settings change does not rewrite old evidence.

Review needs current rules and a matching prepared agent configuration. An explicit model must match the model the agent reports. An empty selection records the reported identity of the native default.

## Agent backends

Each review and repair starts a fresh session without the author's conversation.

| Backend | Session setup |
| --- | --- |
| Pi | JSON mode. Extensions, MCP, skills, context files, prompt templates, themes and project approval are disabled. The native `PI_CODING_AGENT_DIR` stays available for login and routing; inherited session variables are removed. |
| Claude | Safe mode, explicit read tools for review and strict MCP configuration. Native user settings keep authentication and routing. An unexpected tool refuses the result. |
| Codex | Native user configuration stays. Hooks, plugins, shell tools, web search and author context are disabled. Greenhouse asks `codex mcp list --json` for server names and disables each server explicitly; an unreadable server list refuses preparation. Review uses the read-only sandbox; repair uses workspace-write. |

Fixers run without session persistence. A review keeps its session in a private per-review directory, which is deleted afterwards, so Greenhouse can resume the session once for a finalization turn. Claude and Codex reviews get a private config directory that holds copies of the native credentials and settings.

The controller verifies the snapshot after review and checks the repair scope before committing. Child processes have bounded lifetimes. Cancellation keeps the existing writer custody rules. Raw output is kept only in the redacted [transcript log](state.md#transcript-logs).

## Review snapshot

Each review admission records `run.review.snapshot`:

- the candidate commit and its Git tree ID
- the number of entries and bytes
- `digest`: the SHA-256 of `git ls-tree -r -z -l --full-tree` for that commit

The digest is part of the review input, so the input digest binds it. The snapshot holds at most 100,000 entries and 512 MiB.

Before launch, Greenhouse writes every blob of that listing from the object store into a private directory, without Git attributes or filters. It makes the files and directories read-only, and the reviewer runs there.

After the reviewer exits, Greenhouse compares every file, mode and symlink with the listing. A missing, extra or changed path makes the review `inconclusive`, and `snapshot.check` records `modified: <detail>`, for example `modified: unexpected path <path>`. Otherwise `snapshot.check` is `unchanged`.

When a restart adopts an interrupted review, Greenhouse lists the candidate tree from the object store again and compares it with the bound digest. It then checks the reviewer's copy if that copy survived. The check reads `verified after restart: ...` or `failed: ...`, and a failed check makes the review `inconclusive`. It reads `unverified` when the candidate tree is no longer available or the review has no bound snapshot.

## OS confinement

On Linux, reviewers and fixers run under Landlock. Greenhouse probes the kernel's Landlock ABI and records the policy in `run.review.confinement` or in the repair's `confinement`. It applies the policy in the agent process before exec.

Landlock confines file system writes. It does not restrict outbound TCP, because native agents own provider routing. Every confinement record therefore has `status` `degraded`, and `reason` names what stays open.

- The whole file system stays readable and executable.
- A reviewer can write only its private session and temporary directories (its `TMPDIR`), `/dev/null`, and any configured `writable` directories. It cannot write the snapshot or the repository.
- A fixer may also write its private candidate worktree and a private scratch directory, which becomes its `TMPDIR`. The repository's Git metadata stays read-only.
- Pi may create and remove directories inside its native agent directory (`PI_CODING_AGENT_DIR`, or `~/.pi/agent`), which it uses for authentication locks. Native credential files stay read-only.
- Claude Code and Codex get no write access to their real config directories or credentials. Config, hooks, skills, plugins, MCP settings and credentials stay read-only. Their records also note that the CLIs may need writable session state.
- From ABI 4 on, listening sockets are refused. A Landlock ABI below 4 confines writes but not the network.
- `backends.json` can add writable directories per role, for example for a wrapper that keeps its own logs: `"writable": {"reviewer": ["/abs/dir"], "fixer": ["/abs/dir"]}`. Paths must be absolute and clean, cannot be `/`, and are limited to 16 per role. A path that does not exist or is not a directory is dropped. `backends.json` must be private (mode 0600) and at most 64 KiB; otherwise Greenhouse ignores it.

The record lists the mechanism, the ABI, the writable directories (`writable`), the authentication lock directories and the network policy.

When Landlock is unavailable, the network is `unrestricted`, `reason` says why, and the agent runs without OS confinement.

The terminal prints each record as `Confinement: reviewer DEGRADED: <reason>`, followed by the writable paths and the network rule.

If the policy cannot be applied, the agent does not start. The review becomes `inconclusive`, or the repair `fixer-failed`.

## Backend pause, resume and rerun

Greenhouse classifies a failed backend run from the CLI's exit code and output. Each backend matches differently:

- Claude: the `api_error`, `api_error_status` and `result` fields of an error `result` event. Text matches are case-insensitive substrings of `result`.
- Codex: the message of a `turn.failed` event, matched as case-insensitive substrings. When that gives no reason, a last `error` or `turn.failed` message containing "waiting for network" (case-sensitive) means `unavailable`.
- Pi: "No API key found" is a case-sensitive substring of stderr with a non-zero exit. Otherwise Greenhouse reads the error message of the last assistant `message_end` event with `stopReason` `error`, and takes a status code only from a leading `NNN:`.

| Reason | Claude | Codex | Pi |
|---|---|---|---|
| `quota` | `api_error: usage_limit_reached`, status 429, "hit your" or "rate limit"; the reset comes from the `rate_limit_event` `resetsAt` | "usage limit", "quota", "429" or "rate limit"; the reset comes from "try again at ..." | a message starting with `429:` |
| `auth` | status 401 or 403, "not logged in", "/login" or "authentication" | "401", "403", "unauthorized", "not logged in" or "codex login" | "No API key found" on stderr, or a message starting with `401:` or `403:` |
| `unavailable` | status 5xx, `api_error: connection_failed`, "connection" or "overloaded" | "status 5xx", "connection failed", "service unavailable" or "waiting for network" | a message starting with `5xx:`, or containing "connection error" (case-insensitive) |

Greenhouse stops a Codex run after three "waiting for network" lines. The review or repair then ends cancelled or failed, not paused.

Any other failure stays an ordinary `inconclusive` review or `fixer-failed` repair. Its `reason` may still name a cause, such as `model_mismatch`, `crash` or `missing_binary`. A pi run that exits non-zero after a `compaction_start` event with reason `overflow` reports `context_overflow`; see [model context window](embedding.md#model-context-window). The recorded outputs are in [`testdata/backends/`](../testdata/backends/).

### Timeouts and cancellation

Two limits apply to a review:

- `review_budget` in `backends.json` limits each reviewer attempt; the default is 3 minutes. A review that reaches it ends `inconclusive` with `reason: "review_budget"`. See the [embedding contract](embedding.md) for details.
- [`executor_timeout`](axi.md#executor-timeout) (45 minutes by default) limits the executor. A review that reaches it reports `status: "cancelled"`, `reason: "timeout"` and a diagnostic that names the limit.

An explicit cancellation reports `reason: "cancelled"`. The reason is kept in `review.reason`, `host_result.reason` and `attempts[].reason`, including for full-pipeline reviews.

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
- that its private candidate is still on its branch and descends from the base commit

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
3. A quota or unavailable pause without a reset retries with exponential backoff from `base_seconds` up to `max_seconds`. The defaults are 60 and 3600. After `max_attempts` automatic resumes in the run (default 3), the stage stays paused. Every automatic resume counts, including fallback and reset-timed ones, until a manual rerun or pipeline progress resets the count. Set these values under `auto_resume` in `backends.json`.
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
