# Monitor Fleet Health and Observability Cost

Use Netdata Parents as the fleet's operational view: see which devices are online, inspect their system and application health, and troubleshoot connected devices from the same interface. Group devices by labels so your team can work by site, hardware class or software release.

## Organize devices around your business

Host labels turn a fleet into the groups your team works with. Attach a customer name or tenant identifier, location, building, room, device model, hardware revision and software version to each device. These labels travel with the device's metrics to its Parent.

Use consistent label names across your images. For example:

| Host label | Example | Use it to |
|:--|:--|:--|
| `customer` | `tenant-a` | Select one customer's devices |
| `location` | `north-campus` | Review a site |
| `building` | `warehouse-2` | Find devices in a building |
| `room` | `loading-bay` | Narrow an incident to a room or area |
| `model` | `robot-r2` | Compare devices with the same hardware |
| `software-version` | `2026.10.1` | Check a rollout or investigate a release |
| `deployment-ring` | `pilot` | Separate pilot and production devices |

Customer labels support a multi-tenant operating view. Configure Cloud permissions and Room access for the people who should see each tenant's devices; a label or a map filter does not grant or restrict access by itself.

See [deployment and identity](./deployment-and-identity.md) for provisioning labels and unique device identities, and [organizing systems](../netdata-agent/configuration/organize-systems-metrics-and-alerts.md) for label configuration.

## See the fleet in the Nodes Map

Open the **Nodes** view in Netdata Cloud and select **Map**. Devices appear as hexagons, arranged into the groups you choose. The map gives your team a compact view of thousands of devices, with controls for the questions you need to answer:

1. **Filter** to the devices you need: a customer, location, building, room, model or software version. Combine filters to investigate, for example, one model running one release at one site.
2. **Group** devices with **Group By**. Use host labels for business and deployment groups, or a metric with numeric ranges for operational groups such as stopped, moving slowly and moving quickly.
3. **Annotate the view** with the labels and metric fields your team needs. Use **Edit columns** to choose those fields; they are available in the device inspector. Inspect a hexagon to see its identity, operating values and supporting details before opening its charts.
4. **Color** hexagons with **Fill Color** and **Border Color**. These are independent: use a workload metric for the fill and node or alert status for the border, so operational values and monitoring health stay visible together.

For metric colors, set the warning and critical thresholds and choose whether higher or lower values indicate a problem. A motor's excessive RPM and a battery's low charge need opposite severity directions. Map color thresholds control the presentation; configure health alerts separately when you also need notifications.

### Investigate a device or a group

Group summaries show the number of devices, node and alert status distributions, and the values used for fill and border colors. Expand a group to inspect its devices, or collapse it to keep the fleet overview compact. Use search and zoom to find a device in a large fleet.

Select a device to open its inspector. You can see its connection and alert status, the metric behind each color, trends for the selected time window, and the label and metric fields chosen with **Edit columns**. For example, keep customer, building, room, model and software version next to operating-state and motor-speed values. Your team gets both the business context and the measurements needed to investigate.

Node notes add human context, such as a maintenance observation or an issue under investigation. Where your permissions allow it, use the device's **Add note** or **Edit note** action. Notes complement host labels and metrics: labels describe the device, metrics describe its operation, and notes explain what your team knows about it.

Choose a time range and aggregation that match the question. An average over an hour answers a different question from the most recent operating state. Missing metric values are shown separately; they are not equivalent to a healthy zero.

The [Home tab](../dashboards-and-charts/home-tab.md) also provides a Room overview and a node-status map. Use the Nodes Map for the label and custom-metric workflow described here.

## Add your application's operating metrics

Your application can send custom metrics to the device's Netdata Agent using [StatsD](../../src/collectors/statsd.plugin/README.md). The Agent collects them locally and streams them to the Parent alongside system metrics. Your team can use them in charts, alerts and the Nodes Map.

Use gauges for current states and measurements:

| Operational question | Example gauge | Values |
|:--|:--|:--|
| Is the robot working? | `robot.working` | `0` for idle, `1` for working |
| Is the safety interlock engaged? | `robot.interlock` | `0` for disengaged, `1` for engaged |
| How fast is the vehicle moving? | `vehicle.speed` | Current speed in your chosen unit |
| How high is the drone? | `drone.altitude` | Current altitude, with a defined reference and unit |
| How fast is the motor turning? | `motor.rpm` | Revolutions per minute |

