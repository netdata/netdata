# TODO: NetFlow Plugin Code Organization

## Purpose

Reorganize the Rust netflow plugin source tree so it is fit for long-term maintenance:

- no logic changes
- organize files and modules by meaning, not by previous file size accidents
- make ownership boundaries obvious
- reduce oversized files and oversized functions further
- make future work on protocols, enrichment, storage, and queries safer and easier

## TL;DR

- Costa requested a second organization pass after the large-file refactor.
- The goal is not behavior change. The goal is better module ownership, better naming, and smaller single-purpose files/functions.
- The current code is materially better than before, but several large roots still behave like monoliths split by `include!()` rather than real module boundaries.

## Requirements

- Preserve logic and behavior.
- Preserve wire/protocol support and existing decoding semantics.
- Preserve query behavior and output shape.
- Preserve storage layout and tier semantics.
- Preserve runtime configuration keys and config semantics.
- Reorganize files by domain meaning.
- Break up oversized functions where responsibility boundaries are still mixed.
- Prefer real Rust modules/directories over pseudo-modules via `include!()`.
- Do not split small or cohesive files just because they appear near the top of a remaining size list.
- Any further split must have a strong meaning boundary and a clear maintainer benefit.
- Keep tests strong during the refactor.

## Current Status

- The first refactor pass already split the largest monoliths into smaller source files.
- 2026-03-27 implementation progress:
  - Phase 1 is complete:
    - `decoder`, `query`, and `enrichment` no longer use `include!()`
    - they now use real submodule directories under:
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/`
      - `src/crates/netdata-netflow/netflow-plugin/src/query/`
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/`
  - Phase 2 is complete:
    - shared flow model ownership moved out of `decoder`
    - new shared module:
      - `src/crates/netdata-netflow/netflow-plugin/src/flow.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/flow/schema.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/flow/record.rs`
  - Phase 3 is complete:
    - decoder protocol logic is now split by meaning under:
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/entry.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/legacy.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow.rs`
    - `decoder/protocol.rs` is now a thin root module only
    - decoder state logic is now split by meaning under:
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/models.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/persisted.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore.rs`
    - `decoder/state.rs` is now a thin root module only
  - Phase 4 is partially complete:
    - `query.rs` is now reduced to service entrypoints and output/materialization helpers
    - `query/planner.rs`, `query/facets.rs`, and `query/scan.rs` now own their corresponding service responsibilities
    - query-local shadow types were renamed:
      - `FlowRecord` -> `QueryFlowRecord`
      - `FlowMetrics` -> `QueryFlowMetrics`
    - several generic planning and scan helpers moved out of `query/timeseries.rs`
    - query projected journal helpers now live in:
      - `src/crates/netdata-netflow/netflow-plugin/src/query/projected.rs`
    - query field and virtual-field helpers now live in:
      - `src/crates/netdata-netflow/netflow-plugin/src/query/fields.rs`
    - `query/planner.rs` now owns time-bucket and tier-span planning helpers
    - `query/facets.rs` now owns facet vocabulary and facet accumulator helpers
    - `src/crates/netdata-netflow/netflow-plugin/src/query/timeseries.rs` now mainly owns
      timeseries bucket accumulation, chart output, and warning payloads
  - Phase 5 is partially complete:
    - dynamic routing runtime types moved out of `enrichment.rs` into:
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/runtime.rs`
    - BMP and BioRIS listeners moved under:
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris.rs`
    - BMP listener logic is now split by meaning under:
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/listener.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/session.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/rd.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/tests.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp.rs` is now a thin root module
    - routing root module added:
      - `src/crates/netdata-netflow/netflow-plugin/src/routing.rs`
    - network source runtime types moved out of `enrichment.rs` into:
      - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/runtime.rs`
    - network source refresher moved under:
      - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/mod.rs`
    - `FlowEnricher::from_config()`, `dynamic_routing_runtime()`, and
      `network_sources_runtime()` behavior stayed unchanged in this batch
  - Phase 6 is partially complete:
    - flow-function API DTOs, parameter metadata builders, and handler code moved out of
      `main.rs` into:
      - `src/crates/netdata-netflow/netflow-plugin/src/api/flows.rs`
    - API root module added:
      - `src/crates/netdata-netflow/netflow-plugin/src/api.rs`
    - `main.rs` now mainly owns bootstrap, task wiring, and shutdown flow
    - `ingest.rs` is now split by meaning under:
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/metrics.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/persistence.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/encode.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs` is now a thin root module
    - `ingest/service.rs` is now split by meaning under:
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/init.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/tiers.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service.rs` is now a thin root module
    - `plugin_config.rs` is now split by meaning under:
      - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/defaults.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config.rs` is now a thin root module
    - config loading, GeoIP auto-detection, and validation behavior stayed unchanged in this batch
    - `tiering.rs` is now split by meaning under:
      - `src/crates/netdata-netflow/netflow-plugin/src/tiering/model.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/tiering/tests.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs` is now a thin root module
    - tier metrics, open-tier state, rollup schema/materialization, and tests stayed behavior-neutral in this batch
  - 2026-03-28 final reasonable batch is complete:
    - `plugin_config/validation.rs` is now split by validation domain under:
      - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation/listener.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation/journal.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation/enrichment.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation.rs` is now a thin root module
    - `decoder/record/core.rs` is now split by ownership under:
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core/common.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core/fields.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core/record.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core.rs` is now a thin root module
    - `tiering/index.rs` is now split by runtime responsibility under:
      - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index/store.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index/accumulator.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index.rs` is now a thin root module
    - `query/scan/raw.rs` remained one file on purpose, but its oversized projected raw scan path is now decomposed into local helpers for:
      - prefilter construction
      - row scratch reset
      - per-file raw scan orchestration
      - payload iteration and decode application
      - final selection gate
    - no further file splitting is currently justified by meaning boundaries alone
- Current plugin source tree size:
  - total: `35,591` lines across `328` Rust files under `src/crates/netdata-netflow/netflow-plugin/src/`
  - largest test-only files now:
    - `query/scan/bench.rs`: `470`
    - `plugin_config_tests.rs`: `349`
  - largest production files now:
    - `query/scan/raw.rs`: `307`
    - `main.rs`: `276`
    - `ingest/metrics.rs`: `234`
    - `decoder/record/values.rs`: `225`
    - `decoder/protocol/v9/special.rs`: `222`
    - `routing/bioris/runtime/observe.rs`: `221`
- Current key file sizes after this batch:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs`: `173`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol.rs`: `17`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/entry.rs`: `168`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge/identity.rs`: `108`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge/enrich.rs`: `165`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/akvorado.rs`: `46`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/datalink.rs`: `77`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/ip.rs`: `93`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/transport.rs`: `84`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/legacy.rs`: `130`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/records.rs`: `117`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/sampling.rs`: `187`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/special.rs`: `222`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/templates.rs`: `111`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates.rs`: `11`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates/entry.rs`: `70`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates/data.rs`: `63`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates/options.rs`: `49`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates/v9.rs`: `84`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special/packet.rs`: `76`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special/parser.rs`: `43`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special/record.rs`: `184`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record.rs`: `11`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/state.rs`: `181`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/fields.rs`: `74`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/finalize.rs`: `82`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/append.rs`: `69`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow/packet.rs`: `113`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow/record.rs`: `182`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state.rs`: `13`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/models.rs`: `205`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling/types.rs`: `59`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling/state.rs`: `134`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling/templates.rs`: `149`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/init.rs`: `121`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/decode.rs`: `65`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/namespace.rs`: `83`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/observe.rs`: `43`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/persist.rs`: `95`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/persisted.rs`: `73`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore.rs`: `24`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore/v9.rs`: `95`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore/ipfix.rs`: `169`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs`: `69`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply.rs`: `88`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/context.rs`: `57`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/metadata.rs`: `124`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/resolve.rs`: `51`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/write.rs`: `191`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classify.rs`: `164`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/init.rs`: `96`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/resolve.rs`: `148`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data.rs`: `11`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/static_data.rs`: `201`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip.rs`: `10`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/types.rs`: `80`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/files.rs`: `77`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/decode.rs`: `77`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/resolver.rs`: `108`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network.rs`: `15`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/asn.rs`: `43`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/attrs.rs`: `105`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/csv.rs`: `28`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/test_support.rs`: `83`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/write.rs`: `91`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/prefix.rs`: `105`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/runtime.rs`: `208`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp.rs`: `34`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/listener.rs`: `75`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/session.rs`: `236`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes.rs`: `15`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/apply.rs`: `121`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/aspath.rs`: `53`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/helpers.rs`: `19`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/nlri.rs`: `182`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/rd.rs`: `66`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/tests.rs`: `265`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris.rs`: `39`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime.rs`: `62`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/model.rs`: `43`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh.rs`: `5`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh/task.rs`: `106`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh/instance.rs`: `77`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh/dump.rs`: `81`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/observe.rs`: `221`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/client.rs`: `61`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/route.rs`: `105`
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/tests.rs`: `222`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/mod.rs`: `28`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/types.rs`: `46`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/service.rs`: `141`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/fetch.rs`: `118`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/decode.rs`: `56`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/transform.rs`: `58`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/tests.rs`: `171`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/runtime.rs`: `41`
  - `src/crates/netdata-netflow/netflow-plugin/src/main.rs`: `276`
  - `src/crates/netdata-netflow/netflow-plugin/src/api/flows.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/api/flows/model.rs`: `98`
  - `src/crates/netdata-netflow/netflow-plugin/src/api/flows/params.rs`: `141`
  - `src/crates/netdata-netflow/netflow-plugin/src/api/flows/handler.rs`: `142`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs`: `61`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service.rs`: `45`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/init.rs`: `168`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs`: `173`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/tiers.rs`: `185`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/metrics.rs`: `234`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs`: `183`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/persistence.rs`: `163`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest/encode.rs`: `56`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config.rs`: `25`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/defaults.rs`: `128`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types.rs`: `15`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/listener.rs`: `33`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/protocol.rs`: `66`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs`: `208`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/routing.rs`: `156`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment.rs`: `17`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/sampling.rs`: `8`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/providers.rs`: `34`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/metadata.rs`: `52`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/geoip.rs`: `12`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/networks.rs`: `31`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/sources.rs`: `73`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/root.rs`: `77`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/plugin.rs`: `50`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs`: `139`
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation.rs`: `233`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs`: `12`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/model.rs`: `91`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index.rs`: `231`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup.rs`: `14`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema.rs`: `16`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/direction.rs`: `21`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/presence.rs`: `48`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs.rs`: `49`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/core.rs`: `19`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/exporter.rs`: `13`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/interface.rs`: `18`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/network.rs`: `24`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/presence.rs`: `19`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/index.rs`: `15`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode.rs`: `39`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/core.rs`: `90`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/exporter.rs`: `64`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/interface.rs`: `93`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/network.rs`: `130`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/presence.rs`: `99`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/materialize.rs`: `112`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/tests.rs`: `202`
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`: `63`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/service.rs`: `57`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/flows.rs`: `5`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/flows/service.rs`: `180`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/flows/build.rs`: `114`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/flows/materialize.rs`: `50`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/metrics.rs`: `128`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request.rs`: `13`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/constants.rs`: `77`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model.rs`: `5`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model/types.rs`: `117`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model/normalize.rs`: `50`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model/deserialize.rs`: `65`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/output.rs`: `65`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/projected.rs`: `89`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/selection.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/selection/types.rs`: `15`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/selection/decode.rs`: `108`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/selection/normalize.rs`: `98`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/request/selection/extract.rs`: `50`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/projected.rs`: `11`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/projected/prefix.rs`: `75`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/projected/apply.rs`: `181`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/projected/bench_support.rs`: `135`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields.rs`: `15`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/capture.rs`: `67`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/metrics.rs`: `25`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/payload.rs`: `18`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/rules.rs`: `56`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/test_support.rs`: `43`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/virtuals.rs`: `54`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/timeseries.rs`: `141`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/planner.rs`: `20`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/request.rs`: `54`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/spans.rs`: `105`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/timeseries.rs`: `87`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/prepare.rs`: `169`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/facets.rs`: `6`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/service.rs`: `156`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/scan.rs`: `151`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/vocabulary.rs`: `42`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/payload.rs`: `85`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/render.rs`: `154`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping.rs`: `10`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model.rs`: `10`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/core.rs`: `48`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/compact.rs`: `134`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/projected.rs`: `200`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/grouped.rs`: `90`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/labels.rs`: `202`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build.rs`: `11`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate/grouped.rs`: `103`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate/compact.rs`: `105`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate/open_tier.rs`: `61`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/rank.rs`: `186`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/output.rs`: `97`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/scan.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/selection.rs`: `43`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/session.rs`: `4`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/session/records.rs`: `119`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/session/projected.rs`: `181`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/raw.rs`: `197`
  - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/bench.rs`: `469`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/columns.rs`: `81`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/display.rs`: `124`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels.rs`: `46`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/common.rs`: `23`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/protocol.rs`: `86`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/ip.rs`: `56`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/tcp.rs`: `32`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/icmp.rs`: `209`
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation/tests.rs`: `75`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime.rs`: `11`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/model.rs`: `136`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval.rs`: `27`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/boolean.rs`: `79`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition.rs`: `6`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/types.rs`: `17`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/context.rs`: `87`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/compare.rs`: `96`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/text.rs`: `43`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/action.rs`: `139`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value.rs`: `10`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value/expr.rs`: `94`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value/field.rs`: `143`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value/resolved.rs`: `35`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/boolean.rs`: `170`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/action.rs`: `119`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/value.rs`: `152`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split.rs`: `7`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split/separator.rs`: `106`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split/keyword.rs`: `147`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split/delimiters.rs`: `56`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/helpers.rs`: `66`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/core.rs`: `184`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/presence.rs`: `153`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields.rs`: `5`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/parse.rs`: `23`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export.rs`: `35`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export/helpers.rs`: `25`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export/exporter.rs`: `61`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export/endpoints.rs`: `81`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export/interfaces.rs`: `83`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export/headers.rs`: `62`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/import.rs`: `197`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal.rs`: `38`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal/writer.rs`: `129`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal/core.rs`: `27`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal/network.rs`: `36`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal/interfaces.rs`: `29`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal/transport.rs`: `21`
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal/headers.rs`: `25`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record.rs`: `15`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core.rs`: `233`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters.rs`: `49`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/exporter.rs`: `63`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/counter.rs`: `31`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/network.rs`: `133`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/interface.rs`: `69`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/transport.rs`: `107`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet.rs`: `13`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/mappings.rs`: `161`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/agent.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/datalink.rs`: `67`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/ip.rs`: `93`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/transport.rs`: `80`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/sampling.rs`: `70`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields.rs`: `9`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields/common.rs`: `154`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields/sampling.rs`: `95`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields/special.rs`: `104`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/mappings.rs`: `180`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/values.rs`: `225`

