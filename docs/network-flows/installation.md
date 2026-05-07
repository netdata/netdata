<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/installation.md"
sidebar_label: "Installation"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['installation', 'package', 'netdata-plugin-netflow', 'setup']
endmeta-->

# Installation

The netflow plugin is **packaged separately from the main Netdata Agent**. You install it on the same host where Netdata runs, after Netdata itself is in place.

The package name is **`netdata-plugin-netflow`** on both Debian and RPM distributions. It is not installed by the standard `netdata` package or by the netdata-updater on its own — you have to install it explicitly on native-package systems.

The static install (the kickstart `--static-only` path) bundles the plugin automatically. If you used the kickstart installer with the static option, no extra step is needed.

## Prerequisites

- A working Netdata Agent on the host that will receive flow data.
- That host must be reachable on UDP from your routers and switches (default port `2055`).
- Linux. The plugin is Linux-only.

## Install on Debian / Ubuntu / Mint

```bash
sudo apt update
sudo apt install netdata-plugin-netflow
sudo systemctl restart netdata
```

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

…the netflow plugin is already installed under `/opt/netdata/usr/libexec/netdata/plugins.d/netflow-plugin`. No extra step.

To verify:

```bash
ls /opt/netdata/usr/libexec/netdata/plugins.d/netflow-plugin
```

## Source build

Building from source requires a Rust toolchain (rustc + cargo, version 1.83 or later). When CMake detects Rust, the plugin is built and installed alongside the rest of Netdata.

```bash
git clone https://github.com/netdata/netdata.git
cd netdata
sudo ./netdata-installer.sh
```

**Caveat:** source builds do **not** include the stock GeoIP / IP-intelligence database files. The plugin starts fine without them, but country, city, and AS-name fields will be empty until you run the downloader once:

```bash
sudo /usr/sbin/topology-ip-intel-downloader
```

This populates `/var/cache/netdata/topology-ip-intel/` with the DB-IP-based MMDB files. The plugin auto-detects the cache copy on its next 30-second poll. See [GeoIP enrichment](/docs/network-flows/enrichment/ip-intelligence.md) for details and refresh scheduling.

## What gets installed

| Path | Purpose |
|---|---|
| `/usr/libexec/netdata/plugins.d/netflow-plugin` | The plugin binary (mode 0750, root:netdata) |
| `/usr/sbin/topology-ip-intel-downloader` | Helper for refreshing the GeoIP / IP-intel MMDBs |
| `/usr/lib/netdata/conf.d/netflow.yaml` | Stock configuration (read-only reference; copy to `/etc/netdata/netflow.yaml` to customise) |
| `/usr/lib/netdata/conf.d/topology-ip-intel.yaml` | IP-intel downloader configuration |
| `/usr/share/netdata/topology-ip-intel/topology-ip-asn.mmdb` | Stock ASN database (DB-IP) |
| `/usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb` | Stock geographic database (DB-IP) |

(Paths assume native packages. Static installs put everything under `/opt/netdata/`.)

## Verify the plugin is running

After installation and restart:

```bash
sudo journalctl -u netdata --since "5 minutes ago" | grep -E 'netflow|listener'
```

You should see entries indicating that the plugin loaded its config and that the UDP listener bound to its port.

Quick sanity check:

```bash
sudo ss -unlp | grep 2055
```

A line for `netflow-plugin` confirms the listener is up.

## Open Netdata to confirm

Open the Netdata UI in your browser. The **Network Flows** tab should appear in the top navigation. The plugin's operational charts also appear under the standard charts page in the `netflow` family.

If the tab doesn't appear, or appears empty:

- Check that the plugin process is running: `pgrep -fa netflow-plugin`.
- Check Netdata Cloud SSO: the Network Flows function requires authenticated access to the agent's space.
- See [Troubleshooting](/docs/network-flows/troubleshooting.md).

## Configuring flow sources

Installing the plugin enables it. To actually see flow data, you need to configure a router, switch, or software exporter to send NetFlow / IPFIX / sFlow datagrams to this host's UDP port 2055.

That's the next step:

- [Quick Start](/docs/network-flows/quick-start.md) — A 15-minute path to your first flow data.
- [Sources / NetFlow](/src/crates/netflow-plugin/integrations/netflow.md) — Vendor configurations for NetFlow.
- [Sources / IPFIX](/src/crates/netflow-plugin/integrations/ipfix.md) — Vendor configurations for IPFIX.
- [Sources / sFlow](/src/crates/netflow-plugin/integrations/sflow.md) — Vendor configurations for sFlow.

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

- [Quick Start](/docs/network-flows/quick-start.md) — Configure your first source and see traffic in the dashboard.
- [Configuration](/docs/network-flows/configuration.md) — Tune the listener, retention, and enrichment.
- [Troubleshooting](/docs/network-flows/troubleshooting.md) — When something doesn't work.
