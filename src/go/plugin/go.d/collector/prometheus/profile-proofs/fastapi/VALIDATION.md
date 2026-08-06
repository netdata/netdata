<!-- markdownlint-disable MD013 MD043 -->

# FastAPI profile validation interpretation

## Responsibility

This document explains how to interpret the declared case. `proof.yaml` owns its fixture, expected machine facts, and local
integrity; the common replay test owns execution and exact inventory reconciliation.

## Source-complete case

The case validates the shared profile in isolation against the complete audited HTTP instrumentation surface. It proves that
chartable request and duration components are owned, registration timestamps are removed by profile policy, and unsupported
summary shapes do not leak into generic charts.

## Interpretation limits

The fixture is a structural proof input. It does not assert that every FastAPI application uses this middleware, the same
bucket configuration, or the same handler cardinality.
