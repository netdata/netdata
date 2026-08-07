<!-- markdownlint-disable MD013 MD043 -->

# Python GC profile validation interpretation

## Responsibility

This document explains how to interpret the declared case. `proof.yaml` owns its fixture, expected machine facts, and local
integrity; the common replay test owns execution and exact inventory reconciliation.

## Source-complete case

The case composes the Python GC profile in isolation against the complete audited namespace. It proves that every
writer-capable source family and generation dimension is owned without generic fallback.

## Interpretation limits

The fixture is a structural proof input. It does not assert that every supported exporter enables Python GC metrics or that
the synthetic values represent healthy collection behavior.
