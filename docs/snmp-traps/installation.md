<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/installation.md"
sidebar_label: "Installation"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp', 'traps', 'installation', 'go.d', 'udp', 'snmpv3']
endmeta-->

<!-- markdownlint-disable-file -->

# Installation

Netdata receives SNMP traps with the **snmp_traps** collector in `go.d.plugin`. Trap receiving is explicit: Netdata does not create a trap listener job or configure devices to send traps by itself. You must create an explicit listener job before Netdata can receive traps.

The default direct-journal backend requires Linux. OTLP-only jobs are not blocked by the Linux direct-journal requirement, but they do not create local journal files or local SNMP trap log sources.

The default `listen.endpoints` template uses UDP `0.0.0.0` on port `162`. The stock `go.d/snmp_traps.conf` file ships as a commented template; Netdata does not bind any UDP port until a job is uncommented, created, or applied with Dynamic Configuration.

## Prerequisites

- A Netdata Agent host with the `go.d.plugin` component installed.
- A Linux host when you want the default direct-journal backend or local SNMP trap log sources.
- At least one output backend enabled per listener job: `journal.enabled: true` on Linux (the default), `otlp.enabled: true`, or both.
- For direct-journal jobs on Linux, the files `/etc/machine-id` and `/proc/sys/kernel/random/boot_id` must exist, be readable, and contain non-empty values. Minimal containers, chroots, and embedded systems should check these before relying on local journal storage.
- UDP reachability from each trap sender to the Netdata host.
- Network devices configured to send SNMP Trap or INFORM notifications to the Netdata host IP and listener port.
- Firewall, security group, ACL, NAT, and host firewall rules that allow the selected UDP port.
- Permission to bind the selected UDP port. Standard Netdata packages, static installs, and the official Docker image handle this for UDP/162. Custom or hardened deployments may need extra configuration.
- Matching SNMP settings on both sides:
  - SNMPv1/v2c: community strings accepted by the Netdata job.
  - SNMPv3: USM user, authentication/privacy settings, and sender engine ID policy accepted by the Netdata job.
- For SNMPv3 listener jobs, configure at least one `usm_users` entry and choose exactly one sender engine ID policy: `engine_id_whitelist` or `dynamic_engine_id_discovery`. Static config jobs that fail validation do not bind; Dynamic Configuration jobs are rejected when applied.
- For static SNMPv3 jobs that use `engine_id_whitelist`, each `usm_users` entry must include the sender `engine_id`. Dynamic engine ID discovery allows the `engine_id` field to be omitted.

If you configure SNMPv3 credentials, use Netdata secret references instead of plain-text passphrases, such as `auth_key: "${file:/run/secrets/snmp-v3-auth-key}"`. See [Secrets Management](/src/collectors/SECRETS.md) for safe credential handling in `go.d/snmp_traps.conf`.

## Package availability

SNMP traps are part of the Netdata Go collectors component. On native packages, that component is packaged as **`netdata-plugin-go`**. Standard Netdata package installs normally include it, but minimal, custom, or older installs may not.

Check for the runtime and stock configuration:

```bash
sudo ls /usr/libexec/netdata/plugins.d/go.d.plugin
sudo ls /usr/lib/netdata/conf.d/go.d/snmp_traps.conf
```

If either file is missing on a native-package install, install the Go collectors package and restart Netdata:

```bash
# Debian / Ubuntu / Mint
sudo apt update
sudo apt install netdata-plugin-go
sudo systemctl restart netdata
```

```bash
# RHEL / Fedora / CentOS / Rocky / Alma
sudo dnf install netdata-plugin-go
sudo systemctl restart netdata
```

```bash
# openSUSE
sudo zypper install netdata-plugin-go
sudo systemctl restart netdata
```

## Static install (kickstart)

The static install path bundles `go.d.plugin` and the stock `snmp_traps.conf` template. Verify the files under `/opt/netdata/`:

```bash
sudo ls /opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin
sudo ls /opt/netdata/usr/lib/netdata/conf.d/go.d/snmp_traps.conf
```

Static installs also set the low-port capability for `go.d.plugin`. If the capability was removed by local hardening, use a high UDP port or restore low-port binding before using UDP/162.

## Source and custom builds

