# Redfish compatibility fixture attribution

These fixtures preserve only the protocol shapes needed by the tests. Hardware identifiers, names, URIs, values, timestamps,
serial numbers, part numbers, and OEM payloads were removed or replaced with synthetic values. They are not verbatim BMC
captures and must not be used to identify a real system.

## Telegraf-derived shapes

- Source: `influxdata/telegraf @ 9df3fe82d7aa2caa0ef5b78c43b24baa289b7638`
- License: MIT
- Reviewed paths:
  - `plugins/inputs/redfish/testdata/dell/dell_power.json`
  - `plugins/inputs/redfish/testdata/hp/thermal_subsys/hp_thermal_metrics.json`
  - `plugins/inputs/redfish/testdata/hp/thermal_subsys/hp_thermal_subsys_fans_0.json`
  - `plugins/inputs/redfish/testdata/hp/power_subsys/hp_power_subsys_metrics.json`
  - `plugins/inputs/redfish/testdata/hp/storage_subsys/hp_storage_drive0.json`
- Derived fixtures:
  - `telegraf-dell-legacy-power.min.json`
  - `telegraf-hpe-modern-thermal-metrics.min.json`
  - `telegraf-hpe-modern-fan.min.json`
  - `telegraf-hpe-modern-power-supply-metrics.min.json`
  - `telegraf-hpe-modern-drive.min.json`

## OpenTelemetry Collector-derived shapes

- Source: `opentelemetry/opentelemetry-collector-contrib @ e926fa77082af36b1b6e60b5dfb7c47fe198c53f`
- License: Apache-2.0
- Reviewed paths:
  - `receiver/redfishreceiver/internal/redfish/testdata/dell/thermal.json`
  - `receiver/redfishreceiver/internal/redfish/testdata/hpe/thermal.json`
- Derived fixtures:
  - `otel-dell-legacy-thermal.min.json`
  - `otel-hpe-legacy-thermal.min.json`

## Checkmk-derived shapes

- Source: `checkmk/checkmk @ 8d827dbf48dc`
- License: GPL-2.0
- Reviewed path:
  - `packages/cmk-plugins/tests/cmk/plugins/redfish/agent_based/test_redfish_sensors.py`
- Derived fixture:
  - `checkmk-nvidia-modern-sensor.min.json`

The source test demonstrates that monitoring software encounters the modern standalone Sensor schema on NVIDIA and generic
Redfish devices. The fixture is independently authored from standard Redfish properties with synthetic identity and values; it
copies no source code or literal payload content. The source license is recorded for evidence provenance.

## DMTF Redfish Release 2026.1-derived evidence

- Source: `DMTF/Redfish-Publications @ c3a8a4c26d9243640c8f450c7282ace413b70aa5`
- License: BSD-3-Clause
- Reviewed paths:
  - `mockups/DSP2046-examples/`
  - `json-schema/ComputerSystem.v1_26_0.json`
  - `json-schema/LeakDetection.v1_2_0.json`
  - `json-schema/Manager.v1_22_0.json`
  - `json-schema/Power.v1_7_3.json`
  - `json-schema/PowerSubsystem.v1_1_4.json`
  - `json-schema/Redundancy.v1_7_0.json`
  - `json-schema/Sensor.v1_13_0.json`
  - `json-schema/Storage.v1_21_0.json`
  - `json-schema/Thermal.v1_7_3.json`
  - `json-schema/ThermalSubsystem.v1_5_0.json`
  - `json-schema/Volume.v1_10_2.json`
- Derived fixtures:
  - `dmtf-2026.1-schema-types.json`
  - `dmtf-2026.1-embedded-components.min.json`

The schema-type fixture records the exact public example type annotations selected by the collector. `VolumeMetrics` is a
Swordfish resource referenced by the DMTF Volume schema and therefore has schema-reference rather than Redfish mockup evidence.
The embedded fixture is independently authored from the public Redfish property shapes and uses synthetic identities and values.
It preserves the structural distinction between addressable resource links and embedded redundancy/leak-detection components.

## Scope of evidence

Fixture replay proves schema/type decoding and semantic normalization of these reviewed shapes only. It does not prove live TLS,
session, pagination, throttling, latency, concurrency, firmware correctness, or compatibility with other vendor and firmware
combinations.
