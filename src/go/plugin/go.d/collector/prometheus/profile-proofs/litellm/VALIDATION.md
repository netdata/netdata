<!-- markdownlint-disable MD013 MD043 -->

# LiteLLM profile validation interpretation

## Responsibility

This document explains how to interpret the declared validation cases. `proof.yaml` owns every fixture selection, job mode,
expected verdict, count, and finding-code count. The common replay test owns execution and exact source-inventory
reconciliation; this file intentionally repeats none of those machine facts.

## Source-complete case

The source-complete fixture combines mutually optional callbacks, labels, runtime modes, and application features. Its case
tests the declared source boundary through the recommended job and profile; it does not claim that one LiteLLM endpoint emits
the complete union.

Inventory reconciliation compares the exact raw-family and authored-selector sets. This separates source coverage from
operator judgment about identity, aggregation, units, and dashboard placement.

## Without-job case

The no-job case proves that source-known exclusions and untyped classification no longer depend on application sample job
policy. The application and shared profiles retain complete current coverage while leaving unknown future LiteLLM families
open to generic fallback. The exact behavior and findings are executable claims in `proof.yaml`.

## Interpretation limits

- Optional breakdowns may disappear when an operator filters their identity labels; the coarser additive view remains the
  intended complete view.
- Point-in-time gauges preserve their emitted identity rather than borrowing counter aggregation rules.
- The private scrape proves transport and one enabled subset only. Live deployment checks are operational rollout evidence,
  not a replacement for the source-complete case.
