<!-- markdownlint-disable MD013 MD043 -->

# LiteLLM profile evidence manifest

## Supported source boundary

- Observed application: LiteLLM 1.92.0, `BerriAI/litellm @ b3086ccd74553565c9a39716e72303ae985555f9`.
- Structural-union source: `BerriAI/litellm @ 23de7a15d9d40006ee596e617475ba101d60c5e9`.
- Current-source comparison: `BerriAI/litellm @ de706a35a6f1e9cb8c3cb527271df0b76a69f410`.
- Primary source and documentation contract: `litellm/integrations/prometheus.py`,
  `litellm/integrations/prometheus_services.py`, `litellm/integrations/prometheus_helpers/**`,
  `litellm/types/integrations/prometheus.py`, and every metric update callsite reconciled in `SOURCE-INVENTORY.tsv`.
- Runtime collector contract: `prometheus/client_python 0.24.1 @ f417f6ea8f058165a1934e368fed245e91aafc14`,
  `prometheus_client/{gc_collector,platform_collector,process_collector}.py`.

## Feature and configuration gates

- `prometheus_metrics_config` controls exported metric families, included labels, custom labels, service metrics, and optional
  user-budget identity.
- Guardrails, callbacks, routing/fallback, budgets, spend tracking, MCP, media, managed batches/files, Redis, PostgreSQL, and
  spend-update queues appear only when their corresponding LiteLLM paths are enabled.
- Single-process Python GC/process/platform families and multiprocess `pid` shapes are mutually configuration-dependent.
- Optional labels may be absent. They are never required dimensions; bounded optional classifications use explicit fallback
  routes, while state gauges retain the complete identity LiteLLM actually exports.

## Evidence classes and fixture provenance

- A local authenticated scrape established LiteLLM 1.92.0 transport and the enabled subset. It remains private and does not
  define the supported surface.
- `src/go/plugin/go.d/collector/prometheus/testdata/litellm_all_metrics.prom` is a sanitized structural union of observed and
  source-only callback, label-filtered, single-process, and multiprocess shapes. Placeholder identities are synthetic.
- Source registration, HELP, type, bucket, and service files used by the fixture are byte-identical across the structural and
  current comparison revisions; update callsites supply the label and optionality evidence.
- `SOURCE-INVENTORY.tsv` binds every declared family/component to source, owner, identity, population, unit algebra,
  availability gate, disposition, and authored destination.

## Reproduction and integrity

- `VALIDATION-JOB.yaml` is the sanitized objective-validator input corresponding to the recommended metadata job policy.
- `VALIDATION.md` contains the authoritative result and exact command.
- `SHA256SUMS.tsv` fingerprints every committed semantic and executable proof input.
- The result requires zero generic fallback and zero unmatched series for the declared source union. Unknown future
  `litellm_*` families remain eligible for generic fallback.

## Explicit limitations

- The structural union is not one realizable LiteLLM configuration.
- Source-only shapes prove schema and routing, not live enablement, value distribution, cadence, or deployment cardinality.
- Live-Agent behavior is validated separately during an authorized rollout and cannot replace source-completeness evidence.
