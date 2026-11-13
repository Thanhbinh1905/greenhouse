# Compatibility policy

Greenhouse versions its AXI envelope, its per-user configuration files, the repository override, rule packs and the run store separately. Each one refuses a version it does not understand and says which version it supports.

## AXI envelope

Every request carries `version`. This daemon accepts protocol versions 1 and 2 and answers in the version the request used. Every response carries `protocol`:

```json
{"version": 2, "min": 1, "max": 2}
```

| Version | Adds |
| --- | --- |
| 1 | Lifecycle, settings, review, repair, pipeline, decisions and recovery operations |
| 2 | `watch` event subscriptions, host mode (`host`, `grant`, `host_result`) |

A request outside the supported range is refused with `unsupported protocol version N; this daemon supports 1-2` and the `protocol` block, and changes nothing. A version-1 request that uses `watch`, `host` or `grant` is refused with `requires protocol version 2`.

Rules within one version:

- Changes are additive: new response fields, new operations, and new optional request fields whose omission keeps the old behavior.
- Clients ignore response fields they do not know.
- The daemon refuses request fields it does not know, so a client that sends a newer optional field to an older daemon gets `invalid request: json: unknown field`, not a silent misreading.
- Removing or renaming a field, changing its type or meaning, or changing a default needs a new version. The daemon keeps accepting the previous version.

## Negotiation

A client sends the highest version it speaks. On a version refusal it reads `protocol.min` and `protocol.max`, retries with the highest version both sides support, or stops with a message. A daemon that answers a refusal without `protocol` predates this policy and speaks only version 1.

The terminal needs version 2 for live events. Against an older daemon it exits with:

```text
the running Greenhouse daemon speaks only protocol 1, and this client needs protocol 2. Let its work settle, then stop it with SIGTERM to the PID in GREENHOUSE_HOME/pid after confirming that process is the Greenhouse daemon. Then run greenhouse again
```

For current daemons, run `greenhouse stop` with the daemon's `GREENHOUSE_HOME`.
If a daemon predates `stop`, the command exits non-zero and gives this manual shutdown step:
let its work settle, then stop it with SIGTERM to the PID in `GREENHOUSE_HOME/pid` after confirming that process is the Greenhouse daemon.
The command never signals a legacy process. Restart never signals stored PIDs.

Against a daemon whose range excludes version 2 it names both ranges and asks for a matching Greenhouse. The terminal never stops or replaces a daemon on its own; a running daemon keeps its protocol until it is stopped.

## Event streams

A `watch` stream ID identifies one daemon process. Sequence numbers increase within a stream and start again in a new one. A client that presents another stream, a sequence ahead of the daemon, or a sequence older than the retained 1,024 events gets `"wait": "resync"` with the current snapshot. Sequence numbers are not stored and carry no meaning across streams.

## Configuration files

`fixer.json`, `backends.json` and `pipeline.json` carry `"version": 2`; a file without `version` is version 1.

- An older file is refused and listed under `config` until `migrate` upgrades it. Migration is lossless: it checks that the new file decodes to the same values and keeps the original bytes as a mode-0600 backup.
- A newer file is refused with an upgrade message and left unchanged.
- A new configuration version may add keys with defaults that keep the old behavior. Any change to an existing key's meaning, such as a budget unit, needs a new version and a recorded migration; Greenhouse never converts silently.

`.greenhouse.json` and rule packs carry version 1 and refuse a newer version with an upgrade message.

`reviewer.json` and `providers.json` are retired. Inspection reports them without using or migrating their contents. They remain on disk. Native CLIs own executable discovery, login, and provider routing for new invocations. Existing stored inputs retain their original arguments and provider metadata for inspection. Explicit Prepare binds a new native agent without resetting usage or rewriting old reviews.

## Run store

`state.json` carries store version 1. New fields are additive and optional, so older stores load without a rewrite or counter change. An unknown store version refuses daemon startup. Downgrading Greenhouse over a store written by a newer version is not supported.
