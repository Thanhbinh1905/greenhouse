# Configuration

Per-user configuration files live in `GREENHOUSE_HOME`. A repository can add `.greenhouse.json` and `.greenhouse-pipeline.json`, which apply only after you trust them.

A review needs no configuration file, copied key or executable path. Selected repair needs trusted checks in `fixer.json`. The full pipeline also needs a trusted command plan and forge authorization. These files approve commands, not providers.

| File | Location | Purpose |
| --- | --- | --- |
| `fixer.json` | `GREENHOUSE_HOME`, mode 0600 | Fixer identity and trusted checks; see [repair](repair.md#configure-the-fixer). |
| `pipeline.json` | `GREENHOUSE_HOME`, mode 0600 | The full pipeline command plan; see [full pipeline](pipeline.md#command-plan). |
| `backends.json` | `GREENHOUSE_HOME`, mode 0600 | Backend `fallback` choices, extra `writable` directories, `review_budget`, `review_passes`, `review_shards` and the `auto_resume` retry policy; see [agents](agents.md). |
| `.greenhouse.json` | repository | Repository override of settings. |
| `.greenhouse-pipeline.json` | repository | Repository command plan. |

## Configuration versions

`fixer.json`, `backends.json` and `pipeline.json` carry `"version": 2`.

Retired `reviewer.json` and `providers.json` files stay on disk and appear as `retired` in configuration inspection. They do not route new reviews, and migration skips them.

Legacy executable and provider fields in `backends.json`, and executable, model and provider fields in `fixer.json`, do not control new invocations.

Retained review inputs keep their original configuration and arguments. Resuming them needs explicit preparation through the native agent boundary.

## Migration

`migrate` (line client `u`) upgrades every older file in one action. For each file it:

1. checks that the new file decodes to exactly the old values
2. saves the original bytes as `<file>.v1-<digest>.bak` with mode 0600, where `<digest>` is the first 12 hex characters of the SHA-256
3. replaces the file atomically

Each migration is recorded in `migrations` with the file, versions, backup, before and after digests, actor, reason and action ID.

If any file is newer or invalid, nothing changes. Files are then backed up and replaced one at a time, so a write error partway through leaves the earlier files migrated.

`pipeline.json` declares `"budget_unit": "rounds"`. Migration adds it, because version 1 counted rounds. Greenhouse never converts a budget unit and refuses any other unit.

Rule packs keep schema version 1. A newer pack fails its load with an upgrade message.

## Repository overrides and trust

A worktree may hold `.greenhouse.json`:

```json
{"version": 1, "budget_unit": "rounds", "mode": "ask", "limit": 3, "rules": ["unslop"]}
```

The file applies to new runs only after the User trusts the repository with `trust` (line client `t`). Trust binds the exact file digest shown in `repository.digest`.

- An untrusted, changed, newer or invalid file shows as ignored, with the reason. New runs then use the per-user defaults.
- Trust is recorded per repository with its actor, reason and action ID.
- Trusting never changes the current run.

### What an override can change

An override can only narrow or select among per-user choices:

- `mode`, `preset` and `limit` cannot exceed the per-user values. The order is `ask` below `auto` below `yolo`, and `one-pass` below `full`.
- `auto_resume` cannot turn on a per-user `off`.
- `rules` must be built-in or already selected in the per-user defaults.
- Selecting `claude` or `codex` needs a `claude` or `codex` entry in the per-user `backends.json`. `pi` is always allowed.
- `executor_timeout` is not narrowed: an override may set any valid duration.
- Any key other than a settings field makes the whole file ignored. This includes executable, check, command, grant, remote, provider, fallback and YOLO keys.

### Responses

Responses carry:

- `repository`: the path, a `status` of `absent`, `ignored` or `applied`, the reason, the digest and the trust record
- the effective `defaults`, with `defaults_sources`
- `sources` on each configuration snapshot

### Saving to the repository

Saving with `"target":"repository"` needs a trusted, unchanged file. Save writes only the values that differ from the per-user defaults and trusts the written bytes.

Commit the file to share it. Until then, guarded handback accepts an untracked or modified `.greenhouse.json` only while its bytes match the trusted digest. Any other change still blocks handback.

The repository command plan, `.greenhouse-pipeline.json`, uses the same `trust` action; see [command plan](pipeline.md#command-plan).
