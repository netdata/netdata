# SNMP Trap Research

This directory contains evidence used to design and document Netdata SNMP traps.
It is intentionally separate from the Netdata-owned specs one level above.

## Subdirectories

- `domain/`: general SNMP trap observability domain research.
- `playbooks/`: operational playbooks and skill-distillation source material.
- `netdata-existing/`: inventories of existing Netdata subsystems used as
  design inputs.
- `external-systems/`: per-product studies of other trap implementations.
- `comparison/`: cross-system matrices, stress tests, and synthesis.

## Contract Boundary

Files in this directory are research evidence only. They MUST NOT be treated as
Netdata product specs unless a top-level spec or decision explicitly adopts a
specific rule.

When citing research from a Netdata spec, use the research path so readers know
the source is evidence, not the contract itself.
