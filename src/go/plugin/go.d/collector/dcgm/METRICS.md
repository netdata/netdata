# Interpreting DCGM charts

DCGM fields are related measurements, not generally parts of a single GPU utilization total.
Netdata uses explicit field definitions, including legacy and newer aliases, to preserve their units and meaning.
Chart availability depends on the GPU, driver, DCGM version, exporter field selection and profiling support.

## Activity and allocation

| Chart | Interpretation | Presentation |
| --- | --- | --- |
| Graphics and Compute Engine Activity | Fraction of time any graphics or compute engine is active; not the sum of arithmetic pipelines | Line |
| Streaming Multiprocessor Utilization | `activity`: fraction of cycles with an active warp, averaged across SMs; `occupancy`: resident warps relative to the supported maximum | Two independent percentage lines |
| Tensor and Memory Interface Activity | Tensor pipeline activity and DRAM interface activity, with different hardware denominators | Two independent percentage lines |
| Arithmetic Pipeline Activity | FP16, FP32, FP64 and integer pipeline measurements; hardware can share execution resources | Independent lines |
| Tensor Instruction Activity | HMMA, IMMA and DFMA instruction-family activity; availability and execution resources vary by architecture | Independent lines |
| GPU and Memory Busy Time | Device utilization and memory-copy utilization from the device sampling interface; distinct from profiling activity | Independent percentage lines |
| Host/Peer Memory Cache Efficiency | Reported hit and miss percentages; their sum is not assumed to equal 100 | Separate host and peer line charts |
| VRAM Allocation | Used, free and reserved device memory | Stacked bytes |
| BAR1 Mapping Usage | Used and free BAR1 mapping aperture; not additional device VRAM | Separate stacked bytes |

An **SM** is a Streaming Multiprocessor: a GPU execution unit that schedules groups of threads called warps.
Activity, occupancy, tensor activity and DRAM activity answer different questions. They can be compared in time, but
must not be added. Arithmetic and tensor instruction charts are also comparisons, not complete partitions of GPU work.

Profiling fractions are multiplied by 100. Device GPU utilization (including the newer `GPU_UTIL_RATIO` alias) and
cache hit/miss fields already contain percentages. VRAM used percentage is DCGM's fraction of total minus reserved
memory, multiplied by 100; it does not use BAR1 capacity.

## Clocks, temperature and power

- SM, memory and video clocks have separate MHz charts. Current, application and maximum references are visible when supplied.
  Application clocks are requested settings, not an additional measured frequency. CPU clocks are converted from kHz to MHz.
- Absolute GPU/memory temperatures and their operating, slowdown and shutdown references share a Celsius chart.
  Thermal headroom is separate: legacy `GPU_TEMP_LIMIT` and newer `GPU_TEMP_MARGIN_CELSIUS` report a margin, not a temperature limit.
- Power draw, instantaneous draw and configured/enforced limits are watts. The sampling behavior of reported draw depends on
  the GPU; on relevant newer devices it is an average over the preceding second. Configurable minimum/default/maximum limits
  are separate reference lines. CPU draw and its cap are also watts.
- Energy-derived power is the rate of cumulative millijoules divided by 1,000: joules per second are watts.
- Clock event reason masks are decoded into named 0/1 states. Duration counters are nanoseconds; their rate is multiplied
  by `1e-7` to express percentage of elapsed time. Causes overlap and are not stacked. The software power-cap duration and
  legacy power-violation duration select one source because they refer to the same NVML counter.
- Power smoothing separates watts, percentages, watts/second, milliseconds, enablement, privilege, profile identity and
  profile capacity. TMP means Total Module Power; its ceiling and floor are watts. Ramp rates are converted from milliwatts/second. Circuit lifetime remaining is separate from power-floor percentages.
- Reported fan speed is the vendor's percentage, not RPM.

## Reliability and categorical values

- ECC current/pending are enablement states, not error counts. Pending means the setting for the next activation.
- ECC totals have separate count and rate charts. Volatile counts and persistent aggregate counts have different reset
  lifetimes and overlap; they are separate charts. Location detail is separate from each lifetime's total.
- Retired pages and remapped rows have separate cumulative counts and event rates. Pending/failure flags remain states.
  Remap-bank availability is a stacked count of banks by availability class.
- XID and NVSwitch SXid fields contain error identifiers, not cumulative error counters. They remain code gauges.
  NVSwitch ECC lane rates are separate from their total.
- Performance, compute and virtualization modes and Fabric Manager status are decoded into named states, plus `unknown`.
  Fabric Manager error codes remain signed codes. Other source-defined status values retain their reported numeric state.
- Exporter health status retains one dimension per `health_watch`: 0 is PASS, 10 WARN and 20 FAIL. Independent watches are
  never summed. Peer access retains the peer identifier. Clock/XID event dimensions retain their reason/code; windowed sample
  counts use separate chart instances for each `window_size_in_ms`. Event totals use rate charts.
