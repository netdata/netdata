<!-- markdownlint-disable MD013 MD043 -->

# LiteLLM profile evidence record

## Responsibility

This document records upstream provenance, fixture-construction boundaries, and evidence limitations. It does not own
validator verdicts, counts, findings, file paths, or integrity facts; those are declared in `proof.yaml` and checked by the
descriptor-backed replay tests.

## Supported source boundary

- Observed application: LiteLLM 1.92.0, `BerriAI/litellm @ b3086ccd74553565c9a39716e72303ae985555f9`.
- Structural-union source: `BerriAI/litellm @ 23de7a15d9d40006ee596e617475ba101d60c5e9`.
- Current-source comparison: `BerriAI/litellm @ de706a35a6f1e9cb8c3cb527271df0b76a69f410`.
- Application contracts: `litellm/integrations/prometheus.py`, `prometheus_services.py`,
  `prometheus_helpers/**`, `litellm/types/integrations/prometheus.py`, and the metric update callsites recorded by the source
  inventory.
- Runtime collector contract: `prometheus/client_python 0.24.1 @ f417f6ea8f058165a1934e368fed245e91aafc14`.

## Availability boundary

- `prometheus_metrics_config` controls enabled families, included labels, custom labels, service metrics, and user-budget
  identity.
- Guardrails, routing, budgets, spend, MCP, media, managed jobs/files, databases, caches, and background services are gated by
  the corresponding LiteLLM feature paths.
- Single-process runtime collectors and multiprocess shapes are mutually configuration-dependent.
- Optional labels may be absent. The operator model defines which views remain complete when they are filtered.

## Fixture provenance

- A private authenticated scrape established transport and one enabled subset. It is not a committed proof input and does not
  define the supported source surface.
- The committed fixture is a sanitized structural union of observed and source-only callback, label-filtered,
  single-process, and multiprocess shapes.
- Identities and values are synthetic and non-production. No private endpoint, credential, deployment label, or operating
  value is committed.
- The external source inventory is the exact source-to-disposition ledger. The external manifest authenticates the inventory
  and fixture; `proof.yaml` pins that manifest.

## Limitations

- The structural union is not one realizable LiteLLM configuration.
- Source-only shapes prove schema and routing, not live enablement, value distribution, cadence, or deployment cardinality.
- Live-Agent validation is a separate authorized rollout check and cannot replace or narrow source-completeness evidence.
