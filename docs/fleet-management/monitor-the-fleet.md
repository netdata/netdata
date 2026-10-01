# Monitor Fleet Health and Observability Cost

Monitor the fleet from Parents, with device labels that let operators compare equivalent hardware and deployments. A device with low Agent CPU but missing required telemetry is not a successful optimization.

## Monitor coverage first

Track whether each expected device is connected, how recently its required charts were updated, and whether the commissioned collectors and Functions remain available. Compare the device release and capability policy with the expected deployment ring. Retain the image manifest in your fleet release inventory; the script does not automatically publish that manifest as a host label.

Use stable labels for site, hardware class and software version. Group health and resource comparisons by those labels so a busy robot is not evaluated against an idle signage player.

## Track streaming cost

[Per-Child streaming statistics](https://github.com/netdata/netdata/pull/22784) expose streaming activity at the Parent, including statistics retained for disconnected Children. Inspect the corresponding streaming charts and Functions on the deployed version to identify bytes transferred, connection behavior and replication work.

The device's streaming connection also informs the default-interface identification used by Netdata. Verify the selected interface on multi-interface devices; neither identification nor streaming counters prove a carrier's billed byte count. Compare with device and carrier measurements as described in [cellular traffic](./minimize-cellular-traffic.md).

Monitor reconnect frequency and backfill traffic separately from normal streaming. A reduction in keepalive packets that causes repeated NAT expiry can increase total cost.

## Monitor the Agent itself

Record daemon and plugin CPU, RSS, local storage growth and errors. Track the number of collected dimensions: a changed service configuration or newly discovered container can grow resource usage without a Netdata release change.

Use Parent-side alerts for missing telemetry, hardware health, sustained resource pressure and streaming failures where the relevant charts exist. Tune alert lookups and delays for the actual cadence. A rule designed around seconds may behave differently with five-minute samples; test expected failures rather than assuming unchanged detection speed.

## Validate fleet queries against deployed features

Use dashboards and Functions supported by the deployed Parent and Child versions. Proposed time-aggregation features, such as counting flaps or expressing percentage of time in a state, must not be assumed available from an open development PR. Verify API and UI support before building operational automation around them.

For each release ring, compare baseline and candidate observations over the same workload. Record:

- Required telemetry coverage and alert behavior.
- Installed size, runtime disk growth, CPU and memory.
- Steady traffic, reconnect traffic and recovery completeness.
- Remote Function behavior while connected and the expected failure while disconnected.

Use these results as the release gate in the [update workflow](./updates-and-troubleshooting.md).
