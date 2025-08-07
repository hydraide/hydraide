## ✅ Triage Labels

| Label               | Description                                                        | Who/When Can Set It                                      | Color     |
| -------------------| ------------------------------------------------------------------ | -------------------------------------------------------- | --------- |
| `triage:new`        | Initial state for all incoming issues or PRs. Not yet reviewed.    | 🧠 **System default** – automatically set on issue/PR creation. | `#ededed` |
| `triage:needs-info` | Lacks sufficient detail to proceed — contributor needs to clarify. | ✅ **Maintainers** set this when clarification is needed to proceed. | `#f9d0c4` |
| `triage:discussion` | Requires triage-level decision or early-stage architectural input. | ✅ **Maintainers** use this when an item needs broader input or design review. | `#fef2c0` |
| `triage:blocked`    | Awaiting spec or decision. Temporarily not ready for work.         | ✅ **Maintainers** apply when progress is halted by external dependency or pending decision. | `#b60205` |
| `triage:accepted`   | Triage complete — clear scope and ready for dev process.           | ✅ **Maintainers** assign this after full triage is completed. May also be **auto-applied via templates** when all criteria are met. | `#006b75` |


## 🚦 Status Labels

| Label                   | Description                                                     | Who/When Can Set It                                                            | Color     |
| -----------------------| --------------------------------------------------------------- | ------------------------------------------------------------------------------ | --------- |
| `status:in-progress`    | Actively being worked on. Someone is implementing it.           | ✅ **Maintainers or contributors** add this when they start active development. Optionally automated when PR is linked to an accepted issue. | `#1d76db` |
| `status:needs-review`   | Code has been submitted and is awaiting review.                 | 🔁 **Automatically set** when a pull request is opened or marked as “ready for review”. | `#fbca04` |
| `status:changes-needed` | PR reviewed and changes have been requested.                    | ✅ **Maintainers** set this during review when blocking feedback is given.      | `#d93f0b` |
| `status:ready-to-merge` | Approved, clean, and ready to be merged.                        | ✅ **Maintainers** apply after all reviews are approved and checks pass.       | `#0e8a16` |
| `status:merged`         | Already merged into the target branch.                          | 🔁 **Automatically set** by GitHub after PR is merged.                          | `#6f42c1` |
| `status:blocked`        | Technically blocked or waiting for external dependency.         | ✅ **Maintainers** use this when work is halted due to a technical or external blocker (e.g. upstream lib, spec delay). | `#b60205` |
| `status:abandoned`      | PR or issue is no longer pursued — context expired or replaced. | ✅ **Maintainers** mark tasks with this when they're superseded or stale. Can also be set by triage automation after inactivity. | `#cccccc` |


## 📦 Type Labels

| Label              | Description                                                          | Who/When Can Set It                                                                                     | Color     |
| ------------------| -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | --------- |
| `type:bug`         | A defect or unexpected behavior in the system.                       | ✅ **Maintainers** or **contributors** set this manually.<br>🧠 Can be auto-assigned via `bug-report.yaml` template. | `#d73a4a` |
| `type:enhancement` | A new feature, capability or improvement.                            | ✅ **Maintainers** set manually, or auto-assigned from `feature-request.yaml` template.                  | `#84b6eb` |
| `type:refactor`    | Code improvement without functional change.                          | ✅ Set manually during code cleanups or internal restructuring.                                          | `#c2e0c6` |
| `type:docs`        | Documentation-only change or addition.                               | 🔁 **Auto-applied** if `docs-improvement.yaml` template is used, or if PR touches only `docs/` files.   | `#0075ca` |
| `type:chore`       | Internal maintenance, tooling, or non-feature task.                  | ✅ **Maintainers** set manually, or it can be auto-detected from commit/PR title (e.g. `chore:` prefix). | `#fef2c0` |
| `type:idea`        | Conceptual or experimental proposal — not implementation-ready.      | ✅ Used by **maintainers** to flag high-level discussions or architectural sketches.                     | `#bfdadc` |
| `type:question` | Inquiry or clarification that doesn’t require implementation. | ✅ **Maintainers** set manually based on context or judgement. | `#d876e3` |
| `type:example`     | SDK/CLI usage example — intended for documentation and learnability. | 🔁 **Auto-assigned** if issue is created via `example-request.yaml` template.                           | `#fffbdd` |

