# Embedding contract

Start the full pipeline with an optional `title` request field or `axi pipeline --title`. Supply a conventional-commit title, such as `feat(auth): reject expired tokens`. If you omit the title, Greenhouse derives and persists one from the run intent. The legacy `pull_title` field in trusted `pipeline.json` and `.greenhouse-pipeline.json` remains accepted for compatibility but is ignored; it does not set the PR title.

A host such as Posse can use Greenhouse as its independent reviewer without running a second delivery pipeline. In host mode Greenhouse runs reviews and reports exact-head verdicts. The host keeps every other responsibility.

Host mode needs protocol version 2. See [compatibility](compatibility.md).

## Responsibilities

| Responsibility | Owner in host mode | Greenhouse behavior |
| --- | --- | --- |
| Independent review of the exact head | Greenhouse | Runs one fresh-context review per host grant |
| Rule loading and snapshots | Greenhouse | Same as standalone: `apply`, `resolve-rules` |
| Author repair and conflict repair | Host | Refuses `repair` and `resolve-conflict` |
| Candidate custody | Host | Creates no private worktree, candidate ref or lease; refuses `handback` |
| Push, PR and CI | Host | Refuses `pipeline` |
| Review cap and cap decisions | Host | Ignores the Greenhouse `limit`, `mode` and `preset`; refuses `decide` and `rerun-from-start`; never exhausts the host round count |
| Saved configuration | User | Refuses `save`, `trust` and `migrate` from the host |

Each responsibility has one owner. Greenhouse never pushes, opens a PR, writes the worktree, or creates a reviewer pane in host mode.

## Finding priorities and pipeline decisions

P0 and P1 findings with valid actions are blocking and must be resolved or explicitly accepted before the full pipeline can pass. P2 findings with valid actions are actionable but non-blocking: they remain reported, may be included in a repair round already opened for blocking findings, and do not prevent a clean or passing verdict if left unresolved. P3 findings are informational and do not block or enter repair rounds.

## Blocking finding confirmation

A single review can report a P0 or P1 finding that a second look would not reproduce. Greenhouse therefore confirms each P0 and P1 finding before it blocks, in every review that gates: host reviews and the full pipeline's reviews. P2 and P3 findings are not confirmed.

