status: done

# Standalone review pipeline lock card

- Goal: Provide a standalone pipeline tool invoked by its bare command, with one compact TUI, background independent reviewers, and a machine-readable AXI interface; Posse can consume the same tool without duplicating delivery ownership.
- User confirmation: "ok" accepted per-user defaults shared across repositories, with explicit per-repository overrides. Earlier discussion and verbatim requests are in `review-tool.md` in this directory.
- Do: Bare invocation attaches to an existing run for the resolved working context or starts a run from saved settings when none exists. Concurrent launches cannot create duplicate runs. Attaching does not answer gates, retry work, or reset budget.
- Do: Display stage progress, blocked reasons, findings, priority, round usage, actual loaded rules, effective settings, and their sources. No visible reviewer pane or reviewer transcript is required in normal use.
- Do: Support equivalent human and AXI inspection/control over one authoritative durable run state. Reject stale or duplicate mutations using current run/gate/candidate identity. A disconnected client can reattach without creating work.
- Do: Allow in-TUI edits to be applied to the current run without changing defaults. An explicit Save action persists selected settings for future runs, using per-user defaults or explicit trusted repository overrides. Agents do not need to read the TUI.
- Do: Configure review criteria such as unslop and no-comments, with clear load failures, exact content provenance, and matching repair instructions. Do not execute author workflow skills as reviewer procedures. Preserve touched-hunk scope and required comment exceptions.
- Do: Configure a one-pass review versus a full pipeline and the review budget through the TUI/configuration, not mandatory flags. Preserve consumed budget when resuming or explicitly increasing a limit.
- Do: Support ask, auto, and yolo. Ask groups actionable findings at a stage. Auto repairs within approved intent and authority, escalating changes to intent/scope/acceptance or unresolved findings at cap. YOLO delegates supported operational and cap choices without granting unrelated User-only authority.
- Do: Preserve accept-head, final-fix, rethink, and discard semantics with audit provenance. Acceptance binds the reviewed head and does not satisfy tests/docs/lint/CI. Final-fix is bounded repair and independent final Review, not acceptance; rethink preserves prior evidence.
- Do: Standalone full pipeline reaches push, PR creation, and CI-ready, without automatic merge. Embedded use obeys Posse delivery, author ownership, candidate custody, and review-cap authority instead of starting a second competing pipeline.
- Hard constraint: Fresh reviewer context is separate from the author and fixer. Reviewers do not modify candidates; repairs have one authorized writer. Evidence and decisions bind exact source and rule/configuration inputs.
- Hard constraint: Configuration edits do not rewrite old verdicts or silently alter a running agent's instructions. Budget changes retain usage and provenance. A clean pass, repaired pass, and accepted risk remain distinguishable.
- Hard constraint: A published Posse branch is not rebased. Target synchronization follows the embedding execution contract rather than copying no-mistakes' rebase or force-push behavior.
- Done when: A user types the bare command, follows a run and findings in one pane, changes saved rules/limits through Settings, and sees those settings on the next run without mandatory flags.
- Done when: TUI and AXI observe the same findings/gates and an authorized response from either updates the same run; reconnect, concurrent launch, cancellation, and retry do not duplicate work or lose custody.
- Done when: Tests demonstrate one-pass behavior, cap exhaustion, exact-head acceptance, final repair/final Review, rethink, configuration snapshots, and stale-action refusal; provider-backed development evaluations assess rule compliance separately from deterministic prompt-delivery tests.
- Done when: Standalone delivery stops at CI-ready without merging, and Posse integration retains its author/PR custody and validation gates without extra visible reviewer panes or duplicate publication.
- User-confirmed default: Auto is the initial automation mode. Neither behavior nor this lock card changes live configuration.
- User clarification after confirmation: "à nhưng 1 lượt review trả ra finding thì vẫn có thể quyêt định  lựa chọn fix 1 lần mà đúng không, xong cũng đẩy lên commit".
- Clarified one-pass proposal: Run one independent review, report findings, and allow one explicitly authorized fix of selected findings with checks and a local commit. Do not automatically rereview, push, or publish. Record the result as fixed but not independently rereviewed, not a clean review pass.
- Budget distinction: One-pass limits reviewer invocations; an optional repair is separately accounted for within the allowed combined budget. It is not equivalent to a total review-cap of one, and cannot bypass an embedded Posse cap Decision.
- Confirmed scope constraint: Normal review-cap accounting remains explicit and compatible with Posse's combined Review-verdict and repair usage. Special final-fix grants remain separate audited allowances, not silent counter resets.
- Not-do: No quota-aware scheduler, account switching, generic plugin marketplace, arbitrary skill-code execution, shared author/reviewer conversation, replacement Herdr, or wholesale adoption of no-mistakes policy.
- Cut: Product name selection, exact config/AXI syntax, and low-level module/package choices remain for naming/specification; `review-tool` is not the final name. No automatic merge or unconditional approval of user-only decisions.
- User confirmation of the scope read-back, verbatim: "ok rõ rồi đấy".
- Stage boundary: Scope is locked. This card is not a Worker Brief or implementation approval. Offer posse-spec next; do not start the spec, code, another Project, or extraction automatically.


