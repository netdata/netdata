<!-- markdownlint-disable MD013 MD043 -->

# vLLM Ray profile validation interpretation

## Responsibility

This document explains how to interpret the declared validation cases. `proof.yaml` owns every fixture selection, job mode,
expected verdict, count, and finding-code count. The common replay test owns execution and exact source-inventory
reconciliation; this file intentionally repeats none of those machine facts.

## Source-complete case

The source-complete fixture combines optional native vLLM capabilities with Ray transport labels and compatibility behavior.
Its case tests the declared source boundary through the recommended job and profile; it does not claim that one Ray endpoint
emits the complete union.

Inventory reconciliation compares the exact raw-family and authored-selector sets. This distinguishes source coverage from
the human decision to keep replica and worker labels as identity while treating bounded outcomes as dimensions.

## Without-job case

The no-job case proves that the profile removes source-known Ray counter-as-gauge compatibility aliases and pre-canonical
KV-offload duplicates while retaining canonical observations. No application sample job policy is required, and unknown
future Ray-vLLM families remain open to fallback. The executable case, not this prose, owns the resulting behavior.

## Interpretation limits

- Information gauges rejected by the writer retain their lost configuration questions in the semantic inventory.
- The fixture proves a source-derived transport contract, not live Ray enablement or one deployment's cardinality.
- A future authorized Ray rollout can add operational evidence without changing the declared source-completeness boundary.
