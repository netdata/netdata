<!-- markdownlint-disable MD013 MD043 -->

# Process runtime Prometheus operator model

## Responsibility

This document owns the human semantic decisions for the shared runtime surface. The external inventory owns exact family and
selector dispositions; `proof.yaml` and replay tests own executable behavior.

## Entity and signal model

- The collector job is the process identity; these metrics do not expose a safer subordinate process key.
- CPU time is a counter rendered incrementally as cores.
- Resident and virtual memory are separate absolute resource views.
- Open descriptors and the descriptor limit are separate utilization and capacity views.

## Exclusion semantics

- Process start time is an epoch gauge. The profile cannot derive uptime from wall-clock time, so profile relabeling removes
  it instead of charting a misleading age or raw timestamp.
- Python implementation metadata remains in the evidence boundary because it accompanies this source surface, but it
  requires a metadata-capable path and is not represented as a numeric chart.

## Composition boundary

The profile deliberately has no `app`. The selected job supplies the application namespace, so the same runtime views can
compose with multiple application profiles without assigning them to a generic plugin-wide context.
