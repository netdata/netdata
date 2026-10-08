---
name: triage-snmp-diagnostics
description: Investigate captured SNMP diagnostics and support bundles, or review diagnostics-tool and evidence-interpretation changes. Covers missing topology/metrics/BGP/licensing, slow collection, and discovery/lifecycle failures through offline inspection, replay and source tracing. Live queries, traps and collector/profile implementation use separate skills.
---

# Offline SNMP Diagnostics

Developer workflow for investigating captured SNMP evidence with the repository's maintainer tool. Start with the
supplied evidence and narrow the failure to acquisition, processing, or the consuming feature before proposing a fix.

## Select The Operation

| Task | Use this workflow |
|---|---|
| Investigate a supplied capture | Establish the evidence window below, then select the symptom and owner sections |
| Review diagnostics tooling or interpretation changes | Read the affected input, evidence, replay and resource-cost contracts from the owners below; assess the diff and existing fixtures/validation |
| Explain an evidence field or limitation | Read its owner section and relevant producer source; no capture run required solely to explain the contract |

Source-only review does not require acquiring a support bundle, replaying private data or creating investigation
artifacts. Report material evidence gaps without treating loading this skill as authorization for operational steps.
A review of unrelated SNMP collector internals does not select this skill unless diagnostic evidence/contracts are affected.

## Owners

Read the sections needed for the current symptom; command and format details belong to these owners.

| Owner section | Subject |
|---|---|
| `src/go/tools/snmp-diagnostics/README.md#input-selection` | Supported inputs and evidence selectors |
| `src/go/tools/snmp-diagnostics/README.md#collection-status-and-partial-listings` | Inventory and partial collection interpretation |
| `src/go/tools/snmp-diagnostics/README.md#normal-device-evidence` | Metrics, BGP, licensing, and source-reference interpretation |
| `src/go/tools/snmp-diagnostics/README.md#topology-replay-and-inspection` | Device/link inspection and replay queries |
| `src/go/tools/snmp-diagnostics/README.md#read-limits-and-container-cost` | Selected-document limits and repeated bundle reads |
| `src/go/tools/snmp-diagnostics/README.md#collection-cost` | Acquisition timing and accounting limits |
| `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#diagnostic-files-and-publication` | Publication cadence, retention, runs, and terminal behavior |
| `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#offline-diagnostic-inspection` | Capture branches and stage availability |
| `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#portable-archive-codec-and-replay-boundary` | Captured inputs and replay boundary |
| `docs/developer-and-contributor-corner/netdata-support-bundle.md#what-it-collects` | Companion logs, configuration, and bundle inventory |
| `docs/developer-and-contributor-corner/netdata-support-bundle.md#privacy-and-sanitization` | Sanitization boundaries |
| `docs/developer-and-contributor-corner/netdata-support-bundle.md#include-snmp-diagnostics` | Operator capture instructions |

For live API evidence, use `query-netdata-agents` or `query-netdata-cloud`. For received traps, use `query-snmp-traps`.
When the investigation leads to implementation, route collector changes through `collectors-authoring`, profile changes
through `collectors-snmp-profiles`, and topology producer changes through `topology-authoring`.

## Establish The Evidence Window

1. Identify the reported symptom, affected device/link or metric, expected behavior, and incident time. Start inventory
   even if some incident details are missing; ask only for details that change the investigation.
2. Keep reports under `<repo>/.local/audits/snmp-diagnostics/<case>/` with private permissions. Preserve the supplied
   archive. Raw inspections and replay output MUST NOT enter committed fixtures, public issues, or review prompts.
3. Use `list` for a directory or bundle and read both collection notes and component errors. With an individual file,
   start with `validate` and `summary`. For containers, select and validate the relevant document after inventory.
4. Record producer version, run, capture times, and selected checkpoint/device alongside the report. Check whether they
   cover the incident before interpreting the data. Follow the publication owner for retention and freshness semantics;
   do not assume the bundle is one synchronized sample.

Run the CLI from `src/go` as shown in its README. For repeated commands, building the same tool into the private case
folder avoids repeated compilation. Choose the evidence scope before requesting full inspections; consult the container
cost owner before scanning a large tar repeatedly.

## Choose The Investigation

| Symptom | Start with | Follow the evidence |
|---|---|---|
| Missing/wrong topology device or link | Topology `summary`, then `inspect-device` or `inspect-link` | Captured protocol observations → graph membership → rendered actor/link; use `replay` to locate an existing link. |
| Missing/wrong metric | Normal device `summary`, then `inspect-device` | Selected profile and acquisition results → processed metric and emission decision → sample value. |
| Missing/stale BGP peer or wrong peer state | Normal device inspection | Profile BGP rows and source operations → peer cache and refresh outcome. For a missing topology BGP edge, use the topology path too. |
| Missing/wrong license inventory or usage | Normal device inspection | Profile license rows and source operations → normalized licensing state. |
| Discovery, initialization, or missing device evidence | Lifecycle `summary` | Candidate/runtime state and failure → available device cuts → matching logs/configuration. |
| Slow SNMP collection | The affected collector's device inspection | Normal attempt timestamps and referenced operation timing, or topology collection contexts and detailed accounting; use the collection-cost owner for Handler timing limits. |

