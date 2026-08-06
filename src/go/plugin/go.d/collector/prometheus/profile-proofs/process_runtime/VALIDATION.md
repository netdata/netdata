<!-- markdownlint-disable MD013 MD043 -->

# Process runtime profile validation interpretation

## Responsibility

This document explains how to interpret the declared case. `proof.yaml` owns its fixture, expected machine facts, and local
integrity; the common replay test owns execution and exact inventory reconciliation.

## Source-complete case

The case composes the runtime profile in isolation against the complete audited process surface. It proves that every
chartable selector is owned, the process-start epoch is removed by profile policy, and unsupported Python metadata does not
leak into generic charts.

## Interpretation limits

The fixture is a structural proof input. It does not assert that every supported exporter enables the same process collector
surface or that the synthetic values represent a healthy process.
