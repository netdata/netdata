# Monitor Fleet Health and Observability Cost

Turn thousands of devices into a fleet your team can understand and operate. Use Netdata Cloud and Parents to see device health, organize customers and sites, follow your application's operating values, and investigate a device from the same interface.

## Organize the fleet around your business

Host labels give each device business context. Attach customer, location, building, room, model and software-version labels through your provisioning system. They travel with the metrics to the Parent, so the same groups are available throughout your monitoring workflow.

| Label | Example | Operator use |
|:--|:--|:--|
| `customer` | `tenant-a` | Work with one customer's fleet |
| `location` | `north-campus` | Review a site |
| `building` | `warehouse-2` | Locate affected devices in a building |
| `room` | `loading-bay` | Narrow an incident to an area |
| `model` | `robot-r2` | Compare devices with the same hardware |
| `software-version` | `2026.10.1` | Follow a rollout or investigate a release |
| `deployment-ring` | `pilot` | Separate pilot and production groups |

Use customer labels for a multi-tenant operating view, and Cloud permissions and Room access to control which operators can see each customer's devices. Labels organize the fleet; permissions govern access.

See [device provisioning](./deployment-and-identity.md#attach-customer-and-location-labels) for an example and [label configuration](../netdata-agent/configuration/organize-systems-metrics-and-alerts.md) for additional options.

## See the fleet in the Nodes Map

Open **Nodes** in Netdata Cloud and select **Map**. Each device appears as a hexagon. Choose the grouping, information and colors that answer your team's operational questions:

- **Filter by labels** to focus on a customer, location, building, room, model or software release. Combine filters to investigate one model running one release at one site.
- **Group by labels or metrics** with **Group By**. Arrange devices by customer and building, or use metric ranges to separate stopped vehicles from those moving slowly or quickly.
- **Choose labels and metrics for inspection** with **Edit columns**. Open a hexagon's inspector to see its identity, operating values and the context your team selected.
- **Color fill and border independently** with **Fill Color** and **Border Color**. For example, color the fill by motor speed and the border by alert status, showing operating conditions and device health together.

For metric colors, choose warning and critical thresholds and whether higher or lower values indicate a problem. Excessive motor RPM and low battery charge need opposite severity directions. Configure health alerts for the same conditions when your team also needs notifications.

### Move from the overview to an investigation

Group summaries show device counts, connection and alert status distributions, and the values used for fill and border colors. Collapse groups for a compact overview, expand a group to inspect its devices, and use search and zoom to find a particular node.

Select a device to see its connection state, alerts, color metrics, trends and selected label fields. Keep customer, building, room, model and release beside working-state and speed values, then open the device's charts for more detail.

Add a node note to record maintenance observations or an issue under investigation. Labels describe the device, metrics show its operation, and notes capture what the team knows about it.

Choose the time range and aggregation for the question: recent values show current operation; longer averages show patterns. The inspector also distinguishes unavailable readings so you can recognize missing telemetry.

The [Home tab](../dashboards-and-charts/home-tab.md) provides a Room overview and node-status map. Use the Nodes Map for the label and custom-metric workflows described here.

## Add your application's operating metrics

Bring your application's state and measurements into the same charts, alerts and maps as system metrics. Send simple StatsD messages to the local Netdata Agent, which collects and streams the values to the Parent.

The [lightweight Child baseline](./minimize-cpu-and-memory.md#configure-a-lightweight-child) already enables StatsD. For other Child configurations, enable the built-in collector in `netdata.conf`:

```ini
[plugins]
    statsd = yes
```

Use the local listener, normally `localhost:8125`. StatsD is built into the standard static Agent, so it remains available in packages prepared with optional plugins removed. The [StatsD guide](../../src/collectors/statsd.plugin/README.md) covers listener settings and organizing custom charts.

Use gauges for current states and measurements:

| What operators need to know | Gauge | Value |
|:--|:--|:--|
| Is the robot working? | `robot.working` | `0` for idle, `1` for working |
| Is the interlock engaged? | `robot.interlock` | `0` for disengaged, `1` for engaged |
| How fast is the vehicle moving? | `vehicle.speed` | Current speed in a consistent unit |
| How high is the drone? | `drone.altitude` | Altitude with a consistent reference and unit |
| How fast is the motor turning? | `motor.rpm` | Revolutions per minute |

Send `robot.working:1|g` when working and `robot.working:0|g` when idle. Send measurements in the same form, such as `motor.rpm:1450|g`.

Publish current values regularly and pair them with application heartbeat or freshness monitoring so your team can detect an application that stops reporting. Use faster collection for short state changes or motion that operators need to inspect; use sparse collection for slowly changing status.

For example, filter to `customer=tenant-a`, group by `building`, color the fill by `robot.working` and the border by alert status. Your team can see which robots are active and which need attention without opening thousands of charts. Keep metric names and units consistent across devices so one map configuration works across the fleet.

## Locate devices geographically

Attach decimal-degree `latitude` and `longitude` host labels to place devices on the geographic map. It shows node status and active alerts at their locations, helping operators identify affected sites and open the relevant device.

Use the hexagon Nodes Map to organize the same fleet by labels and operating metrics. Both views work with ordinary device monitoring. Retain the [`network` capability](./minimize-disk-footprint.md#choose-what-to-keep) when preparing the package if you also want live connection inspection and network-connection topology.

## Centralize alerts and anomaly detection

Run alert evaluation and machine learning on the Parent to keep that work off the devices. Keep local health enabled as well when you want [autonomous device actions](#automate-actions-on-the-device). Operators get centralized notifications and anomaly visibility alongside the same fleet charts and maps. Anomaly detection learns patterns in your custom StatsD metrics too, helping identify unusual motor RPM, vehicle speed or other operating values.

Alert on the conditions that affect the workload: temperature, storage capacity, service availability and sustained CPU or memory pressure. Match alert lookup windows to collection cadence—for example, allow a 15-minute window for five-minute samples and a 30-minute window for ten-minute samples. Use faster collection when failures need faster detection.

Enable machine learning on the Parent and keep history for its training window. Anomaly detection becomes available as models train on incoming samples. With sparse collection, allow time for samples to accumulate and for scheduled training; the defaults use a six-hour history window and retrain every three hours.

Use Cloud [node states](../netdata-cloud/node-states-and-transitions.md#states-on-netdata-cloud) to follow connectivity and data availability: **Live** nodes provide current metrics, **Stale** nodes have history available through a connected Parent, **Offline** nodes have no available data source, and **Unseen** nodes have been claimed but have never connected. Use labels to distinguish always-on and sleeping devices in fleet views. The [disconnection guide](./disconnected-devices-and-failover.md) covers ephemerality for planned disconnections and history recovery after an outage.

## Automate actions on the device

Let a device respond to operating conditions itself, even while disconnected from its Parent. Enable `[health] enabled = yes` in the Child's `netdata.conf`, configure the local alerts your application needs, and use each alert's [`exec` setting](../../src/health/REFERENCE.md#alert-line-exec) to run a custom script on alert status transitions.

For example, an action can restart an application, capture diagnostics when a fault appears, or restore normal operation when the alert clears. The script receives the alert's current and previous status so it can choose the action for warning, critical and recovery transitions. Apply the alert's delay and repeat settings to match the response your device needs, and choose collection intervals and alert lookup windows that support its required response time.

Local actions and Parent-side alerting work together: the device responds autonomously while your operators receive centralized visibility and notifications. Budget for local alert evaluation, alert-history storage and the scripts' work when enabling this option; the lightweight baseline keeps health on the Parent for devices that need only central monitoring.

## Troubleshoot devices through the Parent

Run live Functions to inspect a connected device without opening a local dashboard or establishing an SSH session:

| Retained capability | What operators can inspect |
|:--|:--|
| `apps` | Processes and their resource use |
| `network` | Network connections and process attribution |
| `journal` | Local systemd journal records |
| Application collector bundles | Their supported application-specific Functions |

The Child executes the request and returns results through the Parent. Historical charts remain available while the device is offline; live Functions return when it reconnects.

## Follow connectivity, traffic and monitoring overhead

Use per-Child streaming statistics on the Parent to follow bytes transferred, reconnects and historical replication, including statistics for disconnected Children. Repeated reconnects, unexpectedly fast charts or large recovery transfers help explain rising cellular use.

Follow Agent and plugin CPU, memory, errors and metric counts after image updates or new collector jobs. Apply changes to the relevant device class, keeping the package selection, runtime configuration and image release together.

Use the [cellular guide](./minimize-cellular-traffic.md) to tune traffic and the [update workflow](./updates-and-troubleshooting.md) to preserve the monitoring policy through releases.