After the review settles with at least one P0 or P1 finding, Greenhouse runs one fresh confirmation pass per such finding. The pass receives the same evidence and the reported finding, and sits in the same read-only snapshot. It has its own [review budget](#per-review-budget-and-finalization) and adds its tokens and cost to the review's `usage`. A pass reports one of:

| `confirmation.status` | Meaning | Effect |
|---|---|---|
| `confirmed` | The pass reports a P0 or P1 finding on overlapping lines. `evidence` carries its explanation. | The finding keeps its priority and blocks. |
| `demoted` | The pass covers the finding's file and reports a P2 or P3 finding on overlapping lines. | The finding takes that priority and no longer blocks. |
| `dropped` | The pass settles, covers the finding's file, and reports no finding on overlapping lines. | The finding leaves `findings` and is listed in `dismissed`. |
| `unconfirmed` | The pass did not settle (inconclusive, paused or failed to start) or did not cover the finding's file (listed in `Uncovered` or missing from coverage). | The finding keeps its priority and blocks. |

Overlap means the same file and intersecting line ranges. A confirmation pass cannot raise a finding's priority. When every blocking finding is dropped, the verdict becomes `clean`, or `partial` when paths remain uncovered.

Every finding in `findings` that went through a pass carries `confirmation` with `status`, `reported_priority` and `evidence`; `run.review.result` and `host_result` show it. Dropped findings appear in `run.review.result.dismissed` and `host_result.dismissed_findings` with the same object, so a host can audit what Greenhouse discarded. A dropped finding also leaves the run's finding ledger, so it does not block later reviews. If the controller restarts after the review verdict but before its confirmation passes complete, [recovery](recovery.md) adopts the verdict and marks each unconfirmed blocking finding `unconfirmed`; it keeps blocking. A cancelled confirmation pass cancels the review.

## Host identity

Every host request carries `host`:

```json
{"name": "posse", "id": "greenhouse/t69", "metadata": {"worker": "w-3"}}
```

`name` and `id` are required text. `metadata` holds at most 16 string pairs and is stored as given. Greenhouse does not interpret the identity; it does not need a Project, Task or Herdr.

- `attach` with `host` creates a host run for the worktree, or attaches to the run of the same host (same `name` and `id`).
- A worktree with a standalone run refuses a host `attach`. The host takes the worktree over only with an explicit `new` carrying `host`.
- A different host may `attach` to a settled run, then explicitly replace it with `new`. Settled means no running or cancelling execution, scheduled resume, writer activity, candidate custody, recovery requirement, or unknown writer or executor outcome. While any of these remain, both requests retain the ownership refusal.
- Cross-host `attach` only observes the old run. `new` creates a run owned by the requesting host with no inherited usage, findings, grants, or lineage. The old run remains available through `inspect` and is unchanged.
- Other mutations of a host run remain restricted to its owner. The terminal and AXI clients without `host` may only observe it. Observer responses list only `attach` and `inspect` in `actions`, and `blocked` names the owning host.

## Review rounds

1. `attach` with `host`, then `apply` the rules and reviewer model, `resolve-rules`, and `prepare` the intent and target branch or commit.
2. `review` with `grant` set to the host's identity for this review authorization, for example its review round ID. Greenhouse refuses a review without a grant and refuses a grant this run or its predecessors already used. Replaying the same `action_id` returns the recorded receipt.
3. To accept the current settled review head, send `accept-head` with a reason. Greenhouse records the candidate and review ID in `run.host_acceptances` and `host_result.accepted_heads`; only that exact head is accepted.
4. Follow the run with `watch`, then read `host_result`.

`prepare` resolves the requested target to a commit and stores it as `run.target`. The review diff uses `git merge-base <target> <candidate>`, reported as `run.review_base` in `inspect` and `review_base` in `host_result`. Pipeline sync and PR creation keep using `target`. If the histories are unrelated, the diff uses `target` and `review_base_reason` explains the fallback.

Run inspection includes `run.started_at` and `run.stage_started_at` for new runs. `stage_started_at` uses RFC 3339 format and changes whenever `run.stage` changes, including in host mode. Pipeline stages expose their own `started_at` and `finished_at`: an active stage has only `started_at`, and a stage not yet started has neither field. Older stored runs remain readable and omit timestamps that were not recorded.

`host_result` is present once a host run has a review:

```json
{
  "host": {"name": "posse", "id": "greenhouse/t69"},
  "grant": "round-1",
  "review_id": "...",
  "head": "exact reviewed commit",
  "target": "exact target commit",
  "review_base": "merge-base of target and head",
  "status": "outstanding-findings",
  "verdict": "findings",
  "current": true,
  "findings": [{"id": "...", "priority": "P2", "origin": "policy", "file": "a.go", "start": 3, "end": 4, "explanation": "...", "action": "...", "rule": "unslop/plain-prose", "head": "exact reviewed commit"}],
  "rules_digest": "...",
  "config_digest": "...",
  "input_digest": "...",
  "backend": "pi",
  "model": "openai/gpt-5.6-luna",
  "reason": "",
  "diagnostic": "",
  "usage": {
    "input_tokens": 100,
    "output_tokens": 20,
    "cached_input_tokens": 0,
    "tokens": 120,
    "cost_usd": 0.01
  }
}
```

`verdict` is `clean`, `findings`, `inconclusive` or `partial`, and is empty until the review settles. Status and verdict agree: inconclusive has no verdict, clean uses `clean-review`, findings uses `outstanding-findings`, and partial uses `partial-review`. `partial` comes with `status: "partial-review"`; see [generated and uncovered paths](#generated-and-uncovered-paths). `current` is false once the worktree branch or head moved, or the review went stale; a host must not record a verdict whose `current` is false as evidence for any head. Every finding carries the head it was reviewed on. Translate findings by `id`.

For new reviews, the ID is the lowercase SHA-256 hex digest of the JSON array `[origin, file, rule, normalized_code]`. Greenhouse reads lines `start` through `end` (inclusive) from `file` at the reviewed `head`. For each line, it joins `strings.Fields(line)` with one ASCII space, which trims and collapses Unicode whitespace. It drops blank lines and joins the remaining lines with a newline (`\n`). JSON encoding uses Go's `encoding/json.Marshal`. Line numbers, head, explanation, priority, and action are excluded. Shifting lines or rewording explanations preserves the ID; changing normalized code changes it. Findings with the same ID keep the first finding in reviewer order, including its priority and explanation. Stored reviews retain their IDs; executions admitted before this rule retain the legacy identity calculation.

Failed or inconclusive reviews include a classified `reason` and a credential-redacted `diagnostic` excerpt, bounded to the last 40 lines and at most 4 KiB, with `[truncated]` when shortened. These fields are omitted when empty. `usage` reports input, output, cached-input, and total tokens, plus cost in USD. Unknown values stay zero; totals retain each backend's accounting.

A review that exceeds the configured executor limit (`executor_timeout`, default `45m`) keeps `status: "cancelled"` and reports `reason: "timeout"`. Its diagnostic names the executor limit. An explicit cancellation reports `reason: "cancelled"`. Standalone and full-pipeline reviews retain the reason in `run.review.reason`, `host_result.reason`, and `run.attempts[].reason`, together with reported model and usage metadata.

After restart, complete executor records preserve the stop reason and captured usage. Legacy stopped records without a stop reason report `cancelled`.

A backend pause, such as an exhausted quota, keeps the review's grant and gives no verdict. `resume` or `rerun` continues that review under the same grant, so the host spends no second grant. Interrupted reviews use [recovery](recovery.md) as in standalone mode.

## Review input and admission refusals

The reviewer receives a JSON metadata header followed by plain-text `Diff`, `Files`, and optional submodule log and diff sections. Submodule pin metadata remains in the JSON header. The file bodies are not JSON-string escaped: source quotes and backslashes remain as written in the candidate. `Diff` contains the complete admitted diff, and `Paths` contains every changed path, including deletions and gitlinks. Coverage must include every path. [Generated and uncovered paths](#generated-and-uncovered-paths) are the exceptions. `Files` contains candidate content for paths that still exist. Each included line retains its candidate line number.

Small files are numbered before applying the review context window. Larger files use the union of candidate hunk ranges with three extra context lines, clamped to the file. `...` marks omitted lines. Each file receives at most 64 KiB. If their combined content exceeds 768 KiB, Greenhouse uses hunk windows for all files. A file whose windows still exceed 64 KiB becomes an uncovered path. Windows that still exceed the combined limit are refused. A finding's `start` and `end` must remain within one changed hunk, including when the displayed window omits surrounding lines. The read-only snapshot supplies additional context. Candidate blob reads stop at 16 MiB; a blob above that limit or hunk windows that cannot fit are refused.

A gitlink has a synthetic `Files` entry: `1: Subproject commit <new pin>`. Findings on that path use line 1. Finding identities normalize this synthetic content using the same `code-v1` calculation as text. `Submodules` contains `Path`, `Old`, `New`, `Log`, `Diff`, and `Truncated`. Pins come from the review base and candidate trees. Logs and diffs each retain at most 128 KiB; `Truncated` and an omission marker identify shortened ranges. Added or removed gitlinks have no two-pin range.

For two-pin ranges, Greenhouse resolves submodule objects locally through the initialized worktree, its `.git` file, or the common Git directory's `modules/<name>` repository. The name comes from the candidate `.gitmodules`. Added or removed gitlinks need no local submodule objects. Admission never fetches or initializes a submodule.

The complete rendered reviewer input, including the JSON metadata header and plain-text evidence sections, must fit within 2 MiB. Admission refusals retain `outcome: "refused"` and `error`, and add `error_code` and `retryable: false`:

| `error_code` | Meaning |
|---|---|
| `review_input_too_large` | The rendered bundle, a candidate blob, or combined hunk windows exceed a limit. For a complete diff up to 64 MiB, when its size exceeds 1 MiB Greenhouse keeps changed paths whose diffs fit within 1 MiB and lists the rest in `uncovered`. Their reason is `diff_bound`. Generated paths count in short form. A diff above 64 MiB is refused with its limit and cause. A refusal names the limit and underlying cause. Rendered-input errors list byte contributors: `diff`, `files`, `criteria`, `responses`, and `total`, plus the largest available paths. The `diff` and `files` values count their literal plain-text representation, including file headings and numbering. Earlier read or window refusals name the path and limit, and identify sizes not yet measured. The check also reports encoded `submodules` metadata and plain-text range bytes when present. |
| `prepare_failed` | Preparation could not capture or validate its input. The error includes the underlying cause. |
| `unsupported_binary_diff` | A binary patch cannot be reviewed. The error names the path. |
| `submodule_unavailable` | A submodule pin is unavailable locally. The error names the path and both pins. |
| `prompt_exceeds_context` | The prompt does not fit the reviewer model's context window, and omitting another changed path would leave nothing to review. The error names the estimated and allowed tokens. See [model context window](#model-context-window). |

These fields are additive in existing protocol versions. Retrying the same input cannot resolve these refusals.

## Generated and uncovered paths

A path the target commit marks as generated is reviewed in short form. Greenhouse reads the `linguist-generated` attribute from the `.gitattributes` files of the target commit (`run.target`). An optional `greenhouse-generator` attribute names the files that produce it, as comma-separated repository paths:

```gitattributes
internal/store/migrations/*.sql linguist-generated greenhouse-generator=tools/migrate/gen.go,tools/migrate/gen_test.go
```

The candidate's own `.gitattributes`, the worktree and the merge base are not consulted. An attribute that also applies without the target's files, such as one set in `.git/info/attributes` or a global or system attributes file, is ignored. A candidate therefore cannot exempt its own files.

For a generated path, the reviewer gets:

- its diffstat in the `Generated` header entry: `Path`, `Added`, `Removed` and `Generators`;
- its `Diff` section with the hunks replaced by `[generated: hunks omitted; see Generated]`;
- a `Files` excerpt of its hunk windows, at most 4 KiB and ending with `...` when shortened;
- the candidate content of each generator that exists, as a `Files` entry of at most 64 KiB.

The path stays in `Paths`, so coverage must still include it, and the verdict can be `clean`. `run.review.generated` and `host_result.generated` list each generated path with `path`, `added`, `removed` and `generators`.

A changed file whose hunk windows exceed 64 KiB and that is not generated becomes an uncovered path instead of refusing the review. Greenhouse omits its hunks and content, replaces its `Diff` hunks with `[uncovered: hunks omitted; see Uncovered]`, lists it in the `Uncovered` header, and removes it from `Paths`. The other files are reviewed as usual. When uncovered paths exist:

- a reviewer verdict of `clean` becomes `verdict: "partial"` with `status: "partial-review"`;
- a `findings` verdict keeps `status: "outstanding-findings"` when the review finishes within budget;
- `run.review.uncovered` and `host_result.uncovered` list each path with its `reason`. A path excluded because its hunk windows exceed the input limit or because of the prompt limit keeps that reason; when the review budget expires, every otherwise-unreviewed path has reason `review_budget`;
- a well-formed reviewer answer that omits coverage for a path adds that path with reason `not covered by reviewer within budget`. Greenhouse retries those paths while review budget remains. If the budget runs out, remaining paths use reason `review_budget` and the verdict is partial.

When a reviewer omits paths, Greenhouse uses another review-budget unit for a follow-up pass limited to those paths. It merges coverage and findings across passes. This applies to standalone reviews and the full pipeline. If the budget runs out before all paths are covered, Greenhouse settles `partial` and marks remaining skipped paths `review_budget`. Paths excluded for another reason, such as `diff_bound` or `prompt exceeds model context`, keep that reason. Greenhouse does not retry those paths.

### Model context window

Before it calls the reviewer, Greenhouse estimates the rendered prompt at one token per 3 bytes. Real pi reviews measured about 3.4 bytes per token, so the estimate errs high. Half of the model's context window is allowed for the prompt; the other half stays free for exploration and the answer.

The context window comes from the backend:

| Backend | Context window |
|---|---|
| pi | the `context` column of `pi --list-models` for the selected model; 128,000 tokens when pi reports none or no model is selected |
| claude | 200,000 tokens |
| codex | 272,000 tokens |

When the prompt exceeds the allowed size, Greenhouse makes the largest changed paths uncovered, one at a time, until the prompt fits. A path's size is its `Diff` section plus its `Files` content. Generated paths are kept. Each omitted path gets the reason `prompt exceeds model context (N tokens > M)`, where `N` is the estimate for the full prompt and `M` is the allowed size. The review then follows the uncovered rules above. If it still does not fit when the only paths left to review are generated ones or a single path, admission is refused with `prompt_exceeds_context` and the model is never called.

A backend can still overflow its context while it explores. When pi exits non-zero after a `compaction_start` event with reason `overflow`, the review ends `inconclusive` with `reason: "context_overflow"`. A retry of the same input overflows again; the host should not retry it unchanged.

## Per-review budget and finalization

Greenhouse enforces a wall-time budget for each reviewer attempt, including its finalization turn. The default is `3m`. Set `review_budget` in the private per-user `backends.json` to a positive Go duration at most `1h`:

```json
{"version": 2, "review_budget": "3m"}
```

The shorter of this budget and `executor_timeout` applies. Reaching `executor_timeout` reports `timeout`; exhausting the shorter review budget reports `review_budget`. The controller reserves the last 50% for finalization. At that boundary, it stops exploration and resumes the same private native session with a request to return the verdict immediately. Pi and Claude have no read tools during finalization; Codex retains its read-only sandbox and disabled tools. Process termination and reaping can add cleanup time after a deadline.

Each review also has a tool-call budget that scales with the change: 2 plus 1 call per changed path, at least 6 and at most 40. Review input carries it as `ToolCallBudget`. For Pi, the controller counts tool calls in the event stream. At the first turn end after the budget is spent, it stops exploration and runs the finalization turn, and the review log records `tool-call budget N spent`. Claude and Codex receive the budget in their input but are not stopped by it.

A missing or invalid final answer also triggers this turn, using the time left in the same budget. Finalization requests low reasoning effort and carries only the verdict contract, head, and paths; the original evidence remains in the session. There is at most one finalization turn. Authentication, quota, process failures, and model mismatches do not trigger output recovery. If the backend provides no resumable session, the attempt remains inconclusive and its log explains why. Native credentials and settings use private copies with mode `0400`. Claude onboarding state uses a writable private copy. Claude also receives a private `HOME`, so its home-level `.claude.json` resolves to that copy. The private session contains no links to the originals. Session writes use a private directory removed after review. When Landlock is unavailable, these copies prevent writes through session paths from reaching the originals; they do not provide OS isolation against direct access to other paths.

Review input includes `ReviewBudget`, `ToolCallBudget` and `ChangedHunks`, mapping changed paths to inclusive candidate line ranges. `Diff` and numbered `Files` are the starting evidence. `Files` contains changed hunks plus up to 20 context lines on each side, with `[omitted]` between excerpts. The bounded windows described above may supply fewer context lines for large files. Pure deletions in surviving files anchor at the candidate hunk start, with a minimum of line 1. Whole-file deletions retain old-file ranges. Empty-diff repair reviews retain full files. The prompt permits reading dependencies to resolve a concrete question about a changed hunk, and tells the reviewer to stop when that evidence is sufficient.

An out-of-range finding is dropped individually. Valid findings remain. When every finding is dropped and coverage is complete, the verdict becomes `clean`. Other identity, coverage, policy, and model validation still apply. `run.review.result.diagnostics` lists dropped findings; `run.review.diagnostic` and `host_result.diagnostic` expose the bounded, redacted explanation even for conclusive reviews.

AXI `inspect` includes these execution fields:

- `run.review.review_budget`: the duration captured when the attempt was admitted.
- `run.review.finalization`: `true` when the controller ran the finalization turn; omitted otherwise.
- `run.review.usage`: tokens and cost from exploration plus finalization.
- `run.attempts[].tokens` and `run.attempts[].usage`: the same aggregate for each attempt, including full-pipeline reviews.
- `run.review.reason`: `review_budget` only when no usable verdict is available at budget expiry. A complete result keeps its `clean` or `findings` verdict; an incomplete result settles as `partial` with uncovered paths marked `review_budget`.

The budget bounds wall time, not an exact token count. Tokens are the totals reported by the native backend; an interrupted request may not report usage. Cache accounting retains backend semantics. Unknown usage remains zero.

Reviews saved before budgets were introduced remain loadable without rewriting their evidence. When `review_budget` is absent, durable validation also accepts the exact historical reviewer prompt arguments. Other arguments and evidence digests remain validated. A resumed attempt without a stored budget uses the default `3m`.

## Responses to earlier findings

A host `review` request accepts an optional `responses` array. Responses attach atomically to that review and its grant. There is no separate response operation or authorization. Omitting `responses`, or sending an empty array, starts a review without responses. Greenhouse does not carry responses into a later grant automatically. A paused review retains its responses on resume or rerun.

Each response has:

- `finding`: an ID from the latest settled review's findings in this run.
- `response`: `fixed` or `disputed`.
- `reason`: required for `disputed`; optional for `fixed`. A non-empty reason must contain non-whitespace text, valid UTF-8, at most 8,192 bytes, and no control characters except tabs and newlines.

A host may respond to a subset of the findings. A successor created with `new` by the same host carries the latest settled findings forward. During `prepare`, Greenhouse compares each carried finding's file and line range with changed hunks in the current diff from `merge-base(target, candidate)` to the candidate. A finding remains outstanding only when that range is still in a changed hunk; otherwise it is dropped. A new review can raise it again. `inspect.run.host_carried_findings` and `host_result.carried_findings` report each carried finding as kept or dropped with a reason. A completed review replaces the carried set, including when its findings array is empty. A failed, cancelled, stale, or inconclusive review does not replace it.

Greenhouse refuses unknown finding IDs, duplicate IDs, other response values, invalid reasons, and any non-empty `authority` value. Deferral remains a standalone decision operation. Responses require protocol 2 and the owning host's `review` request. Standalone clients, other hosts, and other operations cannot submit responses. Validation failure consumes no grant and leaves the run token unchanged.

Replaying the identical request with the same `action_id` returns its recorded receipt, including after the review settles. Reusing a retained action ID with different input is refused. Each new round still needs an unused grant and current run, input, gate, and token values.

Receipts and settled runs outside current worktree slots are archived separately from `state.json`. Each archive retains at most 128 records and 64 MiB, pruning the oldest archived records first. Archived runs and receipts are readable only when listed in the committed state manifest. Replay is guaranteed only while the receipt is retained. Inspecting a missing historical run reports that its data may have been pruned. Slot owners, active executions, recovery, candidate custody, and repository trust remain in the hot state.

Inspect and transcript remain available during a durable write. Watch and model catalog requests receive a retry refusal during a write. Archive pruning starts only after the replacement state is durably committed. Mutations wait for the write to finish before execution. Large legacy stores migrate before the daemon accepts requests; startup allows up to 60 seconds.

Complete request:

```json
{
  "version": 2,
  "op": "review",
  "run_id": "<latest run_id>",
  "input_id": "<latest input_id>",
  "gate_id": "<latest gate_id>",
  "expected": "<latest token>",
  "action_id": "review-round-2",
  "reason": "Review the author responses",
  "host": {"name": "posse", "id": "greenhouse/t69"},
  "grant": "round-2",
  "responses": [
    {
      "finding": "<first finding id from the latest settled review>",
      "response": "fixed",
      "reason": "The author corrected the bounds check"
    },
    {
      "finding": "<second finding id from the latest settled review>",
      "response": "disputed",
      "reason": "The caller already rejects empty input"
    }
  ]
}
```

The reviewer receives each response with its prior finding through the same `Responses` input and resolution contract used after standalone repairs. Every host rereview also receives earlier findings on overlapping code through `PriorFindings`, even without host responses. Each item reports whether it is open, fixed by a later commit, fixed, or disputed. A blocking finding that conflicts with an earlier finding appears under `conflicts` in the review and `host_result`. The finding does not remain outstanding. After the host accepts a review head with `accept-head`, rereviews compare finding lines with each accepted head. They dismiss findings on byte-identical lines unless the reviewer marks `target_introduced`. `host_result.dismissal_reasons` explains these dismissals as out of scope and unchanged since the accepted head. Greenhouse does not edit the worktree or require an uncommitted edit for a host's `fixed` claim. The reviewer checks the committed candidate independently.

A settled `host_result` includes `resolutions` when the review checked responses:

```json
{
  "resolutions": [
    {"finding": "<first finding id>", "check": "confirmed", "reason": "The candidate now checks the bound"},
    {"finding": "<second finding id>", "check": "rejected", "reason": "The caller accepts empty input"}
  ]
}
```

Each response requires exactly one reviewer check. For `fixed`, `confirmed` means the candidate resolves the finding. For `disputed`, `confirmed` means the prior finding was not a defect. `rejected` rejects the author's claim. A defect that remains must also appear in `findings`. Missing, duplicate, or unknown checks make the review inconclusive. Hosts must still read `verdict` and `current`; responses do not change verdict semantics.

## Request examples

Each request block below is a complete JSON request to `greenhouse axi`, invoked from the candidate worktree. AXI reads one request per invocation. It writes one response envelope on stdout and replaces any request `path` with the invoking directory.

The response blocks show selected fields and collection entries; other response data is omitted. Angle-bracket values are placeholders. Each mutation copies `run_id`, `input_id`, and `gate_id` from the latest response, and copies its `token` into `expected`. An `outcome` of `ok` confirms the operation, not a clean review. A refused or failed operation stops the sequence.

### Attach or start a successor

`attach` creates a host run if the worktree has no active run, or reattaches to a run with the same host identity:

```json
{
  "version": 2,
  "op": "attach",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  }
}
```

Response fields:

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<token>",
  "run": {
    "host": {
      "name": "posse",
      "id": "greenhouse/t69"
    }
  }
}
```

After the host commits an author repair, `new` captures the changed head. It also allows explicit takeover of a standalone run or another host's settled run. Copy the current envelope fields from `attach` or `inspect`:

```json
{
  "version": 2,
  "op": "new",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-new-2"
}
```

The response has a new `run_id`, `input_id`, and `token`; `run.predecessor` identifies the previous run only when both runs belong to the same host. The host repeats the remaining operations for the successor with new action IDs.

### Apply settings

`apply` selects rules and the reviewer without saving user defaults. It requires a complete settings object and a reason. An empty model selects the native agent's default model. Host mode ignores the standalone budget settings; they remain required settings fields.

```json
{
  "version": 2,
  "op": "apply",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-apply-1",
  "reason": "Review the host candidate with the selected rules",
  "settings": {
    "mode": "ask",
    "preset": "one-pass",
    "limit": 3,
    "rules": [
      "unslop",
      "no-comments"
    ],
    "reviewer_backend": "pi",
    "reviewer_model": "",
    "fixer_backend": "pi",
    "fixer_model": ""
  }
}
```

The response includes the refreshed envelope fields and a new entry in `run.configs` containing the applied `settings`. Applying rules selects their names; it does not load their bytes.

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<new token>",
  "run": {
    "configs": [
      {
        "id": "<snapshot>",
        "settings": {
          "mode": "ask",
          "preset": "one-pass",
          "limit": 3,
          "rules": [
            "unslop",
            "no-comments"
          ],
          "reviewer_backend": "pi",
          "reviewer_model": "",
          "fixer_backend": "pi",
          "fixer_model": ""
        }
      }
    ]
  }
}
```

### Resolve rules

`resolve-rules` loads and snapshots the selected rule bytes:

```json
{
  "version": 2,
  "op": "resolve-rules",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-rules-1"
}
```

The response includes the refreshed envelope fields, `rules`, `rules_ready`, and `run.rule_snapshots`. `rules_ready: true` means the selected rules loaded successfully; inspect rule errors before preparing a review.

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<new token>",
  "rules_ready": true,
  "rules": [
    {
      "selection": "unslop"
    },
    {
      "selection": "no-comments"
    }
  ],
  "run": {
    "rule_snapshots": [
      {
        "id": "<snapshot>"
      }
    ]
  }
}
```

### Prepare the review input

`intent` states what the change should do. `target` accepts a branch name or commit available in the local repository. The example uses a full, exact commit ID; replace the SHA with your target commit. Greenhouse records the resolved commit and reviews the diff from its merge-base with the candidate to the candidate. The recorded target remains that exact commit, even when the merge-base differs.

```json
{
  "version": 2,
  "op": "prepare",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-prepare-1",
  "intent": "Reject invalid request fields before starting a review",
  "target": "0123456789abcdef0123456789abcdef01234567"
}
```

The response includes the refreshed envelope fields and the retained `run.intent`, `run.target`, and `run.candidate`. Intents longer than 8192 bytes are refused with `error_code: "intent_too_long"`, `retryable: false`, and an error naming the byte limit. The limit remains 8 KiB as a bounded-input limit, not as a proxy for model context. Issue 61 tracks token-based prompt estimation as the appropriate guard against prompt overflow; this limit is not raised in anticipation of that separate check.

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<new token>",
  "run": {
    "intent": "Reject invalid request fields before starting a review",
    "target": "0123456789abcdef0123456789abcdef01234567",
    "candidate": "<candidate commit>"
  }
}
```

### Dispatch one review

`grant` identifies the host's authorization for this round. Each new round needs an unused grant, including rounds on a successor. Replaying the identical request with the same `action_id` returns its receipt; it does not spend another grant.

```json
{
  "version": 2,
  "op": "review",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-review-1",
  "grant": "round-1"
}
```

Response fields at dispatch:

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<token>",
  "host_result": {
    "grant": "round-1",
    "head": "<candidate commit>",
    "target": "<target commit>",
    "status": "running",
    "current": true
  }
}
```

`watch` follows progress; `attach` or `inspect` reads the latest envelope. The settled `host_result` has the shape shown in [review rounds](#review-rounds). Accept a verdict only while `current` is true and `head` matches the host's candidate commit.

## After an author repair

The host's writer commits the repair in the worktree. The run then shows the moved source and `current: false`. The host calls `new` with `host` to open a successor for the new head. The successor:

- links `predecessor` and the old run's `successor`;
- keeps `review_used` and `repair_used` and records them in `inherited`, so usage never resets;
- keeps the used grants, so a spent grant cannot buy another review.

Then the host repeats the review round with a new grant.

## Verification

`TestEmbeddedHostOwnsDelivery` drives a fake host through the daemon, AXI and terminal with an isolated HOME, real Git and a fake reviewer. It proves two grants buy exactly two reviewer invocations and no fixer, a Greenhouse `limit` of 1 does not stop the host's second round, the successor keeps usage, and no candidate ref, private worktree, lease or remote ref appears.

`TestHostReusesSettledWorktree` proves host A can finish a review and host B can attach, create an independent run, and review the same worktree without changing host A's run. `TestCrossHostReplacementSafety` covers refusal during active execution, custody, and unknown outcomes. `TestHostOwnershipBoundaries` covers standalone takeover and a second host, and `TestHostReviewResumesUnderItsGrant` covers a paused review that resumes under its grant.

`TestReviewPromptThatFitsTheContextWindow`, `TestReviewPromptOverTheContextWindowIsPartial`, `TestReviewPromptThatCannotFitIsRefused` and `TestReviewBackendContextOverflowIsTyped` drive a fake pi that declares a small context window through the CLI: a prompt that fits is reviewed in full, a larger one becomes `partial-review`, one that cannot fit is refused before the model runs, and a pi overflow exit reports `context_overflow`.

`TestReviewTrustedGeneratedFile`, `TestReviewUndeclaredLargeFileIsPartial` and `TestReviewCandidateCannotDeclareItsOwnFileGenerated` drive 300 KB files through the CLI: a target-declared generated file gets a clean verdict, an undeclared one gives `partial-review`, and a declaration in the candidate's `.gitattributes` or `.git/info/attributes` gets no exemption.

`TestHostFindingIdentityAcrossRounds` exercises two host review rounds through the daemon with real Git and a fake reviewer. It checks that shifted code and reworded explanations preserve the ID, while edited code changes it.

## Stop the daemon

Run `greenhouse stop` with the daemon's `GREENHOUSE_HOME`, or send an AXI request:

```sh
printf '%s\n' '{"version":2,"op":"stop"}' | greenhouse axi
```

This administrative operation applies to the entire state home and needs no run token, host identity, or grant.
The daemon refuses with `outcome: "refused"` and `error: "daemon is busy; let running work settle before stopping"` while work runs.
Let work settle and retry. A refusal leaves running children untouched.
On success, `outcome` is `ok` and `explanation` is `daemon stopped`.
If no daemon is running, `explanation` is `daemon is not running`.
Neither form starts a daemon. The client waits up to five seconds for shutdown to release the state-home lock.
Shutdown removes the `pid` file. Restart never signals stored PIDs; SIGTERM handling remains available.

If the daemon predates `stop`, the CLI exits non-zero and AXI returns `outcome: "refused"` with manual shutdown instructions.
Let its work settle, then stop it with SIGTERM to the PID in `GREENHOUSE_HOME/pid` after confirming that process is the Greenhouse daemon.
Neither client signals a legacy process or removes its PID file.
