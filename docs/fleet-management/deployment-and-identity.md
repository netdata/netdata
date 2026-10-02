# Deploy and Identify Fleet Devices

Connect each edge device to a Netdata Parent so your team can monitor and troubleshoot the fleet centrally. Use a shared device image for installation and configuration, then provision each device's identity and credentials on first boot.

## Choose the device package

Use a Linux static package compatible with the device's CPU and operating system. Netdata provides `armv6l` and `armv7l` packages for 32-bit ARM devices, alongside packages for other supported architectures.

Prepare your reduced package on a build host and install it into the device image. The [disk-footprint guide](./minimize-disk-footprint.md) explains how to select plugins and strip symbols. For installation details, see [static installations](../../packaging/makeself/README.md).

Create a package and configuration policy for each device class. A signage player might need process metrics and display-service monitoring; a robot might also need hardware sensors, containers and application telemetry. Retain the collectors and live Functions your team uses for that class.

## Give each device its own identity

Generate an Agent identity on first boot so every device appears as a separate node. Keep that identity across normal image updates so its monitoring history stays associated with the same device.

Prepare the shared image before claiming or commissioning the Agent. Follow the [VM template guidance](../learn/vm-templates.md) to avoid copying an existing device's identity or Cloud credentials into new devices.

Assign a stable hostname and [host labels](../netdata-agent/configuration/organize-systems-metrics-and-alerts.md) for:

- Customer name or tenant identifier.
- Location, building and room.
- Device class, model and hardware revision.
- Software version or release.
- Deployment ring, such as pilot or production.

These labels let you find devices, organize dashboards and apply alerts to the right groups. Store credentials in your provisioning system rather than in labels.

## Connect to a Parent

In the Child's `stream.conf`, configure the `[stream]` section with streaming enabled, the Parent destination and a streaming API key. On the Parent, configure the matching API-key receiver section and the allowed source addresses.

For a controlled private network, the minimum configuration is:

Child `stream.conf`:

```ini
[stream]
    enabled = yes
    destination = parent.example:19999
    api key = YOUR_STREAMING_API_KEY
    enable compression = yes
```

Parent `stream.conf`:

```ini
[YOUR_STREAMING_API_KEY]
    enabled = yes
    allow from = YOUR_DEVICE_NETWORK
    db = dbengine
    health enabled = yes
    enable compression = yes
    tcp keepalive idle = auto
```

Replace `YOUR_STREAMING_API_KEY` on both sides with the same UUID and `YOUR_DEVICE_NETWORK` with the allowed source addresses or pattern for your private network. Each device also has its own persistent machine identity; sharing a streaming authorization key does not merge device identities. Follow the streaming reference for more restrictive per-device authorization.

On the Parent, enable health and machine learning in `netdata.conf`:

```ini
[health]
    enabled = yes

[ml]
    enabled = yes
```

Keep the Parent's own profile and storage settings appropriate for its capacity. The Child can disable these features while the Parent evaluates the received metrics. Claim the Parent to your Netdata Cloud Space when your operators use Cloud; live sensitive-data Functions such as process inspection require an authorized signed-in session. A streaming key authorizes ingestion and does not grant operators access to these Functions.

Use TLS with certificate verification for devices connecting over cellular or other untrusted networks. Provision the Parent certificate and the required CA trust alongside your device configuration. Streaming API keys authorize the Child-to-Parent connection; Cloud claiming credentials connect an Agent to Netdata Cloud.

For fleets with changing cellular addresses, configure access through your fleet's network or VPN and the appropriate Parent source-address policy. Netdata streaming uses its own protocol; configure direct connectivity to the streaming port.

The [Parent-Child configuration reference](../../src/streaming/README.md) covers destination syntax, authorization and TLS settings.

## Bring devices online

Start with a small deployment ring. In the Parent dashboard, check that:

- Each device appears with its own identity and expected labels.
- System and application metrics are updating.
- The live Functions your team needs are available.
- Parent-side alerts and anomaly detection are enabled as intended.

Once the device class is ready, roll the same package and configuration out to the rest of that group. Use the [update guide](./updates-and-troubleshooting.md) to keep the image and monitoring policy together through future releases.
