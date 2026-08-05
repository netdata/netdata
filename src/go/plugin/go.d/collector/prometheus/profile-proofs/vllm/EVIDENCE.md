<!-- markdownlint-disable MD013 MD043 -->

# vLLM profile evidence manifest

## Supported source boundary

- Running-source baseline: `vllm-project/vllm @ adf15cadb9d0151663b001a7286674892c4daa3c`.
- Source-completeness revision: `vllm-project/vllm @ dc818c198d3ff50a16f38eba567da006478239c8`.
- Primary registrations and updates: `vllm/v1/metrics/{loggers,perf}.py`, `vllm/v1/spec_decode/metrics.py`,
  `vllm/distributed/kv_transfer/kv_connector/v1/**`, `vllm/v1/kv_offload/**`, and
  `vllm/entrypoints/speech_to_text/realtime/metrics.py` at the source-completeness revision.
- Upstream documentation revision: `docs/design/metrics.md` and `docs/usage/metrics.md` at the source-completeness revision.
- Runtime collector families follow the `prometheus/client_python` version bundled with the running vLLM environment; their
  standard Python GC, process, and platform shapes are separated from vLLM-owned families in
  `src/go/testdata/prometheus/profiles/vllm/SOURCE-INVENTORY.tsv`.

## Feature and configuration gates

- Speculative decoding, diffusion, tool parsing, real-time WebSocket speech, and each KV-transfer/offload connector are
  optional source-owned capabilities.
- Connector families include NIXL, Mooncake, HF3FS, and CPU/tiered KV offload; mutually exclusive connector/model modes are
  represented together only in the structural fixture.
- Python runtime families depend on the process and `prometheus_client` collector configuration.
- Deprecated pre-canonical KV-offload aliases are exact duplicate observations and are excluded by `VALIDATION-JOB.yaml`.

## Evidence classes and fixture provenance

- A read-only local scrape established the native endpoint transport and enabled subset. It remains private because it carries
  deployment labels and values; it does not define the supported surface.
- `src/go/testdata/prometheus/profiles/vllm/fixtures/vllm_all_metrics.prom` is a sanitized synthetic union built from the
  pinned registrations, update callsites, and observed label shapes. Placeholder values and identities are non-production.
- Source-only families prove schema and routing, not live enablement, cadence, value distribution, or cardinality.
- `src/go/testdata/prometheus/profiles/vllm/SOURCE-INVENTORY.tsv` binds every declared family/component to source, owner,
  identity, population, unit algebra, availability gate, disposition, and authored destination.
- `src/go/testdata/prometheus/profiles/vllm/manifest.yaml` records the byte size and SHA-256 digest of every external input;
  `proof.yaml` pins that manifest from the Netdata proof.

## Reproduction and integrity

- `VALIDATION-JOB.yaml` is the sanitized objective-validator input corresponding to the recommended metadata job policy.
- `VALIDATION.md` explains the result and exact command.
- `proof.yaml` records the authoritative expected facts and fingerprints every committed semantic and executable proof input.
- The result requires zero generic fallback and zero unmatched series for the declared source union. Unknown future `vllm:*`
  families remain eligible for generic fallback and are outside that bounded completeness claim.

## Explicit limitations

- The structural union is not one realizable endpoint.
- The proof does not assert that every optional capability is enabled on a particular deployment.
- Live-Agent behavior is validated separately during an authorized rollout and cannot replace source-completeness evidence.