A StatsD gauge message such as `robot.working:1|g` sets the value to one; send `robot.working:0|g` when it becomes idle. Send numeric measurements the same way, such as `motor.rpm:1450|g`. Keep metric names and units consistent across devices so the same map settings apply throughout the fleet.

Use the local StatsD listener, normally port `8125` on localhost, when the application runs on the device. The built-in StatsD collector is part of the standard static Agent executable and remains available when optional external plugins are removed. Enable and configure it using the StatsD guide, and confirm that the resulting charts arrive at the Parent before selecting them as map metrics.

Publish the state regularly as well as when it changes. Gauges can retain their previous value when the application stops sending; use application heartbeat or freshness monitoring alongside operating-state gauges. At sparse collection intervals, short state transitions may be averaged or missed. Keep faster collection for vehicle motion, safety-related diagnostics or other behavior where your team needs that detail.

For example, filter to `customer=tenant-a`, group by `building`, color the fill by `robot.working`, and color the border by alert status. Your team can see which robots are active and which need attention without opening thousands of individual charts.

## Place devices on a geographic map

For geographically distributed fleets, attach `latitude` and `longitude` host labels with decimal-degree coordinates. The geographic map places devices at those coordinates and shows node status and active alerts. Use it to locate affected sites, then open the device for investigation. The Nodes hexagon map provides the label and metric organization described above; geographic coordinates are not required for that view.

Neither map requires `network-viewer.plugin`. Retain that plugin when you also want live network-connection inspection and connection topology on the device.

## Alert on device health

Run alert evaluation on Parents to centralize notifications and reduce work on devices. Include the metrics that matter to your workload, such as temperature, storage space, service availability and sustained CPU or memory pressure.

Configure alert lookups and delays for the collection interval. With five-minute samples, use alert windows that cover those samples. Keep faster collection for failures that require faster detection. During commissioning, check the alert values and statuses at the Parent after at least two collections. Investigate alerts that remain undefined and adjust their lookup windows and delays to cover the device cadence. A short window that works for one-second collection may need to be extended for five- or ten-minute collection.

Set connectivity expectations for always-on devices separately from devices that sleep or operate intermittently. The [disconnection guide](./disconnected-devices-and-failover.md) covers history and recovery.

## Allow anomaly detection to train

Enable machine learning on the Parent and retain enough history for its training window. Moving the analysis off the device keeps that work centralized; the Parent still needs valid collected samples before it can train a model.

The defaults use a six-hour training window and a three-hour retraining interval. At five- and ten-minute collection, the default preprocessing needs at least seven valid values. Allow roughly 35 or 70 minutes to accumulate those values, plus time for scheduled training; gaps can extend the warmup. Verify model coverage and anomaly scores for the dimensions your team relies on before treating the device class as commissioned.

## Troubleshoot live devices

Use live Functions through the Parent to inspect a connected device:

- Retain `apps` for process metrics and live process inspection.
- Retain `network` for live network connections and process information.
- Retain `journal` for journal inspection.
- Retain the relevant application collectors for their metrics and supported Functions.

The device executes the Function and returns its results through the Parent. Your team gets central access to detailed diagnostics without maintaining a local dashboard on each device. Historical charts remain available when the device is offline.

## Track connection and cellular traffic

Use per-Child streaming statistics on the Parent to follow transferred bytes, reconnects and historical replication. Statistics for disconnected Children help your team investigate intermittent coverage.

Look for repeated reconnects and unexpected increases in traffic. Check the actual collection intervals, keepalive policy and amount of history being replicated. Use the [cellular guide](./minimize-cellular-traffic.md) to adjust those settings.

## Keep monitoring lightweight

Monitor the Agent and its plugins for CPU, memory, local storage growth and errors. Watch collected metric counts when devices discover new containers, interfaces or application instances.

Review these signals after an image update and apply changes to the relevant device class. Keep the package policy, runtime configuration and image release together in your fleet inventory, using the [update workflow](./updates-and-troubleshooting.md).
