# Deploy and Identify Fleet Devices

Give every device a place in your fleet's monitoring view. Install a small Netdata Agent in your device image, connect it to a Parent, and attach the labels your team uses to organize customers, sites and releases. The Parent brings the devices' charts, alerts and live troubleshooting into one interface.

## Prepare a package for each device class

Netdata provides static Linux packages for 32-bit ARM devices, including `armv6l` and `armv7l`. Choose the package for your device's CPU and operating system, then use the [disk-footprint guide](./minimize-disk-footprint.md) to retain the capabilities your team needs.

For example, monitor a signage player's system and processes; include hardware sensors, container monitoring and application telemetry for a robot running containerized control software. Use the same package and runtime configuration across devices with the same role.

## Preserve each device's identity

Generate the Agent identity on first boot and preserve it through image updates. Each device then appears as its own node, with monitoring history that follows it across releases.

Prepare shared images before claiming or commissioning their Agents. The [VM template guide](../learn/vm-templates.md) explains which identity and Cloud credentials to initialize separately on each device.

## Attach customer and location labels

Add a `[host labels]` section to the device's `netdata.conf`:

```ini
[host labels]
    customer = tenant-a
    location = north-campus
    building = warehouse-2
    room = loading-bay
    model = robot-r2
    software-version = 2026.10.1
    deployment-ring = pilot
```

Populate these values through your provisioning system. The labels travel with the device's metrics, so operators can filter and group by customer, site, room, hardware or software release. Use a stable hostname to make individual devices easy to recognize.

The [label configuration guide](../netdata-agent/configuration/organize-systems-metrics-and-alerts.md) also shows how to populate labels from environment variables. The [fleet monitoring guide](./monitor-the-fleet.md) shows how to use them in maps and investigations.

## Connect devices to a Parent

Start with this configuration on a private network. In the Child's `stream.conf`:

```ini
[stream]
    enabled = yes
    destination = parent.example:19999
    api key = YOUR_STREAMING_API_KEY
    enable compression = yes
```

In the Parent's `stream.conf`:

```ini
[YOUR_STREAMING_API_KEY]
    enabled = yes
    allow from = YOUR_DEVICE_NETWORK
    db = dbengine
    health enabled = yes
    enable compression = yes
    tcp keepalive idle = auto
```

The Parent uses automatic TCP keepalives by default. The explicit `auto` line above documents that default; you can omit it. It adapts keepalive frequency to the fastest streamed chart interval, as explained in the [cellular guide](./minimize-cellular-traffic.md#match-keepalives-to-collection).

Use the same UUID for `YOUR_STREAMING_API_KEY` on both sides. Replace `YOUR_DEVICE_NETWORK` with the permitted source addresses or network pattern. A shared streaming key can authorize a device group while each device keeps its own identity and history. Per-device authorization is available when you need finer control.

Enable centralized alerting and anomaly detection in the Parent's `netdata.conf`:

```ini
[health]
    enabled = yes

[ml]
    enabled = yes
```

Apply the [lightweight Child configuration](./minimize-cpu-and-memory.md#configure-a-lightweight-child) on the devices. The Children collect; the Parent stores history, evaluates alerts and runs machine learning.

Keep health enabled on Children that need autonomous responses. Their local alerts can [trigger custom device actions](./monitor-the-fleet.md#automate-actions-on-the-device) while the Parent continues central alerting and notifications.

## Secure the connection and operator access

For cellular or other untrusted networks, provision the Parent's server certificate and private key, and make them readable by its Agent. Set their paths in the Parent's `netdata.conf`:

```ini
[web]
    ssl key = /etc/fleet/parent-key.pem
    ssl certificate = /etc/fleet/parent-cert.pem
```

Provision the trusted CA certificate on each device and add these settings to the Child's `[stream]` section:

```ini
    destination = parent.example:19999:SSL
    ssl skip certificate verification = no
    CAfile = /etc/fleet/parent-ca.pem
```

Place the trusted CA certificate at the configured path. Allow outbound access to the Parent's streaming port, directly or through your fleet VPN. See [TLS certificate configuration](../../src/web/server/README.md) for Parent certificates and the [Parent-Child reference](../../src/streaming/README.md) for destination syntax and authorization.

Claim the Parent to your Netdata Cloud Space to give your team a central operating view. Manage customer access through Cloud permissions and Rooms, and give authorized signed-in operators live Functions such as process inspection. Streaming keys authorize device connections; Cloud permissions authorize your operators.

## Roll out the device image

Start with a pilot group. Open the Parent dashboard to see the devices, labels and incoming charts, and run the live Functions your team will use. Enable the Parent's alerts and anomaly detection, then deploy the same image and configuration to the rest of the device class.

Use the [update guide](./updates-and-troubleshooting.md) to keep each device's identity, reduced package and monitoring policy together through future releases.
