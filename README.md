<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/greenhouse-mark-dark.svg">
  <img src="docs/assets/logo/greenhouse-mark.svg" alt="Greenhouse logo" width="120">
</picture>

# Greenhouse

Greenhouse is a standalone terminal and JSON client for a per-user local run store. This slice implements lifecycle, settings, immutable rule resolution, independent Pi review, selected one-pass repair, configured checks, and guarded local handback. Review success is not test, lint, or delivery success.

## Build and open

```sh
go build -o greenhouse .
cd /path/to/a/git/worktree
/path/to/greenhouse
```

The executable requires Linux, Git, and a committed worktree. Runtime operation requires neither Go, Posse, nor Herdr. The bare command opens a single-pane, line-oriented terminal client. Press Enter after each command. `i` refreshes inspection, `c` toggles criteria details, `r` explicitly resolves rules, `s` opens Settings, and `n` requests New run. `p` prepares intent and an exact target commit, `v` dispatches one review, `x` cancels a review, fixer, or check, and `q` detaches. Ctrl-C also disconnects without cancelling the daemon.

Settings edits are drafts until you select Apply or Save. Apply adds an immutable run-local snapshot. Save changes user defaults without changing the current run. Use Settings twice to do both. Rule selection does not load bytes. Explicit resolution snapshots content; dispatch retains the exact emitted criteria and committed diff. The terminal supports mode, preset, combined review/repair limit, and rule selection. Repository overrides are unsupported and untrusted.

## JSON interface

Send one request on stdin to `greenhouse axi`. The invoking directory identifies the worktree; the CLI replaces any request `path` with that directory. Machine output is one JSON envelope on stdout.

