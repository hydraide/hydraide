# 👋 Welcome, HydrAIDE Maintainer

Thank you for joining the HydrAIDE maintainer team. You’re part of the core Team shaping how this system grows, 
keeping it clear, stable, and well-documented. Beyond reviewing code, your work sets the tone for quality, shared 
learning, and thoughtful collaboration. What you build, label, and mentor will influence others, helping them 
learn faster, stay inspired, and grow. And in return, you’ll develop quickly too, by seeing the system from 
more angles than anyone else.

---

**HydrAIDE follows a philosophy of:**

* 📚 **Clarity first** – every major SDK function should include examples and docs
* 🌱 **Supportive mentorship** – contributors grow when guided with context and kind patience
* 💡 **Example-driven culture** – the more working examples in SDKs and issues, the more useful the system becomes
* 🧪 **Responsibility** – core and server logic should be test-backed, even when fast-moving
* ⚡ **Performance matters** – HydrAIDE is built for speed. Core and server features should include benchmarks where relevant, and SDKs benefit from performance profiling as well.

## 🧭 What Maintainers Do

Maintainers set direction, uphold quality, and remove roadblocks. You’ll shape the experience of contributors and protect the project’s clarity and cohesion.

Here are your key responsibilities:

## 🧭 Maintainer Responsibilities

* **Triage incoming issues and PRs**
  Review new issues and pull requests, assess clarity and completeness, and determine how they fit into the roadmap.

* **Label consistently based on** our shared label taxonomy
  Apply labels that describe triage state, task type, SDK area, contributor role, and status.

* **Help contributors choose tasks and clarify open questions**
  Offer guidance on available issues, especially to new contributors, and provide answers or direction when they ask.

* **Ensure SDK changes are tested and documented**
  Every new method or behavior in the SDK should come with a clear usage example and automated test coverage.

* **Encourage example-based contributions (esp. via `type:example` issues)**
  Promote writing of small, real-world examples in issues or `/examples/` folders to grow a usable knowledge base.

* **Organize issues into parent/child tasks when needed**
  Decompose complex features into linked subtasks using references like "blocked by" or "relates to".

* **Assign owners to claimed issues and track activity**
  When someone commits to a task and shares a timeline, assign them formally and add the `meta:claimed` label.

* **Mark good first issues where appropriate**
  Identify self-contained, well-specified, and low-risk tasks that are suitable for newcomers, and label them with `good first issue`.

## 🏷 Label System (Preview)

Maintainers use a structured label set to categorize and manage all tasks and contributions. This includes:

* `triage:*` — tasks under evaluation or needing decision/context
* `status:*` — tracks progress through the dev workflow
* `type:*` — defines the kind of work involved
* `area:*` — module or SDK-specific tagging
* `meta:*` — special flags for visibility, ownership, or process
* `priority:*` — urgency or scheduling guidance

👉 The full label reference is here: [LABEL_REFERENCE.md](LABEL_REFERENCE.md)

## 🤝 Helping New Contributors

* Watch for comments where contributors claim tasks
* Be generous with context and patient with gaps
* Label `meta:onboarding` or `good first issue` where relevant
* Encourage early PRs to be small and easy to review
  Smaller PRs reduce review time and simplify feedback. They increase merge confidence and help contributors build 
  trust gradually, without being overwhelmed. A focused PR is easier to learn from — for both sides.

## 📦 Task Structure & Ownership

* Break large issues into subtasks when possible (this often reveals good opportunities for good-first-issue labels.)
* Use parent → child referencing (`blocked by`, `related to`) to show task trees
* Every larger development effort must be tracked under a parent issue. This parent should define the overall goal and hold the designated branch name (e.g. `feature/xs`).
* All related child issues and PRs should be aligned under that parent, targeting the feature branch, not `main`.
* Contributors must be asked to submit PRs against the parent branch — **not against** `main`.
* Only feature branches that are stable, reviewed, and complete should be merged into `main`.
* Assign an issue when a contributor clearly takes responsibility
* Add `meta:claimed` when contributor confirms intent + timeline

## 🚫 Avoid

The following actions can create confusion or reduce the long-term clarity of the system. Avoid these patterns:

* **Submitting PRs directly to core, server, or Go SDK logic without approval**
  Changes to `app/core`, `app/hydraideserver`, or `sdk/go` must be pre-approved by the repo owner and at least one lead maintainer.

* **Merging core/server logic without docs or examples**
  Every core contribution must be accompanied by clear documentation and at least one working usage example.

* **Accepting vague PRs with unclear purpose or scope**
  Pull requests should state their intent explicitly and fit into a known issue or roadmap topic.

* **Closing discussions prematurely**
  Even if an idea seems unresolved or inactive, maintainers should protect the thinking process and encourage open iteration.

## 🙏 Thank You

Being a maintainer is a leadership role. Your consistency, clarity and encouragement shape this project
more than any one PR ever will. Thank you for investing your time and judgment into HydrAIDE! This means a lot to me and the community.

- Peter
