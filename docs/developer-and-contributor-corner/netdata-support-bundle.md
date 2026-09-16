# Netdata Support Bundle

The Netdata Support Bundle is a diagnostic collector that ships with the Netdata Agent. It gathers configuration, logs, process state, and runtime information from a single node into one archive for troubleshooting. Standard captures are sanitized; explicitly included SNMP diagnostic files are raw and require private sharing.

It exists so you do not have to run a dozen commands by hand and paste the output into a ticket. One command produces one file that contains what the Netdata team needs to diagnose most problems.

The tool ships in two forms with the same behavior and the same archive layout:

- `netdata-support-bundle`, a POSIX shell script for Linux, macOS, BSD, and containers.
- `netdata-support-bundle.ps1`, a PowerShell script for Windows.

## Why use it

- It collects the data support usually asks for, so you avoid several rounds of back and forth.
- It runs at the lowest process priority and stops each command after a timeout, so it does not compete with your production workload.
- Standard captures redact secrets and pseudonymize IP addresses and hostnames. Review the privacy exceptions below before sharing.
- It still produces useful output when the agent is not running, by reading state and logs from disk.

## Run it on Linux, macOS, BSD, or in a container

Run the tool with elevated privileges so it can read every log and configuration file:

```sh
sudo netdata-support-bundle
```

The last lines it prints tell you the archive path, for example:

```sh
 [*] bundle:  /tmp/netdata-support-bundle-20260721-140501-1234.tar.zst
 [*] attach the bundle to your support ticket.
```

The output is a `.tar.zst` archive when `zstd` is available, or a `.tar.gz` archive otherwise. Extract it with `tar -xaf FILE`, which selects the decompressor from the file suffix.

If your agent runs inside a container, run the tool inside the container, then also capture the container logs from the host:

```sh
docker logs --since 24h NETDATA_CONTAINER > netdata-docker.log 2>&1
```

Replace `NETDATA_CONTAINER` with the name or id of your Netdata container, and attach `netdata-docker.log` alongside the bundle.

## Run it on Windows

Open PowerShell as Administrator, then run the script by full path:

```powershell
powershell -ExecutionPolicy Bypass -File "C:\Program Files\Netdata\usr\libexec\netdata\netdata-support-bundle.ps1"
```

`-ExecutionPolicy Bypass` applies only to that single invocation. It does not change the machine policy and does not require `Set-ExecutionPolicy`. On Windows the output is a `.zip` archive, and the script prints its path when it finishes.

## Options

Both scripts accept the same set of options. The POSIX flags and their PowerShell equivalents are:

| POSIX flag | PowerShell parameter | Default | Effect |
|---|---|---|---|
| `-o`, `--output DIR` | `-Output DIR` | `/tmp` on POSIX, `%TEMP%` on Windows | Directory where the archive is written. |
| `--since HOURS` | `-SinceHours N` | `24` | How many hours of logs to include. |
| `--timeout SECONDS` | `-TimeoutSeconds N` | `10` | Per-command timeout, so a slow command cannot stall the run. |
| `--no-obfuscate` | `-NoObfuscate` | off | Turn off IP and hostname pseudonymization. Secrets are still redacted. |
| `--include-snmp-diagnostics` | `-IncludeSnmpDiagnostics` | off | Include existing raw SNMP diagnostic files; no sanitization. Share privately. |
| `--keep-staging` | `-KeepStaging` | off | Keep the temporary working directory for inspection. |
| `--selftest` | `-SelfTest` | off | Run the built-in sanitizer tests and exit without collecting anything. |
| `-v`, `--version` | `-Version` | | Print the tool version and exit. |
| `-h`, `--help` | | | Print usage and exit. |

Example that writes a 48 hour bundle to a chosen directory:

```sh
sudo netdata-support-bundle --output /var/tmp --since 48
```

## What it collects

The archive is organized into numbered directories so a person or an automated reader can find things quickly. A `MANIFEST.json` file at the root lists every file, the command that produced it, and why it was included.

- `01-system/`, platform context such as OS, kernel, CPU, memory, and disk usage.
- `02-install/`, how the agent was installed and its package status.
- `03-process/`, the running agent process, its threads, and resource use.
- `04-config/`, the effective Netdata and streaming configuration.
- `05-logs/`, recent agent logs and relevant kernel messages, bounded by `--since`.
- `06-state/`, persistent state such as the daemon status file used for crash analysis, database disk usage, and cloud claim state.
- `07-runtime/`, live agent state read from the local API, collected only when the agent responds.
- `08-network/`, local connectivity relevant to the agent, including
  `netdata-sockets.txt`: visible, platform-supported sockets owned by the
  Netdata process tree and their native states.
- `09-permissions/`, file modes, ownership, plugin capabilities, extended attributes, security contexts, and ACLs for the agent's directories and plugins.

## Include SNMP diagnostics

For SNMP metrics, BGP, licensing, or topology issues, run:

```sh
sudo netdata-support-bundle --include-snmp-diagnostics
```

On Windows, add `-IncludeSnmpDiagnostics` to the PowerShell invocation. The same
bundle includes available original SNMP evidence under `06-state/snmp-diagnostics/`.
No extra SNMP requests are sent, and no Python or decompressor is required.

**These files bypass sanitization and pseudonymization.** They may contain
addresses, hostnames, inventory, metric values, and arbitrary device-returned
data. Share this bundle through a restricted support ticket, never a public
GitHub issue. The manifest labels raw files and clears its aggregate redaction
claims when they are included.

Check `06-state/snmp-diagnostics-status.txt` for missing files or copy failures.
Files are copied whole, so bundle size and disk work depend on the retained
evidence. They have independent collection times and may rotate during copying;
a partial bundle can still help support. See
[Collect SNMP troubleshooting data](/docs/npm/device-metrics/collect-snmp-troubleshooting-data.md)
for retention, publication timing, and missing-file troubleshooting.

## Privacy and sanitization

- Standard captures remove secrets. This covers API tokens, passwords, bearer and basic credentials, private key blocks, and credentials embedded in URLs.
- The standard capture exception: the **streaming API key** is kept as-is in `04-config/stream.conf`, because support needs it to tell whether a child and its parent agree on the same key. Remove or mask that file before sending the bundle if you would rather not share it.
- Collected files keep their original bytes, including a byte-order mark and CRLF line endings, so an encoding problem in your configuration is still visible.
- Standard captures replace IP addresses and hostnames with stable pseudonyms by default, so the same address reads as the same pseudonym across those captures. Use `--no-obfuscate` to keep the real values.
- When pseudonymization is on, the map from real values to pseudonyms is written next to the archive, not inside it. Keep that map private and do not attach it to a ticket.
- Review the archive before you send it. The tool runs on your own host and you are in control of what leaves it.

## Attach the bundle

Attach the printed archive file to your support ticket. If it includes raw SNMP diagnostics, use a restricted ticket; otherwise, review it before attaching it to a public GitHub bug report. The Netdata bug report form points to this same tool, so a bundle is the fastest way to give the team what it needs.
