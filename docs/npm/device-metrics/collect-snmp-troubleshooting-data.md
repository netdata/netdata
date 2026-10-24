<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/device-metrics/collect-snmp-troubleshooting-data.md"
sidebar_label: "Collect Troubleshooting Data"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Device Metrics"
keywords: ['snmp', 'troubleshooting', 'topology', 'support', 'diagnostics', 'freshdesk']
endmeta-->

<!-- markdownlint-disable-file -->

# Collect SNMP troubleshooting data

For SNMP metrics, BGP, licensing, or topology problems, send Netdata Support a support bundle that includes the Agent's
built-in SNMP diagnostics. There is no separate Python collector to download, and collecting the bundle sends no extra
SNMP requests to your devices.

## Create the bundle

Run this on the Netdata Agent that polls the affected devices:

```sh
sudo netdata-support-bundle --include-snmp-diagnostics
```

For a static installation:

```sh
sudo /opt/netdata/usr/sbin/netdata-support-bundle --include-snmp-diagnostics
```

On Windows, open PowerShell as Administrator:

```powershell
powershell -ExecutionPolicy Bypass -File "C:\Program Files\Netdata\usr\libexec\netdata\netdata-support-bundle.ps1" -IncludeSnmpDiagnostics
```

For a container installation, run the command inside the container. See
[Netdata Support Bundle](/docs/developer-and-contributor-corner/netdata-support-bundle.md) for output locations, container
logs, and other bundle options.

The command prints the path to one archive. Attach it to a restricted ticket in the
[Netdata Support portal](https://support.netdata.cloud/support/home), along with the affected device, the symptom, and the
time it occurred. Include any recent device configuration or firmware changes.

**SNMP evidence is unsanitized.** It can contain addresses, hostnames, interface descriptions, inventory, metric values,
and arbitrary data returned by devices. Connection credentials are excluded from diagnostic records, but device-returned
values are not scrubbed for secrets or personal information. Share this bundle privately, never in a public GitHub issue.
The inclusion flag is required; ordinary support bundles omit SNMP diagnostic files.

## What the evidence contains

The Agent records evidence from its existing collection work:

- Job lifecycle and preparation outcomes, including failures before a device becomes available for topology.
- Per-device metric collection, emitted samples, profile selection context, structured failures, and source operations.
- BGP and licensing results, including cache state and the source operations that produced retained values.
- Topology acquisition evidence and retained historical checkpoints for offline reconstruction and inspection.

This is passive evidence of what Netdata collected. It does not walk additional OIDs, probe unconfigured devices, or keep
every poll forever. A protocol's absence can mean it was not selected by the device's profiles or has not been collected.
Support may request a targeted additional check when the retained evidence cannot answer a question.

## Timing and retained files

Files live under the Agent's state directory, in `snmp/diagnostics/`. This is normally
`/var/lib/netdata/snmp/diagnostics/`, or `/opt/netdata/var/lib/netdata/snmp/diagnostics/` for static installations.
The bundle copies them into `06-state/snmp-diagnostics/`:

| File | Purpose and timing |
|---|---|
| `lifecycle.zst` | Independently updated job lifecycle status and whether topology is active. |
| `topology/checkpoint-*.zst` | The last three meaningful topology checkpoints. Each is self-contained, including its own lifecycle cut. Topology refresh defaults to 30 minutes; file timestamps need not match device metric polls. |
| `normal/runs.json` | Identifies the current and, when available, previous evidence-bearing go.d process run. |
| `normal/<run-id>/device-*.zst` | One file per device containing its latest normal collection attempt, retained last failure, and associated metric, BGP, licensing, and source evidence. First evidence is eligible for prompt publication; subsequent writes are spaced five minutes apart per device. |

Normal evidence is updated in memory on each poll, so the next write captures the latest state without rewriting topology.
Publication is asynchronous and best effort; busy or failing storage can delay it. Previous-run files preserve evidence
across a restart until a later evidence-bearing run replaces them. Registration IDs are scoped to a run, so use run IDs
and captured timestamps when comparing files.

The support bundle copies complete files without decompressing or modifying them. Files can change independently while
it runs: the bundle is not a simultaneous snapshot of the whole directory. Check `06-state/snmp-diagnostics-status.txt`
and `MANIFEST.json` for missing files, copy failures, or an incomplete result. A partial bundle can still be useful.

## If diagnostic files are missing

- Check `06-state/snmp-diagnostics-status.txt`. `not_requested` means the inclusion flag was omitted; `unavailable`
  means no completed evidence was found; `partial` means some requested evidence could not be copied.
- Confirm that your Agent version includes built-in SNMP diagnostics. Downloading a newer bundle script alone cannot
  create evidence that an older Agent never recorded.
- Let the Agent collect the affected device and allow time for publication. Check the Agent logs for collection or
  diagnostic publication failures.
- Run go.d through the Agent service. Diagnostic file publication is disabled when any of go.d's standard streams is a
  terminal; an interactive debug invocation does not produce fresh diagnostic files. Files from a previous daemon run may
  still exist, so check their timestamps.
- Use sufficient permissions to read the Agent's state directory. No Python, Net-SNMP tools, or local Zstandard decoder
  is needed to include existing files in a bundle.

## Related documentation

- [Troubleshooting SNMP device metrics](/docs/npm/device-metrics/troubleshooting.md)
- [SNMP device configuration](/docs/npm/device-metrics/configuration.md)
- [SNMP topology discovery methods](/docs/npm/topology/discovery-methods.md)
