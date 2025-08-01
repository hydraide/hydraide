# Contributing to HydrAIDE

Thanks for your interest in contributing.  
HydrAIDE is a production-ready system, what we're building now is the SDK and tooling ecosystem around it.

We value clean, well-thought-out work. Whether you're improving documentation, shaping an SDK, 
or refining CLI behavior, your contribution matters.

---

## Quickstart

1. Fork the repository
2. Create a feature or fix branch
3. Make your changes
4. Open a Pull Request
5. We'll review and respond. Usually within a day or two

If you're new to HydrAIDE, feel free to:

- Start with a `Contributor Application` issue
- Ask for guidance on Discord: [discord.gg/aBfAuYjR](https://discord.gg/aBfAuYjR)
- Explore the [HydrAIDE Knowledge Engine](https://chatgpt.com/g/g-688779751c988191b975beaf7f68801d-hydraide-knowledge-engine) 
to better understand the system

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

## Testing

- All code should run locally without errors
- Add tests for logic-heavy functions

If you're adding an SDK method, include a simple usage test (call + assert expected result).

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

Thank you for supporting HydrAIDE, and welcome to the team.

***– Péter Gebri***
