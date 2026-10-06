# Minimize the Disk Footprint of Fleet Devices

Fit full device monitoring into a small image. Remove unused plugin bundles and strip debugging symbols while keeping the system metrics, application metrics and live Functions your operators need. Let the Parent provide dashboards, long-term storage, alerts and anomaly detection.

The example package keeps system metrics, process monitoring and hardware sensors in **54.0 MiB on ARMv6** or **40.6 MiB on ARMv7**—a payload reduction of about **90%** and **94%** respectively. The compressed installers are **21.8 MiB** and **19.3 MiB**.

Use the [fleet preparation script](https://github.com/netdata/netdata/blob/master/packaging/makeself/prepare-fleet.py) to create a new installer from the upstream static package. The examples and size reference below use **Netdata v2.12.0-2-nightly** and **LLVM 23.1.1**.

## Prepare the build host

Use a Linux or macOS build host with Python 3.9 or newer and a GNU binutils or [LLVM strip tool](https://llvm.org/docs/CommandGuide/llvm-strip.html) that supports Linux ELF files for your device architecture. Install these tools on the build host; devices need no cross-compilation tools.

Reserve **4 GiB of workspace per ARM package** on a filesystem supporting Unix permissions, symbolic links and hard links, such as ext4, XFS or APFS. You can copy the completed installer and checksum to other storage.

### Debian and Ubuntu

```bash
sudo apt-get update
sudo apt-get install python3 coreutils curl binutils-arm-linux-gnueabihf
```

The [ARM binutils package](https://packages.ubuntu.com/noble/binutils-arm-linux-gnueabihf) supplies `arm-linux-gnueabihf-strip` for both ARMv6 and ARMv7. For 64-bit ARM packages, use `binutils-aarch64-linux-gnu` and `aarch64-linux-gnu-strip`. Your build host's default `strip` may support only its own architecture.

Alternatively, LLVM supplies one tool for multiple target architectures:

```bash
sudo apt-get install llvm
llvm-strip --version
```

### macOS

Install Python and [LLVM through Homebrew](https://formulae.brew.sh/formula/llvm), then add LLVM's tools to the current shell's `PATH`:

```bash
brew install python llvm
export PATH="$(brew --prefix llvm)/bin:$PATH"
python3 --version
llvm-strip --version
```

Use `--strip-tool llvm-strip` for Netdata's Linux ELF binaries. macOS's built-in `strip` handles Mach-O files and is unsuitable for these packages.

For checksum commands below, use macOS's `shasum -a 256` in place of `sha256sum`:

```bash
shasum -a 256 netdata-armv6l-v2.12.0-2-nightly.gz.run
shasum -a 256 netdata-armv7l-v2.12.0-2-nightly.gz.run
```

### Choose the stripping tool

Prebuilt toolchains are also available from [LLVM releases](https://github.com/llvm/llvm-project/releases), including [LLVM 23.1.1](https://github.com/llvm/llvm-project/releases/tag/llvmorg-23.1.1). Choose the toolchain for your build host and put its `bin` directory on `PATH`.

The examples use `--strip-tool llvm-strip`. Substitute `--strip-tool arm-linux-gnueabihf-strip` when using GNU binutils.

## Choose what to keep

For system, process and sensor monitoring, use `--keep apps,debugfs`. Add `network` for live connection inspection or `go` for its application and service collectors. The [capability reference](#plugin-capability-reference) below explains every choice.

Built-in Linux system collectors remain in the Agent: CPU, memory, network interfaces and disks continue to be monitored with their collectors enabled. Your selected plugins provide the additional device-side metrics and Functions that operators use through the Parent.

Choose a stripping mode:

| Option | Use it for |
|:--|:--|
| `--strip-mode all` | The smallest binaries; keep the original package on the build host for crash diagnosis |
| `--strip-mode debug` | Removing debug information while keeping more diagnostic symbols; this is the default |
| `--strip-mode none` | Selecting bundles while leaving binary symbols unchanged |

## Prepare an ARMv6 package

Download the script, package and published checksums:

```bash
curl -fL -o prepare-fleet.py \
  https://raw.githubusercontent.com/netdata/netdata/master/packaging/makeself/prepare-fleet.py
curl -fL -o netdata-armv6l-v2.12.0-2-nightly.gz.run \
  https://github.com/netdata/netdata-nightlies/releases/download/v2.12.0-2-nightly/netdata-armv6l-v2.12.0-2-nightly.gz.run
curl -fL -o sha256sums.txt \
  https://github.com/netdata/netdata-nightlies/releases/download/v2.12.0-2-nightly/sha256sums.txt
sha256sum netdata-armv6l-v2.12.0-2-nightly.gz.run
```

Compare the checksum with `sha256sums.txt`. For this package it is:

```text
4bbb1c330137f8b53f895d173ae176fd71015032c2d75c843a0d5981e0184ebe  netdata-armv6l-v2.12.0-2-nightly.gz.run
```

Run preparation:

```bash
python3 ./prepare-fleet.py \
  --input netdata-armv6l-v2.12.0-2-nightly.gz.run \
  --sha256 4bbb1c330137f8b53f895d173ae176fd71015032c2d75c843a0d5981e0184ebe \
  --keep apps,debugfs \
  --strip-mode all \
  --strip-tool llvm-strip \
  --apply \
  --output netdata-armv6l-edge.gz.run
```

The script shows each command before running it, the before/after size of each stripped executable, and the completed package. This example uses `/build/edge` as the working directory; temporary workspace names vary:

```text
/build/edge > python3 ./prepare-fleet.py --input netdata-armv6l-v2.12.0-2-nightly.gz.run --sha256 4bbb1c330137f8b53f895d173ae176fd71015032c2d75c843a0d5981e0184ebe --keep apps,debugfs --strip-mode all --strip-tool llvm-strip --apply --output netdata-armv6l-edge.gz.run
Workspace retained for inspection: /build/edge/.netdata-fleet-example
Input: netdata-armv6l-v2.12.0-2-nightly.gz.run
Keep: apps, debugfs
Remove: charts, containers, dashboard, go, ioping, log2journal, mcp, network, nfacct, perf, python, scripts, slabinfo
Strip mode: all
Original payload: 514.7 MiB
Removed bundles: 305.7 MiB
Payload before stripping: 209.0 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/bash
Stripped bin/bash: 1.9 MiB -> 1.9 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/curl
Stripped bin/curl: 7.4 MiB -> 7.4 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/nd-run
Stripped bin/nd-run: 475.1 KiB -> 65.4 KiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/netdatacli
Stripped bin/netdatacli: 2.5 MiB -> 358.7 KiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/srv/netdata
Stripped bin/srv/netdata: 169.0 MiB -> 30.2 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/systemd-cat-native
Stripped bin/systemd-cat-native: 11.1 MiB -> 7.3 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/usr/libexec/netdata/plugins.d/apps.plugin
Stripped usr/libexec/netdata/plugins.d/apps.plugin: 7.0 MiB -> 1.2 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/usr/libexec/netdata/plugins.d/debugfs.plugin
Stripped usr/libexec/netdata/plugins.d/debugfs.plugin: 4.8 MiB -> 879.2 KiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/usr/libexec/netdata/plugins.d/ndsudo
Stripped usr/libexec/netdata/plugins.d/ndsudo: 399.0 KiB -> 62.1 KiB
/build/edge > cksum /build/edge/.netdata-fleet-example/payload.gz
Published /build/edge/netdata-armv6l-edge.gz.run
Published /build/edge/netdata-armv6l-edge.gz.run.sha256
Prepared payload: 54.0 MiB
Disk reduction: 89.5%
Installer: netdata-armv6l-edge.gz.run (21.8 MiB)
Checksum: netdata-armv6l-edge.gz.run.sha256
Manifest: usr/share/netdata/fleet-manifest.json
```

The result is `netdata-armv6l-edge.gz.run`, ready to install in your device image, and its `.sha256` checksum file.

## Prepare an ARMv7 package

Download the ARMv7 package and compare its checksum with the same release's `sha256sums.txt`:

```bash
curl -fL -o netdata-armv7l-v2.12.0-2-nightly.gz.run \
  https://github.com/netdata/netdata-nightlies/releases/download/v2.12.0-2-nightly/netdata-armv7l-v2.12.0-2-nightly.gz.run
sha256sum netdata-armv7l-v2.12.0-2-nightly.gz.run
```

Expected checksum:

```text
0994f644457a0c273fdef0a3c05e5e5c8f2fd9ca61843f5a1e1a231b15e74281  netdata-armv7l-v2.12.0-2-nightly.gz.run
```

Run preparation:

```bash
python3 ./prepare-fleet.py \
  --input netdata-armv7l-v2.12.0-2-nightly.gz.run \
  --sha256 0994f644457a0c273fdef0a3c05e5e5c8f2fd9ca61843f5a1e1a231b15e74281 \
  --keep apps,debugfs \
  --strip-mode all \
  --strip-tool llvm-strip \
  --apply \
  --output netdata-armv7l-edge.gz.run
```

Expected output, using the same workspace convention:

```text
/build/edge > python3 ./prepare-fleet.py --input netdata-armv7l-v2.12.0-2-nightly.gz.run --sha256 0994f644457a0c273fdef0a3c05e5e5c8f2fd9ca61843f5a1e1a231b15e74281 --keep apps,debugfs --strip-mode all --strip-tool llvm-strip --apply --output netdata-armv7l-edge.gz.run
Workspace retained for inspection: /build/edge/.netdata-fleet-example
Input: netdata-armv7l-v2.12.0-2-nightly.gz.run
Keep: apps, debugfs
Remove: charts, containers, dashboard, go, ioping, journal, log2journal, mcp, netflow, network, nfacct, otel, perf, python, scripts, slabinfo
Strip mode: all
Original payload: 676.4 MiB
Removed bundles: 482.7 MiB
Payload before stripping: 193.7 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/bash
Stripped bin/bash: 1.3 MiB -> 1.3 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/curl
Stripped bin/curl: 5.4 MiB -> 5.4 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/nd-run
Stripped bin/nd-run: 452.4 KiB -> 45.4 KiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/netdatacli
Stripped bin/netdatacli: 2.3 MiB -> 258.7 KiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/srv/netdata
Stripped bin/srv/netdata: 158.9 MiB -> 22.0 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/bin/systemd-cat-native
Stripped bin/systemd-cat-native: 9.1 MiB -> 5.4 MiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/usr/libexec/netdata/plugins.d/apps.plugin
Stripped usr/libexec/netdata/plugins.d/apps.plugin: 6.6 MiB -> 921.1 KiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/usr/libexec/netdata/plugins.d/debugfs.plugin
Stripped usr/libexec/netdata/plugins.d/debugfs.plugin: 4.6 MiB -> 647.2 KiB
/build/edge > llvm-strip --strip-all -o /build/edge/.netdata-fleet-example/stripped-file /build/edge/.netdata-fleet-example/tree/usr/libexec/netdata/plugins.d/ndsudo
Stripped usr/libexec/netdata/plugins.d/ndsudo: 378.1 KiB -> 46.1 KiB
/build/edge > cksum /build/edge/.netdata-fleet-example/payload.gz
Published /build/edge/netdata-armv7l-edge.gz.run
Published /build/edge/netdata-armv7l-edge.gz.run.sha256
Prepared payload: 40.6 MiB
Disk reduction: 94.0%
Installer: netdata-armv7l-edge.gz.run (19.3 MiB)
Checksum: netdata-armv7l-edge.gz.run.sha256
Manifest: usr/share/netdata/fleet-manifest.json
```

### Preview a different selection

Leave out `--apply` and `--output` to preview which bundles will be kept and removed. Add `--verbose` for the complete file report. Detailed file lists and exact byte counts are also retained in the installed `usr/share/netdata/fleet-manifest.json`.

Each run keeps its preparation workspace for inspection. Include those workspaces in your build-host cleanup policy.

## Install the prepared package

Install into a clean device image so the component selection matches your policy. The static installer uses `/opt/netdata`:

```bash
sudo sh ./netdata-armv6l-edge.gz.run --accept -- \
  --dont-start-it --disable-telemetry
```

For ARMv7, use `netdata-armv7l-edge.gz.run`. `--accept` enables unattended license acceptance. Apply the [Child configuration](./minimize-cpu-and-memory.md) and [streaming credentials](./deployment-and-identity.md) before starting the Agent:

```bash
sudo systemctl start netdata
sudo /opt/netdata/bin/netdatacli version
```

Expected version:

```text
netdata v2.12.0-2-nightly
```

The installer assigns plugin ownership and Linux capabilities, including setuid fallbacks. If your pipeline copies an offline prepared tree instead, apply the same ownership and privilege policy. Use the static installer's `/opt/netdata` prefix; `--target` is an extraction option, not a supported installation-prefix setting.

Save the package manifest and checksum with the image release. Use the [image-controlled update workflow](./updates-and-troubleshooting.md) to keep the selected components through future updates.

## Plugin capability reference

| `--keep` capability | What it provides | Keep it when |
|:--|:--|:--|
| `apps` | Per-process and process-group CPU, memory, disk I/O and other resource metrics, plus the live `processes` Function | You need to identify a busy, leaking or failing device application from the Parent |
| `debugfs` | Linux hwmon sensors through bundled libsensors, the live `sensors` Function, powercap/RAPL energy metrics, memory fragmentation, zswap and Linux audit status | You need the sensors or kernel interfaces present on your hardware |
| `network` | Live TCP/UDP connection inspection with `network-viewer.plugin`, plus connection topology and `apps` for process attribution | You need to investigate which device process is communicating with a service |
| `containers` | Container-name resolution, Kubernetes labels and container-network helpers for the built-in cgroups collector | Devices run containers and you need their names and network attribution. The cgroups collector itself is in the Agent executable |
| `go` | The Go collector bundle, its discovery and supported live Functions, stock collector configurations and Prometheus profiles; also retains `local-listeners` and `snmp-trap-profile-gen` | You collect application, database, UPS, service, SNMP, Prometheus or other metrics supplied by this bundle. Select jobs in `go.d` configuration |
| `journal` | Reading and querying local systemd journals through the journal Function | Operators need device logs through the Parent. Keep readable systemd journals on the device |
| `otel` | OTLP/gRPC ingestion of supported metrics, logs and traces, with local log/trace storage and query Functions | Device applications export OpenTelemetry to the local Agent. Account for log and trace retention separately |
| `netflow` | NetFlow/IPFIX/sFlow ingestion and flow exploration, with bundled IP-intelligence databases | The device receives flow exports. The database assets account for most of this bundle's size |
| `dashboard` | Web dashboard assets hosted by the device | Operators use a device-local dashboard. Omit it when dashboards are served by the Parent |
| `mcp` | `nd-mcp`, the stdio-to-WebSocket bridge used by desktop MCP clients | A desktop client uses this bridge on the device; the Agent provides the MCP server |
| `python` | `python.d.plugin` and its bundled Python collector modules/configurations | You use one of these collectors. The device needs a Python interpreter and any collector-specific dependencies; the package includes its YAML loader, but does not bundle Python |
| `charts` | `charts.d.plugin`, its Bash collector modules and stock configurations | You use a charts.d module. The package retains Bash, but module-specific external commands must be available on the device |
| `scripts` | `scripts.d.plugin` and its Nagios-check configuration | You run Nagios-compatible checks. Provide the check executables, interpreters and their dependencies on the device; they are not bundled with Netdata |
| `perf` | Linux perf-event CPU counters, including cycles, instructions and cache events | You need CPU performance-counter monitoring and the hardware/kernel exposes the counters. Enable this plugin explicitly |
| `slabinfo` | Per-cache Linux kernel slab allocator statistics from `/proc/slabinfo` | You need detailed kernel-memory troubleshooting. Enable it explicitly and retain its privileged permissions |
| `nfacct` | Netfilter connection-tracking statistics and packet/byte totals for configured accounting objects | The device uses the relevant kernel facilities and accounting rules |
| `ipmi` | `freeipmi.plugin`, for IPMI hardware sensors and event-log metrics | The hardware exposes an IPMI management interface and the input package includes this plugin |
| `xen` | `xenstat.plugin`, for Xen host and domain resource statistics | The device is a Xen hypervisor host (dom0); this is not a guest-VM collector |
| `cups` | CUPS printer and print-job metrics | The device operates a CUPS print service and the input package includes this plugin |
| `ebpf` | The packaged C and Go eBPF plugins for kernel instrumentation | You need their kernel/network/process monitoring and have a supported package, kernel and required privileges |
| `systemd-units` | The live `systemd-list-units` Function for service and other unit status over D-Bus | You need live service-status inspection and the input package includes this plugin; service metrics are also available from the separate Go systemd collector |
| `ioping` | The bundled `ioping` executable and its collector/configuration | You need active storage-latency probes against configured devices, files or directories |
| `log2journal` | The `log2journal` utility and stock conversion configurations | Your log pipeline converts application output into structured journal records |

Use `python3 ./prepare-fleet.py --list-capabilities` to see selection names and their paths. `--keep all` keeps all available bundles; `--keep none` keeps the Agent and shared helpers. Select capabilities available in your input package: the ARMv6 input used here lacks `journal`, `otel` and `netflow`; the ARMv7 input contains all three, which this example's `--keep apps,debugfs` selection removes. Neither input package includes `ipmi`, `xen`, `cups`, `ebpf` or `systemd-units`.

Keep `go` when your devices need its collectors, then enable the jobs you use. Bundle selection controls disk space; collector configuration controls runtime work. Custom configuration in `etc/netdata` is retained. With customized source trees, use `--verbose` to inspect files inside bundles selected for removal.

## Package size reference

| Package | ARMv6 | ARMv7 |
|:--|--:|--:|
| Upstream compressed installer | 173.0 MiB | 254.0 MiB |
| Upstream payload | 514.7 MiB | 676.4 MiB |
| All components, debug stripped | 338.5 MiB | 496.4 MiB |
| All components, fully stripped | 332.1 MiB | 473.9 MiB |
| `apps,debugfs`, fully stripped | **54.0 MiB** | **40.6 MiB** |
| Reduced compressed installer | **21.8 MiB** | **19.3 MiB** |

Payload sizes cover package files. Allow additional space for filesystem allocation and any local history or logs you retain.

### Optional bundle sizes

These include the bundle's executables, configuration, profiles and data. The `network` row covers the network viewer; add the `apps` row for its process-attribution dependency.

| Capability | ARMv6 original | ARMv6 `all` | ARMv7 original | ARMv7 `all` |
|:--|--:|--:|--:|--:|
| `apps` | 7.00 | 1.26 | 6.58 | 0.91 |
| `debugfs` | 4.79 | 0.86 | 4.56 | 0.63 |
| `network` | 20.94 | 8.38 | 18.37 | 6.04 |
| `containers` | 9.48 | 7.12 | 9.35 | 7.01 |
| `go` | 189.80 | 186.83 | 189.53 | 186.60 |
| `mcp` | 6.56 | 6.53 | 6.50 | 6.47 |
| `journal` | Absent | Absent | 8.12 | 1.43 |
| `otel` | Absent | Absent | 23.88 | 14.72 |
| `netflow` | Absent | Absent | 149.03 | 142.47 |
| `dashboard` | 51.76 | 51.76 | 51.76 | 51.76 |
| `python` | 0.68 | 0.68 | 0.68 | 0.68 |
| `charts` | 0.05 | 0.05 | 0.05 | 0.05 |
| `scripts` | 14.38 | 14.33 | 14.31 | 14.26 |
| `perf` | 2.56 | 0.38 | 2.44 | 0.28 |
| `slabinfo` | 2.51 | 0.36 | 2.40 | 0.26 |
| `nfacct` | 2.54 | 0.38 | 2.41 | 0.27 |
| `ipmi` | Absent | Absent | Absent | Absent |
| `xen` | Absent | Absent | Absent | Absent |
| `cups` | Absent | Absent | Absent | Absent |
| `ebpf` | Absent | Absent | Absent | Absent |
| `systemd-units` | Absent | Absent | Absent | Absent |
| `ioping` | 0.12 | 0.12 | 0.09 | 0.09 |
| `log2journal` | 4.32 | 1.25 | 3.81 | 0.90 |

The Go bundle is about **187 MiB after stripping**. The ARMv7 NetFlow bundle includes about **132 MiB of IP-intelligence data**. Select these capabilities when your devices use them to match the image size to the monitoring job.

### ARMv6 executable sizes

| Executable | Original MiB | Debug stripped MiB | Fully stripped MiB |
|:--|--:|--:|--:|
| `bash` | 1.86 | 1.86 | 1.86 |
| `curl` | 7.38 | 7.38 | 7.38 |
| `log2journal` | 4.31 | 1.36 | 1.25 |
| `nd-mcp` | 6.56 | 6.53 | 6.53 |
| `nd-run` | 0.46 | 0.08 | 0.06 |
| `netdatacli` | 2.46 | 0.43 | 0.35 |
| `srv/netdata` | 168.99 | 33.14 | 30.19 |
| `systemd-cat-native` | 11.10 | 8.63 | 7.35 |
| `apps.plugin` | 6.99 | 1.38 | 1.25 |
| `cgroup-name` | 6.75 | 6.70 | 6.70 |
| `cgroup-network` | 2.71 | 0.50 | 0.41 |
| `debugfs.plugin` | 4.79 | 0.97 | 0.86 |
| `go.d.plugin` | 162.50 | 162.49 | 162.49 |
| `ioping` | 0.11 | 0.11 | 0.11 |
| `local-listeners` | 3.55 | 0.71 | 0.61 |
| `ndsudo` | 0.39 | 0.08 | 0.06 |
| `network-viewer.plugin` | 20.94 | 9.64 | 8.38 |
| `nfacct.plugin` | 2.54 | 0.46 | 0.38 |
| `perf.plugin` | 2.56 | 0.46 | 0.38 |
| `scripts.d.plugin` | 14.38 | 14.33 | 14.33 |
| `slabinfo.plugin` | 2.51 | 0.44 | 0.36 |
| `snmp-trap-profile-gen` | 9.25 | 9.24 | 9.24 |

### ARMv7 executable sizes

| Executable | Original MiB | Debug stripped MiB | Fully stripped MiB |
|:--|--:|--:|--:|
| `bash` | 1.30 | 1.30 | 1.30 |
| `curl` | 5.44 | 5.44 | 5.44 |
| `log2journal` | 3.80 | 1.01 | 0.89 |
| `nd-mcp` | 6.50 | 6.47 | 6.47 |
| `nd-run` | 0.44 | 0.06 | 0.04 |
| `netdatacli` | 2.34 | 0.33 | 0.25 |
| `srv/netdata` | 158.94 | 25.17 | 22.02 |
| `systemd-cat-native` | 9.11 | 6.65 | 5.37 |
| `apps.plugin` | 6.57 | 1.03 | 0.90 |
| `cgroup-name` | 6.75 | 6.70 | 6.70 |
| `cgroup-network` | 2.58 | 0.38 | 0.30 |
| `debugfs.plugin` | 4.56 | 0.74 | 0.63 |
| `go.d.plugin` | 162.44 | 162.43 | 162.43 |
| `ioping` | 0.08 | 0.08 | 0.08 |
| `local-listeners` | 3.35 | 0.54 | 0.44 |
| `ndsudo` | 0.37 | 0.06 | 0.05 |
| `netflow-plugin` | 17.23 | 17.18 | 10.67 |
| `network-viewer.plugin` | 18.37 | 7.33 | 6.04 |
| `nfacct.plugin` | 2.41 | 0.35 | 0.27 |
| `otel-plugin` | 23.87 | 23.81 | 14.71 |
| `perf.plugin` | 2.44 | 0.36 | 0.28 |
| `scripts.d.plugin` | 14.31 | 14.26 | 14.26 |
| `slabinfo.plugin` | 2.40 | 0.34 | 0.26 |
| `snmp-trap-profile-gen` | 9.25 | 9.24 | 9.24 |
| `systemd-journal.plugin` | 8.12 | 1.70 | 1.43 |
