<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/device-metrics/collect-snmp-troubleshooting-data.md"
sidebar_label: "Collect Troubleshooting Data"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Device Metrics"
keywords: ['snmp', 'troubleshooting', 'topology', 'support', 'diagnostics', 'freshdesk']
endmeta-->

<!-- markdownlint-disable-file -->

# Collect SNMP troubleshooting data

Use this collector when Netdata Support needs the raw SNMP data returned by one or more network devices. It is useful for
investigating missing metrics, incomplete topology, profile matching, authentication failures, timeouts, and unsupported OIDs.

The collector is a standalone troubleshooting tool. It does not inspect a running Netdata Agent, reproduce Netdata topology,
scan networks, or change any device or Netdata configuration.

## What the collector does

The collector follows a simple, visible workflow:

1. You select the affected nodes manually, from an existing Netdata `go.d/snmp.conf`, or both.
2. It asks for any required settings or credentials that are not available from the configuration.
3. It prints the exact nodes, SNMP versions, and protocol data it will request. No SNMP requests have been sent yet.
4. It waits for approval.
5. It prints progress while collecting bounded raw SNMP walks.
6. It creates one private `.tar.gz` archive.
7. It reports successful, partial, and failed protocol collections.

The raw walks cover system identity, interfaces, bridge/STP/FDB, LLDP, LLDPv2, CDP, IP addresses and neighbors, OSPF,
standard BGP, and common vendor BGP tables.

SNMP credentials are stored in a private temporary Net-SNMP configuration, not in command-line arguments or the final archive.
The temporary data is removed when the collector finishes or is interrupted.

## Requirements

Run the collector on a Linux system that can reach the affected devices. It requires:

- Python 3.6 or later;
- PyYAML when using a Netdata configuration file;
- `snmpwalk` from the Net-SNMP command-line tools;
- `curl` and `sha256sum` only when using the download procedure below.

For example, the required packages are commonly named `python3-yaml` and `snmp` on Debian/Ubuntu, or `python3-pyyaml` and
`net-snmp-utils` on RHEL-compatible systems.

## Download the collector

Download the reviewed collector into a private temporary directory and verify it before running it:

```bash
SNMP_DIAG_DIR="$(mktemp -d)"
(
  set -euo pipefail
  chmod 700 "$SNMP_DIAG_DIR"
  SNMP_DIAG_SHA256='d022249eefbcf4c65cd4433dc115d65343efa76a1317d9ac9388f5adb16b009f'
  curl --fail --location --proto '=https' --tlsv1.2 \
    --output "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" \
    'https://raw.githubusercontent.com/netdata/netdata/master/src/go/plugin/go.d/collector/snmp/tools/collect-snmp-troubleshooting-data.py'
  printf '%s  %s\n' "$SNMP_DIAG_SHA256" "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" | sha256sum --check -
  chmod 700 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py"
  python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" --help
)
```

If checksum verification fails, do not run the file. Report the failure in the support ticket.

Running the collector with `--help`, `-h`, or no parameters prints its complete usage without contacting any device:

```bash
python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py"
```

## Select nodes from Netdata configuration

The collector checks these Netdata configuration roots in order:

1. `/etc/netdata`
2. `/opt/netdata/etc/netdata`

The first root containing Netdata SNMP configuration wins. Within that root, the collector reads the standard static jobs file
`go.d/snmp.conf` and SNMP service-discovery file `go.d/sd/snmp.conf` when they exist.

List the available static jobs and discovery credential scopes without exposing credentials:

```bash
sudo python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" --list-configured
```

Select only the affected jobs by repeating `--node`:

```bash
sudo python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" \
  --node 'core-switch' \
  --node 'edge-router'
```

You can also select a configured job by its exact address. If the same address belongs to multiple static jobs, use the job name
so the collector knows which credentials to use.

For SNMP service discovery, select the exact affected IP address. The collector reuses the credential assigned to the single
configured range containing that address:

```bash
sudo python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" \
  --node '192.0.2.10'
```

The collector does not read the discovery cache, enumerate a range, or scan a subnet. If an address belongs to multiple
credential scopes, use manual configuration so the intended credentials are explicit.

Use a different static-jobs or SNMP-discovery configuration file when needed. An explicit path always wins and the collector
stops if that file does not exist:

```bash
sudo python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" \
  --netdata-snmp-config '/private/path/snmp.conf' \
  --node 'core-switch'
```

Use `sudo` when the Netdata configuration or its credentials are protected. Alternatively, copy the configuration to a private,
restricted location and provide its path explicitly.

`--all-configured` selects every enabled static job in the chosen configuration. It never expands discovery ranges. Use it only
when all static jobs are relevant:

