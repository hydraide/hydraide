# Working on HydrAIDE with Claude Code

HydrAIDE ships a [Claude Code](https://claude.com/claude-code) plugin with three skills and three slash commands. Install it once; the right skill activates automatically when you ask a HydrAIDE-related question.

## Install

The plugin lives in [`hydraide/claude`](https://github.com/hydraide/claude) (a Claude Code marketplace). Two steps: start Claude Code, then run two slash commands inside the session.

### Step 1: start Claude Code

If you have not installed it yet, follow [the official Claude Code installation guide](https://docs.claude.com/en/docs/claude-code/quickstart). Once installed, open a terminal in any directory (does not have to be a HydrAIDE project) and run:

```
claude
```

You are now inside a Claude Code session. The next commands start with `/` and run inside this session, not in your shell.

### Step 2: add the marketplace and install the plugin

In the Claude Code session:

```
/plugin marketplace add hydraide/claude
```

This pulls the marketplace metadata (`.claude-plugin/marketplace.json`) from the `hydraide/claude` GitHub repository. You should see a confirmation that the marketplace was added. Then:

```
/plugin install hydraide
```

This installs the `hydraide` plugin from the marketplace. After it finishes, the five skills and three slash commands listed below are immediately available.

### Step 3: verify

Ask Claude something HydrAIDE-related, for example:

```
Which HydrAIDE skills do you have available right now?
```

If the install succeeded, it will list `hydraide`, `hydraidego`, `hydraidectl`, `hydraide-install-and-upgrade`, and `hydraide-data-ops`. The slash commands are visible by typing `/` and looking for `hydraide-` prefixes.

## Updates

The plugin gets new content (skill improvements, new slash commands, new feature docs) whenever a PR lands in [`hydraide/hydraide`](https://github.com/hydraide/hydraide) that touches `.claude/skills/**`, `.claude/commands/**`, or `docs/features/**`. A GitHub Action then mirrors the change into [`hydraide/claude`](https://github.com/hydraide/claude). You do not edit the mirror directly.

To pull those updates onto your machine, you have two options.

### Option A: turn on auto-update (recommended)

In the Claude Code session, open the plugin manager UI:

```
/plugin
```

Switch to the **Marketplaces** tab, find the `hydraide` marketplace in the list, and toggle **auto-update** on. From now on, every time you start Claude Code, it refreshes the marketplace and applies any pending updates. If anything updated, a banner suggests `/reload-plugins`; run that and the new skills / commands are live in the current session.

### Option B: pull updates manually

Skip auto-update, and pull on demand:

```
/plugin marketplace update hydraide
/reload-plugins
```

Run these whenever you want the latest. The `update` step refreshes the marketplace metadata and downloads any plugin changes; `/reload-plugins` makes them active in the current session.

### How to know what changed

Major changes are visible in the [`hydraide/claude` commit history](https://github.com/hydraide/claude/commits/main). Each sync commit references the source SHA in `hydraide/hydraide`, so you can read the underlying PR if you want context.

## Editing the plugin

The plugin is a **mirror**, not a source of truth. To change a skill or a slash command, open a PR against [`hydraide/hydraide`](https://github.com/hydraide/hydraide):

- Skills: [`hydraide/hydraide/.claude/skills/`](https://github.com/hydraide/hydraide/tree/main/.claude/skills)
- Slash commands: [`hydraide/hydraide/.claude/commands/`](https://github.com/hydraide/hydraide/tree/main/.claude/commands)
- Concept docs that the `hydraide` skill routes into: [`hydraide/hydraide/docs/features/`](https://github.com/hydraide/hydraide/tree/main/docs/features)

On merge to `main`, the sync workflow rewrites the mirror automatically. Your change reaches everyone with auto-update on within a few minutes.

## Troubleshooting

### "Marketplace file not found"

Sometimes a previous failed `/plugin marketplace add` leaves a half-cloned, broken cache:

```
Error: Marketplace file not found at /home/<you>/.claude/plugins/marketplaces/hydraide-claude/.claude-plugin/marketplace.json
```

Delete the cache directory and try again:

```
rm -rf ~/.claude/plugins/marketplaces/hydraide-claude
```

Then re-run `/plugin marketplace add hydraide/claude` inside Claude Code.

### "Plugin not found in any marketplace"

You skipped `/plugin marketplace add hydraide/claude` (or it errored silently). Run it first, then `/plugin install hydraide`.

### Skills do not seem to activate

Make sure you ran `/reload-plugins` after the install, or restart Claude Code (`exit`, then `claude` again). Confirm with the verify question in Step 3.

### Auto-update is on but I do not see changes

Open `/plugin`, go to the **Marketplaces** tab, and check the auto-update toggle is actually enabled (the UI shows it per marketplace). Run `/plugin marketplace update hydraide` once manually to force a refresh, then `/reload-plugins`.

## What the plugin gives you

### Five skills (auto-activated by question shape)

| Skill | Activates when you ask about |
|---|---|
| [`hydraide-install-and-upgrade`](../.claude/skills/hydraide-install-and-upgrade/SKILL.md) | Bootstrapping HydrAIDE. Server install on Linux as systemd or via Docker, Go SDK install in your application, server and SDK upgrades, V1 to V2 storage migration, filesystem and hardware guidance, troubleshooting install errors. The first stop for "how do I get HydrAIDE running". |
| [`hydraidego`](../.claude/skills/hydraidego/SKILL.md) | Building Go applications. Profile vs Catalog modelling, struct tags, server-side filters (AND/OR, vector, geo, nested-slice, phrase, IN), atomic increments, distributed locks, real-time subscriptions, structural patches, indexing and pagination, common pitfalls. |
| [`hydraidectl`](../.claude/skills/hydraidectl/SKILL.md) | Operating a running server instance. Start/stop/restart, backup/restore, inspect, observe, compact, explore, destroy, certs. The day-to-day operations companion. |
| [`hydraide-data-ops`](../.claude/skills/hydraide-data-ops/SKILL.md) | Ad-hoc data operation CLIs. Migrations between Swamps, restore from export, bulk import, bulk delete, bulk update, orphan cleanup, cross-environment data sync. The skill that turns a vague "I need to move / fix / clean this data" request into a safe one-shot Go script with dry-run by default and live-env protection. |
| [`hydraide`](../.claude/skills/hydraide/SKILL.md) | "How does X work" questions. Routes to the right concept doc in [`docs/features/`](features/) (Swamp lifecycle, addressing, query engine, msgpack patch, storage engine internals, etc.) without bloating the conversation context. |

If you ask a question that spans two of them (for example "explain Swamps and then write me a Catalog model"), Claude reads the relevant concept doc first and then hands off to `hydraidego` for the code.

### Three slash commands

| Command | What it does |
|---|---|
| `/hydraide-new-model` | Interactive Profile/Catalog model wizard. Asks the questions that matter (model type, addressing, fields, lifecycle), confirms the plan, then generates the struct, `RegisterPattern`, and a real-instance test scaffold. Refuses bare `int`/`uint`, sets `EncodingMsgPack`, sets `CreatedAt = time.Now().UTC()` where required. |
| `/hydraide-review` | Reviews HydrAIDE-related changes against the pitfall checklist from `hydraidego` skill section 17: zero `createdAt` on save (silent drop), bare numeric types, GOB encoding, missing `RegisterPattern`, batch-vs-loop, `ExpireAt` clock-skew margin, lock semantics, etc. Output grouped by severity. |
| `/hydraide-debug` | Guided diagnostic flow. Scopes the symptom in one batch of questions, walks the six most common pitfalls in priority order (clock skew first; that one is the top cause of flaky `ShiftExpired`), then points at logs only if the checklist did not pinpoint it. Refuses to guess without evidence. |

## What the plugin does NOT give you

Honesty up front:

- **No MCP server (yet).** A HydrAIDE MCP server (live tools that let Claude inspect a running instance, run safe queries, watch subscriptions) is on the watchlist. It will be considered when there is organic demand. For now, Claude reads code and docs; it cannot poke a live HydrAIDE.
- **No skills for non-Go SDKs (yet).** The skill set is Go-first. When a Python or Rust SDK ships, a matching `hydraidepy` / `hydraiders` skill follows the same pattern.
- **No automatic workspace setup.** Installing the plugin makes the skills available in any Claude Code conversation; it does not modify your project files. Use `/hydraide-new-model` when you want generated code in your project.

## Using the plugin without `/plugin install`

If you cannot use the plugin marketplace (corp policy, air-gapped environment, etc.), the same content is in this monorepo:

- Skills: [`.claude/skills/`](../.claude/skills/)
- Slash commands: [`.claude/commands/`](../.claude/commands/)
- Concept docs that the `hydraide` skill routes into: [`docs/features/`](features/)

Copy `.claude/skills/<name>/` and `.claude/commands/*.md` into your own project's `.claude/` and Claude Code will pick them up. The skill cross-references inside the monorepo use relative paths that work from this checkout; outside, they resolve to absolute GitHub URLs.

## A predictable layout

The directory layout is consistent and named after the concepts in the docs. If you ask Claude "where is X", these paths usually answer it:

- `proto/hydraide.proto`: the gRPC service definition, single source of truth for the API.
- `app/server/`: server entry point, gRPC plumbing, gateway.
- `app/core/hydra/`: engine internals (Swamps, Treasures, Beacon, Chronicler).
- `app/core/hydra/swamp/chronicler/v2/`: V2 single-file storage engine.
- `app/core/hydra/swamp/treasure/msgpackpatch/`: structural patch primitive.
- `sdk/go/hydraidego/`: the Go SDK (its own Go module).
- `app/hydraidectl/`: management CLI.
- `docs/features/`: concept and feature documentation.
- `docs/sdk/go/`: Go SDK docs (install, reference, examples, testing).
- `docs/benchmarks/`: measured numbers and run scripts.

## English everywhere

Per [`CLAUDE.md`](../CLAUDE.md):

- Code, comments, and commit messages are in English.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/), enforced by a pre-commit hook.

## How to get the most out of Claude on this repo

- Open the conversation with `CLAUDE.md` in scope (Claude Code does this automatically when you launch it inside the repo).
- Let the skills auto-activate. You do not need to invoke them manually; the question shape selects the right one.
- For HydrAIDE-related code reviews, run `/hydraide-review` after staging your changes, instead of asking "review this".
- For HydrAIDE bugs, run `/hydraide-debug` first; half the time the answer is on the pitfall list and you save a round trip.
- For new models, `/hydraide-new-model` produces consistent boilerplate. The wizard refuses footguns by design.
