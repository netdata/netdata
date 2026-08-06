<!-- markdownlint-disable MD013 MD043 -->

# Ceph profile validation interpretation

## Responsibility

This document explains how to interpret the declared validation cases. `proof.yaml` owns every fixture selection, job mode,
expected verdict, count, and finding-code count. The common replay test owns execution and exact source-inventory
reconciliation; this file intentionally repeats none of those machine facts.

## Source-complete case

The source-complete case combines mutually optional releases, producer roles, daemon roles, modules, and transports. A clean
result means the profile and recommended job account for the declared source boundary as a structural contract. It does not
mean one Ceph endpoint can emit the complete union.

Profile-owned relabeling normalizes source-proven dynamic family-name grammars before chart selection. Inventory
reconciliation compares raw source families and authored selectors by exact set, so normalization is not accepted as an
unexplained disappearance.

## Producer-specific cases

Each supplemental producer fixture intentionally omits families belonging to other releases or producers. Strict validation
therefore diagnoses inactive charts/dimensions and may also report that a selector or relabel branch cannot be proven from
that partial input. Those findings describe the evidence boundary; their exact classes and counts remain executable claims in
`proof.yaml`.

The cases exist to ensure that release-specific wire types, dynamic-name normalization, producer exclusions, and current
family shapes keep behaving as declared without pretending that a partial fixture is source-complete.

## Job-policy case

The no-job case makes the recommended job dependency explicit. It exercises behavior without selector, fallback, or
validation-only future-input policy, including the collector's treatment of otherwise untyped Ceph families. The expected
behavior belongs only to `proof.yaml`.

## Operational boundary

Live-Agent checks verify deployment transport and rollout behavior on authorized systems. They do not change the pinned
source boundary, and this proof does not modify any Ceph daemon, exporter, or cluster configuration.
