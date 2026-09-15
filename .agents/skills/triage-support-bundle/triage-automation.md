# Automated triage

`scripts/analyze-bundle.sh` runs this skill's reasoning over a bundle without a human, and emits a
verdict a helpdesk connector can act on. This file is the contract between the two halves.

## The split

The analyzer is generic and lives here: bundle in, verdict out, against any OpenAI-compatible
endpoint. It holds no identities, no helpdesk knowledge and no customer specifics, so it is useful
standalone to anyone holding a bundle.

The connector is not generic and lives in a private project: the helpdesk domain and credential, the
filter that selects tickets, the tag vocabulary, and the mapping from a role key to an actual person.
None of that may be committed here - see `AGENTS.md#sensitive-data-in-durable-artifacts`.

The seam is `verdict.json`. The analyzer writes it; the connector reads it and acts. A scheduled
poller is the same analyzer under a scheduler plus dedup state, not a second implementation.

## The pipeline

```
bundle (+ ticket text)
  -> deterministic extract    bounded evidence pack, already sanitized by the collector
  -> model                    classify, cite evidence, draft the note
  -> deterministic gate       decide what may be published
  -> verdict.json + note.md
```

Three properties this shape buys, each deliberate:

- **The model never sees the raw bundle.** An extracted bundle runs to megabytes, dominated by the
  dynamic configuration tree. The pack is a bounded derivation - on a development-host bundle it is
  roughly two orders of magnitude smaller. This bounds cost, latency and exposure at once.
- **This skill's guardrails are the system prompt.** `prompts/triage-system-prompt.md` is assembled
  with `false-signals.md` and `evidence-limits.md`, so the model is held to the same discipline as a
  human reader: absence is not a finding, and some questions are structurally unanswerable.
  Improving those files improves the automation.
- **The model never decides whether it is publishable.** It classifies and drafts; the gate in the
  script decides. A confidently wrong internal note is worse than none, because it anchors whoever
  reads it next.

## Running it

```sh
.agents/skills/triage-support-bundle/scripts/analyze-bundle.sh <bundle> \
    --ticket ./ticket.txt --incident 2026-09-13T19:30:00Z
```

| Flag | Effect |
|---|---|
| `--ticket <file>` | the reporter's own words; **required for any cloud routing** (see below) |
| `--incident <ISO8601>` | incident time, checked against the collection window |
| `--out <dir>` | output directory; defaults under the skill's audit directory |
| `--pack-only` | build the evidence pack and stop - no model call, no credential needed |
| `--min-confidence <0..1>` | gate threshold, default 0.5 |
| `--selftest` | offline: drives the credential path with a sentinel and asserts it never reaches output, then asserts every gate rule including the cloud-route downgrade |

Configuration comes from `<repo>/.env`; the keys are documented in `.agents/ENV.md`.

## The gate

Three deterministic rules, applied to whatever the model returned:

- **Confidence below the threshold** publishes nothing but an insufficient-evidence note.
- **`unknown` classification** does the same.
- **`snmp` classification** does the same and says the ticket belongs to another workflow.

These rules and the correction below are asserted offline by `--selftest`. The downgrade in
particular is a guard the model may never trigger on its own, so it is tested directly rather than
assumed.

One deterministic correction is applied before the gate. A bundle is an agent artifact: it carries
the agent's own half of the cloud connection and nothing at all about the browser or the console. So
when no ticket text was supplied, any `cloud-backend` or `cloud-frontend` route the model proposed is
downgraded to `unknown`, with the reason recorded. Cloud routing rests on the reporter's symptom;
without it the claim is not supportable, whatever the model asserted.

## verdict.json

Schema `netdata-bundle-triage/v1`. The fields a connector needs:

| Field | Meaning |
|---|---|
| `classification.class` | one of the taxonomy classes, or `unknown` |
| `classification.confidence` | 0 to 1 |
| `classification.summary` | the finding, in prose |
| `route.role` | **an opaque key**: `agent`, `cloud-backend`, `cloud-frontend`, `unknown` |
| `route.rationale` | which input drove it |
| `tags` | suggested tags, kebab-case |
| `evidence[]` | `{artifact, observation}` pairs citing bundle paths |
| `missing_evidence[]` | what could not be checked, and why |
| `next_checks[]` | the smallest next step, or what to ask the customer for |
| `gate.publish` | whether the deterministic gate allows posting a conclusion |
| `gate.reason` | why |
| `note_markdown` | the rendered note body, also written to `note.md` |

`route.role` is a key, never a person. The connector owns that mapping, in its own configuration.

## What a connector must do

Requirements on the private half, so the two stay consistent:

- **Post as an internal note only.** Nothing here is written for a customer to read, and the note
  says so.
- **Never auto-assign.** Tag and suggest an owner; a human routes. This is a decision of record, not
  a default to revisit casually.
- **Respect the gate.** When `gate.publish` is false, post the insufficient-evidence note or nothing
  - never the drafted conclusion.
- **Keep the provenance header**, which `note.md` already carries. A reader must be able to tell at a
  glance that a machine wrote it.
- **Be idempotent.** Keep state keyed on the ticket and the bundle so a re-run does not post twice.
- **Fail closed.** If the model is unreachable or returns something unparseable, post nothing.

## Calibration

Before enabling automatic posting, run the analyzer over real ticket-and-bundle pairs and compare the
verdict against what triage actually concluded. Two failure modes matter more than accuracy:
confident misrouting, and an evidence citation the bundle does not support. Both are visible only
against real tickets, so shadow mode is the first deployment, not a formality.

Tune `--min-confidence` from that evidence rather than from taste.
