<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/snmp-traps/quick-start.md"
sidebar_label: "Quick Start"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['quick start', 'snmp traps', 'snmp', 'trap', 'logs', 'getting started']
endmeta-->

<!-- markdownlint-disable-file -->

# Quick Start

Get one known SNMP trap into Netdata and prove it arrived. The path: create a small local listener, send a synthetic trap, find the row in Logs, and confirm the receiver pipeline metrics moved.

## Before you start

- The Netdata Agent is running on the Linux host that will receive traps.
- The SNMP trap listener is available on that Agent. If not, follow [Installation](/docs/npm/snmp-traps/installation.md) first.
- You can add a listener job from the Netdata UI (Dynamic Configuration), or by editing `go.d/snmp_traps.conf` and restarting the Agent.
- The host has the Net-SNMP `snmptrap` command, or you can send an equivalent SNMPv2c trap from another test tool.

This quick start uses UDP/9162 on `127.0.0.1` so you can test safely without changing production devices or binding the standard UDP/162 trap port.

## Step 1 - Create a local test listener

The fastest path is the Netdata UI with **Dynamic Configuration** (no restart): Integrations → SNMP Trap Listener → **Configure** → **Add job**, set address `127.0.0.1`, port `9162`, version `v2c`, community `example`, source CIDR `127.0.0.1/32`, and journal enabled, then **Test** and deploy. This needs a Cloud-connected node on a paid plan — see [Installation](/docs/npm/snmp-traps/installation.md#enable-via-dynamic-configuration).

Without Cloud, edit the config file instead:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config go.d/snmp_traps.conf
```

Add this job:

```yaml
jobs:
  - name: local-test
    listen:
      endpoints:
        - protocol: udp
          address: 127.0.0.1
          port: 9162
    versions:
      - v2c
    communities:
      - example
    allowlist:
      source_cidrs:
        - 127.0.0.1/32
    journal:
      enabled: true
```

`example` is a placeholder community for this loopback test. Community strings travel in cleartext on the wire, so on real networks prefer SNMPv3 — see [Configuration security](/docs/npm/snmp-traps/configuration.md#snmp-versions-and-communities).

Then restart Netdata so the file change takes effect. On systemd hosts:

```bash
sudo systemctl restart netdata
```

Check that the listener is bound:

```bash
sudo ss -ulnp | grep ':9162'
```

You should see something like:

```text
UNCONN 0 0 127.0.0.1:9162 0.0.0.0:* users:(("netdata",pid=1234,fd=42))
```

If the line is missing, the listener did not bind — recheck the YAML and the service logs.

## Step 2 - Send one known trap

Send a standard SNMPv2c `coldStart` trap to the local listener:

```bash
snmptrap -v 2c -c example 127.0.0.1:9162 '' 1.3.6.1.6.3.1.1.5.1
```

The command sends one UDP datagram to the listener. The destination, version, and community must match the listener job:

- Destination: `127.0.0.1:9162`
- Version: `v2c`
- Community: `example`

The trap OID in this test is `1.3.6.1.6.3.1.1.5.1`, the standard `coldStart` notification.

For production devices, this is the point where you configure the device-side trap destination. Set the destination to the Netdata host IP and listener UDP port, then match the device community or SNMPv3 USM settings to the listener job.

## Step 3 - Find the trap in Logs

In Netdata Cloud, open **Logs** and select **SNMP Trap Logs** (`snmp:traps`), then choose the `local-test` listener job if the source selector is shown. If this Agent is not connected to Netdata Cloud, skip to [Verify with journalctl](#verify-with-journalctl); that is the primary local verification path for standalone Agents.

Look for one row with:

- `TRAP_REPORT_TYPE`: `trap`
- `TRAP_JOB`: `local-test`
- `TRAP_OID`: `1.3.6.1.6.3.1.1.5.1`
- `TRAP_NAME`: `SNMPv2-MIB::coldStart`
- `TRAP_SOURCE_IP`: `127.0.0.1`

If `TRAP_NAME` is not resolved, the raw `TRAP_OID` still proves that Netdata received and stored the test trap.

## Step 4 - Confirm receiver metrics moved

Open the Metrics or Charts view and search for **SNMP trap receiver pipeline** or `snmp.trap.pipeline`.

For the `local-test` job, the trap should produce activity in these pipeline dimensions:

- `received`
- `decoded`
- `accepted`
- `committed`

These are incremental receiver metrics displayed as rates, so a single test trap may appear as a short spike. If you miss the spike, send the test trap again and watch the chart over the last few minutes.

## Step 5 - If it does not appear

Work through the checks in this order:

1. **Listener not bound.**

   ```bash
   sudo ss -ulnp | grep ':9162'
   ```

   If nothing is listening, check the YAML indentation, the job name, and the Netdata service logs after restart.

2. **Trap sent to the wrong address or port.**

   ```bash
   sudo tcpdump -i any -nn -c 5 'udp port 9162'
   ```

   If no packet appears, fix the destination IP, destination port, local firewall, or sender command.

3. **Community, SNMP version, or source allowlist mismatch.**

   The quick-start job accepts only SNMPv2c, community `example`, and source `127.0.0.1/32`. If you send from another host, update the listener address, destination address, and `allowlist.source_cidrs`.

4. **Production device sends nothing.**

   Netdata does not make devices emit traps. Configure each device to send traps or informs to the Netdata listener address and port, then confirm the device uses the same community or SNMPv3 USM settings as the listener.

## Verify with journalctl

When local journal storage is enabled, the same trap is stored in Netdata's own journal files (see [Journal and Querying](/docs/npm/snmp-traps/journal-and-querying.md) for the path and more query examples). On default installations, you can inspect the local test job with:

```bash
sudo journalctl \
  --directory=/var/log/netdata/traps/local-test/$(tr -d '-' < /etc/machine-id) \
  --since "5 minutes ago" \
  --output verbose \
  TRAP_OID=1.3.6.1.6.3.1.1.5.1
```

You should see a row like:

```text
TRAP_REPORT_TYPE=trap
TRAP_JOB=local-test
TRAP_OID=1.3.6.1.6.3.1.1.5.1
TRAP_NAME=SNMPv2-MIB::coldStart
TRAP_SOURCE_IP=127.0.0.1
```

Look for `TRAP_OID=1.3.6.1.6.3.1.1.5.1` in the output. If present, the trap was stored locally.

If your Netdata log directory is different, replace `/var/log/netdata` with the configured log directory.

## What's next

You proved that one known trap can reach Netdata, appear in Logs, and move the receiver pipeline.

To understand the row you just found, continue to [Use SNMP trap data](/docs/npm/snmp-traps/usage-and-output.md). For exact `TRAP_*` field definitions, use the [Field Reference](/docs/npm/snmp-traps/field-reference.md).

Before using this in production, go to [Configuration](/docs/npm/snmp-traps/configuration.md) to harden the listener address, source allowlist, community or SNMPv3 settings, deduplication, retention, and output backends.
