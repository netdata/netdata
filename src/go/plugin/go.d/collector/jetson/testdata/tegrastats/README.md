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

Power uses the first value of each current/average pair, converted from milliwatts to watts.
The [NVIDIA R32.7.6 glossary](https://docs.nvidia.com/jetson/archives/l4t-archived/l4t-3276/Tegra%20Linux%20Driver%20Package%20Development%20Guide/AppendixTegraStats.html)
defines the older unitless pairs and includes the legacy `GPU`, `CPU`, `SOC`, `CV`, `VDDRQ`, and `SYS5V` names.
The R39.2 glossary above defines the explicit `mW/mW` form. Supported rail families also include `VDD_`, `VDDQ_`,
`VIN_`, `POM_`, and `VIN`, as documented or observed in this corpus. Other numeric pairs are not assumed to be power.
Both members must form a valid nonnegative finite milliwatt pair. Only current power is published; the average
window and newer three-value meanings are not established. Duplicate power pairs with the same rail name are
omitted because their identity is ambiguous. `NC` placeholders are not collected.

Rail names remain unchanged and rails are never summed. The
[NVIDIA Orin R36.4.4 power guide](https://docs.nvidia.com/jetson/archives/r36.4.4/DeveloperGuide/SD/PlatformPowerAndPerformance/JetsonOrinNanoSeriesJetsonOrinNxSeriesAndJetsonAgxOrinSeries.html)
shows overlapping rails (`VIN_SYS_5V0` includes `VDDQ_VDD2_1V8AO`). It documents INA3221 voltage/current files,
which do not expose a power feature for Netdata's libsensors collector. On
[Thor](https://docs.nvidia.com/jetson/archives/r39.2/DeveloperGuide/SD/PlatformPowerAndPerformance/JetsonThor.html),
INA3221 component rails have the same gap; the INA238 total-system `VIN` rail can also appear through sensors.
The collector keeps that source-reported rail rather than guessing whether another collector has discovered it.