---

status: done

# Greenhouse specification

Approval state: accepted for local transfer to the Greenhouse Lead. The lock card above is accepted. Area A and decision D1 option B are approved. Area B is approved with the User's clarification: apply TUI edits to the current run, and save defaults for future runs only through explicit Save. Area C was approved with "oke" after the rule/reviewer read-back. Area D was approved with "ok" after the pipeline, modes, and cap read-back. Its detailed nine-stage execution contract, including deterministic commands versus agent work, was approved with a further "ok". Area E and decision D2 option B were approved with "ok" after the private-worktree and guarded handback read-back. Area F and separate-repository placement were approved with "đồng ý các quyêt định trên, về phần tên thì chưa ok". The proposed name proofrun was not approved. The User wants a soft, green, cared-for identity suggesting verified output and green CI, rather than a technical or industrial testing term. Their clarification, verbatim: "Nghe nó xanh, cảm giác thứ đi ra là 1 thứ được kiểm chứng, ci xanh, chắt chiu, nghe những cái từ techinical kia thì hơi thô". The User selected Greenhouse and requested a separate folder and Lead bootstrap package: "ok Green house đi, chắc là tạo 1 repo mới nhỉ, bạn tạo 1 folder rồi dựng ô lead posse rồi delegate brief  qua cho mình được không". All functional areas are approved; public field spelling, implementation details, and baseline defaults remain for the Greenhouse Lead to resolve within this spec. No GitHub issue, remote repository, release, or implementation has been created by this approval. No implementation, publication, or repository initialization is authorized by this draft.

## Problem

Review currently requires visible reviewer Workers even when the User only needs stage progress and findings. The User also wants to invoke the same pipeline outside Posse, configure durable review rules and budgets through the TUI, and let an agent drive it through AXI without screen scraping.

## Solution

Provide a standalone local tool whose bare command opens one compact TUI and attaches to existing work or starts from trusted saved defaults. Independent reviewers run headlessly; TUI and AXI operate on the same durable run, while an embedding host such as Posse retains its own authorship, budget authority, and delivery gates.

Full standalone execution stops at CI-ready without merging. A one-pass review can produce findings, accept one explicitly authorized repair and local commit, and stop without claiming the repaired head has passed independent review.

## Changes by area

### A. Run identity, controller, and reattachment

- A run has a stable opaque ID, an immutable submitted head and target, an evolving candidate head, effective configuration snapshots, a finding ledger, budget usage, and durable actions/events.
- Resolve the invocation path to canonical repository and worktree identities. Folder aliases and symlinks resolve to the same active-run slot. Different Git worktrees remain distinct contexts; shared refs still require writer-custody checks.
- Active-run lookup and creation are atomic. Two clients opening one context attach to the same run and cannot schedule duplicate review or repair.
- Bare invocation attaches to a running, parked, failed, cancelled, or completed run with retained work for that context. Starting a replacement or retrying work is explicit; completed runs do not cause endless attachment without a visible New run action.
- A branch/head mismatch is visible and blocks advancing against stale assumptions. Merely reopening the tool never approves a gate, resets a budget, rewrites findings, or begins another pipeline.
- Client disconnect does not cancel the executor. Cancellation is explicit, stops verified executor processes, and preserves unpublished commits and evidence. Restart inspects persisted state and worker identity before resuming; ambiguous writers or outcomes block safely.
- Controller transport and durable ownership are decision D1 below. Neither the TUI nor AXI may maintain a second authoritative pipeline state.