Topology summaries identify registrations. For normal evidence, select a run from the listing and identify the device
from its normal summary; a lifecycle inventory can help only after its producer run matches. Do not require a topology
checkpoint to investigate normal metrics, BGP, or licensing.

## Explain The Collector Result

Once inspection locates the affected stage, read only the relevant subsystem details:

| Question | Owner sections |
|---|---|
| Why this profile or value? | `src/go/plugin/go.d/collector/snmp/ARCHITECTURE.md#profile-selection`, `src/go/plugin/go.d/collector/snmp/profile-format.md#1-selector`, `src/go/plugin/go.d/collector/snmp/profile-format.md#2-extends`, `src/go/plugin/go.d/collector/snmp/profile-format.md#value-transformation` |
| Why a missing, combined, or derived sample? | `src/go/tools/snmp-diagnostics/README.md#acquisition-processing-and-samples`, `src/go/plugin/go.d/collector/snmp/ARCHITECTURE.md#metric-emission`, `src/go/plugin/go.d/collector/snmp/profile-format.md#virtual-metrics`, `src/go/plugin/go.d/collector/snmp/profile-format.md#chart-metadata` |
| Were these inputs refreshed? | `src/go/tools/snmp-diagnostics/README.md#cached-inputs-and-earlier-outcomes` |
| Why this BGP peer state or licensing result? | `src/go/plugin/go.d/collector/snmp/ARCHITECTURE.md#bgp-collection-and-retained-rows`, `src/go/plugin/go.d/collector/snmp/ARCHITECTURE.md#licensing-normalization-and-collection-results`, `src/go/tools/snmp-diagnostics/README.md#cached-state-versus-function-output` |
| Why these topology observations or refresh state? | `src/go/plugin/go.d/collector/snmp/profile-format.md#41-topology`, `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#topology-profile-composition`, `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#refresh-loop` |
| Why was a topology actor/link changed or filtered? | `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#graph-build-order` |

These sections identify consumer source symbols where code inspection is needed.

## Trace And Compare

- Work from the affected output back to its recorded inputs. Inspect the relevant profile, row, metric, or peer before
  expanding to all source operations. Follow source references using the normal-evidence owner; retain their context
  with any quoted result so another maintainer can reproduce the join.
- Separate latest attempts, retained failures/successes, and cached inputs using their own times. Resolve a stale value
  to its recorded source before attributing it to the latest refresh. Follow the inspection owner when a stage is
  unavailable; missing evidence is not proof that the device did not return data.
- Compare retained topology checkpoints with identical query controls. Match device identity and producer context
  before comparing registrations; obtain link selectors from each replay instead of carrying a row index across cuts.
  Use previous-run normal evidence only when it helps the incident, with the run boundaries made explicit.
- Distinguish profile/acquisition failures from processing and consumer decisions. A captured sample alone does not
  settle whether a chart was created or displayed; use the normal-evidence owner to bound that conclusion.
- For a source-level explanation, check the producer version against the code being read. Treat replay through a
  different checkout as a version-qualified experiment. Do not edit captured values to make a replay succeed.
- For discovery targets, collector enablement, polling settings, or profile overrides, inspect relevant files under
  `04-config/` and persisted UI/API configuration under `06-state/dyncfg/`, when available. Compare them with the
  diagnostic evidence; persisted configuration may differ from what the collector used at capture time.

## Correlate Companion Logs

- After establishing the evidence timeline, use the bundle inventory to find relevant logs and configuration.
  Prioritize available `05-logs/collector.log`, `05-logs/journal-netdata.txt`, and
  `05-logs/journal-namespace-netdata.txt`; an empty collector log is not a reason to skip the journals.
- Use ordinary archive/text tools for logs. Start with SNMP matches, narrow using `collector=snmp`,
  `collector=snmp_topology`, and the affected `job` where present, then inspect nearby context. Include relevant
  `go.d` startup, shutdown, discovery, and job messages; do not restrict the search to warnings/errors or treat
  every SNMP match as a collector failure (alerts can also match).
- Correlate message times, jobs, and restarts with the incident and diagnostic capture/run boundaries. Consult the
  bundle's sanitization owner before joining identifiers: differently sanitized files may not share literal device
  names.
- Check log coverage and collection notes before interpreting missing matches; capped captures, missing history,
  and rate-limited messages cannot establish that no failure occurred. Keep conclusions within the captured window.

## Report The Finding

Report the symptom and evidence coverage, then the supported failure location and its consequence. Cite the selected
file or checkpoint, producer/run and capture time, and the relevant inspection branch or source reference. Separate
observations from hypotheses and state what the capture cannot establish.

Recommend the smallest useful next check when evidence is insufficient, explaining which uncertainty it resolves.
Use the operator capture owner when another bundle is needed; do not silently replace offline analysis with live SNMP
polling, configuration changes, or sharing raw reports. When a fix is justified, hand the evidence and reproduction to
the appropriate authoring skill rather than embedding an implementation workflow here.
