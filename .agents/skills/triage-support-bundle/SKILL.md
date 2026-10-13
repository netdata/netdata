---
name: triage-support-bundle
description: Investigate a Netdata support bundle offline - the archive `netdata-support-bundle` produces - to explain one host's alerts, missing data, collector failures, crashes, high CPU or memory, streaming, cloud claiming, retention, dashboard reachability, permissions, install and update, container and Windows problems. Use for "analyse this support bundle", "a customer sent a bundle", "what does this bundle say", "why did this agent crash", "why is this collector showing no data", "why are alerts not firing", or when reading `MANIFEST.json`, `summary.txt`, `status-file.json`, or anything under `01-system` through `09-permissions`. Not for SNMP evidence under `06-state/snmp-diagnostics` (triage-snmp-diagnostics), not for fleet-wide crash or regression clustering (triage-agent-events), not for live queries against an Agent or Cloud (query-netdata-agents, query-netdata-cloud), and not for changing the bundle collector itself.
---

# Support bundle triage

A support bundle is one host, one collection run, sanitized. Collection is not atomic: files carry
independent timestamps and can rotate while it runs, so artifacts are not a simultaneous snapshot. This skill turns a reported symptom into a
supported conclusion about that host, or into a statement that the bundle cannot settle it. The
bundle's own contents and rationale are owned by its two documents; what follows is the routing, the
absence semantics, and the traps.

Read `./orientation.md` first if you do not already hold a causal model of the Agent. Everything else
is reached from the tables below.

## Owners

| Owner section | What you must get from it |
|---|---|
| `packaging/installer/SUPPORT-BUNDLE.md#what-is-collected-and-why` | every collected item and the support ask it answers |
| `packaging/installer/SUPPORT-BUNDLE.md#what-is-never-collected` | the hard exclusions; what a bundle can never show |
| `packaging/installer/SUPPORT-BUNDLE.md#sanitization` | what the sanitizer rewrites, and its documented scope limits |
| `packaging/installer/SUPPORT-BUNDLE.md#redaction-philosophy` | why a key is judged by its name, not its value |
| `packaging/installer/SUPPORT-BUNDLE.md#the-streaming-api-key-exception` | the one secret kept verbatim, and where it is still redacted |
| `packaging/installer/SUPPORT-BUNDLE.md#encoding-fidelity` | BOM, line terminators and missing final newline survive, so encoding faults stay visible |
| `packaging/installer/SUPPORT-BUNDLE.md#bundle-format-contract` | schema id, provenance headers, which files carry them |
| `packaging/installer/SUPPORT-BUNDLE.md#design-contract-do-not-regress-these` | caps, timeouts, the global deadline, read-only guarantees |
| `packaging/installer/SUPPORT-BUNDLE.md#platform-support` | which implementation produced the bundle |
| `packaging/installer/SUPPORT-BUNDLE.md#raw-snmp-evidence` | what the opt-in SNMP store holds and that it is unsanitized |
| `docs/developer-and-contributor-corner/netdata-support-bundle.md#options` | the flags that shaped this bundle, including the log window |
| `docs/developer-and-contributor-corner/netdata-support-bundle.md#privacy-and-sanitization` | what to tell the customer about what they sent |
| `docs/developer-and-contributor-corner/netdata-support-bundle.md#include-snmp-diagnostics` | how to ask for another bundle with SNMP evidence |

Sibling skills, one line each. `triage-snmp-diagnostics` owns every SNMP question and the whole
`06-state/snmp-diagnostics/` store, including its SNMP-scoped reading of `04-config/` and `05-logs/`;
hand over as soon as the symptom is SNMP-scoped, and take back only host-level causes that are not
SNMP-specific. `triage-agent-events` owns fleet-wide crash, panic and fatal clustering against the
live agent-events namespace; escalate to it once you hold this host's signature and the question
becomes "is this known, is it fixed, how widespread". `query-netdata-agents` and
`query-netdata-cloud` own everything live, which is where every question this bundle cannot answer
goes. `query-snmp-traps` owns received traps.

## Before You Start

The bundle does not carry the incident. Establish these before interpreting any file, and ask rather
than infer:

1. The symptom in the reporter's words, and what they expected instead.
2. The incident timestamp, and whether it falls inside the bundle's log window. The window is a flag
   on the run (`docs/developer-and-contributor-corner/netdata-support-bundle.md#options`); outside
   it, silence means nothing.
3. Which host this is, and for a streaming or Cloud question, whether you also need the other end.

Then run the inventory before reading any single file - from the repository root that is
`.agents/skills/triage-support-bundle/scripts/bundle-summary.sh <bundle>`, or `./scripts/bundle-summary.sh`
from this skill's own directory. It reports what the
bundle holds, what is absent, what was truncated or withheld, and whether the incident time falls
inside the window. Reading files without that inventory is how absence gets misread as evidence.

Keep working notes under `<repo-root>/.local/audits/support-bundle/<case>/`. Bundle contents are
customer data: never paste them into a public issue, a commit, a fixture, or a review prompt.

