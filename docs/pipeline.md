# Full pipeline

The `full` preset runs review, repair, test, docs, lint, push, PR and CI on a private candidate. It ends at `ci-ready`, or at `locally-ready` for a local-only plan. Greenhouse never merges.

To drive a full run without a terminal, see the [Worker contract v1](worker-contract-v1.md).

## Command plan

Apply the `full` preset, then Prepare. Prepare loads the command plan from one of two files:

1. `.greenhouse-pipeline.json` in the repository, when that file exists.
2. Otherwise, the private `pipeline.json` in `GREENHOUSE_HOME`, with mode 0600.

The repository file overrides the per-user plan as a whole. An untrusted repository file never falls back to the per-user plan.

The repository plan must be trusted before Prepare can load it:

1. Inspect the repository status.
2. Send `trust` with the reported `pipeline_digest`.

A file change invalidates that trust. The existing repository `trust` action handles both `.greenhouse.json` and `.greenhouse-pipeline.json`; trust each file by its reported digest. See [repository overrides and trust](configuration.md#repository-overrides-and-trust).

Both files use the same version-2 schema, including `"budget_unit":"rounds"`. A plan can declare test, docs and lint commands, forge delivery, required checks and the PR base:

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

`run.pipeline.plan_sources` identifies the source of each plan field. The pipeline records the digest of each command executable.

## Stages

`pipeline` (line client `g`) runs the stages in order on a managed private candidate worktree.

| Stage | What it does |
| --- | --- |
| intent | Loads the prepared intent, the exact target and the command plan. No process runs. |
| sync | Runs `git merge-base --is-ancestor` and, when needed, `git merge --no-ff` of the target through the command runner. It never rebases. |
| review | An independent reviewer reviews the exact candidate. The full contract also returns a documentation assessment. |
| test, docs, lint | The configured commands run directly in the candidate, never through an agent. |
| push | The controller probes the configured remote ref, then pushes the exact candidate without force. |
| PR | The GitHub forge adapter finds an open PR for the configured branch, or creates one. |
| CI | The adapter waits for the trusted `required_checks` on the exact candidate SHA. |

### Sync conflicts

A conflict aborts the merge, opens a gate and parks the run. The author has two ways out:

- Resolve the conflict in the source and start a New run.
- Send `resolve-conflict` (line client `m`) with the current `gate_id`. This authorizes the fixer to edit only the conflicted paths and commit the merge.

### Review findings

Actionable findings go to a repair or a decision, as the [mode](#modes-and-decisions) decides. A P3 finding, or one without an action, is informational: it stays visible and blocks nothing.

### Command stages

Each test, docs or lint result records the argv, the executable digest, the exit code and the last 16 KiB of output. A command that edits the candidate fails the stage.

The docs stage also needs the current review to report documentation as `documented` or `not-needed`. A passing doc build alone does not pass it.

### Delivery

- After an interrupted push, the controller checks the remote ref before it retries. An unexpected remote head blocks delivery.
- The PR adapter updates an existing PR instead of creating a duplicate. The PR number and URL persist for CI reconciliation.
- `GREENHOUSE_GITHUB_API` can point to a local fixture; production defaults to GitHub's API. Authentication uses `GH_TOKEN` when it is set.
- CI requires each named check exactly once, completed successfully for the exact candidate SHA. During polling, the adapter verifies that the persisted PR number still points to that SHA.
- Unlisted green checks do not establish readiness. Missing, duplicate, pending, failed, stale and unknown checks block CI readiness.
- Transport failures retry at most three times with persisted exponential backoff. Exhaustion parks the run without dispatching a fixer.

### Unconfigured and not-applicable stages

- A stage with no configured command is `unconfigured` and blocks.
- A stage listed in `not_applicable` needs a reason and shows as not applicable.
- A local-only plan must mark push, PR and CI not applicable explicitly. Otherwise missing delivery configuration blocks the pipeline.
- A not-applicable docs stage still fails when the review reports documentation as `missing`.
- An executable whose digest changed since Prepare stops its stage.

## Gate repairs

When the mode allows it, a failing test, lint, docs or CI stage listed in `repairs` starts the fixer from `fixer.json` in the private candidate.

- The fixer may edit only files changed since the target, plus `docs_paths` for docs.
- The controller commits the change as a new candidate.
- A bounded CI repair creates a new candidate, then reruns review and the affected local and delivery gates.
- Gate repairs use `repair_limit`, separate from the review-round limit. Exhausting either parks the run.
- A repaired stage stays `repaired-unreviewed` until the new candidate passes review and the stage reruns.

## What reruns when inputs change

Every stage's evidence binds its declared inputs: the candidate and the plan digest, and for review also the preset, the reviewer configuration and the selected rules with their content digests.

| Change | Reruns |
| --- | --- |
| New candidate | Review and every gate |
| Rule change | Review only |
| Mode or limit change | Nothing |

## Ready states and handback

The run ends `ci-ready` only after the exact candidate passes all configured local gates and the trusted GitHub checks. A local-only plan ends at `locally-ready`, and only when it marks push, PR and CI not applicable explicitly.

The terminal and AXI show the PR URL and head.

The separate `handback` action fast-forwards the unchanged, clean source branch to that candidate; see [handback](repair.md#handback). New run is blocked while new candidate commits remain unhanded.

## Continuing and interruption

Running `pipeline` again continues a parked or cancelled run without rerunning current evidence.

Restart adopts the result of an interrupted review or command stage when its executor record proves the result. It then parks the run until `pipeline` continues it. Any other interrupted step stays `unknown-outcome` and is never retried.

A command or Git step whose runner outcome is unknown does the same. The run needs [recovery](recovery.md), and New run is refused. No other run on the same branch, or in the same worktree, can start a pipeline, repair or conflict fix. Their inspection shows the hazard and omits those actions.

The same rule covers:

- a conflict fix whose merge or abort outcome is unknown
- an ambiguous candidate worktree creation
- a handback fast-forward that leaves the source neither unchanged nor at the candidate

## Pipeline status

AXI returns everything in `run.pipeline`:

- status, failed stage and reason
- gate and conflict paths
- gate-repair usage and limit
- each stage's status, inputs, candidate, runs, repairs and command results
- repair records and superseded reviews

The terminal shows the same stages with distinct markers: `[pass]`, `[pass after repair, rereviewed]`, `[REPAIRED, NOT REVIEWED]`, `[FAIL]`, `[n/a]`, `[UNCONFIGURED]`, `[DECISION]`, `[ACCEPTED RISK]` and `[unsupported]`.

A ready run also shows its outcome, and AXI returns the same value in `run.pipeline.outcome`:

- `CLEAN PASS`
- `REPAIRED PASS (rereviewed after repair)`
- `ACCEPTED RISK (review risk accepted, not a clean pass)`

## Modes and decisions

The mode decides who answers a question the pipeline cannot settle alone.

| Mode | Behavior |
| --- | --- |
| `ask` | Every repair waits for an answer. The actionable findings of one review form one decision of kind `findings`; one answer authorizes the whole group or a selected subset. A failing gate that the plan may repair opens a `gate-repair` decision. |
| `auto` (default) | The controller repairs actionable findings and authorized gates without asking. It parks at the review cap and for user-only authority. |
| `yolo` | As `auto`. In addition, the `yolo` entry of the effective pipeline plan names the actor that answers the cap decision with its `cap` choice (`final-fix` or `accept-head`). Without that entry, YOLO parks the questions `auto` would have answered. The YOLO actor never answers a user-only decision, and never chooses `rethink`, `discard` or a cap increase. |

### Finding repairs

A finding repair runs the fixer on the files of the selected findings and commits a new candidate. The new candidate reruns review and every gate.

A finding is resolved only when the next independent review confirms the fixer's [resolution response](repair.md#resolution-responses) and does not report the finding again.

A later review that reports the finding again reopens it. The finding takes the new report's head and data, and its resolution records the reopening review in `reopened`. `run.pipeline.resolutions` records the repair, review, candidate and kind of each resolution.

### The review cap

The combined cap counts review rounds and finding-repair rounds. `budget` in every response shows `review`, `repair`, their `total` and `limit`. Gate repairs count separately, as `gate_retries` against `repair_limit`.

A finding repair starts only when the cap leaves room for the repair and its rereview. Otherwise, or when another review is needed at the cap, the run opens a `cap` decision with these options:

| Option | Effect |
| --- | --- |
| `accept-head` | Accepts the review risk for the exact candidate and review inputs. The review stage shows `[ACCEPTED RISK]`. All other configured stages, including delivery checks, still run and can fail. A new candidate or a rule change voids the acceptance. |
| `final-fix` | Grants one repair and one independent final review outside the cap, recorded in `run.pipeline.final_fix`. Without a repairable finding, it runs the final review directly. It is offered once per run, survives restart, and is never an acceptance. |
| `increase-cap` | Needs a `limit` above the current one. The `cap-increase` event records the old and new limits, the actor, the reason, and the review and repair usage at that moment. Usage never resets, and Apply refuses a limit below it. |
| `rethink` | Marks the run `rethought` and creates a linked successor run (`predecessor` and `successor`) with the same settings and every finding. The old run keeps its reviews, repairs, decisions and candidate ref; `inspect` with its `run_id` shows them. The successor needs a new Prepare. |
| `discard` | Marks the run `discarded` and keeps every record, the candidate branch and its worktree. It deletes nothing and releases the run's custody, so New run becomes possible. |

### Authority decisions

A repair that needs authority beyond the approved intent parks a user-only `authority` decision in every mode. The controller detects:

- a deleted file (`destructive`)
- a changed file mode (`permissions`)
- a change outside the authorized paths (`scope`)

The fixer can also stop without edits and answer `{"escalate":"intent|acceptance|credentials|scope|permissions|destructive","reason":"..."}`.

Nothing is committed. The options are `rethink` and `discard`, plus `grant` when the fixer added or deleted files. Neither a budget nor `accept-head` can answer an authority decision.

### Answering a decision

Answer the open decision with `decide` (line client `d`).

The terminal accepts:

- an option exactly as listed
- a limit in digits
- finding IDs as shown, separated by commas or spaces

Only an empty line selects the whole group. Other input selects the listed IDs after validation.

Narrow screens abbreviate long finding IDs with `…`. A displayed finding ID selects only a unique matching finding. Other values wider than the screen, such as URLs and refs, print whole on their own line. AXI keeps the full IDs.

The request carries the usual run, input, token and action fields; `gate_id` set to the decision ID; `choice`; a `reason`; and `selected` or `limit` where the choice needs them:

```json
{"version":1,"op":"decide","run_id":"...","input_id":"...","gate_id":"decision id","expected":"token","action_id":"unique","choice":"final-fix","reason":"one last repair"}
```

Each answer records the choice, the actor (`terminal`, `axi`, `auto` or `yolo:<actor>`), the reason as typed and the action ID. Greenhouse generates no quote.

- Replaying an action returns its receipt.
- An answered or superseded decision changes the gate, so a stale answer is refused.
- A rule or limit change supersedes an open decision that is not user-only.
- `pipeline` is refused while a decision is open.

## Fixer authority grants

A repair never adds or deletes files by default. When it does, the controller:

1. commits nothing
2. keeps the edits in the repair's `partial`
3. restores the candidate
4. parks the `authority` decision with category `add-files` or `destructive`, and the attempted paths in `authority_paths`

Answering `grant` records a run grant for exactly those paths at that stage. The stage then repairs again within its normal budget. Only the User can answer it: neither `auto` nor the YOLO actor can.

Either trusted pipeline plan can also declare a grant:

- `authority` is `add-files` or `delete-files`.
- `stages` lists `review`, `test`, `lint`, `docs` or `ci`.
- Each path is a repository-relative file, or a directory ending in `/`.
- A reason is required.

A repository override (`.greenhouse.json`) can never declare a grant.

`run.pipeline.grants` lists every grant with its source (`pipeline.json`, `.greenhouse-pipeline.json` or `run`), actor, reason and deciding decision. The fixer input lists the grants of its stage. Each repair records the changes it made under a grant in `granted`, for example `add-files:testdata/case.json`.

Changed file modes, executable new files and edits outside the authorized paths still park a decision that a grant cannot answer.

## Not supported

Mechanical lint autofix is not supported.
