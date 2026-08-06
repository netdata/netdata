<!-- markdownlint-disable MD013 MD043 -->

# Process runtime profile evidence record

## Responsibility

This document records provenance, fixture-construction boundaries, and evidence limitations. `proof.yaml` and the replay
tests own all machine-verifiable paths, counts, findings, and integrity facts.

## Supported source boundary

- The process collector families are the default `prometheus_client` runtime surface exposed by the vLLM environment
  audited at `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- The shared profile covers process resource scalars with their standard shapes.
- Python implementation metadata is retained in the evidence boundary even though the collector writer cannot represent it
  as an operational scalar chart.

## Fixture provenance

- The fixture is a sanitized structural subset of the source-derived vLLM fixture.
- Labels and values are synthetic and non-production.
- The external source inventory is the exact source-to-disposition ledger; `proof.yaml` declares the consumed fixture and
  verifies the latest external directory as one complete input set.

## Limitations

- This proof establishes the common process collector shape, not every language runtime or optional client-library collector.
- It proves schema and routing, not live enablement, values, cadence, or cardinality.