## Choose The Investigation

Ordered by how often each class reaches support, which is **not** the order `summary.txt` prints.
The bundle's own READ ORDER FOR TRIAGE is a fixed seven-class list that leads with SNMP and crashes
and never mentions alerts. Read it, then use this table.

| Symptom | Start with | Guide |
|---|---|---|
| Alert did not fire, fires always, or flaps | silencers, then the alert's own config and the health transition records | `./alerts.md` |
| A notification never arrived though the alert did raise | the notification configuration and its delivery surface, **not** silencers | `./alerts.md` |
| A chart, collector or job shows nothing | the dyncfg job states where the bundle carries them, then the collector log; reach for plugin capabilities only when those are absent or inconclusive | `./no-data.md` |
| A specific collector fails, or service discovery finds nothing | the job's own state and its config, then the collector log | `./no-data.md` |
| Agent will not start, died, restarted, or was killed | the daemon status file, then the kernel messages, then the logs | `./lifecycle.md` |
| High CPU, memory, file descriptors, or disk I/O | per-thread CPU and the self-monitoring captures | `./lifecycle.md` |
| Retention shorter than expected, or a tier looks empty | per-tier disk usage against the effective config | `./lifecycle.md` |
| Install, update, or auto-update failure | install type and the install-time environment | `./lifecycle.md` |
| Child not on parent, rejected, flapping, or duplicated | the receiver's rejection reason, then the stream config on both ends | `./connectivity.md` |
| Node offline or stale in Cloud, or claiming fails | claim state and the ACLK view, then the network probes | `./connectivity.md` |
| Dashboard unreachable, TLS error, or a proxy in front fails | the socket inventory and the effective web config | `./connectivity.md` |
| Permission denied, a plugin silently collects nothing, or a capability was lost | the permissions captures | `./environment.md` |
| Container, Docker or Kubernetes behaviour | the container context and cgroup evidence | `./environment.md` |
| Anything on a Windows host | the merged event log and the Windows-only captures | `./windows.md` |
| "My config is ignored" | the effective running config, then dyncfg | `./no-data.md` |
| Anything SNMP | hand to `triage-snmp-diagnostics` | - |

Two files support every branch: `./bundle-map.md` is the artifact reference - what each path holds,
which platform produces it, and what its absence means - and `./evidence-limits.md` is what a bundle
structurally cannot answer. Check `./false-signals.md` before reporting any conclusion.

## Rules

Enforced by the bundle collector, so you can rely on them:

- `MANIFEST.json` indexes every file except itself, and its `bytes` matches the file on disk; the
  CI workflow `.github/workflows/netdata-support-bundle.yml` asserts that parity. Use it as the
  navigation contract rather than walking the tree
  (`packaging/installer/SUPPORT-BUNDLE.md#bundle-format-contract`).
- Every standard capture is sanitized. `sanitized: false` appears only for raw SNMP evidence, and
  its presence also clears the top-level `pii_obfuscated` and `secrets_redacted` flags
  (`packaging/installer/SUPPORT-BUNDLE.md#raw-snmp-evidence`).
- The streaming API key in the collected `stream.conf` is verbatim, scoped by source filename and
  exact key; the same key stays redacted in the access log and in log lines
  (`packaging/installer/SUPPORT-BUNDLE.md#the-streaming-api-key-exception`).
- Caps cut at line boundaries; a capped tail with no line break is withheld whole, and a sanitizer
  failure withholds the file (`packaging/installer/SUPPORT-BUNDLE.md#design-contract-do-not-regress-these`).
- Source bytes are preserved through sanitization, so a BOM, a CR-only file, or a missing final
  newline in the customer's config is still visible as itself
  (`packaging/installer/SUPPORT-BUNDLE.md#encoding-fidelity`).
- Pseudonyms are stable within one bundle and meaningless across two; the map is written beside the
  archive and never inside it (`packaging/installer/SUPPORT-BUNDLE.md#sanitization`).

Hand-reviewed, because nothing checks them:

- Never read a missing file as evidence. Resolve it through `./bundle-map.md` first: not collected on
  this platform, capped, deadline-skipped, API down, withheld by the sanitizer, or genuinely absent.
- Never compare pseudonyms across two bundles, and never ask the customer to send the pseudonym map
  unless the identity is load-bearing for the conclusion.
- Prefer the effective running config over the on-disk `netdata.conf`; they disagree whenever an
  environment override or an unrecognized option is involved. It is the merged **daemon** config
  only - it does not contain collector job configuration, so a UI- or API-created job override is
  visible only in the dynamic configuration area.
- State the collection window with any negative finding. "No errors in the log" means "none inside
  the window that survived the caps".
- Separate what the bundle shows from what you infer. Where the bundle cannot settle the question,
  say so and name the live query or the second bundle that would
  (`./evidence-limits.md`).
- Root-cause frequencies in these guides come from support-corpus analysis and are written as
  "commonly", never as a measured ranking; do not harden them into claims the evidence does not
  support.
