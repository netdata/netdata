<!-- markdownlint-disable MD013 MD043 -->

# FastAPI profile evidence record

## Responsibility

This document records provenance, fixture-construction boundaries, and evidence limitations. `proof.yaml` and the replay
tests own all machine-verifiable paths, counts, findings, and integrity facts.

## Supported source boundary

- The audited HTTP families are emitted by the FastAPI instrumentation bundled with
  `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- The surface includes route request counters, coarse per-route duration histograms, a service-wide high-resolution duration
  histogram, request/response-size summaries, and generated registration timestamps.

## Fixture provenance

- The fixture is a sanitized structural subset of the source-derived vLLM fixture.
- Handler identities, status values, and metric values are synthetic and non-production.
- The external source inventory is the exact source-to-disposition ledger; `proof.yaml` declares the consumed fixture and
  verifies the latest external directory as one complete input set.

## Limitations

- The proof covers the audited instrumentation shape, not every FastAPI metrics middleware or custom bucket configuration.
- It proves schema and routing, not live enablement, values, cadence, or endpoint cardinality.
