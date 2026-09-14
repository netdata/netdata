---
name: collectors-authoring
description: Author, modify, or review Netdata collectors across Go, IBM, C, Rust and external plugins. Start here for collector lifecycle, metric identity, missing data, cost, cardinality, SNMP/Prometheus profiles, statsd, logs, topology, flow/OTEL ingestion and Functions. Routes to domain and framework guidance. Operator queries use the query skills.
---

# Collector Authoring And Review

Start here for collector changes and reviews, then follow the affected domain or framework. Selection and action
boundaries are owned by `AGENTS.md#skill-selection`: reading a recipe does not authorize its generation, installation,
credential setup or live operations. A review uses existing design/validation evidence and reports concrete gaps.

## Shared Collector Contracts

Every collector review needs the applicable shared contracts below, including a performance-focused review. These
summaries point to the detailed owner sections; expand further according to changed behavior and dependencies.

| Contract | Details |
|---|---|
| Stable public chart/dimension identities and truthful measurement gaps | `./collector-practices.md#13-ids-are-public-contracts`, `./collector-practices.md#14-gaps-are-data` |
| Known removal versus temporary unreachability; source-appropriate chart lifecycle | `./collector-practices.md#15-obsolete-whats-gone` |
| Bounded work, resource lifetime, failure behavior and useful cardinality | `./collector-practices.md#22-hot-path-discipline`, `./collector-practices.md#25-cardinality-bounding`, `./collector-practices.md#4-production-quality-criteria--pre-pr-checklist` |
| Contextual errors and bounded logging using the framework mechanism | `./collector-practices.md#23-error-handling`, `./collector-practices.md#24-logging-discipline` |
| Supported-source semantics and representative validation evidence | `./collector-practices.md#16-verify-the-supported-source-contract`, `./collector-practices.md#21-test-against-reality` |
| Correct framework/version, configuration and generated-source ownership | `./collector-practices.md#52-god-v1--v2-reality-check`, `./collector-practices.md#26-configuration-discipline`, `./collector-practices.md#27-generated-artifacts-are-not-source` |

For a contract-preserving parser optimization, these shared constraints and the parser/framework evidence may suffice.
For ownership, caching or lifecycle costs, also expand the design references. A lens does not exempt applicable
contracts or require every reference in this skill.

## Route By Task

Use each row for implementation or review of that surface. Implementation-only notes and commands are not review
prerequisites. In Go work, read `src/go/AGENTS.md` and the relevant subtree instructions first.

| Task | Start with |
|---|---|
| New go.d collector | `.agents/skills/collectors-go-design/SKILL.md`, then `src/go/plugin/go.d/docs/how-to-write-a-collector.md`, `.agents/skills/collectors-go-framework-v2/SKILL.md`, and `.agents/skills/integrations-lifecycle/recipes/add-go-collector.md`; implementation records the design before code |
| go.d config option, mode, metric meaning, Functions, ownership or vnode change | `.agents/skills/collectors-go-design/SKILL.md` for affected items; framework/version guidance and `.agents/skills/integrations-lifecycle/consistency.md` for coupled artifacts |
| Existing go.d collector fix | Collector-local files, relevant framework/version contract, `.agents/skills/integrations-lifecycle/consistency.md` and its `recipes/update-collector.md` workflow for authorized delivery |
| V1-to-V2 migration | `src/go/plugin/go.d/docs/migrate-v1-to-v2.md` and `.agents/skills/collectors-go-framework-v2/SKILL.md`; preserve compatibility rather than inventing enrichment |
| Shared framework/helper change | `src/go/plugin/framework/docs/changing-framework-code.md`; the applicable gate precedes implementation |
| Collector metadata content | `.agents/skills/collectors-metadata-yaml/SKILL.md`, affected fields only; pipeline mechanics in `.agents/skills/integrations-lifecycle/SKILL.md` |
| IBM workload collector | `src/go/plugin/ibm.d/AGENTS.md` and `src/go/plugin/ibm.d/framework/README.md`; IBM has its own framework and generated-source contract |
| Rust plugin | `src/crates/netdata-plugin/` SDK and `src/crates/netflow-plugin/` reference; platform/build context in `./landscape-and-domains.md#ibmd-rust-sdk-internal-c-pluginsd` |
| Internal C plugin | `src/collectors/README.md`, adjacent collector and `./landscape-and-domains.md#ibmd-rust-sdk-internal-c-pluginsd` |
| External plugin in another language | `src/plugins.d/README.md` for PLUGINSD |
| SNMP profile or trap profile | `.agents/skills/collectors-snmp-profiles/SKILL.md` or `.agents/skills/collectors-snmp-trap-profiles/SKILL.md`; metric profiles use `src/go/plugin/go.d/collector/snmp/profile-format.md` |
| Prometheus chart profile | `.agents/skills/collectors-prometheus-profiles/SKILL.md`; source and real-pipeline validation for the affected profile, with review action boundaries retained |
| Interactive Function | `src/go/plugin/framework/functions/README.md`, `src/plugins.d/FUNCTION_UI_SCHEMA.json`, `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md` |
| Topology producer | `.agents/skills/topology-authoring/SKILL.md`, `src/go/pkg/topology/v1`, `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` |
| Auto-discovery | Rules under `src/go/plugin/go.d/config/go.d/sd/`; engine `src/go/plugin/agent/discovery/` |
| OTEL ingestion or mapping | `src/crates/otel-plugin/`, `src/crates/otel-ingestor/` and `./dashboard-shaping.md` |
| Log ingestion or exploration | `src/collectors/log2journal/`, its `log2journal.d/` rules and `./landscape-and-domains.md#logs` |
| Cross-plugin enrichment | `./collector-practices.md#29-cross-plugin-enrichment-via-netipc` |
| Credentials or privileged operations | `src/collectors/SECRETS.md` and `src/collectors/utils/ndsudo.c` |

## Expand Only The Relevant Detail

- Metrics, labels, chart shape, grouping and priorities: `./collector-practices.md#3-structuring-dashboards`.
  SNMP, statsd, OTEL and Prometheus shaping mechanisms: `./dashboard-shaping.md`.
- Remote targets and generated nodes: `./collector-practices.md#19-remote-monitored-systems-and-vnodes` and the
  framework/design owners it cites. Multiple targets alone do not justify a vnode per target.
- Language/platform choice, build loop, eBPF migration, data types and domain patterns:
  `./landscape-and-domains.md`. Open the relevant section; preserve its CO-RE versus legacy validation distinction.
- Implementation quality and delivery checks:
  `./collector-practices.md#4-production-quality-criteria--pre-pr-checklist`.
  Reviewers use applicable criteria and existing evidence; authors complete the authorized validation.
- Other source owners: `./collector-practices.md#6-canonical-documentation-pointers`.

## Maintaining This Skill

Capture reusable, evidence-backed gaps under `AGENTS.md#knowledge-capture`. During authorized implementation, update
this skill and coupled pointers in the same task; answer-only findings follow the owner's local-note route. Keep
framework-specific mechanisms with their owners and preserve the domain detail when improving the entry.
When a pointer misled the task, capture the reusable failure mode with its correction. Describe coupled skill updates
in the PR when one is created; do not preserve obsolete instructions as current guidance.
