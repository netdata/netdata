# SNMP Trap Specs

This directory contains Netdata-owned SNMP trap product specs and decisions.

## What Belongs Here

- `netdata.md`: Netdata SNMP trap ingestion, enrichment, journal, OTLP, profile,
  and metric design contract.
- `trap-metrics-profiles.md`: profile-defined trap metrics, chart identity,
  source identity, resource identity, and cardinality contract.
- `netdata-snmp-hub-architecture.md`: distributed SNMP hub architecture
  principle for Netdata network observability.
- `decisions/`: accepted architecture decisions for the Netdata trap subsystem.

## Research Boundary

Research inputs MUST live under `research/`, not beside the Netdata specs.

Research includes:

- external-product studies;
- comparative matrices and stress tests;
- domain playbooks and skill distillations;
- current-state inventories used as design evidence.

Research can inform specs, but it is not itself a Netdata product contract.
When research evidence becomes a product decision, copy the durable decision or
contract into one of the top-level specs or into `decisions/`.