```sh
printf '%s\n' '{"version":1,"op":"attach"}' | /path/to/greenhouse axi
```

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
    "rules": ["unslop", "no-comments"]
  }
}
```

Supported mutations are `apply`, `save`, `new`, `resolve-rules`, `prepare`, `review`, `repair`, `handback`, and `cancel`. All mutations use the same run, input, gate, token, and action fields. `resolve-rules`, `review`, and `cancel` require no settings or reason. `apply` and `save` require a complete settings object and a reason. `new` retains previous evidence and uses saved defaults. Source branch/head movement blocks Apply and Save on the old run. A current token-bound New request can capture the moved source when no writer or unknown-outcome hazard exists. Old evidence remains unchanged. Unknown executor or writer outcomes still block replacement; there is no recovery override in this slice.

Replay an identical mutation with the same action ID to obtain its recorded result, even after reconnect or restart. Reusing an ID with different input refuses. New action IDs require current tokens. Inspect the active run after replay: a recorded result may describe an older snapshot. An outcome of `ok` means the lifecycle request succeeded, never that a validation stage passed. Errors and unsupported operations have `refused` or `error` outcomes. The one-pass `repair` action requires `selected` finding IDs and a private per-user `fixer.json`. It admits one repair after a completed finding review, requires at least one configured trusted check, and binds the action to the exact review, intent, candidate, rules, selected findings, fixer, and check argv. Admission consumes repair budget durably. Replay returns its receipt; a second repair is refused. The controller creates a managed private worktree and branch, gives Pi only read/edit/write tools with extensions, MCP, skills, project context, and sessions disabled, then runs configured checks and creates a local commit. Tool restriction is not OS confinement. Checks run directly, not through an agent.

Create `fixer.json` in `GREENHOUSE_HOME` with mode 0600:

```json
{"executable":"/absolute/path/to/pi","model":"provider/exact-model-id","name":"Greenhouse","email":"greenhouse@example.invalid","checks":[["/absolute/path/to/check","--verify"]]}
```

Each check is an argv array with an absolute executable path. No shell is inserted. Missing checks are unconfigured. Changed fixer/check configuration refuses execution. The controller records check exit codes and bounded output summaries, verifies candidate scope, and commits only authorized paths. Binary changes, submodules, executable Git hooks, configured Git filters, fsmonitor, attributes, or signing requirements are unsupported and refuse rather than being bypassed. Failed or ambiguous candidates remain available in their managed worktree for recovery.

The `handback` action is separate. It fast-forwards the original source branch only when source identity is unchanged, the fixer is known stopped, candidate lineage is exact, and the source worktree and index are clean. Refusal preserves the candidate commit and source work. A repaired head is `fixed-not-independently-rereviewed`; Greenhouse never automatically rereviews or publishes it. New run is blocked while candidate custody is unpublished or ambiguous.

The terminal `f` action requests selected finding IDs for repair; `h` requests handback. AXI requests place the selected ID list in `selected`. All states, selected findings, check results, commit identities, handback status, and budget usage are available in the same run snapshot. There are no acceptance, publication, or automatic recovery commands.

Defaults are `auto`, `one-pass`, a combined limit of 4, and selected `unslop` and `no-comments` names. The numeric limit is provisional. Valid limits are 1 through 1000; reductions cannot erase consumed usage. Modes and presets are stored selections, not active automation.

## Independent review configuration

Create a private `reviewer.json` in `GREENHOUSE_HOME` (or the default state directory), with mode 0600:

```json
{"executable":"/absolute/path/to/pi","model":"provider/exact-model-id"}
```

Only per-user configuration grants executable authority. There are no arbitrary arguments, repository overrides, or session selectors. Models require an exact provider-qualified ID without a thinking suffix or wildcard. A successful response must report the same provider and model. The adapter targets the installed Pi 1.5.0 CLI; incompatible flags or event output yield an inconclusive result.

Prepare (`p`, or AXI `prepare` with `intent` and `target`) captures a full commit object ID, the current candidate, selected committed diff, and reviewer configuration. Inspect `run.selected_diff` or toggle `c` before dispatch. Resolve rules, then request `review` with current mutation tokens. Review rejects changed reviewer configuration until another explicit Prepare. Dirty files are neither included nor changed. Empty diffs, binary patches, submodules, and inputs exceeding 1 MiB refuse; the encoded bundle is capped at 2 MiB.

Each run permits one invocation. Admission consumes review usage even if launch fails, the process is cancelled, or its output is inconclusive. Apply cannot reset usage. `full`, repairs, cap decisions, acceptance, forge operations, and automatic retries are unsupported. New run retains prior finding identities; omission does not resolve them. Resolution actions are not implemented.

Pi receives a fresh ephemeral session, explicit read-only tools, no extensions, MCP, skills, context files, prompt templates, or project approval. Its working directory and agent directory are fresh and private. Author session routing and process-injection environment variables are removed. The adapter uses environment-based provider credentials; existing Pi auth files and custom provider files are not copied or loaded. Read-only tools are not an OS sandbox. Pi gets no repository checkout. The bundle carries the committed diff and the candidate content of each changed file, with line numbers, but no other source files.

`run.review` exposes backend, requested and reported model, deterministic argv, configuration digest, exact emitted input and criteria digests, coverage, findings, status, and bounded execution summaries. Output is capped at 4 MiB, stderr at 64 KiB, and runtime at ten minutes. Review uses `exec.CommandContext` with a 250 ms pipe-wait grace; it does not verify descendant cleanup. Repair and check commands use a Linux subreaper that terminates and reaps descendants after completion or cancellation. If that runner dies or cannot verify cleanup, Greenhouse parks the writer outcome. Raw transcripts and stderr are withheld to avoid persisting credentials. The terminal displays the same findings, provenance, and summary; `c` also shows the diff. A successful verdict certifies only the bound independent-review input. Config changes and source movement mark current evidence stale; historical verdicts remain inspectable.

## State and compatibility

`GREENHOUSE_HOME` overrides `~/.local/state/greenhouse`. The state directory must be private, with mode 0700. The Unix socket path must fit the platform's Unix-socket path limit; use a short state path in deeply nested environments.

One daemon owns an advisory `flock` before reading recovery state or binding the socket. It never unlinks a live listener. Requests execute serially. Each request commits the entire version-1 store by writing and syncing a same-directory temporary file, renaming it, and syncing the directory before responding. Action receipts and run changes share that commit. Startup parks persisted `running` or `writing` states as `recovery-required`; it never reruns them. Invalid run records and unknown store versions refuse startup. Rule snapshots are an additive version-1 extension: old stores need no rewrite or counter migration. New snapshots validate source, exact bytes, digest, parsed criteria, and configuration identity on load.

This design assumes a local filesystem with reliable flock, atomic rename, and fsync. It does not support network filesystems, concurrent external store editors, cross-user access, online migration, or mixed protocol versions. Disk corruption has no automatic repair. A directory-sync failure can leave an uncertain commit; inspect after restart instead of inventing another mutation. Back up the state directory while the daemon is stopped. Storage grows with retained runs, events, receipts, and snapshots; compaction is not implemented.

The daemon commits review or repair admission and budget consumption before starting execution. Completion returns through the serialized controller; inspection remains available during execution. Review cancellation stops its current child. Repair and check cancellation asks the Linux subreaper to terminate and reap descendants. A killed runner or unverified cleanup leaves an unknown outcome. Restart parks unknown outcomes without signalling stored PIDs or retrying. To stop the daemon administratively, send SIGTERM to the PID recorded in the isolated state's `pid` file after verifying the process. Client exit is not daemon shutdown. Review-stage Git commands remain observational. The separately authorized one-pass repair creates a managed candidate branch/worktree and local commit; guarded handback may fast-forward the unchanged source branch. Greenhouse does not stash, reset, rebase, force-update, push, or discard source work.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
test -z "$(gofmt -l *.go)"
go build -o "$TMPDIR/greenhouse" .
python3 tools/terminal_check.py "$TMPDIR/greenhouse" docs/evidence
```