## Verification Results

- Passed in `src/crates/netdata-netflow/netflow-plugin/`:
  - `cargo fmt`
  - `cargo check --all-targets`
  - `cargo test`
- Latest full test result on `2026-03-28`:
  - `257` passed
  - `0` failed
  - `7` ignored

## Analysis

### Current Remaining Structural Issues (2026-03-28 after the final reasonable batch)

- There are no remaining high-confidence file splits justified purely by meaning boundaries.
  - evidence:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/raw.rs` is still the largest production file at `307` lines, but it is now one coherent raw projected-scan module with the oversized path decomposed into local helpers
    - `src/crates/netdata-netflow/netflow-plugin/src/main.rs` is `276` lines and remains a cohesive bootstrap/wiring root
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest/metrics.rs` is `234` lines and remains a single-purpose metrics state owner
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs` is `208` lines and remains a cohesive journal config owner
  - implication:
    - additional file splitting at this point would mostly be style churn and would weaken navigability instead of improving it

- The final reasonable split candidates from the focused re-scan are now complete:
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation.rs`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core.rs`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index.rs`
  - function-level refactor in `src/crates/netdata-netflow/netflow-plugin/src/query/scan/raw.rs`

- Further work, if any, should now be driven by new concrete ownership problems or logic changes, not by residual line counts.

- `decoder/state/restore` is now meaningfully organized:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore.rs` is now only `24` lines
  - restore packet generation ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore/v9.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore/ipfix.rs`
  - meaning:
    - V9 restore packet construction and IPFIX restore packet construction are no longer co-located
    - the restore entrypoint now only coordinates packet assembly for populated protocol states

