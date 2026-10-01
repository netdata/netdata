# Update and Troubleshoot Reduced Fleet Images

Treat the reduced Netdata package as a reproducible image-build artifact. The standard static updater downloads a full upstream package; it does not retain the preparation script's capability policy. Keep security updates flowing through the fleet image pipeline instead.

## Rebuild for every selected release

1. Download the intended upstream release for each architecture and verify its published checksum.
2. Run the pinned preparation script with the recorded capability and stripping policy into a fresh output.
3. Inspect the manifest, omitted paths, measured sizes and derived checksum.
4. Install the derived package into a clean image and apply the fleet runtime configuration.
5. Validate on representative devices, including privileges, selected collectors, streaming, Functions, outages and recovery.
6. Roll out through a canary ring, then progressively expand while monitoring coverage and resource budgets.

A fresh upstream release can add new components or change archive structure. Unknown files survive preparation, which protects compatibility but can increase size. Review the manifest and sizing on every update. Unsupported formats and unfamiliar installer permission policies fail rather than being silently rewritten.

## Keep one update authority

In an image-managed fleet, do not independently enable the standard Agent auto-updater. Audit provisioning and kickstart options as well as existing schedules: disabling an option in a new build does not remove an updater schedule on an already commissioned device. Use the documented [updater controls](../../packaging/installer/UPDATE.md) as part of your fleet's authorized configuration process.

This preparation script neither modifies updater schedules nor restarts services. It cannot guarantee a reduced footprint if another updater installs the full upstream archive later. Define the fleet release cadence and security-response ownership so avoiding the stock updater does not mean avoiding updates.

## Use clean images, not overlay pruning

An installer extracts over an existing installation. Files absent from the reduced archive can remain from a previous full installation. Therefore applying a reduced installer over an existing full installation is not a reliable cleanup operation.

Use a clean image filesystem, or a separately designed migration process with explicit backups, service coordination and approved removal rules. The preparation script intentionally does not perform that live migration. Preserve device identity, configuration and required state through the image platform's normal update mechanism.

## Retain a recovery set

Keep the original installer and checksum, matching unstripped binaries, preparation script revision, policy, manifest and validated image. Retain the preceding known-good image for rollback. Database and identity migration rules still apply when rolling back Agent versions; test the fleet platform's recovery path.

Do not deploy a failed preparation output. Private workspaces remain available for diagnosis, and a failed tree copy may leave an incomplete destination. Retry with a fresh output path after resolving the failure.

## Troubleshoot by symptom

| Symptom | Check |
|:--|:--|
| `objcopy` fails | Tool supports the target ELF architecture; use the correct cross-tool or LLVM tool |
| Requested capability absent | Input architecture and build options; ARMv6 and ARMv7 can include different plugins |
| Required metrics or Functions missing | Retained collector, runtime enablement, permissions, job configuration and connected Child |
| Package is still large | Retained Go bundle, local dashboard, IP-intelligence assets and unknown new files |
| Installed files unexpectedly reappear | Stock updater, another deployment agent, or an overlay over an old image |
| RAM or writes exceed budget | Cardinality, local database mode, metadata/logs, buffers and plugin processes |
| Cellular traffic exceeds budget | Faster charts, reconnects, NAT expiry, replication, TLS, downloads and Function calls |
| Sparse charts are marked disconnected | Parent version includes cadence-aware liveness; actual fastest chart and keepalive configuration |

For unexpectedly high idle CPU on 32-bit Linux, check whether the release contains the [time64 futex fix](https://github.com/netdata/netdata/pull/24049), merged September 27, 2026. That fix addresses a specific wait-loop issue; it does not establish a general CPU budget.

Collect diagnostics before replacing the image. Use the [support bundle tool](../developer-and-contributor-corner/netdata-support-bundle.md) where available, and handle bundles as potentially sensitive operational data. Keep original symbols off-device for crash investigation when deploying stripped binaries.
