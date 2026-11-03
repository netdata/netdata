# Update and Troubleshoot Reduced Fleet Images

Keep monitoring lightweight through every device release. Prepare a reduced Netdata package with the same capability policy, deliver it through your image pipeline, and preserve each device's identity so its history stays with the same node.

## Build and deliver each release

1. Download the upstream installer for each architecture and verify its published checksum.
2. Run the [preparation script](./minimize-disk-footprint.md) with your saved plugin selection and stripping mode.
3. Install the prepared package into a clean device image and apply the device class's streaming, collection and storage settings.
4. Preserve the device identity and provision any updated labels or credentials.
5. Deploy to a pilot group, open its charts and live Functions, then roll out to the rest of the group.

Build each image from a clean filesystem so the installed plugins match your selection. For existing devices, use your fleet platform's migration process to preserve identity and configuration when switching to the reduced image.

## Keep one update authority

Use your image pipeline as the update authority for both Netdata and its configuration. Turn off the on-device updater schedule when provisioning a reduced installation so each release follows your fleet's package policy. For the static package:

```bash
sudo /opt/netdata/usr/libexec/netdata/netdata-updater.sh --disable-auto-updates
```

Include existing schedules when migrating devices already in service. See [updater controls](../../packaging/installer/UPDATE.md) for scheduler and installation-specific settings.

Deliver security updates through the same pipeline. Save the selected capabilities, preparation-script revision, package manifest and checksums with each image release so you can rebuild the package and identify what is installed.

## Roll back and diagnose issues

Keep the preceding working image and configuration available for rollback. Retain the matching original upstream binaries on the build host when stripping symbols, so crash diagnostics can use them.

| Symptom | What to do |
|:--|:--|
| The stripping tool fails | Select a `strip` tool that supports the device architecture with `--strip-tool` and rerun preparation into a fresh output |
| A selected plugin is unavailable | Choose a package containing it or adjust the device class's selection |
| Metrics or live Functions are missing | Check the retained collector, job settings, permissions and Child connection |
| The package is larger than expected | Review retained bundles such as `go`, dashboard assets and IP-intelligence data |
| Removed plugins return after an update | Disable the stock updater and build the next reduced image from a clean filesystem |
| CPU or memory use increases | Review new collector jobs, discovered metrics, collection intervals and retention |
| Cellular traffic increases | Inspect reconnects, chart intervals, replication and image downloads |
| A device cannot reconnect | Check Parent destinations, network access, TLS trust and streaming authorization |

Use the [support bundle tool](../developer-and-contributor-corner/netdata-support-bundle.md) to collect diagnostics when you need further help.