- `query/grouping/build/accumulate` is now meaningfully organized:
  - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate.rs` is now only `9` lines
  - accumulation ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate/grouped.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate/compact.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate/open_tier.rs`
  - meaning:
    - plain grouped accumulation, compact grouped accumulation, and open-tier test support are no longer co-located
    - runtime query paths now depend only on the production accumulation helpers, while the open-tier helpers remain test-only
    - the grouping build root now has clear ownership boundaries between accumulate, rank, and output work

- `routing/bioris/runtime/refresh` is now meaningfully organized:
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh.rs` is now only `5` lines
  - refresh ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh/task.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh/instance.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh/dump.rs`
  - meaning:
    - instance refresh scheduling, one-pass router/peer refresh, and dump-stream route loading are no longer co-located
    - the BioRIS runtime root now only wires refresh vs observe responsibilities instead of carrying the full refresh pipeline in one file

- `query/fields` is now meaningfully organized:
  - `src/crates/netdata-netflow/netflow-plugin/src/query/fields.rs` is now only `15` lines
  - field-helper ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/capture.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/metrics.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/payload.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/rules.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/test_support.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/fields/virtuals.rs`
  - meaning:
    - facet-capture helpers, field support rules, payload parsing helpers, metric extraction helpers, virtual-field helpers, and test-only open-tier helpers are no longer co-located
    - the remaining query-side pressure has moved away from generic field plumbing and into more specific request/scan/parser paths

- `decoder/protocol/packet` is now meaningfully organized:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet.rs` is now only `7` lines
  - packet-helper ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/datalink.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/ip.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/transport.rs`
  - meaning:
    - Ethernet/MPLS framing, IPv4/IPv6 parsing, and transport/decapsulation logic are no longer co-located
    - the remaining decoder-side pressure has moved away from the shared packet helper and into the record-path parser twin

- `decoder/record/packet/parse` is now meaningfully organized:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse.rs` is now only `9` lines
  - record-packet parse ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/agent.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/datalink.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/ip.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/transport.rs`
  - meaning:
    - sFlow agent-address conversion, Ethernet/MPLS parsing, IPv4/IPv6 parsing, and transport/decapsulation logic are no longer co-located
    - the decoder packet-path symmetry is now much stronger between the FlowFields helper path and the FlowRecord helper path

- `query` is materially better organized now, and the last mixed grouping builder root is gone:
  - meaning:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/request.rs` is now only `13` lines
    - request/control ownership is now explicit under:
      - `src/crates/netdata-netflow/netflow-plugin/src/query/request/constants.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/request/output.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/request/projected.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/request/selection.rs`
    - request model, serde normalization, query-output payloads, and projected-scan support types are no longer co-located
    - `src/crates/netdata-netflow/netflow-plugin/src/query/facets.rs` is now only `6` lines
    - facet ownership is now explicit under:
      - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/render.rs`
    - facet vocabulary/cache scanning and grouped-facet accumulator rendering are no longer co-located
    - `src/crates/netdata-netflow/netflow-plugin/src/query.rs` is now only `63` lines
    - query service root ownership is now explicit under:
      - `src/crates/netdata-netflow/netflow-plugin/src/query/service.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/query/flows.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/query/metrics.rs`
    - service bootstrap, table/grouped-flow query handling, and timeseries metrics query handling are no longer co-located
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build.rs` is now only `11` lines
    - grouped-flow build ownership is now explicit under:
      - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/accumulate.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/rank.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/build/output.rs`
    - accumulation/update hot paths, ranking and overflow merge logic, and aggregate-to-output shaping are no longer co-located
    - the query monolith-risk areas are now gone
    - any further `query` work is a lower-priority refinement, not a structural emergency

- `decoder` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state.rs` is now only `13` lines
  - decoder state ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/models.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/persisted.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/restore.rs`
  - meaning:
    - decoder monolith-risk is now mostly limited to protocol-specific files such as `decoder/protocol/ipfix.rs`
    - decoder state persistence, namespace hydration, runtime ownership, and restore packet generation are no longer co-located

- `decoder/protocol/ipfix` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix.rs` is now only `9` lines
  - IPFIX ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record.rs`
  - meaning:
    - template observation, raw-payload special decoding, and main record-build/finalize logic are no longer co-located
    - decoder protocol pressure is now mainly in `decoder/protocol/v9.rs` rather than `ipfix.rs`

- `decoder/protocol/v9` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9.rs` is now only `9` lines
  - V9 ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/templates.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/sampling.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/special.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9/records.rs`
  - meaning:
    - template observation, options/sampling handling, raw special decode, and normal record append logic are no longer co-located
    - decoder protocol pressure is now mostly outside the V9 root module

- `enrichment` is materially better organized now, but some medium-large files remain:
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply.rs`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split.rs`
  - meaning:
    - the classifier runtime is now split into:
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/model.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value.rs`
    - the root `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs` is now only `69` lines
    - constructor/runtime wiring, provider/routing resolution, classifier-cache logic, and field/record apply paths are no longer co-located
    - the classifier parser is now split into:
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/boolean.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/action.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/value.rs`
      - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split.rs`
    - the parser/evaluator smell is removed
    - static metadata, GeoIP, network-attribute writing, and prefix-map helpers no longer live in one file
    - the remaining enrichment-side pressure has moved away from `enrichment/apply.rs`; parser pressure is now limited to delimiter scanning helpers

- `plugin_config` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types.rs` is now only `15` lines
  - plugin config type ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/listener.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/protocol.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/routing.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/plugin.rs`
  - meaning:
    - listener, protocol, journal retention/query limits, routing config, enrichment config, and the top-level plugin shell are no longer co-located
    - the remaining config-side pressure is now in validation and runtime helpers, not in the type definitions

- `tiering/rollup` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup.rs` is now only `14` lines
  - rollup ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/materialize.rs`
  - meaning:
    - rollup schema/constants, field-id encoding, and rollup materialization are no longer co-located
    - the encoder root is now also split by field domain into core/exporter/interface/network/presence helpers
    - storage-side pressure is no longer concentrated in a mixed rollup root

- The shared flow model is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/flow/record.rs` is now only `9` lines
  - flow record ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/core.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/presence.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal.rs`
  - meaning:
    - the shared model no longer co-locates struct ownership, presence flags, field-map bridging, and journal encoding
    - the field-map bridge is now also split by parse helpers, test-only export, and cold-path import
    - the next organization pressure is now in enrichment/apply and classifier evaluation paths instead of the shared record root

- `routing/bmp` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp.rs` is now only `34` lines
  - BMP ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/listener.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/session.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/rd.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/tests.rs`
  - meaning:
    - listener/bootstrap, message/session handling, route conversion, and RD parsing are no longer co-located
    - the next routing organization pressure is now in `routing/bioris.rs`

- `routing/bioris` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris.rs` is now only `39` lines
  - BioRIS ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/model.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/observe.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/client.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/route.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/tests.rs`
  - meaning:
    - instance refresh, peer dump/load, observe-stream lifecycle, endpoint construction, and route conversion are no longer co-located
    - the remaining structural pressure has moved away from BioRIS and into broader query/model files such as `query/grouping/model.rs` and `query/scan/bench.rs`

- Test placement is still inconsistent in several non-trivial modules:
  - `charts.rs` now follows the sibling `*_tests.rs` rule
  - other large modules still need the same consistency pass

- `presentation` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/presentation.rs` is now only `9` lines
  - presentation ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/columns.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/display.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/common.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/protocol.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/ip.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/tcp.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/icmp.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/tests.rs`
  - meaning:
    - column schema building, field display naming, protocol label maps, IPTOS labeling, TCP flag rendering, ICMP rendering, and tests are no longer co-located
    - the remaining presentation-side pressure moved away from a mixed root and into the narrower ICMP-specific leaf module

- `network_sources` is materially better organized now:
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/mod.rs` is now only `28` lines
  - network source ownership is now explicit under:
    - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/types.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/service.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/fetch.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/decode.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/transform.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/network_sources/tests.rs`
  - meaning:
    - refresh-loop orchestration, HTTP/TLS fetching, remote-row decoding, jq-style transform handling, and tests are no longer co-located
    - the remaining network-source pressure is now in `service.rs`, not in a mixed root

### Historical Baseline Before Implementation

The numbered findings below are the original evidence snapshot captured before implementation started. They are kept for historical context; the current remaining issues are listed above.

### 1. The split is still based on old file boundaries, not true module ownership