### B. TUI and persistent settings

- The bare command is the normal human entry point. Mandatory flags, reviewer panes, and reading agent transcripts are not part of normal use.
- Show the current stage, status, blocked reason, findings with priority, review/repair usage, effective mode, selected rules, and loaded-rule status. Details and logs remain accessible within the same TUI.
- Distinguish a clean review pass, a repaired head not yet reviewed, accepted review risk, failed checks, and CI-ready. Never use a clean-pass indicator for accepted or unreviewed work.
- Settings can be edited and applied to the current run without saving defaults. Explicit Save persists selected values for subsequent runs; without Save, later new runs use the previously saved defaults. The UI identifies draft values, effective current-run values, their sources, and the save destination.
- User clarification, verbatim: "tức là có thể chỉnh trực tiếp trong TUI đó để chạy luôn, và có thể chọn save để lưu vào các run sau, con không ấn thì vẫn dựa vào config cũ".
- Applied run-local settings remain in the durable run snapshot for reattachment even without Save; not saving defaults does not revert an existing run on reconnect. Editing a field alone does not authorize a pipeline action.
- Never trust a proposed branch's configuration to expand executable or approval authority. Initial repository guidance requires trust before use; later instruction snapshots identify the trusted revision.
- Saving defaults and applying settings are distinct actions. One can apply and optionally save through the same UI interaction. Rule changes take effect at the next eligible review with a new snapshot, never rewrite completed verdicts, and never silently alter instructions in flight.
- A permitted cap increase records old/new limits, actor, reason, and prior usage. A reduction cannot erase usage or make already consumed work disappear. Embedded caps remain host-authorized.

### C. Rule loading and agent roles

- Named rule packs resolve to versioned local declarative criteria. Settings allows selecting packs, adding a local file, and inspecting the effective content. Initial packs cover unslop and no-comments; additional local packs use the same contract without arbitrary script execution or a plugin marketplace.
- Packs distinguish reviewer criteria from repair guidance. They apply to the selected diff, preserve User quotations and identifiers, and retain necessary license, directive, generated marker, external-quirk, and required documentation comments.
- Loading reports configured, resolved, delivered, or failed status with content digests. A named pack is not considered delivered merely because its name appears in a prompt. A missing required pack blocks the stage.
- Reviewer and fixer receive consistent applicable criteria. Reviewer context is fresh and cannot inherit the author's conversation; fixes use an authorized writer, not the reviewer. Review access is read-only through the executor's actual controls, not an unsupported claim of OS confinement.
- Findings carry stable IDs, priority, rule origin, file/range, explanation, actionability, and the reviewed head. Repeated root causes retain identity across rounds; omission from a later response does not by itself resolve a finding.
- Correctness and configured policy violations are distinguishable. Advisory prose preferences do not silently become correctness blockers. Required rules remain required unless an authorized decision explicitly accepts their risk; the UI exposes that distinction.
- Record the model, backend, input/config digests, verdict, and evidence. Prompt-contract tests establish delivery, not model compliance.

### D. Pipeline, modes, and bounded convergence

