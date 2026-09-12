---
name: repo-skill-authoring
description: Create, amend, restructure, or review Netdata skills under .agents/skills/ and docs/netdata-ai/skills/. Use for skill instructions, triggers, routing, stale claims, duplication, and preservation of domain knowledge. Review is read-only; small amendments use a bounded workflow. Repository bootstrap and PR-comment triage have separate workflows.
---

# Skill Authoring

Use skills to preserve project-specific knowledge and route each task to the instructions it needs. Loading a skill
for review or maintenance does not authorize the operational procedures it describes. Root `AGENTS.md` and
`.agents/skills/README.md` remain the owners of project authorization, SOW, delivery and skill-layout policy.

## Pick Your Task

| Task | Read |
|---|---|
| Explain guidance or investigate possible rot | Relevant sections of `./authoring-rules.md#rot-signals`; report evidence without implementation ceremonies |
| Review an existing skill change | `./change-method.md#review-round` and the relevant rules/owners; use the supplied diff and scope |
| Correct or add a bounded rule, example, trigger or local duplicate | `./change-method.md#bounded-amendment` and the affected `./authoring-rules.md#authoring-rules` sections |
| Restructure, slim, split or migrate substantial knowledge | `./change-method.md#restructure-evidence`, then its preservation and review sections |
| Create a skill | `./change-method.md#creating-a-skill` and `./authoring-rules.md#authoring-rules` |
| Locate a governing project rule | `./authoring-rules.md#where-the-rules-live` |

A slimming pass over touched content does not by itself turn an amendment into a restructure. Choose by loss risk and
changed contracts, not line count. Existing user approval persists; this skill adds no options round for a fixed goal.
Non-trivial implementation still needs the project SOW gate; read-only and trivial work retain the owner's exemptions
(`AGENTS.md#when-a-sow-is-required`, `AGENTS.md#plan-before-non-trivial-work`).

## What Good Looks Like

The description makes the skill discoverable; the short entry states shared constraints and routes to details.
The relevant owner supports factual claims, commands are valid for their stated environment, and valid exceptions
survive changes. A reviewer can assess the full assigned change without being told to run live procedures or rewrite
unrelated material. Validation and review establish acceptance; shorter text alone is not proof of improvement.

## Maintaining This Skill

Capture applicable lessons under `AGENTS.md#knowledge-capture` and the active task's artifact gate. Updating one
lesson uses the bounded amendment path unless it changes the method structurally. When the method changes, include a
fresh-context walkthrough from this skill, root `AGENTS.md` and the skills README in the required review: exercise a
representative amendment, restructure and read-only review, including their action boundaries. This can be part of the
correctness review, not an automatic additional review round.