- `decoder.rs` still includes sibling files directly:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:1405`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:1406`
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:1407`
- `query.rs` still includes sibling files directly:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs:36`
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs:37`
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs:38`
- `enrichment.rs` still includes sibling files directly:
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs:26`
  - `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs:27`

- Meaning:
  - these are still one lexical namespace per root
  - ownership boundaries are weaker than they look
  - the split helps line count, but not enough for architecture

- External grounding:
  - Rust std docs explicitly warn that `include!()` is usually not what multi-file Rust projects want
  - Rust module/file layout is designed to mirror logical module hierarchy

### 2. The shared flow domain model still lives under `decoder`, but many subsystems depend on it

- Shared model data still sits in `decoder.rs`:
  - canonical field defaults: `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:82`
  - field-name interning: `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:241`
  - `FlowRecord::to_fields()`: `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:615`
  - `FlowRecord::from_fields()`: `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:854`
  - `FlowRecord::encode_to_journal_buf()`: `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs:1053`

- Other subsystems import decoder-owned model types:
  - `enrichment.rs`: `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs:2`
  - `enrichment.rs`: `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs:3`
  - `tiering.rs`: `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs:1`
  - `rollup.rs`: `src/crates/netdata-netflow/netflow-plugin/src/rollup.rs:1`
  - `query.rs`: `src/crates/netdata-netflow/netflow-plugin/src/query.rs:1`

- Meaning:
  - `FlowRecord`, `FlowFields`, `FlowDirection`, and canonical field schema are crate-wide domain state
  - they are not decoder-only concerns
  - keeping them under `decoder` creates wrong dependency direction

### 3. `query` is still split by file size, not by query domain meaning

- `query.rs` still owns multiple responsibilities:
  - planning: `src/crates/netdata-netflow/netflow-plugin/src/query.rs:286`
  - generic journal scan: `src/crates/netdata-netflow/netflow-plugin/src/query.rs:414`
  - projected scan: `src/crates/netdata-netflow/netflow-plugin/src/query.rs:530`
  - raw projected scan: `src/crates/netdata-netflow/netflow-plugin/src/query.rs:1174`
  - table query: `src/crates/netdata-netflow/netflow-plugin/src/query.rs:1367`
  - time-series query: `src/crates/netdata-netflow/netflow-plugin/src/query.rs:1546`

- `query_timeseries.rs` mixes concerns that are not time-series-specific:
  - time bounds: `src/crates/netdata-netflow/netflow-plugin/src/query_timeseries.rs:1`
  - effective group-by: `src/crates/netdata-netflow/netflow-plugin/src/query_timeseries.rs:28`
  - selection matching: `src/crates/netdata-netflow/netflow-plugin/src/query_timeseries.rs:410`
  - virtual-field dependencies: `src/crates/netdata-netflow/netflow-plugin/src/query_timeseries.rs:740`
  - raw-tier requirement logic: `src/crates/netdata-netflow/netflow-plugin/src/query_timeseries.rs:810`
  - virtual field population: `src/crates/netdata-netflow/netflow-plugin/src/query_timeseries.rs:1425`

- `query_grouping.rs` defines query-local shadow domain types:
  - `FlowRecord`: `src/crates/netdata-netflow/netflow-plugin/src/query_grouping.rs:1`
  - `FlowMetrics`: `src/crates/netdata-netflow/netflow-plugin/src/query_grouping.rs:17`

- There is also another `FlowMetrics` in storage/tiering:
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs:43`

- Meaning:
  - query planning, filtering, facet logic, grouping, and time-series output are still entangled
  - type naming now hides architectural boundaries instead of clarifying them

### 4. `decoder_protocol.rs` is still a protocol omnibus

- Same file contains:
  - packet entrypoint: `src/crates/netdata-netflow/netflow-plugin/src/decoder_protocol.rs:43`
  - v9 special record handling: `src/crates/netdata-netflow/netflow-plugin/src/decoder_protocol.rs:916`
  - IPFIX special record handling: `src/crates/netdata-netflow/netflow-plugin/src/decoder_protocol.rs:1082`
  - v9 record append path: `src/crates/netdata-netflow/netflow-plugin/src/decoder_protocol.rs:1937`
  - IPFIX record append path: `src/crates/netdata-netflow/netflow-plugin/src/decoder_protocol.rs:2382`
  - sFlow extraction: `src/crates/netdata-netflow/netflow-plugin/src/decoder_protocol.rs:2450`
  - sFlow record build: `src/crates/netdata-netflow/netflow-plugin/src/decoder_protocol.rs:2562`

- Meaning:
  - the first refactor reduced file size, but protocol ownership is still mixed
  - the decoder still lacks stable boundaries like `v9`, `ipfix`, `sflow`, `legacy`

### 5. `enrichment` owns runtime state that is not enrichment-local

- `enrichment.rs` defines shared runtime state:
  - `FlowEnricher`: `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs:30`
  - `DynamicRoutingRuntime`: `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs:68`
  - `NetworkSourcesRuntime`: `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs:99`

- But these runtimes are actively driven by other modules:
  - BMP listener: `src/crates/netdata-netflow/netflow-plugin/src/routing_bmp.rs:40`
  - BioRIS listener: `src/crates/netdata-netflow/netflow-plugin/src/routing_bioris.rs:79`
  - network source refresher: `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs:63`
  - ingest service stores and returns them: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:307`
  - ingest service stores and returns them: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:308`

- Meaning:
  - routing and remote source state are integration/runtime concerns
  - `FlowEnricher` should depend on them, not be their owner

### 6. Several roots still own too many jobs

- `main.rs`
  - response DTOs: `src/crates/netdata-netflow/netflow-plugin/src/main.rs:35`
  - handler: `src/crates/netdata-netflow/netflow-plugin/src/main.rs:125`
  - UI/function parameter metadata: `src/crates/netdata-netflow/netflow-plugin/src/main.rs:235`
  - process bootstrap and task wiring: `src/crates/netdata-netflow/netflow-plugin/src/main.rs:383`

- `ingest.rs`
  - metrics export snapshot: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:105`
  - service construction: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:313`
  - main event loop: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:445`
  - materialized tier rebuild: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:607`
  - decoder-state persistence: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:924`
  - decoder-state preload: `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs:1026`

- `plugin_config.rs`
  - one broad validator for many unrelated config domains: `src/crates/netdata-netflow/netflow-plugin/src/plugin_config.rs:1047`

- `tiering.rs`
  - flow metrics, tier row/index state, rollup field schema, rollup materialization, and tests all coexist
  - large rollup-specific function still present: `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs:684`

### 7. Test organization is still inconsistent

- Large modules already use sibling test files:
  - `decoder_tests.rs`
  - `query_tests.rs`
  - `enrichment_tests.rs`
  - `main_tests.rs`
  - `ingest_tests.rs`
  - `plugin_config_tests.rs`

- But several non-trivial files still keep inline test modules:
  - `routing_bmp.rs:758`
  - `routing_bioris.rs:732`
  - `network_sources.rs:424`
  - `tiering.rs:1029`
  - `presentation.rs:624`

- Meaning:
  - test placement rules are still inconsistent
  - this makes the structure harder to predict

## External Grounding

- Rust docs:
  - `include!()` is generally not the right tool for multi-file project structure
  - module files should mirror logical module hierarchy

- Local mirror examples:
  - Cloudflare `goflow` separates protocol decoder files:
    - `decoders/netflow/ipfix.go`
    - `decoders/netflow/nfv9.go`
    - `decoders/sflow/sflow.go`
  - NetGauze separates flow model and wire codec:
    - `crates/flow-pkt/src/lib.rs`
    - `crates/flow-pkt/src/ipfix.rs`
    - `crates/flow-pkt/src/wire/deserializer/ipfix.rs`
  - Akvorado keeps query concerns in dedicated query packages:
    - `console/query/column.go`
    - `console/query/filter.go`

## Decisions

### Decisions Made

- Costa requested a dedicated TODO for the organization pass.
- Scope is architecture and organization planning.
- Logic changes are explicitly out of scope.
- Organization must be by meaning and single job, not only by line count.
- 2026-03-27: Costa approved implementation start.
- 2026-03-27: The organization pass should proceed with no functionality changes.
- 2026-03-27: Use the recommended direction from this TODO as the implementation baseline:
  - replace `include!()` pseudo-modules with real module directories
  - move shared flow domain ownership out of `decoder`
  - reorganize `decoder`, `query`, `enrichment`, `ingest`, `routing`, `network_sources`, and `storage` by domain meaning
  - keep all behavior, protocol handling, query semantics, and config semantics unchanged
- 2026-03-28: Costa explicitly rejected further micro-splitting.
- 2026-03-28: Remaining work should be based only on real meaning boundaries and real maintainer benefit, not on residual size lists.
- 2026-03-28: The final reasonable batch was completed:
  - `plugin_config/validation.rs` split by validation domain
  - `decoder/record/core.rs` split by representation ownership
  - `tiering/index.rs` split by runtime responsibility
  - `query/scan/raw.rs` refactored by helper extraction only, without further file splitting

### Decisions Pending

- None required yet for this TODO file.
- Before implementation starts, the following decisions may need explicit confirmation:
  - whether to introduce directory modules such as `decoder/`, `query/`, `enrichment/`, `flow/`, `ingest/`, `routing/`, `storage/`, `api/`
  - whether to rename shared domain types while moving them out of `decoder`
  - whether to move all remaining inline test modules to sibling test files in the same pass

## Recommended Target Structure

```text
src/
  main.rs
  api/flows.rs
  flow/{record.rs,schema.rs,journal.rs}
  decoder/{mod.rs,state.rs,legacy.rs,v9.rs,ipfix.rs,sflow.rs,mappings.rs}
  enrichment/{mod.rs,core.rs,classifiers.rs,geoip.rs,networks.rs,metadata.rs}
  routing/{mod.rs,runtime.rs,bmp.rs,bioris.rs}
  network_sources/{mod.rs,client.rs,transform.rs,runtime.rs}
  query/{mod.rs,request.rs,model.rs,planner.rs,filters.rs,facets.rs,scan.rs,grouping.rs,timeseries.rs,output.rs}
  ingest/{mod.rs,metrics.rs,service.rs,rebuild.rs,persistence.rs}
  storage/{mod.rs,tiering.rs,rollup.rs}
  presentation.rs
  charts.rs