- Full stages cover intent, target synchronization, independent review, test, documentation, lint, push, PR, and CI. The User requested explicit execution semantics: "còn 1 cái mình chưa thấy là các step của review là gì, nó phải có các bước lint và chạy test, sủa doc giốn no-mistake chứ, và khôg phải bucớ nào cũng dùng agent". The detailed stage contract below was approved by the User with "ok" after its read-back.
- Intent: Load the submitted goal, scope, acceptance criteria, and trusted command plan. Use deterministic loading when these exist. Ask for missing product intent rather than silently invent it; invoke an agent only for an explicitly needed assessment.
- Target synchronization: Use Git through the command runner. A conflict becomes a gated author/fixer action, not permission for an independent reviewer to write. Never rebase published Posse history.
- Review: Run an independent fresh-context review agent against the exact candidate and selected rule snapshots; return structured findings and coverage.
- Test: Execute trusted configured test commands through the command runner, capturing exit code, bounded logs, command identity, and candidate/config inputs. An agent is not required to launch tests or interpret a successful exit. Failures enter a separately bounded diagnosis/repair path only when authorized, followed by rerunning affected commands. Missing required test commands are unconfigured, not a pass.
- Documentation: Assess documentation impact against changed public behavior and the applicable rule set. When documentation review or repair is needed, use an author/fixer agent; it may inspect or update docs and commit within writer custody. Run configured doc build/link checks as deterministic commands. A verified not-applicable result needs a reason; passing a doc build alone does not prove changed behavior is documented.
- Lint: Run trusted configured format/lint/static-check commands directly. Configured safe mechanical autofixes use the same writer/custody checks; other repairs use an authorized fixer. Run the checks again after any fix.
- Push: Use the command runner to publish only the authorized validated lineage, without a free-form agent push or silent rewrite.
- PR: Use a forge adapter to create or update the existing PR idempotently. An agent may draft text from evidence, but the adapter owns the actual mutation and publication checks.
- CI: Use the forge adapter to inspect exact-head checks. An authorized bounded failure diagnosis may invoke an agent, but polling and successful check evaluation do not require one. Missing checks or unknown results must not become a clean pass unless the trusted plan explicitly declares no CI.
- The command runner and agent runner report through the same stage/event contract. Only one writer owns repairs. Each stage declares its input identities and consumed budget; diagnostics and fixes remain visible at the stage that failed.
- Any test, doc, lint, or mechanical repair that changes code creates a new candidate identity. Earlier review cannot certify that new head. Before full delivery, rerun independent review and the other gates whose declared inputs changed; do not retain a pass by merely changing the displayed head. One-pass with an optional fix remains explicitly unreviewed after repair and does not auto-enter this full convergence flow.
- Mark genuinely inapplicable steps as not applicable with a reason rather than claiming a pass. Gate eligibility and current evidence, not the visual stage order alone, determine whether publication may proceed.
- Ask mode groups actionable findings at the current stage for authorization. Informational findings remain visible without creating approval questions.
- Auto, the initial default, repairs within approved intent and authority. A change to product intent, scope, acceptance criteria, credentials, executable permissions, or destructive effects requires the appropriate User decision; a round budget cannot grant it.
- YOLO delegates supported operational and review-cap decisions to its configured authorized actor. It does not grant arbitrary User-only authority. Record actual actor/reason, never fabricate a User quote.
- Normal review-cap accounting includes Review verdict rounds and repair rounds, displayed separately and as total usage. Cap is a ceiling, not a target. Test/lint/CI retries are bounded and shown separately, not hidden in an unlimited repair loop.
- At review-cap exhaustion, expose accept-head, final-fix, rethink, and discard according to authority. Accept-head accepts review risk only on the exact candidate/configuration input; it does not satisfy failed tests, documentation, lint, or CI.
- Final-fix provides one explicit repair allowance and one independent final Review. With no repairable findings, it allows the final Review directly. Reapplication is idempotent and grants survive restart; it never implies acceptance.
- Rethink preserves the old run and findings and creates an explicitly linked successor. It does not clear history or create a new run implicitly on attach. Discard preserves evidence and requires applicable approval; it is not an automatic deletion of unpublished branches or commits.
- One-pass is a review preset, not a combined cap of one. It schedules one independent reviewer invocation, permits one selected repair only after explicit authorization, runs appropriate checks, and creates a local commit. It does not automatically rereview, push, create a PR, or continue the full pipeline. Its budget reserves that optional repair without bypassing host limits.

### E. Standalone custody and delivery

