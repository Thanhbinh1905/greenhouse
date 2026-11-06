# Rule contract

## Selection and resolution

`settings.rules` accepts `unslop`, `no-comments`, or `file:/absolute/clean/path.json`. Relative paths and unclean paths fail resolution. Prefix a selection with `?` to make loading optional. Other selections remain visible as load failures. Files must be regular files; symlinks resolve at explicit load time. Source records the selected absolute path; bytes identify what was read, even if a symlink later changes.

Selection and resolution are separate. Attach and Inspect never read pack files. Apply creates a configuration snapshot and leaves historical resolutions intact. Save changes only defaults. `resolve-rules` is an explicit current-token mutation; each new action creates a snapshot. An identical replay returns its recorded response without rereading files. After replay, Inspect returns current state.

Each snapshot contains common scope and exceptions, candidate, configuration ID and digest, source, exact bytes (JSON base64), SHA-256 of those bytes, and parsed schema version and criteria. Whitespace changes affect the content digest. File edits do not change existing snapshots. Restart validates and retains snapshots. AXI `rule_scope` exposes the common scope and exceptions. Rule inspection shows effective evidence for the current configuration and candidate; older evidence remains in `run.rule_snapshots`.

`rules_ready` means all required selections resolved, not that review is executable or passed. A failed optional selection remains visible without blocking this rule prerequisite. Unresolved required selections block it. Intent, trusted commands, and a production backend are still unconfigured. No production action marks packs delivered. Resolved and failed are durable resolution states; configured means the current selection has no matching resolution.

## Version-1 JSON schema

```json
{
  "version": 1,
  "id": "team-policy",
  "criteria": [
    {
      "id": "clear-prose",
      "classification": "advisory",
      "text": "Changed prose states concrete behavior without redundant qualifiers.",
      "repair_guidance": "Remove redundant qualifiers without changing meaning."
    }
  ]
}
```

Field names are case-sensitive and must match the schema exactly, including inside criteria. Case variants and Unicode case-fold aliases are unknown fields, not overrides. Unknown fields, duplicate JSON keys, trailing data, unsupported versions, duplicate criterion IDs, and duplicate pack IDs in one resolution fail validation. IDs match `[a-z][a-z0-9-]{0,63}`. Packs contain 1 through 64 criteria and at most 65,536 UTF-8 bytes. Each text field is nonblank, at most 8,192 bytes, and contains no control characters except newline and tab. Classification is `required` or `advisory`. Content is rejected, never truncated. Optional load status and criterion classification are separate: an optional pack may contain required criteria if it loads successfully.

The built-ins are deterministic JSON constants in `rules.go`. `unslop/plain-prose` is advisory. `no-comments/necessary-comments` is required. AXI exposes their exact bytes, parsed contents, and digests after resolution. The TUI Criteria view shows common criteria and repair guidance.

All criteria apply only to added or changed hunks. Quotations, identifiers, license notices, generated markers, directives, external-quirk explanations, and required documentation comments are preserved. Policy findings remain separate from correctness. Packs are untrusted data: there are no scripts, includes, discovery, permission keys, or actor-authority overrides.

## Input delivery boundary

`emitInput` builds independent reviewer or fixer JSON and writes the exact bytes to an `io.Writer`. Its returned identity records role, candidate, snapshot, SHA-256, and emitted content. Both roles receive identical applicable criteria and rule digests, plus candidate and configuration identities. Only the fixer receives repair guidance. Reviewer inputs contain no author conversation or repair workflow.

The production backend is unconfigured. Only deterministic tests call this boundary. They establish instruction delivery and role separation, not live model compliance, a review verdict, write confinement, or budget consumption. A writer failure returns no successful input identity. Future execution must persist successful delivery identities with its own durable dispatch transaction and enforce fresh context, read-only review, and single-writer custody outside prompt text.