```

## Plan

### Final Reasonable Batch (2026-03-28)

- Status: complete

- Completed:
  - split `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation.rs` by validation domain
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core.rs` by ownership boundary
  - split `src/crates/netdata-netflow/netflow-plugin/src/tiering/index.rs` by runtime responsibility
  - reduce `scan_matching_grouped_records_projected_raw_direct()` in `src/crates/netdata-netflow/netflow-plugin/src/query/scan/raw.rs` by extracting local helpers only
  - re-run `cargo fmt`, `cargo check --all-targets`, and `cargo test`

- Stop condition reached:
  - no further file/module splitting is currently recommended without a new concrete ownership problem
  - remaining medium-sized files are cohesive enough to leave as-is
  - future changes should be feature-driven, not refactor-driven

### Phase 1: Replace pseudo-modules with real modules

- Status: complete

- Convert:
  - `decoder.rs` + `decoder_*` files into `decoder/`
  - `query.rs` + `query_*` files into `query/`
  - `enrichment.rs` + `enrichment_*` files into `enrichment/`
- Replace `include!()` with `mod` boundaries.
- Keep public/internal visibility exactly as needed.
- Do not change logic in this phase.

### Phase 2: Extract the shared flow domain

- Status: complete
- 2026-03-27 implemented additional flow record split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/flow/record.rs` by meaning into:
    - `flow/record/core.rs`
    - `flow/record/presence.rs`
    - `flow/record/fields.rs`
    - `flow/record/journal.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/flow/record.rs` to a thin root module
  - kept cold-path field bridging and journal encoding behavior unchanged
- 2026-03-27 implemented additional flow-field-export split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export.rs` by meaning into:
    - `flow/record/fields/export/helpers.rs`
    - `flow/record/fields/export/exporter.rs`
    - `flow/record/fields/export/endpoints.rs`
    - `flow/record/fields/export/interfaces.rs`
    - `flow/record/fields/export/headers.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export.rs` to a thin test-only root module
  - kept test-only `FlowRecord::to_fields()` export behavior unchanged
- 2026-03-27 implemented additional journal split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal.rs` by meaning into:
    - `flow/record/journal/writer.rs`
    - `flow/record/journal/core.rs`
    - `flow/record/journal/network.rs`
    - `flow/record/journal/interfaces.rs`
    - `flow/record/journal/transport.rs`
    - `flow/record/journal/headers.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/flow/record/journal.rs` to a thin root module
  - kept journal field emission order, round-trip semantics, and cold-path record reconstruction behavior unchanged

- Create `flow/` or `model/` for:
  - canonical field schema/defaults
  - field-name interning
  - `FlowDirection`
  - `FlowFields`
  - `FlowRecord`
  - journal encode/decode helpers tied to the record format
- Update all dependent modules to use the new owner.
- Goal:
  - decoder depends on flow model
  - enrichment/query/storage depend on flow model
  - no subsystem should need `decoder` only to get the domain types

### Phase 3: Reorganize decoder by protocol meaning

- Status: in progress
- 2026-03-27 implemented additional V9 split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9.rs` by meaning into:
    - `decoder/protocol/v9/templates.rs`
    - `decoder/protocol/v9/sampling.rs`
    - `decoder/protocol/v9/special.rs`
    - `decoder/protocol/v9/records.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/v9.rs` to a thin root module
  - preserved template observation, sampling extraction, raw special decode, and normal v9 record decode behavior unchanged
- 2026-03-27 implemented additional decoder protocol split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix.rs` by meaning into:
    - `decoder/protocol/ipfix/templates.rs`
    - `decoder/protocol/ipfix/special.rs`
    - `decoder/protocol/ipfix/record.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix.rs` to a thin root module
  - preserved IPFIX template observation, raw-payload special decode, record-build, and finalization behavior unchanged
- 2026-03-28 implemented additional IPFIX-template split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates.rs` by meaning into:
    - `decoder/protocol/ipfix/templates/entry.rs`
    - `decoder/protocol/ipfix/templates/data.rs`
    - `decoder/protocol/ipfix/templates/options.rs`
    - `decoder/protocol/ipfix/templates/v9.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/templates.rs` to a thin root module
  - preserved raw payload traversal, IPFIX data-template observation, IPFIX options-template observation, and V9-compat template observation unchanged
- 2026-03-27 implemented:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol.rs`
  - current boundaries:
    - `entry.rs`
    - `shared.rs`
    - `packet.rs`
    - `legacy.rs`
    - `v9.rs`
    - `ipfix.rs`
    - `sflow.rs`
- 2026-03-27 implemented additional decoder split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/record.rs` by meaning into:
    - `decoder/record/core.rs`
    - `decoder/record/setters.rs`
    - `decoder/record/packet.rs`
    - `decoder/record/fields.rs`
    - `decoder/record/mappings.rs`
    - `decoder/record/values.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/record.rs` to a thin root module
- 2026-03-27 implemented additional decoder state split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/state.rs` by meaning into:
    - `decoder/state/models.rs`
    - `decoder/state/sampling.rs`
    - `decoder/state/runtime.rs`
    - `decoder/state/persisted.rs`
    - `decoder/state/restore.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/state.rs` to a thin root module
- Constraint:
  - keep all decoder behavior, template handling, sampling logic, and timestamp semantics unchanged

- Split protocol logic into:
  - `legacy.rs` or `v5.rs` and `v7.rs`
  - `v9.rs`
  - `ipfix.rs`
  - `sflow.rs`
  - `shared.rs` for common helpers only
- Keep decoder state/template cache ownership under `decoder/state/`.
- Keep field-mapping logic in a dedicated decoder mapping module.

### Phase 4: Reorganize query by meaning

- Status: in progress

- Split into:
  - `request.rs`
  - `planner.rs`
  - `filters.rs`
  - `facets.rs`
  - `scan.rs`
  - `grouping.rs`
  - `timeseries.rs`
  - `output.rs`
  - `model.rs`
- Move generic helpers currently misplaced in `query_timeseries.rs` to the correct modules.
- Rename query-local shadow types so they do not look like the crate-wide flow model.
- Done in this batch:
  - `planner.rs`
  - `scan.rs`
  - local type rename to `QueryFlowRecord` / `QueryFlowMetrics`
  - split `query/facets.rs` by meaning into:
    - `query/facets/cache.rs`
    - `query/facets/render.rs`
  - reduced `query/facets.rs` to a thin root module
  - split `query.rs` by meaning into:
    - `query/service.rs`
    - `query/flows.rs`
    - `query/metrics.rs`
  - reduced `query.rs` to a thin root module
  - split `query/request.rs` by meaning into:
    - `query/request/constants.rs`
    - `query/request/model.rs`
    - `query/request/output.rs`
    - `query/request/projected.rs`
    - `query/request/selection.rs`
  - reduced `query/request.rs` to a thin root module
  - split `query/grouping.rs` by meaning into:
    - `query/grouping/model.rs`
    - `query/grouping/labels.rs`
    - `query/grouping/build.rs`
  - reduced `query/grouping.rs` to a thin root module
  - split `query/grouping/build.rs` by meaning into:
    - `query/grouping/build/accumulate.rs`
    - `query/grouping/build/rank.rs`
    - `query/grouping/build/output.rs`
  - reduced `query/grouping/build.rs` to a thin root module
  - split `query/scan.rs` by meaning into:
    - `query/scan/selection.rs`
    - `query/scan/session.rs`
    - `query/scan/raw.rs`
    - `query/scan/bench.rs`
  - reduced `query/scan.rs` to a thin root module
  - split `query/request/selection.rs` by meaning into:
    - `query/request/selection/types.rs`
    - `query/request/selection/decode.rs`
    - `query/request/selection/normalize.rs`
    - `query/request/selection/extract.rs`
  - reduced `query/request/selection.rs` to a thin root module
- Remaining:
  - query root/module ownership is now structurally sound
  - any further query work is optional refinement around medium-size leaf files such as `query/scan/bench.rs`, `query/request/model.rs`, `query/facets/render.rs`, and `query/projected/apply.rs`

### Phase 5: Separate enrichment from routing and network-source runtime ownership

- Status: in progress
- 2026-03-27 implemented additional classifier runtime split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime.rs` by meaning into:
    - `enrichment/classifiers/runtime/model.rs`
    - `enrichment/classifiers/runtime/eval.rs`
    - `enrichment/classifiers/runtime/value.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime.rs` to a thin root module
  - kept classifier rule context, evaluator, field-value resolution, and parser-facing API behavior unchanged