- Initial support is Git repositories. Refuse unsupported or ambiguous execution contexts without initializing Git or silently moving the User's work.
- Capture the intended diff and committed source identity. Pre-existing unrelated changes must not be included, discarded, stashed, or overwritten automatically. Custody of repairs and return of local commits is decision D2 below.
- Only one authorized writer may change a candidate/ref. A head change invalidates evidence unless an explicit, mechanically proven carry is recorded. Rule/configuration changes likewise cannot inherit a verdict silently.
- Full standalone execution pushes the authorized reviewed lineage, creates or updates its PR, and observes exact-head CI to CI-ready. It never merges. Do not treat unrelated green checks or checks for a previous head as current evidence.
- Failure, cancellation, or controller restart preserves unpublished fixes and provides an explicit recovery action. Reopening is observational until that action is authorized.

### F. AXI and Posse integration

- AXI provides home/status, findings/log inspection, attach/start, selected-finding response, configuration inspection/editing, cancellation, and explicit recovery/new-run actions. Exact command spelling is deferred until naming.
- Machine output has a versioned structured envelope with state, run/candidate/gate identity, outcome, errors, and supported next actions. Progress output must not corrupt machine output.
- Mutations carry stable action identities and current expected run/gate/input tokens. Repeated submissions return the recorded result; stale responses do not approve a different gate. After interrupted responses, clients inspect durable results rather than blindly resubmitting.
- Events notify clients; authoritative snapshots recover reconnects and event gaps. A client can inspect a parked run without authorizing it. A bounded wait ending is not cancellation or failure.
- Posse remains authoritative for Task/PR ownership, candidate custody, effective cap grants, and decision authority. The adapter translates tool findings/verdicts into exact-head evidence without inventing a second budget or accepting stale input.
- In embedded mode, Posse's Change Worker performs repairs under its existing writer contract. Tool-owned delivery steps do not run alongside Posse-owned publication/CI stewardship; one execution owner is selected explicitly per responsibility.
- Initially, standalone use does not require Posse, Herdr, a Project, or Task IDs. The integration may supply host identity as metadata, not as a standalone prerequisite.

## Decisions

### D1. Controller and public machine protocol: option B approved

- Option A: Each run owns a background controller and local endpoint, discovered through an atomic durable registry. TUI and AXI attach directly. This localizes failure to one run but requires robust endpoint discovery, stale-process recovery, and cross-run ref locking.
- Option B: One per-user local controller owns run registration/execution and persistence; TUI and AXI are clients. This centralizes atomic attachment, reconnect, and writer coordination but requires controller startup, version compatibility, and recovery without losing all active work.
- Recommendation: B, because durable concurrent attachment and shared writer coordination are explicit requirements. No network exposure or cloud control plane. Reinspection of installed no-mistakes v1.84.0 source confirms this architecture: EnsureDaemon, local IPC, shared run lookup, and a singleton lock per storage home.
- Source-confirmed caveat: no-mistakes bare invocation first matches an active run to repository/current branch, and some detached/default-branch situations route to a setup wizard. Its behavior does not prove the stricter desired attach-only treatment of retained work or stale input. Keep those explicit tool requirements and test them.
- Runtime requirements for B: Acquire controller ownership before recovery or socket binding; never replace a live listener. Client disconnect must not cancel work. Recover only provable durable states and retain ambiguous runs for explicit recovery. Source reference: no-mistakes revision 4822244e763fd4c3d7f48be6433f92df10bb5b33, internal/cli/attach.go, internal/daemon/daemon.go, and internal/ipc/transport_unix.go. The User approved option B with "ok" after the runtime read-back.
- The selected protocol must be versioned and give TUI and AXI identical authority checks. Storage details and exact wire fields remain unapproved until this decision is made.

### D2. Standalone repair custody: option B approved

- Option A: Repair in the invoking worktree under an explicit writer lease and external-head checks. Local commits appear immediately, but the User and other tools must stop editing; unrelated dirty changes block repair.
- Option B: Repair in a managed private worktree and return commits through an explicit guarded handback. Source and evidence remain stable, but returning a commit must refuse divergence and preserve recovery refs.
- Selected: B, approved by the User with "ok". Both full and one-pass fixes use a managed private worktree. One-pass returns the fix commit through guarded handback only if the source branch and working context still match the authorized state. Never automatically stash, reset, rebase, or delete source changes to force success. A refusal preserves the commit and evidence as awaiting handback, not completed delivery. Embedded Posse always uses host custody, not an additional nested private branch.

