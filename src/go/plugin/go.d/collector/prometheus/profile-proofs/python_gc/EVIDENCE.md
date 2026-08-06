<!-- markdownlint-disable MD013 MD043 -->

# Python GC profile evidence record

## Responsibility

This document records provenance, fixture-construction boundaries, and evidence limitations. `proof.yaml` and the replay
tests own all machine-verifiable paths, counts, findings, and integrity facts.

## Supported source boundary

- The Python garbage-collector counter families are the default `prometheus_client` runtime surface exposed by the vLLM
  environment audited at `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- The shared profile covers collected objects, uncollectable objects, and collection cycles with the standard `generation`
  label shape.

## Fixture provenance

- The fixture is a sanitized structural subset of the source-derived vLLM fixture.
- Generation labels and metric values are synthetic and non-production.
- The external source inventory is the exact source-to-disposition ledger; `proof.yaml` declares the consumed fixture and
  verifies the latest external directory as one complete input set.

## Limitations

- This proof establishes the common Python GC collector shape, not every Python runtime or client-library version.
- It proves schema and routing, not live enablement, values, cadence, or cardinality.