Set `TMPDIR` to an isolated scratch directory. Integration fixtures use private state homes and temporary Git repositories, and build the executable with the race detector. The Python check drives actual 56- and 100-column PTYs through selected repair, check results, dirty-handback refusal, safe handback, and AXI parity, alongside lifecycle and settings flows. Logs are under the selected evidence directory. Restart fixtures seed synthetic usage and interrupted-writer state only while their daemon is stopped; they are not review evidence.

## Review evaluation

`eval/` is a development command that measures independent review with a real model. It is not part of the `greenhouse` executable, and `go test ./...` never calls a model.

The corpus in `eval/testdata/corpus` holds small Git changes with known findings: planted correctness bugs, `unslop` and `no-comments` violations, out-of-scope noise, and clean changes. Each `case.json` records the change intent, the selected rules, and every expected finding by file, candidate line range, origin, rule, severity, and acceptable priorities. `noise` lists lines that must not be flagged. `corpus.json` lists the cases and a revision number; reports also record a digest of every corpus byte.

```sh
go run ./eval check
export OPENAI_API_KEY="$(pi auth print-api-key --provider openai)"
go run ./eval run -model openai/gpt-5.6-luna -base-url http://127.0.0.1:8317/v1 \
  -parallel 4 -label "what changed" -out docs/evidence/review-eval/NN-name
```

For each case, `run` commits the base and head trees with fixed identities and dates, starts an isolated daemon, and sends `attach`, `apply`, `resolve-rules`, `prepare`, and `review` over AXI. `reviewer.json` names the eval binary, which acts as a shim: it records the exact argv and stdin Greenhouse delivers, then runs the real Pi with the same arguments and input and records its JSON events. Greenhouse loads no custom provider file, so `-base-url` makes the shim write a `models.json` that routes the model's provider to that URL inside the reviewer's private agent directory. Credentials come from the environment, as for Greenhouse. Credential-like environment values are redacted from the output. Runs are paid model calls.

`report.md` summarizes a run; `report.json` holds the configuration, every case record, and the summary. Each case directory holds `input.json` (the exact stdin bytes), `argv.json`, `events.jsonl` (Pi's events without streaming deltas), and `timing.json`. The metrics are:

- A finding matches an expected finding when file, origin, and rule are equal, the line ranges overlap, and the finding spans at most 15 lines. Only the first match detects; later matches count as duplicates.
- Recall is detected over expected findings; high-severity recall counts only planted `high` findings. Precision is matching findings over all findings.
- Priority accuracy counts detections whose priority is in the expected list.
- A false positive matches no expected finding. A blocking result is a P0 or P1 false positive; an inconclusive review on a clean case also blocks.
- The bar is high-severity recall of at least 80% and no blocking result on a clean case.
- Inconclusive reviews score no findings. `model_answer` keeps the model's raw answer.
- Cost uses Pi's catalog pricing; a proxy or subscription may bill differently. Latency is the wall time of the Pi process.

[The recorded runs](docs/evidence/review-eval/README.md) document each reviewer change and its measured effect.

See [the rule contract](docs/rules.md) for selection, validation, provenance, and fixture delivery limits.

See [the implementation sequence](docs/implementation-plan.md) for the unsupported spec areas.
