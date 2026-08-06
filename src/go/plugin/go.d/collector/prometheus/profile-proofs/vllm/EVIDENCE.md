<!-- markdownlint-disable MD013 MD043 -->

# vLLM profile evidence record

## Responsibility

This document records upstream provenance, fixture-construction boundaries, and evidence limitations. It does not own
validator verdicts, counts, findings, file paths, or integrity facts; those are declared in `proof.yaml` and checked by the
descriptor-backed replay tests.

## Supported source boundary

- Running-source baseline: `vllm-project/vllm @ adf15cadb9d0151663b001a7286674892c4daa3c`.
- Source-completeness revision: `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- Application registrations and updates: `vllm/v1/metrics/{loggers,perf}.py`,
  `vllm/v1/spec_decode/metrics.py`, `vllm/distributed/kv_transfer/kv_connector/v1/**`,
  `vllm/v1/kv_offload/**`, and `vllm/entrypoints/speech_to_text/realtime/metrics.py` at the completeness revision.
- Documentation contract: `docs/design/metrics.md` and `docs/usage/metrics.md` at the completeness revision.
- Python runtime families follow the `prometheus_client` version bundled with the observed vLLM environment and are audited
  separately from vLLM-owned families.

## Availability boundary

- Speculative decoding, diffusion, tool parsing, realtime speech, and each KV transfer/offload connector are optional
  source-owned capabilities.
- Connector families cover mutually exclusive NIXL, Mooncake, HF3FS, CPU, and tiered-offload modes.
- Python runtime families depend on process and collector configuration.
- Pre-canonical KV-offload aliases expose the same observations as canonical families and require duplicate suppression.

## Fixture provenance

- A private read-only scrape established the native endpoint transport and one enabled subset. It is not a committed proof
  input and does not define the supported source surface.
- The committed fixture is a sanitized structural union built from pinned registrations, update callsites, and observed label
  shapes.
- Identities and values are synthetic and non-production. No private endpoint, served model, handler path, configuration
  label, credential, or operating value is committed.
- The external source inventory is the exact source-to-disposition ledger. `proof.yaml` declares the consumed fixture and
  verifies the latest external directory as one complete input set.

## Limitations

- The structural union is not one realizable endpoint.
- Source-only families prove schema and routing, not live enablement, value distribution, cadence, or cardinality.
- Live-Agent validation is a separate authorized rollout check and cannot replace or narrow source-completeness evidence.
