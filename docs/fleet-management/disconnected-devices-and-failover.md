# Handle Disconnected Devices and Parent Failover

Keep monitoring history available when devices lose coverage, enter tunnels or switch networks. Netdata Children collect locally during a connection outage and can send retained history to a Parent when they reconnect.

## Keep history for coverage gaps

Choose local storage according to the device's recovery needs:

- **RAM history** keeps samples through a network outage while the Agent continues running.
- **DBengine history** keeps samples across Agent restarts and device power loss.
- **No local history** resumes current monitoring after reconnection, without backfilling the disconnected period.

Configure retention for the coverage gaps you want to recover and enable historical replication. The Parent requests available history after reconnection. See [CPU and memory settings](./minimize-cpu-and-memory.md) for database choices and the [Parent-Child reference](../../src/streaming/README.md) for replication periods and steps.

The streaming send buffer handles short delays while sending. Use local history for outage recovery, rather than increasing the send buffer to cover a long disconnection.

## Provide alternative Parents

Configure a list of Parent destinations on the Child. It connects to the first available destination and can use another Parent when the current one is unavailable.

Provision matching streaming authorization and TLS trust on each Parent, and allow the device's network sources. Use the [centralization architecture guide](../deployment-guides/deployment-with-centralization-points.md) to arrange Parent replication and retention when your team needs shared historical coverage across Parents.

A destination list provides connection failover; history on each Parent follows your centralization and replication configuration.

## Keep the fleet understandable during outages

The Parent continues to serve the history it has received while a device is offline. Live Functions become available again when the Child reconnects.

Organize alerts around how your devices operate. A sleeping device and a device that should be online need different connectivity expectations. Use labels and alert routing to distinguish planned offline periods from unexpected loss of monitoring.

For cellular devices, combine the recovery policy with [keepalive and traffic settings](./minimize-cellular-traffic.md). After reconnecting a device, check that its current metrics and retained history appear at the Parent.
