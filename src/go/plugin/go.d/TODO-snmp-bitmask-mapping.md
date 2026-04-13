# TL;DR

Add bitmask-aware value transformation support to SNMP profile processing so profile `value_transformation` can decode integer bit fields into mapped states.

# Requirements

> See src/go/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition and src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector. Doc is src/go/plugin/go.d/collector/snmp/profile-format.md
>
> See value transformation mapping.
>
> The goal is to add bitmask mapping support for value tranformation. So we can collect values like
>
> DellProcessorDeviceStatusReading                ::= INTEGER {
>     -- Note: These values are bit masks, so combination values are possible.
>     internalError(1),                           -- Internal Error
>     thermalTrip(2),                             -- Thermal Trip
>     configurationError(32),                     -- Configuration Error
>     processorPresent(128),                      -- Processor Present
>     processorDisabled(256),                     -- Processor Disabled
>     terminatorPresent(512),                     -- Terminator Present
>     processorThrottled(1024)                    -- Processor Throttled
> }

# Facts

1. Current metric `mapping` behavior is exact-match only for multi-value dimensions.
   Evidence:
   - `collector/snmp/ddsnmp/ddsnmpcollector/metric_builder.go:107-159` builds `MultiValue` by comparing `value == intKey`; there is no bitwise check.
   - `collector/snmp/profile-format.md:1489, 1529-1558` documents `mapping` as one active dimension and all others `0`.
   - `collector/snmp/ddsnmp/ddsnmpcollector/collector_scalar_test.go:455-495` and `collector/snmp/ddsnmp/ddsnmpcollector/collector_table_test.go:1315-1387` assert exact-match enum behavior.

2. Current metric `mapping` also supports numeric remapping before metric construction.
   Evidence:
   - `collector/snmp/ddsnmp/ddsnmpcollector/value_processor.go:84-101` remaps numeric values only when `mapping[raw]` exists and the mapped value is numeric.
   - `collector/snmp/ddsnmp/ddsnmpcollector/value_processor.go:107-140` does the same for string PDUs after extraction/matching.
   - `collector/snmp/ddsnmp/ddsnmpcollector/collector_scalar_test.go:536-567` asserts `int -> int` remapping with no `MultiValue`.

3. Virtual metric validation infers available dimensions directly from `mapping`, so any new bitmask mode must be visible there too.
   Evidence:
   - `collector/snmp/ddsnmp/ddprofiledefinition/virtual_metric_dim_validation.go:60-105` derives known dimensions from `sym.Mapping`.
   - `collector/snmp/ddsnmp/ddprofiledefinition/validation.go:416-519` uses that inferred dimension set to validate virtual metric `sources[*].dim`.

4. There is already a real profile need for bitmask support in the default SNMP profiles.
   Evidence:
   - `config/go.d/snmp.profiles/default/dell-poweredge.yaml:881-885` has a TODO for `processorDeviceStatusReading`.
   - `config/go.d/snmp.profiles/default/dell-poweredge.yaml:974-978` has a TODO for `memoryDeviceFailureModes`.

5. `metric_type: flag_stream` exists as a profile enum but is not wired into the current `mapping` runtime.
   Evidence:
   - `collector/snmp/ddsnmp/ddprofiledefinition/metrics.go:28-35` defines `flag_stream`.
   - Repository search found no collector logic branching on `flag_stream`; only commented profile examples reference it.

6. The requested Dell example is a true bitmask, not an enum.
   Evidence:
   - The MIB values are powers of two (`1, 2, 32, 128, 256, 512, 1024`) and the ASN.1 comment explicitly says combination values are possible.
   - Under current exact-match behavior, a combined value such as `129` would incorrectly produce all mapped dimensions as `0`.

# User Made Decisions

- The work is limited to SNMP profile value transformation support.
- The requested feature is bitmask mapping support for value transformation.
- The motivating example is Dell status/failure fields where multiple bits may be set simultaneously.
- Confirmed: when unknown bits are present, set known mapped dimensions and ignore unknown bits.
- Confirmed: preserve the raw numeric metric `Value` and add decoded `MultiValue` dimensions.
- Clarified preference under discussion: `mapping_bitmask` may be a separate alternative mapping field rather than a boolean modifier.
- Confirmed: refactor `mapping` into a structured object with backward-compatible YAML unmarshal.
- Confirmed: apply that refactor consistently across all `mapping` uses, not only metric value mapping.
- Confirmed: create a branch and commit the current refactor state before addressing the review findings in a follow-up change.

# Implied Decisions

