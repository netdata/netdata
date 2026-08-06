<!-- markdownlint-disable MD013 MD043 -->

# vLLM profile validation interpretation

## Responsibility

This document explains how to interpret the declared validation cases. `proof.yaml` owns every fixture selection, job mode,
expected verdict, count, and finding-code count. The common replay test owns execution and exact source-inventory
reconciliation; this file intentionally repeats none of those machine facts.

## Source-complete case

The source-complete fixture combines optional connectors, model modes, feature gates, and runtime collectors. Its case tests
the declared source boundary through the recommended job and profile; it does not claim that one vLLM endpoint emits the
complete union.

Inventory reconciliation compares the exact raw-family and authored-selector sets. This separates source coverage from
operator judgment about entity identity, observation population, unit algebra, and causal dashboard placement.

## Job-policy case

The no-job case makes the recommended job dependency explicit. Job policy suppresses source-known creation epochs, raw
process-start time, and duplicate pre-canonical KV-offload observations. The stock profile retains exact defensive exclusions
inside its own namespace while leaving unknown future vLLM families open to fallback. The executable case, not this prose,
owns the resulting behavior.

## Interpretation limits

- A partial runtime endpoint can legitimately leave charts for optional capabilities inactive; strict source-completeness
  validation of that partial input is not a collector-health verdict.
- Unsupported information gauges and summaries retain their lost operator questions in the semantic inventory rather than
  being treated as chartable scalar suffixes.
- The private scrape proves transport and one enabled subset only. Live deployment checks are operational rollout evidence,
  not a replacement for the source-complete case.
