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

## Corrected AGX Orin R36.5.0 captures

Public attachments to [issue #24165](https://github.com/netdata/netdata/issues/24165)
contain paired root (`tegrastats-caller.stdout`) and Netdata-account
(`tegrastats-collector.stdout`) captures from one AGX Orin running Jetson Linux R36.5.0:

- `orin-r36.5-15w-root.txt`: attachment 33305200, caller line 1, from the
  [corrected 15W capture](https://github.com/netdata/netdata/issues/24165#issuecomment-6110123951).
  GPC 1 explicitly reports zero.
- `orin-r36.5-15w-account.txt`: the same attachment, collector line 9. GPU clocks and EMC are absent;
  CPU/CV current power is zero even though its running average is nonzero.
- `orin-r36.5-50w-root.txt`: attachment 33305375, caller line 1, from the
  [corrected 50W capture](https://github.com/netdata/netdata/issues/24165#issuecomment-6110210682).
- `orin-r36.5-maxn-root.txt`: attachment 33305418, caller line 1, from the
  [corrected MAXN capture](https://github.com/netdata/netdata/issues/24165#issuecomment-6110239232).

Only raw telemetry lines were copied. These samples verify decoding, zero preservation, and missing-field
withdrawal through the collector; they do not establish packaged Netdata privilege, shutdown, or performance
behavior on Jetson hardware. The captures are sequential, so their values are not simultaneous comparisons.

## SoC temperatures

The [NVIDIA R36.5 glossary](https://docs.nvidia.com/jetson/archives/r36.5/DeveloperGuide/AT/JetsonLinuxDevelopmentTools/TegrastatsUtility.html#reported-statistics)
defines `name@valueC` as a named processor-block temperature in Celsius. All 304 records in the four corrected
AGX Orin captures report scalar `cpu`, `gpu`, `soc0`, `soc1`, `soc2`, and `tj` values under both accounts.
This stage collects those exact names without case aliases; it does not infer coverage of every legacy sensor.
Fractions, signed scalar values and zero are preserved without deriving a temperature from other readings.
Non-sentinel below-zero values have parser coverage, not direct device validation.

The [NVIDIA AGX Orin FAQ](https://forums.developer.nvidia.com/t/jetson-agx-orin-faq/237459)
identifies `-256C` as an unavailable powered-off GPU/CV sensor. It is omitted, as are absent, malformed and duplicate
sensor fields. Losing one temperature does not discard other readings; a later valid value resumes the same identity.

[R39.2 public captures](https://forums.developer.nvidia.com/t/devkit-idle-soc-temperature/383251)
also show paired temperature fields. Their member meanings are unproved, so the parser rejects the complete pair
rather than treating its prefix as a scalar. Additional sensor names and paired forms remain evidence-gated follow-ups.