### Other recorded decisions

- Standalone full mode ends at CI-ready, without automatic merge.
- Human invocation requires only the product command. TUI changes can apply to one run; explicit Save updates per-user defaults or trusted repository overrides for subsequent runs.
- Independent reviewer context, exact-head evidence, explicit cap outcomes, and truthful accepted-risk states are mandatory.
- Public configuration and rule-pack schemas must be versioned and migratable. Do not silently reinterpret a budget unit or execute text instructions as code.
- Selected placement: A separate product repository, with Posse retaining its integration adapter. The User approved this placement and AXI/integration with "đồng ý các quyêt định trên, về phần tên thì chưa ok". Product name: Greenhouse; local folder: greenhouse. proofrun was not approved. This Project's Lead will not start/adopt the new Project; product implementation must occur in that repository's own workspace and Lead.

## Testing

- Exercise the real command/controller/client interface with isolated HOME, Git repositories, fake agent backends, and forge fixtures. Prove concurrent bare launches create one run; aliases attach; different worktrees do not conflate; reopen never executes work.
- Drive a gate through AXI and observe the same state in the TUI model. Test duplicate/stale actions, event loss, client disconnect, crash-after-commit, writer replacement, and branch movement. No production daemon or receipt mutation.
- Execute one-pass with a fake reviewer/fixer: one review, selected authorized fix, check and commit, no second review or publication, and an unreviewed-fixed result. Prove unrelated dirty files survive.
- Exercise ask/auto/YOLO, changed intent refusal, normal cap exhaustion, permitted cap increases with retained usage, exact-head acceptance, final-fix idempotency and snapshot recovery, and rethink inheritance.
- Test that unsaved applied settings affect the current run and survive reattachment, while subsequent new runs retain old defaults; explicit Save updates the selected default scope. Also test repository trust, missing packs, resolved/delivered rule digests, immutable round snapshots, and consistent reviewer/fixer prompts at the emitted instruction interface. Behavioral evaluations for prose/comment rules are development-only, with fixed diffs and recorded model versions.
- Test real executable TUI sizing and stage/finding/settings interactions in an isolated terminal. Do not substitute grepping UI source for interaction evidence.
- Exercise full standalone delivery against a forge fixture through CI-ready without merge; exercise embedded Posse delivery without duplicate writer, publication, cap reset, or reviewer pane. Later changes rerun the gates required by their actual input/configuration changes.
- With instrumented fake command and agent backends, prove passing tests/lint and Git/forge actions do not dispatch agents. Prove configured commands actually execute with captured exit codes, missing commands cannot pass, failure fixes are bounded, and doc/lint changes invalidate earlier candidate evidence. Prove doc-impact assessment is not replaced by a successful build and that one-pass never enters automatic full-pipeline revalidation.

## Deferred / not-do

- No quota-aware dispatch, account management, cloud service, Desktop UI, generic plugin marketplace, or arbitrary code in rule packs.
- No automatic merge, unrestricted YOLO approval, shared author/reviewer conversation, or rebase of published Posse history.
- No wholesale extraction of either existing executor or assumption that no-mistakes supports unchanged Posse workflow skills.
- Backend support matrix, configuration filename, numeric default budgets, and exact AXI/wire/schema spelling remain deferred decisions. Greenhouse and separate-repository placement are approved; `review-tool` was a placeholder and proofrun was not approved.
- Multi-repository standalone orchestration is deferred; existing Posse workspace behavior must not regress through integration.
- All areas and the product name are approved. The User requested a local bootstrap package for a separate Project. Product implementation and any issue publication belong to the Greenhouse Lead in its own Herdr workspace. The Posse Lead must not start or adopt that Project. Remote repository creation/publication is not included in the local bootstrap.

## Evidence

- The User-approved lock card and discussion above define product scope.
- Task t1025 Report: no-mistakes v1.84.0 TUI/controller/lifecycle source tracing; Task t1026 Report: reviewer configuration, instruction delivery, and rule portability.
- Posse current domain vocabulary and ADR 0024 describe the existing custody, independent Review, and delivery contract. These are implementation constraints for the adapter, not authority for an unapproved standalone design.