- 2026-03-27 implemented additional classifier parser split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse.rs` by meaning into:
    - `enrichment/classifiers/parse/boolean.rs`
    - `enrichment/classifiers/parse/action.rs`
    - `enrichment/classifiers/parse/value.rs`
    - `enrichment/classifiers/parse/split.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse.rs` to a thin root module
  - kept boolean-expression parsing, action parsing, value parsing, and delimiter-scanning behavior unchanged
- 2026-03-27 implemented additional enrichment root split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs` by meaning into:
    - `enrichment/init.rs`
    - `enrichment/resolve.rs`
    - `enrichment/classify.rs`
    - `enrichment/apply.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs` to a thin root module
  - kept enricher construction, runtime accessors, routing/network resolution, classifier-cache behavior, and field/record enrichment behavior unchanged
- 2026-03-27 implemented in this batch:
  - split `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris.rs` by meaning into:
    - `routing/bioris/runtime.rs`
    - `routing/bioris/client.rs`
    - `routing/bioris/route.rs`
    - `routing/bioris/tests.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris.rs` to a thin root module
  - preserved BioRIS refresh, observe-stream, endpoint construction, peer-key generation, and route-conversion behavior unchanged
- 2026-03-27 implemented additional BioRIS runtime split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime.rs` by meaning into:
    - `routing/bioris/runtime/model.rs`
    - `routing/bioris/runtime/refresh.rs`
    - `routing/bioris/runtime/observe.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime.rs` to a thin root module
  - preserved AFI/SAFI mapping, refresh scheduling, peer dump/load, observe-stream retry behavior, and update application behavior unchanged
- 2026-03-28 implemented additional BioRIS refresh split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh.rs` by meaning into:
    - `routing/bioris/runtime/refresh/task.rs`
    - `routing/bioris/runtime/refresh/instance.rs`
    - `routing/bioris/runtime/refresh/dump.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/routing/bioris/runtime/refresh.rs` to a thin root module
  - preserved refresh retry scheduling, router enumeration, dump-stream loading, peer clearing, and observe-task lifecycle behavior unchanged
- 2026-03-28 implemented additional query field-helper split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/fields.rs` by meaning into:
    - `query/fields/capture.rs`
    - `query/fields/metrics.rs`
    - `query/fields/payload.rs`
    - `query/fields/rules.rs`
    - `query/fields/test_support.rs`
    - `query/fields/virtuals.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/fields.rs` to a thin root module
  - preserved facet-capture behavior, field support rules, projected payload parsing, metric extraction, virtual-field population, and test-only open-tier helpers unchanged
- 2026-03-28 implemented additional decoder packet-helper split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet.rs` by meaning into:
    - `decoder/protocol/packet/datalink.rs`
    - `decoder/protocol/packet/ip.rs`
    - `decoder/protocol/packet/transport.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet.rs` to a thin root module
  - preserved Ethernet/MPLS frame parsing, IPv4/IPv6 packet parsing, decapsulation behavior, transport-header extraction, and MAC formatting unchanged
- 2026-03-28 implemented additional decoder record-packet parse split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse.rs` by meaning into:
    - `decoder/record/packet/parse/agent.rs`
    - `decoder/record/packet/parse/datalink.rs`
    - `decoder/record/packet/parse/ip.rs`
    - `decoder/record/packet/parse/transport.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse.rs` to a thin root module
  - preserved FlowRecord Ethernet/MPLS parsing, IPv4/IPv6 parsing, decapsulation behavior, transport-header extraction, and sFlow agent address conversion unchanged
- 2026-03-27 implemented in this batch:
  - split `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp.rs` by meaning into:
    - `routing/bmp/listener.rs`
    - `routing/bmp/session.rs`
    - `routing/bmp/routes.rs`
    - `routing/bmp/rd.rs`
    - `routing/bmp/tests.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp.rs` to a thin root module
  - preserved BMP listener lifecycle, peer/session cleanup, route conversion, RD filtering, and unit-test behavior unchanged
- 2026-03-27 implemented additional enrichment data split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data.rs` by meaning into:
    - `enrichment/data/static_data.rs`
    - `enrichment/data/geoip.rs`
    - `enrichment/data/network.rs`
    - `enrichment/data/prefix.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data.rs` to a thin root module
  - kept static metadata, GeoIP loading, prefix lookup, and network-attribute write behavior unchanged
- 2026-03-27 implemented in this batch:
  - moved dynamic routing runtime types from `enrichment.rs` to `routing/runtime.rs`
  - moved network source runtime types from `enrichment.rs` to `network_sources/runtime.rs`
  - replaced root files `routing_bmp.rs`, `routing_bioris.rs`, and `network_sources.rs`
    with domain directories:
    - `routing/{bioris.rs,bmp.rs,runtime.rs}` plus root `routing.rs`
    - `network_sources/{mod.rs,runtime.rs}`
  - split `enrichment/classifiers.rs` by meaning into:
    - `enrichment/classifiers/runtime.rs`
    - `enrichment/classifiers/parse.rs`
    - `enrichment/classifiers/helpers.rs`
  - reduced `enrichment/classifiers.rs` to a thin root module
  - kept `FlowEnricher::from_config()`, `dynamic_routing_runtime()`, and
    `network_sources_runtime()` behavior unchanged
  - split `src/crates/netdata-netflow/netflow-plugin/src/network_sources/mod.rs` by meaning into:
    - `network_sources/types.rs`
    - `network_sources/service.rs`
    - `network_sources/fetch.rs`
    - `network_sources/decode.rs`
    - `network_sources/transform.rs`
    - `network_sources/tests.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/network_sources/mod.rs` to a thin root module
  - kept network-source refresh scheduling, HTTP/TLS client behavior, transform execution, and remote-row decoding unchanged
- Remaining:
  - decide whether runtime construction should stay inside `FlowEnricher::from_config()`
    or move later to a higher-level wiring layer as part of Phase 6
  - `enrichment/apply.rs` no longer needs a structural decision; any later follow-up would only be a smaller split inside `apply/write.rs` if that becomes useful

- Keep `FlowEnricher` and its direct classification/data helpers under `enrichment/`.
- Move dynamic routing runtime types to `routing/runtime.rs`.
- Move remote network-source runtime types to `network_sources/runtime.rs`.
- Keep `FlowEnricher` as a consumer of runtime state, not the owner of unrelated runtime modules.

### Phase 6: Thin the remaining mixed roots