Source builds install `go.d.plugin` unless the build or installer is run with Go collectors disabled. If `go.d.plugin` is not built or installed, the SNMP trap listener cannot run.

After installing from source, verify the runtime and stock configuration in the install prefix you used:

```bash
sudo ls /usr/libexec/netdata/plugins.d/go.d.plugin
sudo ls /usr/lib/netdata/conf.d/go.d/snmp_traps.conf
```

If you installed with `--install-prefix /opt`, use the static paths shown above.

## Docker

The official Netdata Docker image includes `go.d.plugin`. To receive traps in Docker, publish the listener UDP port and make sure your Netdata configuration volume includes the `go.d/snmp_traps.conf` job you create.

Docker port mappings need the UDP suffix, for example `-p 162:162/udp`; without `/udp`, Docker publishes TCP.

For UDP/162, the container must be allowed to bind low ports. See [UDP/162 permissions](#udp162-permissions) for restricted runtimes and hardened service policies.

## Listener jobs

The stock `snmp_traps.conf` file is a commented template. Netdata will not listen for traps until you create a job. There are two ways to add one.

### Enable via Dynamic Configuration

This is the recommended way to add a job. Add, edit, test, and remove a listener job from the Netdata UI, with no file editing and no service restart:

1. In Netdata, open **Integrations** and find **SNMP Trap Listener**.
2. Click **Configure**, then **Add job**.
3. Fill in the job form: listener address and port, SNMP versions, communities or SNMPv3 users, and the source allowlist.
4. Click **Test** to validate the job before it goes live.
5. Deploy the job. It takes effect immediately, with no `netdata` restart, and can be deployed to many nodes at once.

This path requires a node connected to Netdata Cloud on a paid plan; Dynamic Configuration handles permissions and security through that connection. See [Dynamic Configuration](/docs/netdata-agent/configuration/dynamic-configuration.md) for the full UI access paths and role requirements. This is the single place this caveat is stated; other pages reference it.

### Fallback: edit the file and restart

Use this path for headless or automated deployments, for free-tier nodes, or when the node is not connected to Netdata Cloud. Edit the collector configuration on the Netdata host:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config go.d/snmp_traps.conf
```

After editing, restart Netdata so the listener job is created. See [Verify the listener starts](#verify-the-listener-starts) for the restart commands.

Either way, each job binds one or more UDP endpoints under the `listen:` key. A job can listen on UDP/162, or on another UDP port if you prefer to avoid privileged bind requirements. If you choose a non-standard port, configure every trap sender to use that same destination port. For the full option list, see [Configuration](/docs/snmp-traps/configuration.md).

## Device identity and enrichment

Trap receiving does not require SNMP polling or topology discovery. A listener can receive, decode, store, and forward traps using the trap source address from the UDP peer or trusted relay configuration. When co-located SNMP or `snmp_topology` jobs are also running, Netdata can enrich traps with device, vendor, interface, and neighbor context. See [Enrichment](/docs/snmp-traps/enrichment.md).

## UDP and firewall preflight

Before creating the job, confirm the network path:

- The device can route to the Netdata host IP.
- The device sends traps to the same UDP port the job will bind.
- Firewalls allow inbound UDP from the device or relay source IP.
- By default, `allowlist.source_cidrs` accepts any IPv4 or IPv6 source. For production listener jobs, restrict it to the expected sender or relay CIDRs and confirm the sender source IP is inside one of those CIDRs.
- If traps pass through a relay, decide whether the relay itself is the authoritative source or whether it must be configured as a trusted relay in the job.

SNMP traps are one-way UDP messages. A successful ping or TCP connection test does not prove that UDP traps can reach the listener.

The listener requests a UDP receive buffer of `4194304` bytes by default. Hardened kernels may clamp that value; use [Configuration](/docs/snmp-traps/configuration.md) if you need to tune `listen.receive_buffer`.

## UDP/162 permissions

Binding to UDP/162 requires low-port privileges. Standard Netdata packages grant `CAP_NET_BIND_SERVICE` to `go.d.plugin`, and the systemd service allows that capability. Static installs set the same capability during installation.

The official Docker image also handles low-port binding when run with the documented Docker command or Compose file. Custom containers, rootless or restricted runtimes, Kubernetes security contexts, `cap_drop` policies, or service overrides that remove low-port privileges may need extra configuration.

If your deployment cannot bind UDP/162, use one of these approaches:

- Use a high UDP port such as `9162` and configure devices to send traps to that port.
- Grant `CAP_NET_BIND_SERVICE` to `go.d.plugin` or to the container/service that runs it.
- Run Netdata in an environment where the service is allowed to bind UDP/162.

After the job starts, verify that Netdata is listening:

```bash
sudo ss -ulnp | grep ':162'
```

Replace `162` with your configured port when using a custom listener port.

## Journal and OTLP preflight

On Linux, direct journal storage is enabled by default for listener jobs. The job writes trap data under the configured Netdata log directory and is exposed as an SNMP trap log source. For the exact per-job path and how the `journalctl --directory` path is built, see [Journal and Querying](/docs/snmp-traps/journal-and-querying.md).

For direct-journal jobs, confirm that the Netdata log directory exists and is writable by the service that runs `go.d.plugin`. Standard systemd packages run the Netdata service as root with group `netdata`; if you changed the service user, run equivalent checks as that user.

```bash
sudo test -d /var/log/netdata && echo "OK: log directory exists" || echo "MISSING: /var/log/netdata"
sudo test -w /var/log/netdata && echo "OK: log directory is writable" || echo "NOT WRITABLE: /var/log/netdata"
sudo test -s /etc/machine-id && echo "OK: /etc/machine-id exists and is non-empty" || echo "MISSING OR EMPTY: /etc/machine-id"
sudo test -s /proc/sys/kernel/random/boot_id && echo "OK: boot_id exists and is non-empty" || echo "MISSING OR EMPTY: /proc/sys/kernel/random/boot_id"
```

Replace `/var/log/netdata` with your configured Netdata log directory when it is different.

If you configure OTLP export, make sure the OTLP/gRPC receiver is reachable before starting the job. When OTLP is enabled, Netdata preflights the receiver during job creation. If the receiver, TLS settings, or headers are wrong, the job fails to start with an OTLP preflight error; Dynamic Configuration jobs are rejected when applied. OTLP can run together with local journal storage, or as the only backend when `journal.enabled: false`.

OTLP-only jobs do not create local journal files and do not appear as local SNMP trap log sources.

## Verify the listener starts

After you add a listener job by editing `go.d/snmp_traps.conf`, restart Netdata with the method for your deployment. Jobs created with Dynamic Configuration take effect after the configuration is applied and do not need a service restart. On systemd hosts:

```bash
sudo systemctl restart netdata
sudo journalctl --namespace netdata --since "5 minutes ago" | grep -E 'snmp_traps|SNMP trap|trap listener'
```

This command checks Netdata service logs, not stored trap rows. If your install logs the Netdata service to the default journal instead of the `netdata` namespace, check the service logs with the same `grep` pattern:

```bash
sudo journalctl -u netdata --since "5 minutes ago" | grep -E 'snmp_traps|SNMP trap|trap listener'
```

After traps arrive, local trap rows are queried from the per-job journal directory; see [Journal and Querying](/docs/snmp-traps/journal-and-querying.md).

In Docker, restart the Netdata container and inspect the container logs for the same listener messages.

```bash
sudo docker restart netdata
sudo docker logs --tail 50 netdata | grep -E 'snmp_traps|SNMP trap|trap listener'
```

The first healthy signal is that the configured listener job starts and, when local journal storage is enabled, the job becomes available as an SNMP trap log source through the Cloud-required `snmp:traps` Function. After a device sends a matching trap, receiver metrics update.

If the listener does not start, check:

- The configured UDP port is not already in use.
- UDP/162 capability is available, or the job uses a high port.
- The Netdata log directory exists and is writable by the user running `go.d.plugin`.
- SNMPv3 credentials, engine IDs, and dynamic engine ID discovery settings are consistent.
- OTLP is reachable when `otlp.enabled: true`.

## What's next

- [Quick Start](/docs/snmp-traps/quick-start.md) - Configure one listener, send one test trap, and prove it arrived.
- [Configuration](/docs/snmp-traps/configuration.md) - Harden source allowlists, SNMP versions, outputs, retention, deduplication, and profile metrics.
- [Troubleshooting](/docs/snmp-traps/troubleshooting.md) - Diagnose inactive listeners, missing rows, SNMPv3 failures, and forwarding issues.
