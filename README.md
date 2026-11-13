<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/greenhouse-mark-dark.svg">
  <img src="docs/assets/logo/greenhouse-mark.svg" alt="Greenhouse logo" width="120">
</picture>

# Greenhouse

Greenhouse is a standalone terminal and JSON client for a per-user local run store. It implements lifecycle, settings with repository trust and versioned configuration, immutable rule resolution, independent review and selected-finding repair through Pi, Claude Code or Codex under OS confinement, resolution responses, configured checks, guarded handback, a full pipeline with ask, auto and YOLO authority, review-cap decisions, GitHub delivery to CI readiness, explicit recovery after interruption, live event subscriptions, and a host mode for embedding. Development evaluations measure review and repair quality with real models. Review success is not test, lint, or delivery success.

## First review

Install Greenhouse on Linux with Git, then install and sign in to Pi, Claude Code, or Codex using that CLI's instructions. Greenhouse uses the installed agent's login, provider routing, and default model.

From a feature branch with committed changes, run:

```sh
greenhouse
```

The screen shows the branch, the agent it will use, the suggested Git target and one highlighted action, **Run review**. Press Enter. Run collects the intent, saves it with the target, loads the selected rules and starts one independent review. Each step is a separate controller action. A refused or failed step stops Run and says why; Run never retries it. Opening, resizing, navigating or detaching only attaches and follows events; nothing runs and no budget is used.

Run reads the intent from your own agent history: Claude Code, Codex and Pi sessions that were started inside this worktree and mention the changed files. It keeps your requests from that session word for word, redacts secrets, drops tool output and labels the result with the agent, session and time. It is never a summary and never comes from the diff. When no session matches, Run asks what the change should do. When several sessions match equally, it asks which one. When the requests exceed the 8192-byte intent limit, Run asks for a shorter intent; it never cuts them. To skip collection, Tab to the Intent row and type the intent before Run; Tab to Agent or Target to change them; changing the agent changes the reviewer only and keeps the fixer settings. Dirty files are excluded from review.

No `reviewer.json`, `providers.json`, `backends.json`, copied keys, or executable paths are needed for the first review. Missing authentication reports the native login action. Greenhouse never runs login or changes credential files.

Selected repair still requires explicitly trusted checks in `fixer.json`. Full delivery still requires a trusted `pipeline.json` command plan and forge authorization. These approve commands, not providers.

## Build and open

```sh
go build -o greenhouse ./cmd/greenhouse
cd /path/to/a/git/worktree
/path/to/greenhouse
```

The executable requires Linux, Git, and a committed worktree. Runtime operation requires neither Go, Posse, nor Herdr. Building requires Go 1.24.2 or later. The interactive screen uses [Bubble Tea, Bubbles and Lip Gloss](THIRD_PARTY_NOTICES.md).

On a terminal, the bare command opens an interactive screen. Keys act at once, without Enter:

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
| `x` | Cancel running work. Press `x` again to confirm. |
| `r` | Recheck installed agents and reconnect. |
| `?` | Help: keys, and every action with whether it is available now. |
| `q`, Ctrl-C | Detach. Work keeps running in the daemon; run `greenhouse` again to reattach. |

Inside a text field, typed letters go to the field: `q` and `x` neither detach nor cancel. New run, Rerun from start, Trust and Migrate need their word typed (`new`, `rerun`, `trust`, `migrate`); a recovery needs a reason and its option name typed; a decision needs a reason. An action the controller does not allow is not offered; help lists it as not available. The default screens hide run IDs, tokens, full commits and digests; details and AXI show them.