- Status: in progress
- 2026-03-27 implemented in this batch:
  - moved `main.rs` flow-function DTOs, parameter metadata builders, and handler code
    into:
    - `src/crates/netdata-netflow/netflow-plugin/src/api/flows.rs`
  - added:
    - `src/crates/netdata-netflow/netflow-plugin/src/api.rs`
  - kept the existing test import path stable by re-exporting the moved API items from `main.rs`
  - split `src/crates/netdata-netflow/netflow-plugin/src/plugin_config.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/defaults.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation.rs`
  - kept config loading, GeoIP database auto-detection, and validation behavior unchanged
  - split `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/listener.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/protocol.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/routing.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/plugin.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types.rs` to a thin root module
  - kept clap/serde field names, defaults, aliases, and helper methods unchanged
  - split `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/model.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/tests.rs`
  - kept tier rollup schema, open-tier accumulation, and materialization behavior unchanged
  - split `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/materialize.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup.rs` to a thin root module
  - kept rollup field schema, field-id encoding order, bucket math, and rollup materialization behavior unchanged
  - split `src/crates/netdata-netflow/netflow-plugin/src/presentation.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/columns.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/display.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/tests.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/presentation.rs` to a thin root module
  - kept field display names, value labels, virtual ICMP naming, and column payload schemas unchanged
- 2026-03-27 implemented additional ingest service split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/ingest/service.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/init.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest/service/tiers.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/ingest/service.rs` to a thin root module
  - preserved journal writer setup, decoder/enricher initialization, UDP ingest loop behavior, sync scheduling, tier observation, and tier flush behavior unchanged
- 2026-03-27 implemented additional rollup encoder split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/core.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/exporter.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/interface.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/network.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode/presence.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/encode.rs` to a thin root module
  - preserved rollup field indexes, encoding order, missing-value sentinels, and presence-flag encoding behavior unchanged
- 2026-03-27 implemented additional flow field bridge split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/parse.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/export.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields/import.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/flow/record/fields.rs` to a thin root module
  - preserved MAC/prefix parsing, test-only FlowFields export, cold-path FlowFields import, and presence-flag restoration behavior unchanged
- 2026-03-27 implemented additional decoder packet split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/mappings.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/sampling.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet.rs` to a thin root module
  - preserved packet parsing, decapsulation behavior, special-field mappings, and sampling-state application behavior unchanged
- 2026-03-27 implemented additional classifier evaluator split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/boolean.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/action.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval.rs` to a thin root module
  - preserved boolean rule evaluation, condition dispatch, action evaluation, and classification rule matching behavior unchanged
- 2026-03-27 implemented additional enrichment apply split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/context.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/metadata.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/resolve.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply/write.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/apply.rs` to a thinner pipeline root
  - preserved enrichment context construction, static metadata and sampling application, routing/network resolution, and field/record writeback behavior unchanged
- 2026-03-27 implemented additional query grouping model split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/core.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/compact.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/projected.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model/grouped.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/grouping/model.rs` to a thin root module
  - preserved query-row construction, compact aggregation state, projected grouping accumulation, and grouped-label/result container behavior unchanged
- 2026-03-27 implemented additional decoder setter split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/exporter.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/counter.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/network.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/interface.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters/transport.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/setters.rs` to a thinner dispatch root
  - preserved canonical field dispatch, override semantics, and raw-metric synchronization behavior unchanged
- 2026-03-27 implemented additional query facet-cache split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/service.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/scan.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/vocabulary.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache/payload.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/facets/cache.rs` to a thin root module
  - preserved closed/open facet cache behavior, targeted journal scans, vocabulary merging, and payload ordering/rendering behavior unchanged
- 2026-03-27 implemented additional charts split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/charts.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/charts/runtime.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/charts/snapshot.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/charts/metrics.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/charts/tests.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/charts.rs` to a thin root module
  - preserved chart registration, snapshot sampling, chart metadata, and chart tests unchanged
- 2026-03-27 implemented additional presentation label split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/common.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/protocol.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/ip.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/tcp.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels/icmp.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/presentation/labels.rs` to a thinner dispatch root
  - preserved protocol labeling, ethertype/forwarding/interface-boundary labels, IPTOS rendering, TCP flag rendering, and ICMP label/value rendering unchanged
- 2026-03-27 implemented additional query planner split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/planner.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/request.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/spans.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/timeseries.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/planner/prepare.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/planner.rs` to a thin root module
  - preserved request time-bound normalization, group-by resolution, span planning, timeseries layout math, and query preparation/stat collection unchanged
- 2026-03-27 implemented additional IPFIX record split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/state.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/fields.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/finalize.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record/append.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/record.rs` to a thin root module
  - preserved IPFIX record build-state tracking, field application, reverse-flow construction, record finalization, and packet record append behavior unchanged
- 2026-03-27 implemented additional decoder runtime split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/init.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/decode.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/namespace.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/observe.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/persist.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime.rs` to a thin root module
  - preserved decoder construction, UDP decode flow, enrichment refresh, namespace bookkeeping, persisted state hydrate/export, and raw payload observation behavior unchanged
- 2026-03-27 implemented additional flows API split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/api/flows.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/api/flows/model.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/api/flows/params.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/api/flows/handler.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/api/flows.rs` to a thin root module
  - preserved function schema/version constants, response payload shapes, required parameter metadata, handler behavior, and function declaration semantics unchanged
- 2026-03-27 implemented additional rollup schema split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/direction.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/presence.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema.rs` to a thin root module
  - preserved rollup bucket constant ownership, presence-field metadata, field-definition ordering, index construction, and direction encoding behavior unchanged
- 2026-03-27 implemented additional BMP routes split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/apply.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/aspath.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/helpers.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes/nlri.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/routing/bmp/routes.rs` to a thin root module
  - preserved update application, AS-path flattening, route conversion, path-id handling, RD filtering, and BMP test behavior unchanged
- 2026-03-27 implemented additional network-attribute split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/asn.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/attrs.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/csv.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/test_support.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network/write.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/network.rs` to a thin root module
  - preserved network-attribute model construction, prefix-map building, ASN naming/override rules, record writeback, CSV serialization, and test-only field helpers unchanged
- 2026-03-27 implemented additional projected-query split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/projected.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/projected/prefix.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/projected/apply.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/projected/bench_support.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/projected.rs` to a thin root module
  - preserved projected prefix matching, field-spec construction, grouped payload application, planned projected matching, and benchmark checksum behavior unchanged
- 2026-03-27 implemented additional query-flows split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/flows.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/flows/service.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/flows/build.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/flows/materialize.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/flows.rs` to a thin root module
  - preserved grouped query orchestration, compact/projected aggregate ranking, grouped-row materialization, stats population, warnings, and response shaping unchanged
- 2026-03-27 implemented additional decoder field-helper split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields/common.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields/sampling.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields/special.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/fields.rs` to a thin root module
  - preserved field normalization, zero-IP filtering, MPLS label handling, IP parsing, sampling-option observation, and V9/reverse-IPFIX special mappings unchanged
- 2026-03-27 implemented additional sampling-state split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling/types.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling/state.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling/templates.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/state/sampling.rs` to a thin root module
  - preserved sampling-rate lookup/update behavior, namespace clearing, template ownership, datalink-template detection, and persisted-namespace restore behavior unchanged
- 2026-03-27 implemented additional GeoIP split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/types.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/files.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/decode.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/resolver.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip.rs` to a thin root module
  - preserved database loading, signature checking, hot reload behavior, ASN/Geo decoding, and resolver lookup semantics unchanged
- 2026-03-27 implemented additional rollup-schema-fields split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/index.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields.rs` to a thin root module
  - preserved rollup field-definition ordering, field metadata ownership, and field-id index lookup behavior unchanged
- 2026-03-27 implemented additional rollup-field-definitions split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/core.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/exporter.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/interface.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/network.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs/presence.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/tiering/rollup/schema/fields/defs.rs` to a thin ordered-table assembly module
  - preserved exact rollup field index order; `SRC_VLAN` and `DST_VLAN` stay in the network-positioned segment to match the existing encoder field ids
- 2026-03-27 implemented additional decoder-protocol-shared split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/akvorado.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared.rs` to a thin root module
  - preserved flow deduplication/identity merge behavior, enrichment merge fill-in rules, Akvorado unsigned decoding, and template-error detection unchanged
- 2026-03-27 implemented additional IPFIX-special split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special/packet.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special/parser.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special/record.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/ipfix/special.rs` to a thin root module
  - preserved raw IPFIX packet traversal, template-length parsing, datalink/MPLS special decoding, sampling derivation, and final record construction unchanged
- 2026-03-27 implemented additional classifier-parse split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split/separator.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split/keyword.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split/delimiters.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/parse/split.rs` to a thin root module
  - preserved top-level separator splitting, keyword-boundary detection, prefix stripping, and delimiter-wrap validation unchanged
