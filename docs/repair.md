# Repair and handback

In the `one-pass` preset, a fixer repairs the findings you select after a completed review. Trusted checks run on the result, and a separate handback fast-forwards your branch. The full pipeline uses the same fixer; see [full pipeline](pipeline.md).

## Configure the fixer

Create `fixer.json` in `GREENHOUSE_HOME` with mode 0600:

```json
{"version":2,"name":"Greenhouse","email":"greenhouse@example.invalid","checks":[["/absolute/path/to/check","--verify"]]}
```

- Each check is an argv array with an absolute executable path. Greenhouse inserts no shell.
- Missing checks are unconfigured. Repair needs at least one configured trusted check.
- A changed fixer or check configuration refuses execution.

## Request a repair

Send `repair` with the finding IDs in `selected`. On the interactive screen, select findings under `f` and press `r`; in the line client, use `f`.

The controller admits one repair after a completed finding review. The action binds the exact review, intent, candidate, rules, selected findings, fixer and check argv.

Admission consumes repair budget durably. A replay returns the admission receipt. A second repair is refused.

## What a repair does

1. The controller creates a managed private worktree and branch.
2. The fixer edits the candidate. Pi gets only the read, edit and write tools, with extensions, MCP, skills, project context and sessions disabled. The fixer runs under [OS confinement](agents.md#os-confinement).
3. The fixer answers each selected finding with a [resolution response](#resolution-responses).
4. The controller runs the configured checks directly, not through an agent. It records exit codes and bounded output summaries.
5. The controller verifies the candidate scope and creates a local commit of the authorized paths only.

Greenhouse refuses, rather than bypasses, these cases:

- binary changes
- submodules
- executable Git hooks
- configured Git filters
- fsmonitor
- Git attributes
- signing requirements

A failed or ambiguous candidate stays in its managed worktree for recovery. An unknown repair outcome is answered with [recovery](recovery.md).

The run snapshot holds every state, the selected findings, check results, commit identities, handback status and budget usage. There are no publication commands.

## Handback

`handback` (line client `h`) is a separate action. It fast-forwards the original source branch only when all of these hold:

- The source identity is unchanged.
- The fixer is known to have stopped.
- The candidate lineage is exact.
- The source worktree and index are clean.

A refusal preserves the candidate commit and the source work.

A repaired head is `fixed-not-independently-rereviewed`. Greenhouse never rereviews or publishes it automatically.

New run is blocked while candidate custody is unpublished or ambiguous.

## Resolution responses

A finding repair must end with one response per selected finding, as the fixer's final answer:

```json
{"responses":[{"finding":"<id>","response":"fixed","reason":"rewrote the bounds check"}]}
```

| `response` | Meaning |
| --- | --- |
| `fixed` | The fixer edited the candidate to resolve the finding. |
| `disputed` | The finding is not a real defect; `reason` says why. The fixer leaves its lines unchanged. |
| `deferred` | Postponed under an answered decision. `authority` must name one of the `Authorities` in the fixer input: decisions answered by the terminal, AXI or a YOLO actor, never by `auto`. |

The repair is refused as `fixer-failed`, and nothing is committed, when any of these occur:

- a missing, duplicate or unknown response
- a deferral without a listed authority
- edits without any `fixed` response

When every response is a dispute or deferral and nothing changed, the repair ends `responded-no-change` without a commit.

### How the next review checks responses

In the full pipeline, the next independent review receives the responses, each with the prior finding and any cited decision. It must return `resolutions` with one `confirmed` or `rejected` check per response.

The responses change the review stage's inputs, so a `responded-no-change` repair also triggers that review.

A finding counts as resolved only when the review confirms the response and does not report the finding again:

| Review check | Result |
| --- | --- |
| Confirmed `fixed` | Resolved as `fixed`. |
| Confirmed `disputed` | Resolved as `dispute-upheld`. A dispute never counts as a fix: it creates no commit and no `REPAIRED PASS`. |
| Confirmed `deferred` | Resolved as `deferred`. A ready run reports `ACCEPTED RISK`. |
| Rejected | The finding stays outstanding. |

AXI carries the same data in the repair's `responses`, the review's `result.resolutions` and `run.pipeline.resolutions` (with `kind`).

The terminal lists each finding as `Outstanding`, `Resolved (fixed)`, `Resolved (dispute upheld)` or `Resolved (deferred)`. Below it come `Response <kind>: <reason>` and `Review check <check>: <reason>`.

A one-pass repair shows its responses as `Not independently checked`, because one-pass never rereviews.