- Any implementation should preserve existing `mapping` behavior for non-bitmask values.
- The profile format documentation and tests need to be updated together with runtime behavior.
- The work should cover both scalar and table metric symbols because both use the same value processing and metric building path.
- Virtual metric dimension discovery must remain coherent with the new mapping behavior.

# Pending Decisions

1. Mapping schema refactor shape and scope.
   Evidence:
    - Existing per-symbol boolean feature toggles use dedicated snake_case booleans, for example `constant_value_one` in `collector/snmp/ddsnmp/ddprofiledefinition/metrics.go:128-141`.
    - Existing per-symbol multi-mode settings use string enums only where there are already multiple values, for example `format` and `metric_type` in `collector/snmp/ddsnmp/ddprofiledefinition/metrics.go:128-141`.
    - There is no existing generic `*_mode` field pattern in `ddprofiledefinition`.
    - `metric_type: flag_stream` exists, but it is not connected to this runtime path and would mix metric-kind semantics with mapping semantics.
    - Additional evidence:
     - `collector/snmp/ddsnmp/ddsnmpcollector/value_processor.go:90-95,128-130` uses `mapping` for numeric/string remapping before metric construction.
     - `collector/snmp/ddsnmp/ddsnmpcollector/metric_builder.go:112-157` uses the same `mapping` field for exact-match MultiValue generation.
     - `collector/snmp/profile-format.md:1489,1529-1538` documents `mapping` as exact-match one-hot state mapping.
     - `collector/snmp/ddsnmp/ddprofiledefinition/metrics.go:140` and `collector/snmp/ddsnmp/ddprofiledefinition/metrics.go:187` show that both metric symbols and metric tags currently expose `mapping`.
     - `collector/snmp/ddsnmp/ddprofiledefinition/yaml_utils.go:13-58` shows the repo already uses custom `UnmarshalYAML` for backward-compatible schema evolution.
     - There are at least 727 `mapping:` occurrences in `config/go.d/snmp.profiles/default/*.yaml`, so any non-compatible syntax migration would be expensive.
   - Option A: `mapping_bitmask:` as a separate map field, alternative to `mapping`
     Pros: keeps existing `mapping` semantics untouched, avoids overloading remap/exact-match behavior, and makes bitmask intent self-documenting in profiles.
     Cons: duplicates the map shape and needs validation to reject using both fields together.
   - Option B: keep `mapping:` and add `mapping_bitmask: true` as a mode flag
     Pros: smaller schema expansion and reuses the existing mapping payload.
     Cons: `mapping` already has overloaded remap + exact-match semantics, so adding bitmask mode makes one field do too many different things.
   - Option C: evolve `mapping:` into a structured object, for example `mapping.mode` + `mapping.items`, with custom YAML unmarshal for backward compatibility.
     Pros: cleanest long-term normalized schema if multiple mapping families are expected; can separate enum/remap/bitmask semantics explicitly under one concept.
     Cons: largest implementation scope; affects both value and tag/metadata mapping schema decisions; requires careful backward-compatible parsing and broader docs/tests updates.
   - Option D: `metric_type: flag_stream`
     Pros: no new field.
     Cons: semantically overloaded and currently unsupported in this path.
   - User decision:
     - Choose structured `mapping` (`mode` + `items`) with backward-compatible YAML unmarshal.
     - Apply it consistently across all `mapping` uses.

# Plan

1. Introduce a shared structured mapping type with backward-compatible YAML unmarshal from the legacy flat-map syntax. Done.
2. Replace flat `map[string]string` mapping fields in symbols and metric tags with the new structured type. Done.
3. Update validation so supported mapping modes are enforced per context. Done.
   - metric values: default exact/remap mode plus bitmask mode
   - tags/metadata: default exact mode only
4. Update runtime processing for metric values, tags, metadata, and virtual metric dim inference. Done.
5. Add unit tests for YAML compatibility, scalar/table/runtime behavior, and validation errors. Done.
6. Update `collector/snmp/profile-format.md` for the new schema, legacy compatibility note, and bitmask examples. Done.
7. Update the default Dell PowerEdge profile entries that were previously blocked on bitmask support. Done.

# Testing Requirements

- Add or update unit tests for profile definition validation and collector value processing.
- Verify existing `mapping` tests still pass.
- Add positive tests for combined bitmask values on scalar and table metrics.
- Add coverage for virtual metric dim inference/validation when a bitmask-mapped source metric is referenced.
- Executed: `go test ./collector/snmp/ddsnmp/...`

# Documentation Updates Required

- Update `collector/snmp/profile-format.md` for the new value transformation capability.
- Add a bitmask-specific example based on a real SNMP status/failure field.

# Status

Implementation is complete in code and tests are passing. Awaiting user verification before the TODO is removed.