The screen follows the run through [events](#events): it redraws when the daemon commits a change, without a keypress. If the daemon stops, the header shows `RECONNECTING`, the last state stays on screen and actions are disabled until fresh state arrives. The screen never starts work or restarts a daemon to follow events; `r` starts the daemon when you ask. Progress shows the stage, agent and elapsed time; `≥` marks a time measured since you attached rather than since the stage started. A run owned by a [host](docs/embedding.md) can be followed but not changed. The screen adapts to the terminal size and needs at least 40x12. `NO_COLOR` removes color; every status also has a symbol and words.

When stdin or stdout is not a terminal, or `TERM` is unset or `dumb`, the bare command keeps the line-oriented client. Press Enter after each command. `i` refreshes inspection, `c` toggles criteria details, `l` explicitly resolves (loads) rules, `s` opens Settings, and `n` requests New run. `p` prepares intent and an exact target commit, `v` dispatches one review, `g` starts or continues the full pipeline, `m` authorizes the fixer for a target conflict, `d` answers the open decision, `x` cancels a review, fixer, check, or pipeline step, `r` reruns a paused, failed or cancelled stage, `a` cancels a scheduled automatic resume, `b` starts a linked rerun from start, `t` trusts the repository override, `u` migrates older configuration files, `o` shows the transcript log, and `q` detaches. Ctrl-C also disconnects without cancelling the daemon. Its screen shows `Events: live` or `Events: reconnecting (daemon unavailable)`. Single-letter keys elsewhere in this README name line-client commands; on the interactive screen the same actions are in the actions menu (`a`).

Settings edits are drafts until you select Apply or Save. Settings lists each current value with its source: `built-in default`, `per-user`, `trusted repository override` or `run-local`, and marks drafted values with `*`. Apply adds an immutable run-local snapshot. Save changes user defaults or the trusted repository override without changing the current run. A draft survives leaving Settings until you Apply, Save or reset it. In the line client, Save uses `d` for user defaults and `r` for the repository override; use Settings twice to do both. Rule selection does not load bytes. Explicit resolution snapshots content; dispatch retains the exact emitted criteria and committed diff. The terminal wraps prose at word boundaries and abbreviates tokens wider than the screen. AXI retains their full values. The terminal supports mode, preset, combined review/repair limit, separate reviewer and fixer backend/model selections, rule selection, and the auto-resume switch. Each role uses `pi`, `claude` or `codex`; see [agent backends](#agent-backends). An empty model uses the native default. Explicit Pi models use provider-qualified IDs. Executables come from absolute PATH directories; repository overrides cannot name executables. See [repository overrides](#repository-overrides-and-trust).

## JSON interface

Send one request on stdin to `greenhouse axi`. The invoking directory identifies the worktree; the CLI replaces any request `path` with that directory. Machine output is one JSON envelope on stdout.

```sh
printf '%s\n' '{"version":2,"op":"attach"}' | /path/to/greenhouse axi
```

The daemon accepts protocol versions 1 and 2, answers in the requested version, and reports its range in `protocol`. Version 2 adds `watch` and host mode. See the [compatibility policy](docs/compatibility.md).

For host integration, see the [embedding contract](docs/embedding.md) for complete requests, prepare fields, and the grant-based review flow.

Git identity commands discard inherited `GIT_*` variables so the daemon cannot reuse the first client's repository selection for later clients. The invoking path determines identity, including linked worktrees.

`attach` creates a run only when its canonical context has no active slot. `inspect` never creates a run. Reopening retained work does not consume budget, change snapshots, or execute a stage.

Copy `run_id`, `input_id`, `gate_id`, and `token` from the response into a mutation. Use `token` as `expected`:

```json
{
  "version": 1,
  "op": "apply",
  "run_id": "from response",
  "input_id": "from response",
  "gate_id": "none",
  "expected": "from response token",
  "action_id": "a unique stable client action ID",
  "reason": "increase the run limit",
  "settings": {
    "mode": "ask",
    "preset": "one-pass",
    "limit": 6,
    "rules": ["unslop", "no-comments"],
    "reviewer_backend": "pi",
    "reviewer_model": "openai/gpt-5.6-luna",
    "fixer_backend": "pi",
    "fixer_model": "openai/gpt-5.6-luna",
    "auto_resume": "on"
  }
}
```

Supported reads include `attach`, `inspect`, `watch`, `models`, and `transcript`. `models` queries the trusted Pi executable's catalog. Supported mutations are `apply`, `save`, `trust`, `migrate`, `new`, `resolve-rules`, `prepare`, `review`, `repair`, `pipeline`, `resolve-conflict`, `decide`, `handback`, `cancel`, `resume`, `rerun`, `rerun-from-start`, and `cancel-auto-resume`, and `recover`. Mutations use the same run, input, gate, token, and action fields. `resolve-rules`, `review`, and `cancel` require no settings or reason. `apply` and `save` require a complete settings object and a reason; `save` takes `"target":"user"` (the default) or `"target":"repository"`. `new` retains previous evidence and uses saved defaults. Source branch/head movement blocks Apply and Save on the old run. A current token-bound New request can capture the moved source when no writer or unknown-outcome hazard exists. Old evidence remains unchanged. Unknown executor or writer outcomes block replacement until an explicit [recovery](#recovery-after-interruption) answers them.

Replay an identical mutation with the same action ID to obtain its recorded result, even after reconnect or restart. Reusing an ID with different input refuses. New action IDs require current tokens. Inspect the active run after replay: a recorded result may describe an older snapshot. An outcome of `ok` means the lifecycle request succeeded, never that a validation stage passed. Errors and unsupported operations have `refused` or `error` outcomes. The one-pass `repair` action requires `selected` finding IDs and a private per-user `fixer.json` containing trusted checks. It admits one repair after a completed finding review, requires at least one configured trusted check, and binds the action to the exact review, intent, candidate, rules, selected findings, fixer, and check argv. Admission consumes repair budget durably. Replay returns its receipt; a second repair is refused. The controller creates a managed private worktree and branch, gives Pi only read/edit/write tools with extensions, MCP, skills, project context, and sessions disabled, then runs configured checks and creates a local commit. The fixer runs under [OS confinement](#review-snapshot-and-os-confinement) and must answer each selected finding with a [resolution response](#resolution-responses). Checks run directly, not through an agent.

Create `fixer.json` in `GREENHOUSE_HOME` with mode 0600:

```json
{"version":2,"name":"Greenhouse","email":"greenhouse@example.invalid","checks":[["/absolute/path/to/check","--verify"]]}
```

Each check is an argv array with an absolute executable path. No shell is inserted. Missing checks are unconfigured. Changed fixer/check configuration refuses execution. The controller records check exit codes and bounded output summaries, verifies candidate scope, and commits only authorized paths. Binary changes, submodules, executable Git hooks, configured Git filters, fsmonitor, attributes, or signing requirements are unsupported and refuse rather than being bypassed. Failed or ambiguous candidates remain available in their managed worktree for recovery.

The `handback` action is separate. It fast-forwards the original source branch only when source identity is unchanged, the fixer is known stopped, candidate lineage is exact, and the source worktree and index are clean. Refusal preserves the candidate commit and source work. A repaired head is `fixed-not-independently-rereviewed`; Greenhouse never automatically rereviews or publishes it. New run is blocked while candidate custody is unpublished or ambiguous.

The terminal `f` action requests selected finding IDs for repair; `h` requests handback. AXI requests place the selected ID list in `selected`. All states, selected findings, check results, commit identities, handback status, and budget usage are available in the same run snapshot. There are no publication commands. An unknown repair outcome is answered with [recovery](#recovery-after-interruption).

Defaults are `auto`, `one-pass`, a combined limit of 4, and selected `unslop` and `no-comments` names. The numeric limit is provisional. Valid limits are 1 through 1000; reductions cannot erase consumed usage. Modes apply to the full preset; see [modes and decisions](#modes-and-decisions). A one-pass repair always needs an explicit selection.

### Events

`watch` (version 2) waits for committed run changes in the invoking repository:

```json
{"version":2,"op":"watch","stream":"from the last answer","after":17,"wait_ms":25000}
```

Each answer carries the current snapshot of the worktree's run, as `inspect` returns it, plus `stream`, `sequence` and `wait`:

- `events`: `events` lists each change after `after` with its `sequence`, `run_id`, `stage`, `status` and `token`.
- `elapsed`: nothing changed within `wait_ms` (0 to 60000; 0 answers at once). The outcome is `ok`. An elapsed wait cancels nothing and is not a failure; a review keeps running.
- `resync`: the client's position is unusable because it sent no or another `stream` (for example after a daemon restart), a sequence ahead of the daemon, or one older than the retained 1,024 events. Treat the snapshot as authoritative and continue from its `stream` and `sequence`.

Every other version-2 response also carries `stream` and `sequence`: the last event the daemon committed before it handled the request. A replay returns the values recorded with its receipt. An event with a higher sequence is newer than that response, so a client can drop a watch answer that arrives late. Pass `stream` and `sequence` from each answer into the next request. A client that loses its connection reconnects with the same values and receives either the missed events or a resync. Watching never creates a run or authorizes a gate. At most 64 watches can wait at once.

### Host mode

A host such as Posse can embed Greenhouse as its reviewer. Requests with `host` (version 2) create or act on a host-owned run that only reviews: the host owns author repair, candidate custody, publication, CI and cap decisions, each `review` needs a host `grant`, and `host_result` reports the exact-head verdict and findings. Other clients can only observe a host run. See the [embedding contract](docs/embedding.md).

## Independent review configuration

Greenhouse discovers `pi`, `claude`, and `codex` on PATH. The selected executable is bound during Prepare. Relative PATH entries are ignored. Model and agent choices remain ordinary Greenhouse preferences. Native CLIs own authentication, model discovery, and provider routing.

Prepare resolves a Git branch or commit to an immutable full commit ID and records the candidate, intent, and committed diff. Settings changes do not rewrite old evidence. Review requires current rules and a matching prepared agent configuration. An explicit model must match the model reported by the agent; an empty selection records the native default's reported identity.

## Agent backends

Each review and repair starts a fresh session without an author conversation.

- Pi uses ephemeral JSON mode with extensions, MCP, skills, context files, prompt templates, themes, and project approval disabled. Native `PI_CODING_AGENT_DIR` remains available for login and routing; inherited session variables are removed.
- Claude uses safe mode, explicit read tools for review, strict MCP configuration, and no session persistence. Native user settings retain authentication and routing. Unexpected tools refuse the result.
- Codex retains native user configuration, disables hooks, plugins, shell tools, web search, and author context, and uses an ephemeral session. Greenhouse asks `codex mcp list --json` for server names and disables each server explicitly. An unreadable server list refuses preparation. Review uses the read-only sandbox; repair uses workspace-write.

The controller verifies snapshots after review and checks repair scope before committing. Child processes have bounded lifetimes and cancellation retains existing writer custody rules. Raw output is kept only in the redacted transcript log.

## Review snapshot and OS confinement

Each review admission records `run.review.snapshot`: the candidate commit, its Git tree ID, the number of entries and bytes, and `digest`, the SHA-256 of `git ls-tree -r -l --full-tree` for that commit. The digest is part of the review input, so the input digest binds it. Before launch, Greenhouse writes every blob of that listing from the object store into a private directory, without Git attributes or filters, and makes files and directories read-only. The reviewer runs there. After the reviewer exits, Greenhouse compares every file, mode and symlink with the listing. A missing, extra or changed path makes the review `inconclusive`, and `snapshot.check` records `modified: <path>`; otherwise it is `unchanged`. When restart adopts an interrupted review, Greenhouse lists the candidate tree from the object store again and compares it with the bound digest, then checks the reviewer's copy if it survived. The check reads `verified after restart: ...` or `failed: ...`, and a failed check makes the review `inconclusive`. It reads `unverified` only when the candidate tree is no longer available. The snapshot holds at most 100,000 entries and 512 MiB.

On Linux, reviewers and fixers run under Landlock. Greenhouse probes the kernel's Landlock ABI, records the policy in `run.review.confinement` or the repair's `confinement`, and applies it in the agent process before exec:

- The whole file system stays readable and executable. Pi may create and remove authentication lock directories; native credential files remain read-only. Claude Code and Codex receive no write access to their real config directories or credentials. Greenhouse records these roles as `degraded` because the CLIs may require writable session state; config, hooks, skills, plugins, MCP settings and credentials remain read-only. No reviewer can write the snapshot, the repository or any other path.
- A fixer may also write its private candidate worktree and a private scratch directory, which becomes its `TMPDIR`. The repository's Git metadata stays read-only.
- From ABI 4 on, listening sockets are refused. Outbound TCP is unrestricted because native agents own provider routing. This is recorded as degraded network confinement; filesystem write restrictions still apply.
- `backends.json` can add writable directories per role, for example for a wrapper that keeps its own logs: `"writable": {"reviewer": ["/abs/dir"], "fixer": ["/abs/dir"]}`. Paths must be absolute and cannot be `/`.

The record lists the mechanism, ABI, writable directories (`writable`), authentication lock directories, and network policy. When Landlock is unavailable, the record's `status` is `degraded`, the network is `unrestricted`, and `reason` says why; the agent then runs without OS confinement. A Landlock ABI below 4 confines writes but not the network and is also `degraded`. The terminal prints each record as `Confinement: reviewer landlock enforced (ABI 6)` or `Confinement: reviewer DEGRADED: <reason>`, followed by the writable paths and the network rule. If the policy cannot be applied, the agent does not start and the review is `inconclusive` or the repair `fixer-failed`.

## Resolution responses

A finding repair must end with one response per selected finding, as the fixer's final answer:

```json
{"responses":[{"finding":"<id>","response":"fixed","reason":"rewrote the bounds check"}]}
```

- `fixed`: the fixer edited the candidate to resolve the finding.
- `disputed`: the finding is not a real defect; `reason` says why. The fixer leaves its lines unchanged.
- `deferred`: postponed under an answered decision. `authority` must name one of the `Authorities` in the fixer input: decisions answered by the terminal, AXI or a YOLO actor, never by `auto`.

A missing, duplicate or unknown response, a deferral without a listed authority, or edits without any `fixed` response refuse the repair as `fixer-failed`; nothing is committed. When every response is a dispute or deferral and nothing changed, the repair ends `responded-no-change` without a commit.

In the full pipeline, the next independent review receives the responses, each with the prior finding and any cited decision, and must return `resolutions` with one `confirmed` or `rejected` check per response. The responses change the review stage's inputs, so a `responded-no-change` repair also triggers that review. A finding counts as resolved only when the review confirms the response and does not report the finding again:

- A confirmed `fixed` response resolves it as `fixed`.
- A confirmed dispute resolves it as `dispute-upheld`. A dispute never counts as a fix: it creates no commit and no `REPAIRED PASS`.
- A confirmed deferral resolves it as `deferred`, and a ready run reports `ACCEPTED RISK`.
- A rejected check leaves the finding outstanding.

The repair's `responses`, the review's `result.resolutions` and `run.pipeline.resolutions` (with `kind`) carry the same data in AXI. The terminal lists each finding as `Outstanding` or `Resolved (fixed)`, `Resolved (dispute upheld)` or `Resolved (deferred)`, followed by `Response <kind>: <reason>` and `Review check <check>: <reason>`. A one-pass repair shows its responses as `Not independently checked`, because one-pass never rereviews.

## Backend pause, resume and rerun

Greenhouse classifies a failed backend run from the CLI's exit code and output:

| Reason | Claude | Codex | Pi |
|---|---|---|---|
| `quota` | `api_error: usage_limit_reached` or status 429; the reset comes from the `rate_limit_event` `resetsAt` | "You've hit your usage limit", "Quota exceeded" or a final 429; the reset comes from "try again at ..." | `429:` error message |
| `auth` | "Not logged in" or status 401/403 | 401 Unauthorized | "No API key found" or `401:`/`403:` |
| `unavailable` | status 5xx or a connection failure | 5xx, connection failure, or three "waiting for network" retries | `5xx:` or "Connection error." |

Any other failure stays an ordinary `inconclusive` review or `fixer-failed` repair. The recorded outputs are in `testdata/backends/`.

A `quota`, `auth` or `unavailable` outcome parks only the affected stage as `paused: backend <reason>`. Completed stages, verdicts, findings, candidate commits and counters stay as they are. The attempt does not consume a review or repair round: the round taken at admission is returned. `run.attempts` records every backend attempt with its backend, model, outcome, reason, cost and tokens. Before the controller resets a paused repair's private candidate to its last committed candidate, it saves the fixer's committed and uncommitted edits since the repair base, including the contents of new untracked files, in the repair record: `partial` holds the diff, cut at 64 KiB, and `partial_digest` and `partial_bytes` identify the full diff. The attempt in `run.attempts` carries the same `partial`.

`resume` (terminal `r`) continues at the paused stage. A paused review runs again in a fresh session with the same bound input; a paused repair runs again from the restored candidate. Greenhouse never reuses a partial transcript.

`rerun` (terminal `r`) works whenever the current stage is paused, failed, or cancelled. Cancelling a stage with `x` is an abort; the run stays rerunnable. Rerun starts again at that stage with the current settings and keeps every earlier stage and its evidence. It never resets budgets: a rerun review or repair counts as a new round. A rerun of a stopped one-pass repair first checks the budget, the trusted fixer, that its writer stopped, and that its private candidate is still at the base commit. It then saves the candidate's edits the same way as a paused repair and resets the candidate. The stopped repair, with `partial`, `partial_digest` and `partial_bytes`, stays in `run.prior_repairs`. A refused rerun changes nothing. It refuses while an execution of the stage is live or its outcome is unknown. `rerun-from-start` (terminal `b`, confirmed by typing `rerun`) instead creates a linked New run with the same settings, intent and target, and marks the old run `restarted` with its evidence and candidate branch intact. Both actions are token-bound and replay their receipts.

### Automatic resume

Settings has an `auto_resume` switch (`on`, `off`, or empty for the default). The default is on in `auto` and `yolo`; `ask` never resumes automatically. When it is on:

- A configured fallback for the paused role is preferred over waiting. On a quota pause, Greenhouse switches the role to the next entry of `fallback` in `backends.json` and resumes at once. The switch is recorded with the `auto` actor (or the YOLO actor).
- Otherwise, a quota pause with a known reset resumes 5 seconds after the reset, with the same backend and model.
- A quota or unavailable pause without a reset retries with exponential backoff from `base_seconds` up to `max_seconds`, at most `max_attempts` times (defaults 60, 3600 and 3), then stays paused.
- An auth pause always waits for the User.

The daemon stores the schedule in the run (`run.pause.auto_resume_at`), reschedules it after a restart, and records the resume as an `auto` event. Any manual action on the run, including Apply, cancels a pending automatic resume; `cancel-auto-resume` (terminal `a`) cancels it alone.

The paused screen and the AXI `explanation` field show the same sentence, for example: "Codex quota exhausted; it resets at 14:30. Auto mode will resume at 14:30 with the same agent. Press r to rerun now, a to cancel auto-resume, s to change agent." AXI `actions` lists `resume`, `rerun`, `rerun-from-start` and `cancel-auto-resume` when they apply.

### Switching agents

While a run is paused, change the reviewer or fixer backend and model in Settings and Apply to this run, Save as default for later runs, or both; then press `r`. Each change is recorded as a `backend-switch` event with the old and new backend and model, the actor and the reason. Reviews and repairs from the old backend keep their recorded backend and model. A new reviewer changes the review stage's inputs, so review and every later gate rerun; a new fixer changes no gate input. In a one-pass run, resolve rules again (`l`) after Apply, because rule snapshots bind the applied settings.

## Full pipeline

Apply the `full` preset, then Prepare. Prepare also reads a private `pipeline.json` from `GREENHOUSE_HOME` with mode 0600 and records its commands and executable digests. Repository files never add commands.

```json
{"version":2,"budget_unit":"rounds","name":"Greenhouse","email":"greenhouse@example.invalid",
 "remote":"git@github.com:acme/garden.git","github_owner":"acme","github_repo":"garden","pull_base":"main",
 "required_checks":["CI / test"],
 "test":[["/absolute/path/to/test-runner","./..."]],
 "lint":[["/absolute/path/to/linter"]],
 "docs":[["/absolute/path/to/doc-build"]],
 "docs_paths":["README.md"],
 "not_applicable":{},
 "repairs":["test","lint","docs"],
 "repair_limit":2,
 "yolo":{"actor":"night-shift","cap":"final-fix"},
 "grants":[{"authority":"add-files","stages":["test"],"paths":["testdata/"],"reason":"test repairs may add fixtures"}]}
```

`pipeline` (terminal `g`) runs the stages in order on a managed private candidate worktree:

- intent: loads the prepared intent, exact target and command plan. No process runs.
- sync: runs `git merge-base --is-ancestor` and, when needed, `git merge --no-ff` of the target through the command runner. It never rebases. A conflict aborts the merge, opens a gate and parks the run. The author can resolve it in the source and start a New run, or `resolve-conflict` (terminal `m`) with the current `gate_id` authorizes the fixer to edit only the conflicted paths and commit the merge.
- review: an independent reviewer on the exact candidate. The full contract also returns a documentation assessment. Actionable findings go to a repair or a decision, as the mode decides. A P3 finding or one without an action is informational: it stays visible and blocks nothing.
- test, docs, lint: configured commands run directly in the candidate, never through an agent. Each result records argv, executable digest, exit code and the last 16 KiB of output. A command that edits the candidate fails the stage. The docs stage also requires the current review to report documentation as `documented` or `not-needed`; a passing doc build alone does not pass it.
- push: the controller probes the configured remote ref, then pushes the exact candidate without force. After an interrupted push, the controller checks the remote ref before it retries. An unexpected remote head blocks delivery.
- PR: the GitHub forge adapter finds an open PR for the configured branch or creates one. It updates an existing PR instead of creating a duplicate. The PR number and URL persist for CI reconciliation. `GREENHOUSE_GITHUB_API` can point to a local fixture; production defaults to GitHub's API. Authentication uses `GH_TOKEN` when set.
- CI: the trusted plan names `required_checks`. The adapter requires each named check exactly once, completed successfully for the exact candidate SHA, and verifies the persisted PR number still points to that SHA during polling. Unlisted green checks do not establish readiness. Missing, duplicate, pending, failed, stale, and unknown checks block CI readiness. Transport failures retry at most three times with persisted exponential backoff; exhaustion parks the run without dispatching a fixer. Greenhouse never merges.

A stage with no configured command is `unconfigured` and blocks. A stage listed in `not_applicable` needs a reason and is shown as not applicable. A local-only plan must explicitly mark push, PR and CI not applicable; otherwise missing delivery configuration blocks the pipeline. A not-applicable docs stage still fails when the review reports documentation as `missing`. An executable whose digest changed since Prepare stops its stage.

When the mode allows it, a failing test, lint, docs or CI stage listed in `repairs` starts the fixer from `fixer.json` in the private candidate. A bounded CI repair creates a new candidate, then reruns review and the affected local and delivery gates. The fixer may edit only files changed since the target, plus `docs_paths` for docs. The controller commits the change as a new candidate. Every stage's evidence binds declared inputs: the candidate, plan digest, and for review the preset, reviewer configuration and selected rules with their content digests. A new candidate reruns review and every gate; a rule change reruns review only, while a mode or limit change reruns nothing. Gate repairs use `repair_limit`, separate from the review-round limit; exhausting either parks the run. A repaired stage stays `repaired-unreviewed` until the new candidate passes review and the stage reruns.

The run ends `ci-ready` only after the exact candidate has passed all configured local gates and trusted GitHub checks. A local-only plan ends at `locally-ready` only when it explicitly marks push, PR and CI not applicable. The TUI and AXI expose the PR URL and head. The separate `handback` action fast-forwards the unchanged, clean source branch to that candidate. New run is blocked while new candidate commits remain unhanded. `pipeline` again continues a parked or cancelled run without rerunning current evidence. Restart adopts the result of an interrupted review or command stage when its executor record proves it, and parks the run until `pipeline` continues it; any other interrupted step stays `unknown-outcome` and is never retried. A command or Git step whose runner outcome is unknown does the same: the run requires [recovery](#recovery-after-interruption), New run is refused, and no other worktree of the repository can start a pipeline, repair or conflict fix; their inspection shows the hazard and omits those actions. The same rule covers a conflict fix whose merge or abort outcome is unknown, an ambiguous candidate worktree creation, and a handback fast-forward that leaves the source neither unchanged nor at the candidate.

AXI returns everything in `run.pipeline`: status, failed stage and reason, gate, conflict paths, gate-repair usage and limit, each stage's status, inputs, candidate, runs, repairs and command results, repair records, and superseded reviews. The terminal shows the same stages with distinct markers: `[pass]`, `[pass after repair, rereviewed]`, `[REPAIRED, NOT REVIEWED]`, `[FAIL]`, `[n/a]`, `[UNCONFIGURED]`, `[DECISION]`, `[ACCEPTED RISK]`, and `[unsupported]`. A run with all delivery stages explicitly not applicable ends at `locally-ready`; a run that passes trusted delivery checks ends at `ci-ready`. A ready run also shows its outcome: `CLEAN PASS`, `REPAIRED PASS (rereviewed after repair)`, or `ACCEPTED RISK (review risk accepted, not a clean pass)`. AXI returns the same value in `run.pipeline.outcome`.

### Modes and decisions

The mode decides who answers a question the pipeline cannot settle alone:

- `ask`: every repair waits for an answer. The actionable findings of one review form one decision of kind `findings`; one answer authorizes the whole group or a selected subset. A failing gate that the plan may repair opens a `gate-repair` decision.
- `auto` (the default): the controller repairs actionable findings and authorized gates without asking. It parks at the review cap and for user-only authority.
- `yolo`: as `auto`, and the `yolo` entry of `pipeline.json` names the actor that also answers the cap decision with its `cap` choice (`final-fix` or `accept-head`). Without that entry, YOLO parks the questions `auto` would have answered. The YOLO actor never answers a user-only decision, rethinks, discards or raises the cap.

A finding repair runs the fixer on the files of the selected findings and commits a new candidate, which reruns review and every gate. A finding is resolved only when the next independent review confirms the fixer's [resolution response](#resolution-responses) and does not report the finding again. A later review that reports it again reopens it: the finding takes the new report's head and data, and its resolution records the reopening review in `reopened`. `run.pipeline.resolutions` records the repair, review, candidate and kind of each resolution.

The combined cap counts review rounds and finding-repair rounds; `budget` in every response shows `review`, `repair`, their `total`, and `limit`. Gate repairs count separately as `gate_retries` against `repair_limit`. A finding repair starts only when the cap leaves room for the repair and its rereview. Otherwise, or when another review is needed at the cap, the run opens a `cap` decision with these options:

- `accept-head`: accepts the review risk for the exact candidate and review inputs. The review stage shows `[ACCEPTED RISK]`; all other configured stages, including delivery checks, still run and can fail. A new candidate or a rule change voids the acceptance.
- `final-fix`: grants one repair and one independent final review outside the cap, recorded in `run.pipeline.final_fix`. Without a repairable finding it runs the final review directly. It is offered once per run, survives restart, and is never an acceptance.
- `increase-cap`: needs a `limit` above the current one. The `cap-increase` event records the old and new limits, actor, reason, and the review and repair usage at that moment. Usage never resets, and Apply refuses a limit below it.
- `rethink`: marks the run `rethought` and creates a linked successor run (`predecessor` and `successor`) with the same settings and every finding. The old run keeps its reviews, repairs, decisions and candidate ref; `inspect` with its `run_id` shows them. The successor needs a new Prepare.
- `discard`: marks the run `discarded` and keeps every record, the candidate branch and its worktree. It deletes nothing and releases the run's custody, so New run becomes possible.

A repair that needs authority beyond the approved intent parks a user-only `authority` decision in every mode. The controller detects a deleted file (`destructive`), a changed file mode (`permissions`) and a change outside the authorized paths (`scope`). The fixer can also stop without edits and answer `{"escalate":"intent|acceptance|credentials|scope|permissions|destructive","reason":"..."}`. Nothing is committed. The options are `rethink` and `discard`, plus `grant` when the fixer added or deleted files; neither a budget nor `accept-head` can answer it.

### Fixer authority grants

A repair never adds or deletes files by default. When it does, the controller commits nothing, keeps the edits in the repair's `partial`, restores the candidate, and parks the `authority` decision with category `add-files` or `destructive` and the attempted paths in `authority_paths`. Answering `grant` records a run grant for exactly those paths at that stage; the stage then repairs again within its normal budget. Only the User can answer it: neither `auto` nor the YOLO actor can.

A grant can also be declared in the trusted `pipeline.json`. `authority` is `add-files` or `delete-files`, `stages` lists `review`, `test`, `lint`, `docs` or `ci`, and each path is a repository-relative file or a directory ending in `/`. A reason is required. A repository override can never declare a grant.

`run.pipeline.grants` lists every grant with its source (`pipeline.json` or `run`), actor, reason, and deciding decision. The fixer input lists the grants of its stage, and each repair records the changes it made under a grant in `granted`, for example `add-files:testdata/case.json`. Changed file modes, executable new files, and edits outside the authorized paths still park a decision that a grant cannot answer.

Answer the open decision with `decide` (terminal `d`). The terminal accepts an option exactly as listed, a limit in digits, and finding IDs as shown, separated by commas or spaces. Narrow screens abbreviate long finding IDs with `…`; a displayed finding ID selects only a unique matching finding. Other values wider than the screen, such as URLs and refs, are printed whole on their own line. Full IDs remain available through AXI. Only an empty line selects the whole group. Other input selects the listed IDs after validation. The request carries the usual run, input, token and action fields, `gate_id` set to the decision ID, `choice`, a `reason`, and `selected` or `limit` where the choice needs them:

```json
{"version":1,"op":"decide","run_id":"...","input_id":"...","gate_id":"decision id","expected":"token","action_id":"unique","choice":"final-fix","reason":"one last repair"}
```

Each answer records the choice, the actor (`terminal`, `axi`, `auto`, or `yolo:<actor>`), the reason as typed, and the action ID; no quote is generated. Replaying an action returns its receipt. An answered or superseded decision changes the gate, so a stale answer refuses. A rule or limit change supersedes an open decision that is not user-only. `pipeline` refuses while a decision is open.

Still unsupported in step 4: mechanical lint autofix.

### Recovery after interruption

Every reviewer, fixer, check, stage command and pipeline Git command runs through the runner and gets an executor record under `GREENHOUSE_HOME/executors/`. The runner holds `<id>.lease` with `flock` and passes the descriptor to the child, captures bounded output in `<id>.out` and `<id>.err`, and writes `<id>.json` with the exit code and input digest only after it reaped the whole process tree. When the daemon dies, its executors keep running until they finish or reach the 10-minute runner deadline; nothing signals them by PID.

On restart, and every 250 ms while an orphaned executor still holds its lease, the daemon probes the open recovery:

- An interrupted review whose record is complete and whose input digest matches is adopted as if it had finished live. Its budget stays consumed.
- An interrupted command stage whose commands all have complete records, with a clean candidate at the recorded head, is adopted with the recorded exit codes and output. A full pipeline then parks (`parked`) until `pipeline` continues it.
- Everything else, including every fixer, conflict fix, target merge, candidate creation, push and handback, stays `recovery-required`.

`run.recovery` holds the evidence: the kind and stage, each executor's state (`running`, `exited`, `stopped`, or `unverified` when the runner left no complete record), and for a candidate its ref, recorded and observed heads, cleanliness and merge state. If the runner itself is killed, a descendant that closed the inherited descriptor can outlive the lease undetected; `unverified` marks that risk. The terminal shows the same evidence under `RECOVERY REQUIRED` and answers it with `e`. AXI uses `recover` with `gate_id` set to `run.recovery.id`, a `choice`, and a `reason`:

```json
{"version":1,"op":"recover","run_id":"...","input_id":"...","gate_id":"recovery id","expected":"token","action_id":"unique","choice":"adopt","reason":"the commit matches the checked repair"}
```

- `inspect`: probes again and records a `recovery-inspect` event.
- `adopt`: offered only when no executor still holds a lease and the observed candidate matches what the durable record predicted: a repair commit whose parent is the repair base and whose diff equals the checked diff, with every configured check passed; a conflict or target merge whose parents are the recorded candidate and target and, for a conflict, without markers; an intact new candidate worktree; or a source branch that is unchanged or already at the candidate after an interrupted handback. A full pipeline continues from there.
- `discard`: keeps an advanced head at `refs/greenhouse/recovered/<recovery id>` and uncommitted changes in `run.recoveries[].partial`, resets the private candidate to the recorded head, and reruns the stage. The rerun spends new budget; when the budget refuses it, the run stays stopped at that stage with the reason. Interrupted handbacks cannot be discarded.
- `new-run`: marks the run `superseded`, keeps its records and candidate ref, and links a successor that inherits the consumed review and repair usage, the gate repair usage, and any final-fix grant. `run.inherited` records them. The successor can be prepared again until it dispatches its own review, and Prepare keeps the inherited usage and grant.

Each answer is stored in `run.recoveries` with its choice, actor, reason, action ID and outcome, and changes the token and gate, so a stale or repeated answer refuses or replays its receipt. No choice refunds budget.

A candidate ref is guarded by a lease file in the repository's common Git directory under `greenhouse-leases/`. It names the owning state home and an epoch, and survives daemon restarts. Creating a candidate takes the lease; every fixer start, commit, target merge, conflict fix and recovery write first checks that this state home holds it at the epoch the run recorded. A daemon on another state home, or a restored copy of an older store, is refused. `discard` moves the lease to the next epoch. The daemon removes a lease of its own state home only when the run holding that exact epoch has released custody and has no running executor. Runs created before leases stay unleased.

## Configuration versions

`fixer.json`, `backends.json`, and `pipeline.json` carry `"version": 2`. Retired `reviewer.json` and `providers.json` files remain on disk and appear as `retired` in configuration inspection. They do not route new reviews. Migration skips them. Legacy executable and provider fields in `backends.json`, and executable/model/provider fields in `fixer.json`, do not control new invocations. Retained review inputs keep their original configuration and arguments; resuming requires explicit preparation through the native agent boundary.

`migrate` (terminal `u`) upgrades every older file in one action. It checks that the new file decodes to exactly the old values, saves the original bytes as `<file>.v1-<digest>.bak` with mode 0600, and replaces the file atomically. Each migration is recorded in `migrations` with the file, versions, backup, before and after digests, actor, reason, and action ID. If any file is newer or invalid, nothing changes.

`pipeline.json` declares `"budget_unit": "rounds"`; migration adds it, because version 1 counted rounds. Greenhouse never converts a budget unit and refuses any other unit. Rule packs keep schema version 1, and a newer pack fails its load with an upgrade message.

## Repository overrides and trust

A worktree may hold `.greenhouse.json`:

```json
{"version": 1, "budget_unit": "rounds", "mode": "ask", "limit": 3, "rules": ["unslop"]}
```

It applies to new runs only after the User trusts the repository with `trust` (terminal `t`), which binds the exact file digest shown in `repository.digest`. An untrusted, changed, newer or invalid file is shown as ignored with the reason, and new runs use the per-user defaults. Trust is recorded per repository with its actor, reason, and action ID. Trusting never changes the current run.

An override can only narrow or select among per-user choices. `mode`, `preset` and `limit` cannot exceed the per-user values, with `ask` below `auto` below `yolo` and `one-pass` below `full`. `auto_resume` cannot turn on a per-user `off`. `rules` must be built-in or already selected in the per-user defaults, and a backend must be configured in per-user files. Keys that would set an executable, command, grant, remote, or YOLO actor make the whole file ignored.

Responses carry `repository` (path, `status` of `absent`, `ignored` or `applied`, reason, digest and trust record), the effective `defaults` with `defaults_sources`, and `sources` on each configuration snapshot. Saving with `"target":"repository"` requires a trusted, unchanged file. It writes only the values that differ from the per-user defaults and trusts the written bytes. Commit the file to share it. Until then, guarded handback accepts an untracked or modified `.greenhouse.json` only while its bytes match the trusted digest; any other change still blocks handback.

## Transcript logs

Normal use never needs a transcript: findings, verdicts and bounded summaries are in the run. For diagnosis, each reviewer and fixer invocation appends its raw stdout and stderr to `transcripts/<run-id>.log` in the state directory (mode 0600). Before writing, Greenhouse replaces the values of environment variables whose names contain `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `AUTH`, `CREDENTIAL`, `COOKIE` or `SESSION`, along with bearer tokens and common API-key shapes, with `[REDACTED]`. A run's log stops at 4 MiB with an omission marker. AXI `transcript` returns the whole log of the current run, or of `run_id`; the terminal `o` shows its last 40 lines.

## State and compatibility

`GREENHOUSE_HOME` overrides `~/.local/state/greenhouse`. The state directory must be private, with mode 0700. The Unix socket path must fit the platform's Unix-socket path limit; use a short state path in deeply nested environments.

One daemon owns an advisory `flock` before reading recovery state or binding the socket. It never unlinks a live listener. Requests execute serially. Each request commits the entire version-1 store by writing and syncing a same-directory temporary file, renaming it, and syncing the directory before responding. Action receipts and run changes share that commit. Startup parks persisted `running` or `writing` states as `recovery-required` and reconciles them as described in [recovery](#recovery-after-interruption); it never reruns them. Invalid run records and unknown store versions refuse startup. Rule snapshots are an additive version-1 extension: old stores need no rewrite or counter migration. New snapshots validate source, exact bytes, digest, parsed criteria, and configuration identity on load.

This design assumes a local filesystem with reliable flock, atomic rename, and fsync. It does not support network filesystems, concurrent external store editors, cross-user access, or online migration. Protocol and configuration versions follow the [compatibility policy](docs/compatibility.md). Disk corruption has no automatic repair. A directory-sync failure can leave an uncertain commit; inspect after restart instead of inventing another mutation. Back up the state directory while the daemon is stopped. Storage grows with retained runs, events, receipts, snapshots, and executor records; compaction is not implemented.

The daemon commits review or repair admission and budget consumption before starting execution. Completion returns through the serialized controller; inspection remains available during execution. Review cancellation stops its current child. Repair and check cancellation asks the Linux subreaper to terminate and reap descendants. A killed runner or unverified cleanup leaves an unknown outcome. Restart never signals stored PIDs or retries. Run `greenhouse stop` to stop the daemon for the current `GREENHOUSE_HOME`. It refuses while work is running; let the work settle and retry. It reports when no daemon is running. SIGTERM handling remains available. Client exit is not daemon shutdown. Review-stage Git commands remain observational. The separately authorized one-pass repair creates a managed candidate branch/worktree and local commit; guarded handback may fast-forward the unchanged source branch. Greenhouse does not stash, reset, rebase, force-update, push, or discard source work.

## Verification

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
```

Set `TMPDIR` to an isolated scratch directory. Integration fixtures use private state homes and temporary Git repositories, and build the executable with the race detector. The Python check drives actual 56-, 80- and 100-column PTYs through configuration migration, repository trust, per-value sources, a repository Save, the transcript log, a backend quota pause with its explanation, auto-resume cancellation, an agent switch and rerun, selected repair, check results, dirty-handback refusal, safe handback, full-pipeline repair to CI readiness, a failed gate with diagnostics, a repaired-unreviewed stage, grouped ask and cap decisions answered with `d`, final-fix, accepted risk, and AXI parity, alongside lifecycle and settings flows. The events check opens the terminal in 56-, 80- and 100-column PTYs, starts a review over AXI without a keypress and requires the terminal to redraw through `running` to the findings, then follows a host-owned run and requires its terminal mutation to refuse; `before` as a third argument records an older binary's behavior instead. The recovery check kills the daemon right after a selected repair commits, then captures the recovery screen before and after `adopt` at 56 columns, `discard` at 80 and `new-run` at 100. The four checks above set `TERM=dumb` and so drive the line client. The TUI check drives the interactive screen in an isolated tmux server at 56, 80 and 100 columns by 24 rows, with fake native agents and synthetic agent sessions in a private `HOME`: missing agent, one Run with collected intent and history isolation, the no-match and ambiguity questions, intent editing with Esc, a moved head, progress, resize while running, cancel versus detach and reattach, findings selection, repair and handback, details and help, a Settings draft with Apply and Save, a decision, daemon loss and resync, `NO_COLOR`, and the line client under piped stdin and `TERM=dumb`. It needs `tmux`. After a dependency change, regenerate the notices with `python3 tools/notices.py "$TMPDIR/greenhouse"`. Logs are under the selected evidence directory. Restart fixtures seed synthetic usage and interrupted-writer state only while their daemon is stopped; they are not review evidence.

## Review evaluation

`eval/` is a development command that measures independent review with a real model. It is not part of the `greenhouse` executable, and `go test ./...` never calls a model.

The corpus in `eval/testdata/corpus` holds small Git changes with known findings: planted correctness bugs, `unslop` and `no-comments` violations, out-of-scope noise, and clean changes. Each `case.json` records the change intent, the selected rules, and every expected finding by file, candidate line range, origin, rule, severity, and acceptable priorities. `noise` lists lines that must not be flagged. `corpus.json` lists the cases and a revision number; reports also record a digest of every corpus byte. Each expected finding has an `id`. After a ground-truth change, `go run ./eval rescore docs/evidence/review-eval/*/` scores the recorded findings again from their adjudication records.

```sh
go run ./eval check
go run ./eval run -model openai/gpt-5.6-luna \
  -parallel 4 -label "what changed" -out docs/evidence/review-eval/NN-name
```

Add `-backend claude -model claude-sonnet-5-5` to review through the Claude backend; `-agent` names the `claude` executable (default: `claude` on `PATH`). The judge still runs on Pi with `-judge-model`.

For each case, `run` commits fixed base and head trees and drives Greenhouse through AXI. A PATH-installed recording shim forwards the emitted arguments and input to the native CLI. Configure evaluation routing in that CLI. Greenhouse provider files are not created. Runs are paid model calls.

`report.md` summarizes a run; `report.json` holds the configuration, every case record, and the summary. Each case directory holds `input.json` (the exact stdin bytes), `argv.json`, `events.jsonl` (Pi's events without streaming deltas), and `timing.json`. The metrics are:

- A finding is eligible for an expected finding when file, origin, and rule are equal, the line ranges overlap, and the finding spans at most 15 lines. Eligibility earns no credit by itself.
- `adjudications.json` holds one record per eligible finding: finding index and digest, expected defect ID, verdict (`detects`, `different-defect`, or `duplicate`), and a one-sentence rationale quoting the finding. `run` produces it with a separate judge call per finding (`-judge-model`, default `-model`); `go run ./eval judge -model provider/id run-dir` produces it for saved runs. The judge model and prompt are recorded. An eligible finding without a record earns no credit.
- Recall is detected over expected findings; high-severity recall counts only planted `high` findings. Precision is matching findings over all findings.
- Priority accuracy counts detections whose priority is in the expected list.
- A false positive detects and duplicates nothing; a `different-defect` record makes it a wrong-defect false positive. A blocking result is a P0 or P1 false positive; an inconclusive review on a clean case also blocks.
- The bar is high-severity recall of at least 80% and no blocking result on a clean case.
- Inconclusive reviews score no findings. `model_answer` keeps the model's raw answer.
- Cost uses Pi's catalog pricing; a proxy or subscription may bill differently. Latency is the wall time of the Pi process.

[The recorded runs](docs/evidence/review-eval/README.md) document each reviewer change, its measured effect, and a model comparison. On this corpus, `openai/gpt-5.6-luna` through Pi stays the recommended default reviewer. One run of `claude-sonnet-5-5` through the Claude backend matched or beat it on recall and precision at about 20 times the cost, so it is the recommended reviewer `fallback`. `xai` answered `402` (no credits), and `google`, `openrouter`, and `meta` had no credentials.

## Fixer evaluation

`go run ./eval fix-run` measures selected-finding repair with a real model through the product's one-pass repair path. Like the review evaluation, it is development-only, and `go test ./...` never calls a model.

`eval/testdata/fixer` holds nine defect cases and one dispute case. Each records base and head trees, findings, selected findings, the expected response (`fixed` or `disputed`), and the allowed edit ranges. Defect cases include a reference repair in `fix/`; every case has hidden tests in `hidden/`. Hidden `TestAccept` tests check the selected finding; `TestRegress` tests and the candidate's own tests check behavior that must survive. The fixer never sees the hidden tests. `go run ./eval fix-check`, also run by `go test ./eval`, proves that every acceptance test of a fixed case fails on the head while the other tests pass, and that the reference repair passes every test inside the scope.

```sh
go run ./eval fix-check
go run ./eval fix-run -parallel 4 \
  -label "what changed" -out docs/evidence/fixer-eval/NN-name
```

For each case, `fix-run` seeds a fixed review, records the native fixer invocation, runs trusted checks, and evaluates the resulting commit with hidden tests. The native CLI owns login and routing. The metrics are:

- A case is fixed when the response matches the expected one, every acceptance test passes, no other test fails, and the repair has no scope violation.
- A regression is a failing `TestRegress` or candidate test.
- A scope violation is a changed line outside the case's scope ranges, an insertion that borders none, an added or deleted file, or an edit to another path that Greenhouse refused. Unselected findings lie outside the scope ranges.
- Resolution-response accuracy counts cases whose responses match the expected ones.
- Cost uses Pi's reported cost, or the reported tokens at `-price` when Pi reports none. Latency is the wall time of the fixer process.
- The bar is at least 75% of cases fixed, with no regression and no scope violation on any case.

[The recorded runs](docs/evidence/fixer-eval/README.md) show the default fixer meeting the bar at 90% with no regression or scope violation. A `fixed` response without an edit to the finding's file now fails the repair. The latest run uses Pi's `medium` thinking level; the remaining dispute case fails honestly.

See [the rule contract](docs/rules.md) for selection, validation, provenance, and fixture delivery limits.

See [the implementation sequence](docs/implementation-plan.md) for v1 status and known limits.