- 2026-03-27 implemented additional query-scan-session split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/scan/session.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/session/records.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/session/projected.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/scan/session.rs` to a thin root module
  - preserved plain journal-record scan behavior, projected grouped scan behavior, regex filtering, prefilter cursor matching, and projected payload aggregation unchanged
- 2026-03-27 implemented additional sFlow split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow/packet.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow/record.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow.rs` to a thin root module
  - preserved sFlow sample traversal, interface normalization, forwarding-status derivation, header/extended-record decoding, and final flow-record construction unchanged
- 2026-03-27 implemented additional enrichment-config-types split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/sampling.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/providers.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/metadata.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/geoip.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/networks.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/sources.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/root.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment.rs` to a thin root module
  - preserved enrichment configuration schema, serde aliases, default provider ordering, metadata/network source defaults, and top-level `EnrichmentConfig` defaults unchanged
- 2026-03-27 implemented additional classifier-runtime-value split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value/expr.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value/field.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value/resolved.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/value.rs` to a thin root module
  - preserved runtime value-expression resolution, field lookup semantics, numeric/string conversion, list handling, and format placeholder expansion unchanged
- 2026-03-27 implemented additional shared-merge split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge/identity.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge/enrich.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/shared/merge.rs` to a thin root module
  - preserved flow identity hashing/matching, unique-flow append behavior, duplicate suppression, and enrichment fill-in merge behavior unchanged
- 2026-03-28 implemented additional query-request-model split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/query/request/model.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model/types.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model/normalize.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/request/model/deserialize.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/query/request/model.rs` to a thin root module
  - preserved request defaults, selection-hoisting precedence, field validation, and facet/group-by normalization semantics unchanged
- 2026-03-28 implemented additional classifier-condition-eval split:
  - split `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition.rs` by meaning into:
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/types.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/context.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/compare.rs`
    - `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition/text.rs`
  - reduced `src/crates/netdata-netflow/netflow-plugin/src/enrichment/classifiers/runtime/eval/condition.rs` to a thin root module
  - preserved the `ConditionExpr` API, operand resolution behavior, numeric comparison error texts, list-membership semantics, and regex error behavior unchanged
- Remaining:
  - no mixed root remains in the same risk class as the original `main.rs`, `ingest.rs`, `plugin_config.rs`, and `tiering.rs`
  - the highest-value remaining work is now inside `query/scan/bench.rs` (test-only), `main.rs`, `ingest/metrics.rs`, `plugin_config/validation.rs`, and `decoder/record/core.rs`
  - next production target:
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest/metrics.rs`
    - current mixed jobs there are:
      - metric field ownership on `IngestMetrics`
      - decode-stat application in `apply_decode_stats()`
      - metric snapshot serialization in `snapshot()`
    - likely split into metric model/update helpers and snapshot rendering helpers

- 2026-03-28 read-only scan findings after the no-micro-splitting decision:
  - do not split just because of size:
    - `src/crates/netdata-netflow/netflow-plugin/src/ingest/metrics.rs`
      - cohesive single responsibility: metrics storage plus trivial update/snapshot helpers
    - `src/crates/netdata-netflow/netflow-plugin/src/main.rs`
      - cohesive binary/bootstrap ownership, but `main()` is still large and may deserve helper extraction without file splitting
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs`
      - cohesive config/type ownership
    - `src/crates/netdata-netflow/netflow-plugin/src/routing/runtime.rs`
      - cohesive dynamic-routing runtime CRUD/lookup ownership
  - strongest remaining meaningful targets:
    - `src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation.rs`
      - still validates multiple domains in one place:
        - listener/protocol basics
        - journal/query retention
        - dynamic BMP
        - dynamic BioRIS
        - network source TLS/transform
    - `src/crates/netdata-netflow/netflow-plugin/src/decoder/record/core.rs`
      - still mixes 2 different representations:
        - `FlowFields` canonicalization helpers
        - `FlowRecord` native helpers
    - `src/crates/netdata-netflow/netflow-plugin/src/tiering/index.rs`
      - still mixes 2 runtime concerns:
        - `TierFlowIndexStore`
        - `TierAccumulator`
    - `src/crates/netdata-netflow/netflow-plugin/src/query/scan/raw.rs`
      - file is single-purpose, but `scan_matching_grouped_records_projected_raw_direct()` is still too large and mixes:
        - journal file setup
        - entry iteration/window filtering
        - payload extraction/decompression
        - projected capture matching
        - selection filtering
        - grouped accumulation
  - recommended order now:
    - 1. `plugin_config/validation.rs`
    - 2. `decoder/record/core.rs`
    - 3. `tiering/index.rs`
    - 4. function-level split inside `query/scan/raw.rs`
  - approved implementation scope:
    - proceed with the reasonable remaining targets above
    - avoid any further size-driven or cosmetic splits outside these targets

- `main.rs`
  - move flow function response DTOs and metadata builders to `api/flows.rs`
  - leave only bootstrap/wiring in `main.rs`

- `ingest.rs`
  - split into metrics, service construction, event loop, rebuild, persistence

- `plugin_config.rs`
  - split by config domain plus validation
  - likely `defaults.rs`, `types.rs`, `validation.rs`, `load.rs`

- `tiering.rs`
  - move rollup-specific logic toward `storage/rollup.rs`
  - keep open-tier/index runtime and materialization concerns separate

### Phase 7: Make test layout consistent

- Rule:
  - small leaf modules may keep inline tests
  - non-trivial modules use sibling `*_tests.rs`
- Apply that rule consistently across routing, network sources, storage, presentation, and charts.

## Implied Decisions

- File moves and renames are acceptable as long as behavior is preserved.
- Public API changes are not intended; this is internal crate organization.
- Performance-sensitive code paths must remain readable to profiling and benchmarking.
- Benchmark helpers may stay, but they should live in the module that owns the benchmarked concern.
- Costa explicitly rejected unnecessary micro-splitting.
- Remaining work should now prioritize only files/functions that are still meaningfully mixed, not merely medium-sized.

## Risks

- Large pure-move refactors can accidentally widen visibility or create circular dependencies.
- Moving the flow domain model out of `decoder` may expose hidden coupling that was previously masked by the old layout.
- Replacing `include!()` with real modules may require explicit imports and visibility tuning in many places.
- Query reorganization is the highest semantic-risk area because filtering, virtual fields, grouping, and time-series all currently cross file boundaries.
- Query reorganization was the highest semantic-risk area and is now substantially de-risked.
- The classifier parser/evaluator split is now also de-risked.
- The decoder state split is now de-risked too.
- The shared flow record split is now de-risked too.
- The enrichment data split is now de-risked too.
- The BMP split is now de-risked too.
- The BioRIS split is now de-risked too.
- The IPFIX split is now de-risked too.
- No remaining mixed root is in the same risk class as the earlier `query`, `ingest`, `decoder`, or `tiering` roots.
- `query/scan/bench.rs`, `main.rs`, `ingest/metrics.rs`, `plugin_config/validation.rs`, and `decoder/record/core.rs` are now the clearest remaining size/maintainability targets.

## Testing Requirements

- Must pass:
  - `cargo fmt -p netflow-plugin`
  - `cargo check -p netflow-plugin --all-targets`
  - `cargo test -p netflow-plugin`

- Must preserve:
  - existing fixture-driven decoder tests
  - enrichment tests
  - query tests
  - ingest tests
  - end-to-end function response tests

- Recommended after the organization pass:
  - reinstall the plugin locally
  - verify startup succeeds
  - verify no query/runtime regressions in a simple end-to-end smoke test

## Documentation Updates Required

- User-facing documentation:
  - none expected, unless behavior changes unintentionally

- Developer-facing documentation:
  - recommended in the eventual PR description:
    - explain the new module tree
    - explain new ownership boundaries
    - explain any renamed internal types

- Code-level documentation:
  - add short module-level comments where ownership may not be obvious, especially for:
    - `flow/`
    - `decoder/`
    - `query/`
    - `storage/`
    - `routing/`

## Implementation Order Recommendation

1. Real module directories and `mod` files only
2. Shared flow domain extraction
3. Decoder protocol split
4. Query split by meaning
5. Routing/network-sources runtime extraction
6. Main/ingest/config/storage cleanup
7. Final consistency and tests

## Notes

- This TODO is a follow-up planning document to `TODO-netflow-plugin-refactor.md`.
- This TODO is now an active implementation tracker.
- Implementation has started and is verified for the completed phases above.
