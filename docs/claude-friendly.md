# Working on HydrAIDE with Claude Code

HydrAIDE is set up to be straightforward to navigate with AI coding tools — Claude Code, Cursor, Windsurf, Zed, and similar. This page describes what is in place today and what to expect.

## What is in place

### `CLAUDE.md` at the repo root

The repository ships a [`CLAUDE.md`](../CLAUDE.md) at the root. It is loaded automatically by Claude Code in every session opened against this repo and tells the assistant the project's communication conventions, architecture, key file paths, and code conventions. The same file is useful as context for any other AI coding tool that supports a `CLAUDE.md`-style instructions file.

### Claude Code skills

The repository ships two Claude Code skills under [`.claude/skills/`](../.claude/skills/). Both are auto-discovered when you open this repo in Claude Code; either can be copied into another project's `.claude/skills/` to use it there.

| Skill | Use it for |
|---|---|
| [`hydraidego`](../.claude/skills/hydraidego/SKILL.md) | Building Go applications on HydrAIDE — Profile/Catalog modelling, struct tags, server-side filters (AND/OR, vector, geo, nested-slice, phrase, IN), atomic increments, distributed locks, real-time subscriptions, structural patches, indexing/pagination, common pitfalls. |
| [`hydraidectl`](../.claude/skills/hydraidectl/SKILL.md) | Operating HydrAIDE servers — install, start/stop/restart, upgrade, backup/restore, V1→V2 migration, inspect, observe, compact, explore, destroy, certs. |

To use either skill **in your own project that depends on HydrAIDE**, copy the matching folder from this repo's `.claude/skills/` into your own project's `.claude/skills/`. Claude Code will load it for that project as well.

### A predictable layout

The directory layout is consistent and named after the concepts in the docs:

- `proto/hydraide.proto` — the gRPC service definition, single source of truth for the API.
- `app/server/` — server entry point, gRPC plumbing, gateway.
- `app/core/hydra/` — engine internals: Swamps, Treasures, Beacon (index), Chronicler (storage).
- `app/core/hydra/swamp/chronicler/v2/` — the V2 single-file storage engine.
- `app/core/hydra/swamp/treasure/msgpackpatch/` — the structural patch primitive.
- `sdk/go/hydraidego/` — the Go SDK.
- `app/hydraidectl/` — the management CLI.
- `docs/features/` — concept and feature documentation.
- `docs/benchmarks/` — measured numbers and run scripts.

If you ask Claude "where is X", these paths and names mean the answer is usually one grep away.

### English code, English commits, Hungarian discussion

Per [`CLAUDE.md`](../CLAUDE.md):

- Code, comments, and commit messages are in English.
- Conversation with the maintainer is in Hungarian.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).

This split is enforced by a pre-commit hook.

## What is not (yet) in place

Honesty up front:

- **No published Claude skill.** A Hungarian-language internal skill exists in the maintainer's separate monorepo and will be polished and published later. Until then, this page and `CLAUDE.md` are the canonical Claude-facing guidance.
- **No MCP server.** A HydrAIDE MCP server (live tools that let an AI inspect a running instance) is on the watchlist, not a v1 commitment. It will be considered if there is organic demand.
- **No JS/TS or Python SDK examples for browser/Node-first AI workflows.** The Go SDK is the reference; gRPC clients can be generated for any language from the proto, but the examples directory is Go-centric.

## How to get the most out of Claude on this repo

- Open the conversation with `CLAUDE.md` already in scope (Claude Code does this automatically).
- For storage internals, point Claude at [`docs/features/v2-storage-engine.md`](features/v2-storage-engine.md) and [`docs/benchmarks/V2_RESULTS_SUMMARY.md`](benchmarks/V2_RESULTS_SUMMARY.md).
- For the query API, point Claude at [`docs/features/query-engine.md`](features/query-engine.md) and the `--- QUERY FILTER SYSTEM ---` block inside [`proto/hydraide.proto`](../proto/hydraide.proto).
- For the SDK shape, point Claude at [`docs/sdk/go/go-sdk.md`](sdk/go/go-sdk.md).
- For the rules of the road (no `panic()`, use `panichandler.SafeGo`, conventional commits, no AI co-author lines, etc.), `CLAUDE.md` has the full list.