## 🧭 Area Labels

| Label              | Description                                      | Who/When Can Set It                                                                                  | Color     |
| ------------------| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------ | --------- |
| `area:core`        | HydrAIDE Core (`app/core/`) related.             | ✅ **Maintainers** set manually, or auto-detected if PR affects files under `app/core/`.               | `#0052cc` |
| `area:server`      | HydrAIDE Server internals (`app/hydraideserver/`). | ✅ **Maintainers** set manually, or auto-applied if paths under `app/hydraideserver/` are touched.     | `#5319e7` |
| `area:hydraidectl` | HydrAIDE CLI implementation specifically.        | ✅ Manually set by maintainers. Can also be inferred from PRs affecting `cmd/hydraidectl/` or CLI docs. | `#008672` |
| `area:sdk-go`      | Go SDK and related logic.                        | ✅ Auto-assigned if files under `sdk/go/` or `docs/sdk/go/` are modified.                              | `#3572A5` |
| `area:sdk-python`  | Python SDK and tooling.                          | ✅ Maintainers set manually or via path-based CI rule (e.g. `sdk/python/**`).                          | `#3572A5` |
| `area:docs`        | Documentation or markdown-based assets.          | 🔁 **Auto-applied** if PR includes `.md`, `.rst`, or lives under `docs/`, `README.md`, etc.            | `#006b75` |

## 🏷 Priority Labels

| Label             | Description                                                  | Who/When Can Set It                                                                                  | Color     |
| ----------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------ | --------- |
| `priority:high`   | Important and urgent — should be addressed soon.             | ✅ **Maintainers** assign this based on business impact, production bugs, or time-sensitive needs.     | `#e11d21` |
| `priority:normal` | Default priority — planned work but not urgent.              | 🔁 **Default value** if no other priority is set. Can be auto-assigned by issue templates or bots.     | `#ededed` |
| `priority:low`    | Low urgency — can be delayed or picked up opportunistically. | ✅ **Maintainers** set this for non-critical, cosmetic, or stretch goal tasks.                         | `#c5def5` |

## 📌 Meta Labels

| Label                   | Description                                                                | Who/When Can Set It                                                                                              | Color     |
| -----------------------| -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | --------- |
| `meta:claimed`     | Someone has claimed this task and is working on it.                       | ✅ **Maintainers** set manually, or added automatically when assignee is attached.            | `#fbca04` |
| `meta:ai-assisted` | Contribution (partly) generated using AI — requires manual understanding. | ✅ **Maintainers** set manually based on context or author’s note. Optionally flagged by bot. | `#e4e669` |
| `meta:onboarding`       | Assigned to a newcomer or first-time contributor.                          | ✅ Maintainers assign manually based on user profile or intent (e.g. via `contributor-application.yml`).          | `#c2e0c6` |
| `meta:help-wanted`      | Maintainer explicitly welcomes external contributions.                     | ✅ Maintainers set manually. Can also be toggled from GitHub UI or issue template.                                | `#159818` |
| `meta:do-not-pick`      | Should not be picked — deprecated, internal or intentionally disabled.     | ✅ Maintainers apply this to internal-only tasks, deprecated flows, or reserved issues.                           | `#b60205` |
| `meta:discussable`      | Meant for long-form discussion or evolving collective input.               | ✅ Maintainers set this when an item is intentionally left open-ended or conceptual (e.g. RFCs, architectural docs). | `#fef2c0` |
| `meta:reference`        | Issue is used as a live example or long-term reference — not to be closed. | ✅ Maintainers apply to sticky issues like API contracts, decisions, glossary, or process definitions.           | `#0366d6` |


## Good First Issue Labels

| Label              | Description                                                                | Who/When Can Set It                                                                                            | Color     |
|--------------------| -------------------------------------------------------------------------- |----------------------------------------------------------------------------------------------------------------| --------- |
| `good first issue` | Suitable for newcomers — simple and self-contained.                        | ✅ Maintainers add manually, often in combination with `meta:onboarding` and `type:bug` or `type:enhancement`. Unlike other labels, however, good first issue is treated specially only because websites like goodfirstissues.com and goodfirstissues.dev currently only parse issues with that exact label format. This makes it functionally different — not semantically — from other triage labels. | `#7057ff` |
