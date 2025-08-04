# Contributing to HydrAIDE

Thanks for your interest in contributing.  
HydrAIDE is a production-ready system, what we're building now is the SDK and tooling ecosystem around it.

We value clean, well-thought-out work. Whether you're improving documentation, shaping an SDK, 
or refining CLI behavior, your contribution matters.

---

## Quickstart

1. Fork the repository

2. Create a feature or fix branch  
   → **Use the branch specified in the task or its parent issue.**  
   → If in doubt, ask before starting work.

3. Before you begin working:
  - **Check if someone is already assigned or working on the task** (look for assignees or comments).
  - If it's free, **comment on the issue** to say you're starting work.
  - Please include an **estimated date** when you expect to open the PR (even if approximate).  
    → This helps avoid duplication and gives others visibility on progress.
  - If your task is blocking others, please **help them estimate when they can begin** based on your timeline.

4. Make your changes

5. Open a Pull Request  
   → **Target the correct development branch**, not `main` unless explicitly stated.  
   → Most features are merged into a dedicated SDK or CLI branch, and later batched into `main`.

6. We'll review and respond. Usually within a day or two.


If you're new to HydrAIDE, feel free to:

- Start with a [Contributor Application](https://github.com/hydraide/hydraide/issues/new?template=contributor-application.yml) issue
- Ask for guidance on Discord: [discord.gg/aBfAuYjR](https://discord.gg/aBfAuYjR)
- Explore the [HydrAIDE Knowledge Engine](https://chatgpt.com/g/g-688779751c988191b975beaf7f68801d-hydraide-knowledge-engine) to better understand the system

---

## Docs & SDK Reference

- [Installation Guide](docs/how-to-install-hydraide.md)
- [Go SDK reference](docs/sdk/go/go-sdk.md)
- [Thinking in HydrAIDE](docs/thinking-in-hydraide/thinking-in-hydraide.md)
- [FAQ for AI & SDK usage](docs/hydraide-questions-answers-for-llm.md)

These are optimized for both humans and tools like ChatGPT — use whatever helps you learn faster.

---

## Project Layout

- Docs: `/docs`
- SDKs: `sdk/<language>`
- Examples: `/docs/sdk/<language>/examples`
- Main Applications: 
  - HydrAIDE Core: `app/core`
  - HydrAIDE Server: `app/hydraideserver`
  - HydrAIDE CLI: `app/hydraidectl`

Please follow the Go SDK as a reference for structure, naming, and documentation style. SDK `.md` files should be 
clear, parseable, and contain example code.

---

## Looking for a Task?

- Check the **pinned issues** — these are the main areas we're actively working on, and help is always welcome there
- Browse issues labeled [`help wanted`](https://github.com/hydraide/hydraide/issues?q=label%3A%22help+wanted%22) — these are larger or strategic tasks
- See if there’s any [`good first issue`](https://github.com/hydraide/hydraide/issues?q=label%3A%22good+first+issue%22) available — smaller, self-contained starters
- Or, if you have your own idea, feel free to open a new issue and suggest it

---

## Commit Style

Use [Conventional Commits](https://www.conventionalcommits.org/) when possible:

- `fix: handle empty Swamp hydration`
- `feat: add TTL support to Python SDK`
- `docs: clarify Catalog usage`

---

## ✅ PR Style (Conventional Commits-alapú)

Use [Conventional Commits](https://www.conventionalcommits.org/) to format pull request titles.
This helps us **automatically assign labels**, generate changelogs, and maintain consistent history.

### ✅ Allowed PR title prefixes:

| Prefix      | Purpose                                  | Example                                   |
| ----------- | ---------------------------------------- | ----------------------------------------- |
| `fix:`      | Bugfix, unexpected behavior              | `fix: handle empty Swamp hydration`       |
| `feat:`     | New feature or capability                | `feat: add TTL support to Python SDK`     |
| `docs:`     | Documentation-only change                | `docs: clarify Catalog usage`             |
| `refactor:` | Internal code change, no behavior change | `refactor: simplify hydration handler`    |
| `chore:`    | Build system, tooling, or meta change    | `chore: update GitHub Actions matrix`     |
| `test:`     | Adding or updating tests                 | `test: add coverage for Catalog shifting` |
| `style:`    | Code formatting, whitespace, linter      | `style: reformat SDK with gofumpt`        |
| `perf:`     | Performance-related improvement          | `perf: optimize hydration loop`           |

### ❌ Avoid vague or non-standard titles:

* ✗ `Update stuff`
* ✗ `bugfix`
* ✗ `Final version`
* ✗ `Quick fix`

### 📌 Additional Guidelines:

* Always use the prefix **in the PR title**, not just in commit messages.
* The prefix is **case-insensitive** but we recommend lowercase for consistency.
* Draft PRs are welcome, but please use the prefix already when opening.

---

## Testing

- All code should run locally without errors
- Add tests for logic-heavy functions

If you're adding an SDK method, include a simple usage test (call + assert expected result).

## ⚡ Benchmarking

For changes in `app/core` or `app/server`, benchmarking is **strongly recommended** — especially for critical 
functions or logic that runs frequently. HydrAIDE is optimized for speed, and performance regressions must be avoided.

Include `Benchmark*` tests when relevant.
Use `go test -bench .` to measure impact.

---

## Pre-commit Hooks

We use [pre-commit.ci](https://pre-commit.ci/) to run our pre-commit hooks automatically on every Pull Request.  
It handles formatting, linting, and basic validation — and can fix issues automatically by committing changes to your PR.

To run hooks locally before committing:

```bash
uv tool install pre-commit
# or
pipx install pre-commit
```
Then activate hooks:
```bash
pre-commit install
```
Run all hooks:
```bash
pre-commit run --all-files
```
---

## 🏷 Contributor-facing Labels

The following labels are visible and relevant to general contributors. You don't need special
permissions to understand or act based on them, just use them to stay aligned with the workflow.

### ✅ Triage & Workflow Awareness

| Label                   | Meaning                                                          |
| ----------------------- | ---------------------------------------------------------------- |
| `triage:needs-info`     | Maintainers need more detail before progress can begin.          |
| `triage:accepted`       | Task is understood, scoped, and ready to be picked up.           |
| `status:in-progress`    | Someone is already working on this task. Avoid duplicate effort. |
| `status:needs-review`   | Waiting for code review by maintainers.                          |
| `status:changes-needed` | PR was reviewed — needs updates before it can move forward.      |

### 📦 Type of Work

| Label              | Meaning                                                   |
| ------------------ | --------------------------------------------------------- |
| `type:bug`         | This task involves fixing a bug.                          |
| `type:enhancement` | This adds a new feature or improves an existing one.      |
| `type:docs`        | Focused on documentation improvements or additions.       |
| `type:example`     | Real-world usage example to be added to our SDK/CLI docs. |

### 🧭 Area Tags

| Label              | Meaning                              |
| ------------------ | ------------------------------------ |
| `area:sdk-go`      | This task relates to the Go SDK.     |
| `area:sdk-python`  | This task relates to the Python SDK. |
| `area:hydraidectl` | CLI logic and tooling.               |

### 📌 Contribution Meta

| Label                   | Meaning                                                            |
| ----------------------- | ------------------------------------------------------------------ |
| `meta:claimed`          | Someone has commented they are working on this. Respect ownership. |
| `meta:ai-assisted`      | Submission was aided by an AI tool — requires careful review.      |
| `meta:help-wanted`      | Maintainers would love help on this one. Feel free to contribute!  |
| `meta:onboarding`       | Task assigned to a newcomer — often mentoring involved.            |

### 🔥 Good First Issues

| Label                   | Meaning                                                            |
| ----------------------- | ------------------------------------------------------------------ |
| `good first issue` | Great place to start if you're new.                                |

ℹ️ **Note:** You won't be able to assign labels yourself unless you're part of the triage team. If you're working on something, just leave a comment saying so — a maintainer will handle the rest.

---

## AI-assisted Contributions

We welcome contributors who learn and build with tools like ChatGPT or Claude. They can help accelerate learning and simplify tasks.
That said, **HydrAIDE is a human-centered system**: we care deeply about intentional design and domain understanding.

If you're using AI to assist your contribution:

* ✅ **Allowed** for documentation writing, task summaries, tooling, CLI improvements, test coverage, and other minor enhancements
(but always read, review, and understand the output fully before submitting)
* ❌ **Not allowed** for HydrAIDE Core, Server internals, Swamp mechanics, or SDK infrastructure
  *(unless explicitly discussed and understood)*
* **You must understand the code you submit.** If it’s AI-generated, make sure you can explain it line by line.
* Please add a note in your PR description if it was AI-assisted (e.g., "Generated with Claude").

We’re not anti-AI. We just want to ensure that all code reflects human understanding!
HydrAIDE wasn’t generated by a tool, and we believe that matters.

If you're learning through contribution, that's perfect. Just be transparent, and feel free to ask questions.
We’ll mentor you with pleasure.

Thank you for supporting HydrAIDE, and welcome to the team.

***– Péter Gebri***
