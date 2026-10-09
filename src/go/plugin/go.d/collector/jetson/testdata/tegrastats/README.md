# Tegrastats input fixtures

These are public example/captured records, not captures from Netdata test hardware.
Only the raw input records were retained; parser code and expected results are independently authored.

- `tx1.txt`, `tx2.txt`, `nano.txt`, `agx-xavier.txt`, `xavier-nx.txt`, `power-units.txt`,
  `orin-r36.txt`, `orin-utilization.txt`, `thor-frequency.txt`, and `thor-no-gpu-emc.txt`:
  [Datadog Agent Jetson tests](https://github.com/DataDog/datadog-agent/blob/abe4e1de21ca84920a053e27d2a2b4443a8b89d3/pkg/collector/corechecks/nvidia/jetson/jetson_test.go),
  constants `tx1Sample` through `thorNoGPUFields`. Copyright 2016-present Datadog, Inc.; Apache-2.0.
  Board/release labels follow that source and do not establish release-wide compatibility.
- `thor-r38.4.txt`:
  [NVIDIA skills recorded Thor sample](https://github.com/NVIDIA/skills/blob/0e0d506f4eb67204a62586ac5f19df3cb7ad9b1f/skills/jetson-diagnostic/references/tegrastats-fields.md),
  identified by that source as Jetson AGX Thor, R38 revision 4.0.

The [NVIDIA R39.2 field glossary](https://docs.nvidia.com/jetson/archives/r39.2/DeveloperGuide/AT/JetsonLinuxDevelopmentTools/TegrastatsUtility.html)
defines utilization-only, frequency-only and combined GPU/EMC forms, MHz units and per-GPC array meaning.
Scalar GPU clocks come from the older captured records; they are not relabeled as GPC 0.
Synthetic boundary cases in `parser_test.go` cover omissions, invalid numbers and array indexes.

Record recognition requires the leading RAM/largest-free-block header and a CPU array, with an optional numeric
hyphenated date and time prefix. Those fields remain present across this corpus, including the record with neither
GPU nor EMC. This distinguishes records from warnings and isolated metric fragments; it does not claim support for
unobserved future changes to the record envelope. Unsupported and unavailable measurements are omitted, including
`off`; explicit numeric zero remains a measurement.
