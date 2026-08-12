# Netdata Agent Maintenance Operations Overview

This section covers the routine lifecycle operations for an installed Netdata Agent: controlling its service, applying an
update, and removing an installation. Choose the procedure for the Agent's operating system and original installation type.
Package-manager, kickstart, source-built, container, Homebrew, and Windows installations do not all use the same files or
update mechanism.

## Control the Agent service

Use [Service Control](/docs/netdata-agent/start-stop-restart.md) to start, stop, restart, or inspect the Agent on systemd and
non-systemd UNIX systems, Windows, and macOS. Most configuration changes require a restart. Alert configuration is an
important exception: reload health configuration with `netdatacli reload-health` to avoid a collection gap.

Before and after a service operation, verify that the Agent responds and is ready. A local API request checks reachability:

```bash
curl http://localhost:19999/api/v1/info
```

On supported installations, `sudo netdatacli ping` provides a readiness check. A restart temporarily interrupts collection
while the Agent and its collectors initialize, so coordinate restarts on systems where a short metric gap matters.

## Update the Agent

Follow [Update Netdata](/packaging/installer/UPDATE.md) to identify the installation type and select its supported update
path. Native DEB and RPM packages are updated through package-management workflows. Kickstart installations use the
installed updater through the kickstart script. Homebrew, Windows, Docker, and manual source builds each have separate
procedures.

Do not choose an update command only from the operating system name. First inspect the installation type with the command in
the update guide, preserve any custom installation prefix or build options, and review the release and packaging changes
relevant to your environment. After the update, confirm the running version, API reachability, collector operation, Cloud
connection if used, and alert state.

For a fleet, test the update on representative systems before broad rollout. Include the operating systems, architectures,
installation methods, enabled collectors, and Parent or Child roles present in production. Staggering the rollout limits the
number of nodes affected by an unexpected packaging or compatibility problem.

## Uninstall the Agent

Use [Uninstall Netdata](/packaging/installer/UNINSTALL.md) when the Agent must be removed or when changing to an incompatible
installation method. The correct uninstaller depends on how Netdata was installed. Native packages should be removed with
the system package manager; kickstart and source-based installations use their supplied uninstall tooling; Windows uses the
MSI uninstall path.

Uninstalling binaries and services does not always remove metric history, cache files, or edited configuration. That
preservation supports later reinstallation but also means a full data removal is a separate, destructive decision. Read the
platform-specific section, back up anything required, and confirm the exact paths before deleting retained data.

## Maintenance checklist

1. Identify the node's role, platform, Agent version, and installation type.
2. Confirm the procedure and whether it causes a collection or streaming interruption.
3. Preserve configuration, credentials, and historical data when the operation could replace or remove them.
4. Perform the service, update, or uninstall procedure for that installation type.
5. Verify service state, API access, collectors, streaming, alerts, and Cloud connectivity as applicable.
6. Record the result in the fleet's normal change-management or automation system.
