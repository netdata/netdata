<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/network-flows/installation.md"
sidebar_label: "Installation"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['installation', 'package', 'netdata-plugin-netflow', 'setup']
endmeta-->

<!-- markdownlint-disable-file -->

# Installation

The netflow plugin is **packaged separately from the main Netdata Agent**. You install it on the same host where Netdata runs, after Netdata itself is in place.

The package name is **`netdata-plugin-netflow`** on both Debian and RPM distributions. It is not installed by the standard `netdata` package or by the netdata-updater on its own — you have to install it explicitly on native-package systems.

The static install (the kickstart `--static-only` path) bundles the plugin automatically on x86_64, ARMv7, and ARM64. It is **not** included in the ARMv6 static build (Raspberry Pi 1 / Zero). If you used the kickstart installer with the static option on a supported architecture, no extra step is needed.

## Prerequisites

- A working Netdata Agent on the host that will receive flow data.
- That host must be reachable on UDP from your routers and switches. The stock plugin listens on UDP `2055` for NetFlow/IPFIX and UDP `6343` for sFlow.
- A Netdata installation that includes `netdata-plugin-netflow`. Native Linux packages install it as a separate package; static installs bundle it automatically (except the ARMv6 build — Raspberry Pi 1 / Zero); source builds need a Rust toolchain and the `--enable-plugin-netflow` installer flag, since the plugin is disabled by default.

## Install on Debian / Ubuntu / Mint

```bash
sudo apt update
sudo apt install netdata-plugin-netflow
sudo systemctl restart netdata
```

:::note

`netdata-plugin-netflow` ships only in Netdata's own package repository — it is not in the Debian, Ubuntu, or Mint default repositories. If `apt` reports `Unable to locate package`, the Netdata repository is not configured on this host, which is expected when Netdata was installed with the kickstart `--static-only` option or built from source. A static install already bundles the plugin at `/opt/netdata/usr/libexec/netdata/plugins.d/netflow-plugin` (see [Static install](#static-install-kickstart) below); to install the package on a native system instead, re-run the [kickstart installer](/packaging/installer/methods/kickstart.md) with `--reinstall-clean` and without `--static-only` to reconfigure the Netdata repository and switch to a native install, then retry `apt install netdata-plugin-netflow`. A plain re-run of kickstart only updates the existing static install, and `--reinstall` alone reinstalls it but keeps it static — neither one switches the install method or configures the repository.

:::

## Install on RHEL / Fedora / CentOS / Rocky / Alma

```bash
sudo dnf install netdata-plugin-netflow
sudo systemctl restart netdata
```

