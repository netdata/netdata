# Monitor Fleet Health and Observability Cost

Use Netdata Parents as the fleet's operational view: see which devices are online, inspect their system and application health, and troubleshoot connected devices from the same interface. Group devices by labels so your team can work by site, hardware class or software release.

## Find devices and organize the fleet

Use stable host labels for device class, location, hardware revision and deployment ring. Keep a software-release label alongside them so you can identify devices running a particular image.

Build views around these groups. For example, use one view for robots at a site and another for signage players on a particular software release. Start with device connectivity, recent metric updates and active alerts, then drill into the affected system or application.

See [deployment and identity](./deployment-and-identity.md) for provisioning labels and unique device identities.

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