```bash
sudo python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" --all-configured
```

## Configure nodes manually

A Netdata configuration is optional. Select a manual SNMPv1 or SNMPv2c node by address and assign a useful name with
`NAME=ADDRESS`:

```bash
python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" \
  --node 'access-switch=192.0.2.10' \
  --snmp-version 2c \
  --port 161 \
  --timeout 5 \
  --retries 1
```

The collector requests the community with a hidden prompt. It never accepts communities or passphrases as command-line
arguments, where other local users could see them.

For SNMPv3, provide the non-secret protocol settings and enter the passphrases at the hidden prompts:

```bash
python3 "$SNMP_DIAG_DIR/collect-snmp-troubleshooting-data.py" \
  --node 'edge-router=192.0.2.20' \
  --snmp-version 3 \
  --v3-user 'operator' \
  --v3-level authPriv \
  --v3-auth-proto sha256 \
  --v3-priv-proto aes256 \
  --v3-context 'routing-instance'
```

Omit `--v3-context` when the device uses the default SNMPv3 context.

`NAME=ADDRESS` explicitly requests manual settings even when the address belongs to a configured discovery scope. Repeat
`--node` to select several manual nodes that use the same command-line protocol settings. Run separate collections when manual
nodes require different settings, or describe them as separate jobs in a private Netdata-format configuration file.

When every selected node uses `NAME=ADDRESS`, the collector does not load automatically detected Netdata configuration. An
explicit `--netdata-snmp-config` path is still loaded and validated.

If a Netdata configuration value contains a canonical `${env:VARIABLE}` reference available to the collector, it is resolved
using Netdata's trimmed-value behavior. Text such as `${TOKEN}` without a resolver scheme remains literal. For other unresolved
active references, such as protected `${file:/path}` secrets, the collector prompts for the effective value instead of trying to
reproduce Netdata's secret-resolution machinery.

## Review and approve the plan

Before sending any SNMP request, the collector prints:

- each selected node name and address;
- the SNMP port and version;
- whether the settings came from manual input or a Netdata configuration;
- every protocol data group it will collect.

Review the plan and answer `y` or `yes` to proceed. Any other answer cancels collection without contacting a device.

`--yes` accepts the printed plan non-interactively. Use it only after the same command and scope have already been reviewed.

## Read and submit the result

The collector prints every SNMP command before running it, followed by progress and a result for each node and protocol group.
One of these statuses is recorded for each group:

- `success`: every walk in the group completed;
- `partial`: some data was collected, but a walk failed or its bounded output was truncated;
- `failed`: no walk in the group completed.

The final archive is named `netdata-snmp-data-<UTC timestamp>.tar.gz`. It contains:

- `plan.txt`: the approved collection scope;
- `collection-status.tsv`: per-node and per-protocol results;
- `summary.txt`: successful, partial, and failed totals;
- `devices/`: bounded raw SNMP output;
- `manifest.sha256`: integrity hashes for the files in the archive;
- `README.txt`: handling guidance.

The process exit code is `0` when every protocol group succeeds, `2` when the archive contains partial or failed collections,
`1` for a fatal setup error, `130` for Ctrl-C, `129` for `SIGHUP`, and `143` for `SIGTERM`. An exit code of `2` still produces
useful diagnostic evidence.

Attach the single `.tar.gz` archive to a restricted ticket in the
[Netdata Support portal](https://support.netdata.cloud/support/home). Include what is missing or incorrect, when the problem was
visible, and whether device configuration or firmware changed recently.

Raw SNMP data can contain private hostnames, addresses, MAC addresses, interface descriptions, device identifiers, and other
network inventory. Do not upload the archive to Slack, GitHub, email, a public ticket, or another public location.

## If collection fails

- **A required command is missing:** install the package named in the error and rerun the same command.
- **The Netdata configuration is unreadable:** rerun with `sudo`, or pass a private readable copy with
  `--netdata-snmp-config`.
- **A configured secret reference cannot be resolved:** enter the effective value at the hidden prompt.
- **Some walks fail or time out:** submit the archive. Failures are useful evidence and do not stop the remaining probes.
- **Collection is interrupted:** rerun it. The collector removes private temporary data and does not keep an incomplete archive.

## Related documentation

- [Troubleshooting SNMP device metrics](/docs/npm/device-metrics/troubleshooting.md)
- [SNMP device configuration](/docs/npm/device-metrics/configuration.md)
- [SNMP topology discovery methods](/docs/npm/topology/discovery-methods.md)
- [Secrets Management](/src/collectors/SECRETS.md)