(`yum install` works on older systems where `dnf` isn't present.)

## Install on openSUSE

```bash
sudo zypper install netdata-plugin-netflow
sudo systemctl restart netdata
```

## Static install (kickstart)

If you installed Netdata using:

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && \
  sh /tmp/netdata-kickstart.sh --static-only
```

…the netflow plugin is already installed under `/opt/netdata/usr/libexec/netdata/plugins.d/netflow-plugin`. No extra step. (The ARMv6 static build — Raspberry Pi 1 / Zero — does not include the plugin; build from source there, see [Source build](#source-build) below.)

To verify:

```bash
ls /opt/netdata/usr/libexec/netdata/plugins.d/netflow-plugin
```

## Docker / OCI image

The netflow plugin is **already bundled in the official `netdata/netdata` Docker image** — there is no separate package to install and no extra build step. The plugin is enabled by default and opens its stock UDP listeners (`2055` for NetFlow/IPFIX, `6343` for sFlow), the same as a native install.

The only Docker-specific detail is the network mode, because it determines whether the flow ports are reachable:

- **Host networking (`--network=host`)** — the recommended run mode for Netdata containers. The container shares the host's network, so the UDP listeners are reachable by your routers and switches with **no extra port flags**. Use the command from the [Docker installation guide](/packaging/docker/README.md#create-a-new-netdata-agent-container) unchanged.
- **Bridge networking** — if you run without `--network=host`, the container's network is isolated. The image only declares the dashboard port (`19999`) in its `EXPOSE`, so you must publish it and the flow UDP ports yourself, otherwise no flow data arrives. Add these to the [recommended `docker run` command](/packaging/docker/README.md#create-a-new-netdata-agent-container) in place of `--network=host`, keeping its other mounts and privileges:

```bash
-p 19999:19999 \
-p 2055:2055/udp \
-p 6343:6343/udp \
```

:::warning

Dropping `--network=host` isn't free even beyond netflow — `proc.plugin`, `go.d.plugin`, `local-listeners`, and `network-viewer.plugin` all require host network mode for full functionality (see the [privileges table](/packaging/docker/README.md#create-a-new-netdata-agent-container)). Only switch to bridge networking if you have another reason to avoid host networking.

:::

To listen on different ports (for example, if `2055` or `6343` is in use on the host), edit `netflow.yaml` inside the container — see [Configure Agent Containers](/packaging/docker/README.md#configure-agent-containers) for how to edit a config file in a running container — then publish the matching ports. See [Configuration](/docs/npm/network-flows/configuration.md) for the netflow-specific options.

## Source build

The netflow plugin is **disabled by default**. Building it from source requires both a Rust toolchain (rustc + cargo, version 1.85 or later) and an explicit enable flag passed to the installer:

```bash
git clone https://github.com/netdata/netdata.git
cd netdata
sudo ./netdata-installer.sh --enable-plugin-netflow
```

Without `--enable-plugin-netflow` the plugin is skipped, even when a Rust toolchain is installed.

**Caveat:** source builds do **not** include the stock GeoIP / IP-intelligence database files. Packaged 32-bit installs ship the stock MMDB payload but do not include `topology-ip-intel-downloader`. The plugin starts fine without cache files, but country, city, and AS-name fields will be empty until you run the downloader once on an install that includes it:

```bash
sudo /usr/sbin/topology-ip-intel-downloader
```

This populates `/var/cache/netdata/topology-ip-intel/` with the DB-IP-based MMDB files. The plugin auto-detects the cache copy on its next 30-second poll. See the [Enrichment Intel Downloader page](/docs/npm/network-flows/intel-downloader.md) for the refresh tool and the [DB-IP integration card](/src/crates/netflow-plugin/integrations/db-ip_ip_intelligence.md) for cadence and license details.

## IP intelligence defaults

| Item             | Behaviour                                                                                                      |
|------------------|----------------------------------------------------------------------------------------------------------------|
| Native packages  | Ship stock DB-IP ASN and Geo MMDB files under `/usr/share/netdata/topology-ip-intel/`.                         |
| Source builds    | Do not include stock MMDB files; run the downloader once if you want GeoIP / ASN enrichment.                   |
| Fresh copies     | The downloader writes to `/var/cache/netdata/topology-ip-intel/`, which takes precedence over the stock files. |
| Refresh schedule | Netdata does not install a timer or cron job for the downloader. Schedule it yourself if freshness matters.    |

## What gets installed

| Path                                                        | Purpose                                                                                     |
|-------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| `/usr/libexec/netdata/plugins.d/netflow-plugin`             | The plugin binary (mode 0750, root:netdata)                                                 |
| `/usr/sbin/topology-ip-intel-downloader`                    | Helper for refreshing the GeoIP / IP-intel MMDBs; not included in packaged 32-bit installs  |
| `/usr/lib/netdata/conf.d/netflow.yaml`                      | Stock configuration (read-only reference; copy to `/etc/netdata/netflow.yaml` to customise) |
| `/usr/lib/netdata/conf.d/topology-ip-intel.yaml`            | IP-intel downloader configuration                                                           |
| `/usr/share/netdata/topology-ip-intel/topology-ip-asn.mmdb` | Stock ASN database (DB-IP)                                                                  |
| `/usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb` | Stock geographic database (DB-IP)                                                           |

(Paths assume native packages. Static installs put everything under `/opt/netdata/`.)

## Verify the plugin is running

After installation and restart:

```bash
sudo journalctl --namespace netdata --since "5 minutes ago" | grep -E 'netflow|listener'
```

You should see entries indicating that the plugin loaded its config and that the UDP listeners bound to their ports.

Quick sanity check:

```bash
sudo ss -unlp | grep -E ':(2055|6343)([[:space:]]|$)'
```

Lines for `netflow-plugin` confirm the stock listeners are up.

## Open Netdata to confirm

Open the Netdata UI in your browser. Click the **Live** tab in the top navigation; **Network Flows** appears in the Functions list on the right (see [Live tab](/docs/dashboards-and-charts/live-tab.md)). Selecting it opens the Sankey + Table view. The plugin's operational charts also appear under the standard charts page in the `netflow` family.

If Network Flows doesn't appear under Live, or the view is empty:

- Check that the plugin process is running: `pgrep -fa netflow-plugin`.
- Check Netdata Cloud SSO: Functions require authenticated access to the agent's space.
- See [Troubleshooting](/docs/npm/network-flows/troubleshooting.md).

## Configuring flow sources

Installing the plugin enables it and opens the stock listener ports. To actually see flow data, configure a router, switch, or software exporter to send NetFlow/IPFIX datagrams to this host's UDP port `2055` or sFlow datagrams to UDP port `6343`.

That's the next step:

- [Quick Start](/docs/npm/network-flows/quick-start.md) — A 15-minute path to your first flow data.
- [Flow Protocols / NetFlow](/src/crates/netflow-plugin/integrations/netflow.md) — Vendor configurations for NetFlow.
- [Flow Protocols / IPFIX](/src/crates/netflow-plugin/integrations/ipfix.md) — Vendor configurations for IPFIX.
- [Flow Protocols / sFlow](/src/crates/netflow-plugin/integrations/sflow.md) — Vendor configurations for sFlow.

## Uninstall

```bash
# Debian / Ubuntu
sudo apt remove netdata-plugin-netflow

# RHEL / Fedora / CentOS / Rocky / Alma
sudo dnf remove netdata-plugin-netflow

# openSUSE
sudo zypper remove netdata-plugin-netflow
```

Remove the configuration if you also want to clean up:

```bash
sudo rm /etc/netdata/netflow.yaml /etc/netdata/topology-ip-intel.yaml
```

The flow journals at `/var/cache/netdata/flows/` and `/var/cache/netdata/topology-ip-intel/` are not removed by the package manager. Delete them manually if you want to reclaim the disk:

```bash
sudo rm -rf /var/cache/netdata/flows /var/cache/netdata/topology-ip-intel
```

(Warning: this deletes all your historical flow data.)

## What's next

- [Quick Start](/docs/npm/network-flows/quick-start.md) — Configure your first source and see traffic in the dashboard.
- [Configuration](/docs/npm/network-flows/configuration.md) — Tune the listener, retention, and enrichment.
- [Troubleshooting](/docs/npm/network-flows/troubleshooting.md) — When something doesn't work.
