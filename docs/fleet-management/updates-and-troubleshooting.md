# Update and Troubleshoot Reduced Fleet Images

Keep edge devices current through your fleet image pipeline. Prepare a reduced Netdata package for each release, apply the device class's runtime configuration, and deliver both together as part of the image update.

## Prepare each release

1. Download the upstream installer for each device architecture and verify its published checksum.
2. Run the [preparation script](./minimize-disk-footprint.md) with your saved capability and stripping policy into a fresh output.
3. Keep the generated manifest and checksum with the image release.
4. Install the reduced package into a clean image and apply streaming, collection and local-storage settings.
5. Bring a pilot group online, check its metrics and live Functions, then expand the rollout.

Keep each device's identity and required configuration through normal updates so its history remains associated with the same node.

## Manage updates through one pipeline

Use the image pipeline as the update authority for reduced installations. The standard Netdata auto-updater installs the full upstream package, so configure [updater controls](../../packaging/installer/UPDATE.md) as part of device provisioning. Include existing updater schedules when migrating a previously commissioned device to image-managed updates.

Deliver security updates through the same pipeline and maintain a release schedule for each device class.

## Install into a clean image

Prepare a clean image filesystem for each reduced package. The installer extracts its files into the destination; it does not remove files left by an older, larger installation. Building a clean image ensures that the installed components match the capability policy.

For existing devices, use your fleet platform's migration and state-preservation process when moving to the reduced image.

## Keep recovery straightforward

Retain the preceding working image and its runtime configuration for rollback. Keep the original upstream installer, preparation script revision, capability policy and generated manifest in the release inventory.

When stripping all removable symbols, keep the matching original binaries on the build host for crash diagnosis. If package preparation fails, resolve the reported error and retry with a fresh output rather than deploying the incomplete result.

## Troubleshoot common issues

| Issue | Action |
|:--|:--|
| The stripping tool fails | Select an `objcopy` implementation that supports the device architecture |
| A requested capability is unavailable | Use a package that includes that plugin or adjust the device class's selection |
| Metrics or live Functions are missing | Check the retained collector, job settings, permissions and Child connection |
| The package is larger than expected | Review retained bundles such as `go`, dashboard files and IP-intelligence data |
| Removed files return after an update | Check the stock updater and other deployment agents; build the next image from a clean filesystem |
| Device CPU or memory increases | Check newly discovered metrics, collector intervals and local retention |
| Cellular traffic increases | Check reconnects, chart intervals, replication and image downloads |
| A device does not reconnect | Check network access, Parent destinations, TLS trust and streaming authorization |

Use the [support bundle tool](../developer-and-contributor-corner/netdata-support-bundle.md) to collect diagnostics for unresolved issues. Handle bundles through your organization's normal process for operational data.