- GPU inventory count is emitted once per exporter host, even when the exporter repeats it on GPU-labeled samples.

## Interconnect measurements and source selection

The overview shows observed PCIe and NVLink receive-plus-transmit throughput as independent lines.
It is not a bandwidth capacity chart or a sum of GPU utilization.

| Source | Input representation | Conversion to bytes/second |
| --- | --- | --- |
| Profiling PCIe/NVLink byte fields, including per-link profiling fields | Bytes/second gauge | None |
| Profiling PCIe byte totals and NVLink byte counters | Cumulative bytes | Counter delta / elapsed scrape time |
| Legacy NVLink bandwidth and corresponding throughput fields | Rate computed by DCGM as NVML KiB delta / seconds / 1,000 | Multiply by 1,024,000 |
| C2C maximum bandwidth | MB/second capacity | Multiply by 1,000,000; separate capacity chart |

For each transport, the overview prefers a complete profiling RX/TX pair, then a complete legacy rate source, then a
complete normalized byte-counter pair. Aliases select one valid source, never sum. RX from one source family is not paired
with TX from another. A legacy combined total can supply the overview without directional fields.

A complete device aggregate takes precedence over its component links. Otherwise the overview can sum the complete
observed links. It does not infer missing links or a missing direction as zero, so this fallback represents observed traffic,
not guaranteed whole-device coverage. Per-link combined totals are kept separate from directional charts.
Raw byte counters require two monotonic observations; first observations, resets and return after a successful absent
scrape establish a baseline without manufacturing a zero rate. Failed scrapes retain the baseline, and the next rate uses
actual elapsed time. The dimension's algorithm and scale stay stable when a source disappears and a fallback becomes available.

C2C all-traffic and payload-only throughput are separate because they overlap. NVSwitch throughput fields and unsupported
legacy PCIe throughput fields do not have a verified, common physical-unit contract across their available backends;
they remain individual raw measurements and do not contribute to byte-throughput totals.

