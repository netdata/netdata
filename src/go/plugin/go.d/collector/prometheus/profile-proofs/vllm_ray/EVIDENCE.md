<!-- markdownlint-disable MD013 MD043 -->

# vLLM Ray profile evidence record

## Responsibility

This document records upstream provenance, fixture-construction boundaries, and evidence limitations. It does not own
validator verdicts, counts, findings, file paths, or integrity facts; those are declared in `proof.yaml` and checked by the
descriptor-backed replay tests.

## Supported source boundary

- vLLM metric contract: `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- Ray transport contract: `ray-project/ray @ 03491225d59a1ffde99c3628969ccf456be13efd` (Ray 2.48.0).
- Primary contracts: the vLLM registration paths cited by the native profile evidence plus Ray
  `src/ray/stats/metric_exporter.cc` and the Ray metrics-export implementation at the pinned revisions.
- vLLM documentation contract: `docs/design/metrics.md` and `docs/usage/metrics.md` at the pinned revision.

## Availability boundary

- Ray adds replica, worker, session, version, and component labels to enabled vLLM families.
- `RAY_EXPORT_COUNTER_AS_GAUGE` controls compatibility aliases for counters; the aliases represent the same observations as
  canonical counter families.
- Native vLLM capability and connector gates still determine the underlying family surface.
- A Ray node endpoint may also expose unrelated Ray system families; this profile owns only the Ray-vLLM namespace.

## Fixture provenance

- The committed fixture is a sanitized source-derived structural union. No live Ray deployment was used or claimed.
- The fixture combines mutually optional vLLM capabilities and Ray alias behavior with synthetic, non-production identities
  and values.
- The external source inventory is the exact source-to-disposition ledger. `proof.yaml` declares the consumed fixture and
  verifies the latest external directory as one complete input set.

## Limitations

- The structural union is not one realizable Ray endpoint.
- Local runtime evidence covers native vLLM only; Ray-specific enablement, values, cadence, and cardinality remain unobserved.
- Live-Agent validation is a separate authorized rollout check and cannot replace or narrow source-completeness evidence.
