---
type: change
title: "feat: build Greenhouse standalone validation pipeline"
done_when:
  - A Greenhouse Lead has inspected SPEC.md and the two research reports and identified a bounded implementation sequence without redoing approved product discovery.
  - Implementation proceeds only through Workers in this Project's Task Sandboxes, with tests and exact-head review evidence, and without altering the Posse Project or installed tools.
  - The delivered product satisfies the approved SPEC.md behavior, including standalone operation, one-pane TUI, AXI, bounded independent review, persistent settings, safe custody, and no automatic merge.
---

## Intent

User's words requesting this transfer, verbatim:
> ok Green house đi, chắc là tạo 1 repo mới nhỉ, bạn tạo 1 folder rồi dựng ô lead posse rồi delegate brief  qua cho mình được không

Goal: Establish Greenhouse as its own Project and implement the approved standalone validation pipeline, with a future Posse integration adapter.

Lead assumptions: This is intake for the Greenhouse Lead, not one monolithic Worker Brief. The previous Lead prepared local documents only. It did not start/adopt another Project, publish a remote repository or issue, or create product code. Product naming and all functional spec areas are approved; remaining engineering choices are identified in SPEC.md.

## Read first

1. SPEC.md: accepted scope, constraints, area decisions, and executable acceptance expectations.
2. docs/research/no-mistakes-runtime.md: installed v1.84.0 source tracing of TUI, daemon, agent roles, gates, YOLO, and recovery.
3. docs/research/no-mistakes-reviewer-rules.md: actual reviewer configuration, rule delivery, limitations, and portability of unslop/no-comments.
4. Follow the current Project's Posse runtime instructions, global agent guidance, and applicable skills. Do not treat historical research paths as permission to control another Project.

The copied research is evidence about the inspected version, not proof that a current upstream or installed binary behaves identically. Report source gaps rather than silently substituting another version.

## Accepted product direction

Greenhouse is a separately callable tool, not only a Posse display. A bare command opens a compact TUI; reviewer agents run headlessly. A per-user local daemon owns runs; TUI and AXI share the same durable authority. Reopening an existing context attaches without duplicating work or resetting history.

The full pipeline covers intent, target synchronization, independent review, actual test commands, documentation assessment/repair, lint/format/static checks, push, PR, and exact-head CI. Git/command/forge work does not require an agent; invoke agents only for work needing assessment or repair. Full standalone runs stop at CI-ready, never auto-merge.

Settings can apply to the current run without changing defaults. Explicit Save persists defaults per user or a trusted repository override. Unsaved applied settings still survive attach to the same run. Show effective rule content provenance and load failures; repair and review use consistent criteria. A rule name in a prompt is not proof that a skill was loaded or followed.

Default auto repairs within approved intent and authority, escalating scope/intent/acceptance changes and unresolved findings at cap. Ask and YOLO remain explicit modes. Preserve exact-head accept-head, bounded final-fix/final Review, rethink lineage, and authorized discard. Budgets retain consumed usage across attach and approved changes.

One-pass means one independent reviewer invocation, with one optional selected authorized fix, checks, and local commit. Do not automatically rereview or publish it. Mark the repaired head as not independently rereviewed.

Standalone repairs use a managed private worktree. Handback must refuse source movement or dirty-state ambiguity, preserve unpublished commits, and never reset/stash/rebase the User's work to force success. Embedded Posse uses host custody and the original Change Worker for repairs instead of a second competing writer or publication pipeline.

## First outcome

Check the actual local repository baseline and available tooling. Choose a small executable vertical slice and record the implementation sequence against the approved spec. Prefer a working bare-command/AXI run lifecycle with deterministic fake backends and reattachment tests over a broad nonfunctional scaffold.

Do not reopen already settled product choices. Ask only for remaining consequential decisions such as initial backend support or a public schema trade-off that source inspection cannot settle. The spec's controller, custody, and mode decisions are not permission to change this Project's user-only config keys.

Implement through bounded Worker Change Briefs in this Project's own Sandboxes. Every such Brief must carry the original User intent. Keep unimplemented stages visibly unsupported instead of claiming they passed. Preserve tests for duplicate launch, stale gates, single writer, crash recovery, exact input/config snapshots, and bounded fix loops.

Do not execute a live no-mistakes run to implement Greenhouse, copy either executor wholesale, rewrite installed integrations, or use source-string tests as behavioral proof. No large agent swarm is authorized. A proposed Posse adapter must be handed back to the Posse Project's own Lead for implementation there.

## Repository and authority boundaries

The User must open a separate Herdr workspace rooted here and run posse up. The previous Posse Lead must not create that Lead or control its panes.

No GitHub remote, parent issue, executable code, release, package name reservation, Project registration, or other Project configuration was created in this bootstrap. Verify the live state at startup. Do not assume the other Project's YOLO setting or cleanup consent applies here.

Once a forge is selected and authorized, publish this accepted spec as one parent issue and record its URL before splitting forge tickets. Do not silently create a remote or infer broad publication authority from the local bootstrap request.