Bit error ratios use **errors per trillion bits**. For example, a ratio of `1e-12` displays as 1, and `1e-18` as `0.000001`.
Packed BER values are decoded as `((value >> 8) & 15) * 10^-(value & 255)` before scaling and take precedence over
float aliases of the same measurement. [DCGM Exporter 4.6.0-4.8.3](https://github.com/NVIDIA/dcgm-exporter/blob/4.6.0-4.8.3/internal/pkg/collector/gpu_collector.go)
formats doubles with six decimal places: `1e-12` becomes `0.000000`, and `6e-7` becomes `0.000001`. Prefer the optional
packed `*_BER_RAW` or legacy `COUNT_*_BER` field when available. A float-only source retains the exporter's precision
limit; Netdata cannot recover rounded-away information.

After decoding, six fractional places in errors-per-trillion units retain ratios through `1e-18` while keeping dimension
divisors compatible with 32-bit builds. Smaller positive values round at that storage resolution; zero is not proof of
zero errors when either the source or storage precision is insufficient.

Unknown numeric fields appear under `dcgm.<entity>.raw.<field>` with `value` or `value/s` units. The collector does not infer
physical units from words such as CLOCK, ECC or BANDWIDTH. Raw fallback follows the exporter's gauge/counter declaration
unless an explicit field definition corrects it. Non-identity labels distinguish raw dimensions; punctuation is escaped and
case preserved to prevent collisions. Unsupported sentinels, non-finite values and unrepresentable scaled values leave gaps.

## Chart migration

Chart IDs and internal metric keys now include a hash of the length-framed original identity components. Entity label values are
escaped before key construction, so punctuation and delimiter characters cannot merge distinct devices or workloads.
This changes all DCGM chart instance IDs, including contexts whose names stay the same. New instances start new history;
old history is retained under its previous IDs. Label-qualified dimension names use `key=value` tokens, for example
`value_health_watch=MEM`, to preserve the boundary between label names and values. Prefer context-and-label queries,
and update any saved chart-ID references.

Existing stored history is not rewritten. Update custom dashboards, exports and alert overrides that reference the
following contexts or renamed dimensions. Prefixes below use `dcgm.gpu`; the corresponding entity-specific changes also apply.

| Previous context or grouping | Replacement |
| --- | --- |
| `compute.activity` | `compute.engine_activity`, `compute.sm.utilization`, `compute.resource_activity`, `compute.pipe.activity` |
| `compute.tensor.activity` | Retained for `tensor_hmma`, `tensor_imma`, `tensor_dfma`; independent lines |
| `compute.cache.activity` | `compute.cache.host`, `compute.cache.peer`, now percentages |
| `clock.frequency` | `clock.sm.frequency`, `clock.memory.frequency`, `clock.video.frequency`; `current`, `application`, `maximum` |
| `memory.ecc_errors`, `memory.ecc_error_rate` | `memory.ecc_mode`, lifetime-specific `memory.ecc_*` count/rate/detail charts |
| `memory.page_retirements` | Retained as pages/second, plus `memory.retired_pages` and `reliability.page_retirement_status` |
| `thermal.temperature` | Absolute temperatures/limits retained; margin moves to `thermal.headroom` |
| `power.energy` | `power.energy_rate`, now watts |
| `power.smoothing` | Separate `power.smoothing.*` unit groups |
| `throttle.violations` | `throttle.duration`, now percentage; power/thermal alert dimension names retained |
| `throttle.reasons` | Named states instead of a numeric bitmask |
| `health.status` | Health watches only; exporter clock/XID samples and totals move to their event charts |
| `reliability.memory_health` | Flags retained; remap-bank histogram moves to `reliability.remap_banks` |
| `state.performance`, `state.configuration`, `state.virtualization`, `interconnect.fabric` | Named categorical states; Fabric Manager errors move to `interconnect.fabric_error` |
| `capability.support` | MIG slice count moves to `capability.mig_slices`; unverified fields use individual raw contexts |
| `inventory.device` device count | `dcgm.host.inventory.gpu_count` |
| Generic vGPU grouping | Separate `virtualization.vgpu.license`, `.memory`, `.frame_rate`; unverified fields use raw contexts |
| Interconnect bandwidth/error groupings | Explicit transport throughput, capacity, rate, BER and code charts; unverified fields use raw contexts |

The stock power/thermal alerts follow `throttle.duration` and report five-minute average percentages.
The remapped-row alert reports an average event rate rather than a sum mislabeled as rows. All five existing alert names,
thresholds, windows, evaluation intervals, recipients and notification delays are preserved. Their policy remains detection
of any positive observation in the window. Missing data uses Netdata's existing lookup and obsoletion behavior.
A user-owned `health.d/dcgm.conf` shadows the stock file and must be migrated separately.

## Historical duration wording

Some older DCGM headers describe power-violation duration as microseconds. The actual
[v3.0.4 field mapping](https://github.com/NVIDIA/DCGM/blob/f6fe5654b780873da528b84cb3d7de10d7abe0d1/dcgmlib/src/dcgm_fields.cpp#L1657-L1666)
selects the NVML power-policy counter, and the
[bulk collection path](https://github.com/NVIDIA/DCGM/blob/f6fe5654b780873da528b84cb3d7de10d7abe0d1/dcgmlib/src/DcgmCacheManager.cpp#L5751-L5759)
stores the value without a unit conversion. The
[v4.1.0 header](https://github.com/NVIDIA/DCGM/blob/37b325dee7e166a0fce7da8261432d760d4e9fc7/dcgmlib/dcgm_fields.h#L763-L776)
corrected the wording to nanoseconds without changing that conversion behavior. Netdata therefore uses the nanosecond
counter semantics rather than introducing a version multiplier based on the old wording. This is source verification;
not every historical driver/exporter combination has been exercised.

## Sources

- [NVIDIA DCGM field reference](https://docs.nvidia.com/datacenter/dcgm/latest/dcgm-api/dcgm-api-field-ids.html)
  and [exporter metrics and entity labels](https://docs.nvidia.com/datacenter/dcgm/latest/reference/dcgm-exporter-metrics.html).
- [DCGM v4.6.1 field definitions and aliases](https://github.com/NVIDIA/DCGM/blob/v4.6.1/dcgmlib/dcgm_fields.h)
  and [field metadata](https://github.com/NVIDIA/DCGM/blob/v4.6.1/dcgmlib/src/dcgm_fields.cpp).
- [DCGM v4.6.1 cache manager](https://github.com/NVIDIA/DCGM/blob/v4.6.1/dcgmlib/src/DcgmCacheManager.cpp):
  memory accounting, utilization units, power policy fields and legacy NVLink bandwidth conversion.
  The same legacy conversion is present in [v4.4.1](https://github.com/NVIDIA/DCGM/blob/v4.4.1/dcgmlib/src/DcgmCacheManager.cpp).
- [DCGM v4.6.1 NVML definitions](https://github.com/NVIDIA/DCGM/blob/v4.6.1/sdk/nvidia/nvml/dcgm_nvml.h):
  power counter aliases, states and smoothing units;
  [NVML device queries](https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html): power and fan sampling semantics.
- [DCGM v4.6.1 CPU monitoring](https://github.com/NVIDIA/DCGM/blob/v4.6.1/modules/sysmon/DcgmSystemMonitor.cpp):
  utilization fractions, temperature and power conversion.
- [DCGM BER decoding](https://github.com/NVIDIA/DCGM/blob/v4.4.1/common/DcgmUtilities.cpp),
  [NVSwitch field definitions](https://github.com/NVIDIA/DCGM/blob/v4.6.1/modules/nvswitch/FieldDefinitions.h)
  and [NVSwitch backend implementation](https://github.com/NVIDIA/DCGM/tree/v4.6.1/modules/nvswitch).
