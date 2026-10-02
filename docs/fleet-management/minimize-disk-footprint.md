# Minimize the Disk Footprint of Fleet Devices

Fit Netdata into your device image by removing unused plugin bundles and stripping debugging symbols before deployment. Keep system and application monitoring on the device, and use a Parent for dashboards, historical analysis, alerts and anomaly detection.

For the worked example below, a package retaining host metrics, process monitoring and hardware sensors occupies **54.0 MiB on ARMv6** or **40.6 MiB on ARMv7**, before filesystem overhead and runtime data. The compressed installers are **21.8 MiB** and **19.3 MiB**. These are the results for Netdata **v2.12.0-2-nightly**, using LLVM 23.1.1 and `--keep apps,debugfs --strip-mode all`.

Use the [fleet preparation script](https://github.com/netdata/netdata/blob/master/packaging/makeself/prepare-fleet.sh) on your build host. It leaves the upstream package intact and produces a new installer or offline package tree for your image pipeline.

## Prepare the build host

You are modifying an existing package, so you need binary utilities, not a cross compiler, target sysroot or Go/Rust build environment. The preparation tools run on the build host and do not add dependencies to your devices.

On a Debian or Ubuntu build host, install the tools from the distribution's package repositories:

```bash
sudo apt-get update
sudo apt-get install bash python3 coreutils curl binutils-arm-linux-gnueabihf
```

Python must be version 3.9 or newer. `coreutils` supplies `cksum` and `sha256sum`. The [ARM binutils package](https://packages.ubuntu.com/noble/binutils-arm-linux-gnueabihf) supplies `arm-linux-gnueabihf-objcopy`, which handles both ARMv6 and ARMv7 packages. For a 64-bit ARM package, install `binutils-aarch64-linux-gnu` and use `aarch64-linux-gnu-objcopy`.

Alternatively, use [LLVM's `llvm-objcopy`](https://llvm.org/docs/CommandGuide/llvm-objcopy.html), which handles multiple target architectures with one executable:

```bash
sudo apt-get install llvm
llvm-objcopy --version
```

LLVM is also available as prebuilt host toolchains from the [official releases](https://github.com/llvm/llvm-project/releases). Choose the build matching your **build host**, extract it into your tool directory, and add its `bin` directory to `PATH`. The recorded example uses [LLVM 23.1.1](https://github.com/llvm/llvm-project/releases/tag/llvmorg-23.1.1). Check the version when reproducing its byte counts; another tool version may produce different output sizes.

Confirm the prerequisites:

```bash
bash --version
python3 --version
cksum --version
llvm-objcopy --version
```

If using GNU binutils, replace `--objcopy llvm-objcopy` in the examples with `--objcopy arm-linux-gnueabihf-objcopy`.

Allow space for the upstream installer, its uncompressed tar, a staging tree, and the rebuilt tar and gzip payload. **4 GiB of free build-host space per ARM package** provides working room for these example inputs. Preparation workspaces remain on disk for inspection; account for each run in your build workspace lifecycle.

## Choose the capabilities to keep

The Agent executable contains the built-in Linux system collectors. Removing optional bundles does not remove that executable or the streaming, query and storage engines. CPU, memory, interfaces and disk metrics continue to be collected when their built-in collectors are enabled.

Choose optional components according to the information your team needs from the device. A Parent stores and presents the device's data; device-side collectors and Functions still need to be present to supply it.

| `--keep` capability | What it provides | Keep it when |
|:--|:--|:--|
| `apps` | Per-process and process-group CPU, memory, disk I/O and other resource metrics, plus the live `processes` Function | You need to identify a busy, leaking or failing device application from the Parent |
| `debugfs` | Linux hwmon sensors through bundled libsensors, the live `sensors` Function, powercap/RAPL energy metrics, memory fragmentation, zswap and Linux audit status | You need the sensors or kernel interfaces present on your hardware; unsupported interfaces are skipped |
| `network` | Live TCP/UDP connection inspection and network-connection topology; keeps `apps` for process attribution | You need to investigate which device process is communicating with a service |
| `containers` | Container-name resolution, Kubernetes labels and container-network helpers for the built-in cgroups collector | Devices run containers and you need their names and network attribution. The cgroups collector itself is in the Agent executable |
| `go` | The Go collector bundle, its discovery and supported live Functions, stock collector configurations and Prometheus profiles; also retains `local-listeners` and `snmp-trap-profile-gen` | You collect application, database, UPS, service, SNMP, Prometheus or other metrics supplied by this bundle. Select jobs in `go.d` configuration |
| `journal` | Reading and querying local systemd journals through the journal Function | Operators need device logs through the Parent. The device must have readable systemd journal files |
| `otel` | OTLP/gRPC ingestion of supported metrics, logs and traces, with local log/trace storage and query Functions | Device applications export OpenTelemetry to the local Agent. Account for log and trace retention separately |
| `netflow` | NetFlow/IPFIX/sFlow ingestion and flow exploration, with bundled IP-intelligence databases | The device receives flow exports. The database assets account for most of this bundle's size |
| `dashboard` | Web dashboard assets hosted by the device | Operators use a device-local dashboard. Omit it when dashboards are served by the Parent; the required empty web directory is retained |
| `mcp` | `nd-mcp`, the stdio-to-WebSocket bridge used by desktop MCP clients | A desktop client uses this bridge on the device. The MCP server is part of the Agent; removing the bridge does not remove it |
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

Run `bash ./prepare-fleet.sh --list-capabilities` to see the exact paths for every selection. `--keep all` retains all capabilities present in that package. `--keep none` omits the optional bundles and retains the Agent and shared helpers. An explicitly requested capability that is absent from the package causes an error.

Availability depends on the release and architecture. In the pinned example, ARMv6 does not include `journal`, `otel` or `netflow`; ARMv7 does. Neither package includes `ipmi`, `xen`, `cups`, `ebpf` or `systemd-units`.

For a host/process/sensor device class, start with `apps,debugfs`. Add `network` for connection troubleshooting and `go` for the application collectors you use. Omitting `go` removes all of its collectors, not just unused jobs; disabling jobs at runtime reduces collection work while retaining the bundle on disk.

Configuration under `etc/netdata`, state and files outside omitted stock-component paths are retained. Review the removal preview when preparing a tree containing custom additions: a file inside an omitted bundle's directory is removed with that directory's assets.

## Choose a stripping mode

| Option | Result |
|:--|:--|
| `--strip-mode debug` | Default. Removes debug information while retaining more symbols for diagnosis |
| `--strip-mode all` | Also removes removable symbols for the smallest binaries; retain the original package for crash analysis |
| `--strip-mode none` | Selects components without changing their symbols |

Stripping creates smaller files. Use [CPU and memory configuration](./minimize-cpu-and-memory.md) to reduce the work and history kept by the running Agent. Already-stripped Go binaries change very little; selecting which bundles to ship brings the larger savings.

## Prepare an ARMv6 package

Download the script and the pinned upstream package:

```bash
curl -fL -o prepare-fleet.sh \
  https://raw.githubusercontent.com/netdata/netdata/master/packaging/makeself/prepare-fleet.sh
curl -fL -o netdata-armv6l-v2.12.0-2-nightly.gz.run \
  https://github.com/netdata/netdata-nightlies/releases/download/v2.12.0-2-nightly/netdata-armv6l-v2.12.0-2-nightly.gz.run
curl -fL -o sha256sums.txt \
  https://github.com/netdata/netdata-nightlies/releases/download/v2.12.0-2-nightly/sha256sums.txt
sha256sum netdata-armv6l-v2.12.0-2-nightly.gz.run
```

The upstream package checksum is:

```text
4bbb1c330137f8b53f895d173ae176fd71015032c2d75c843a0d5981e0184ebe  netdata-armv6l-v2.12.0-2-nightly.gz.run
```

Confirm it against the downloaded checksum file. For another release, obtain its checksum and use that value instead.

Preview the selection first:

```bash
bash ./prepare-fleet.sh \
  --input netdata-armv6l-v2.12.0-2-nightly.gz.run \
  --sha256 4bbb1c330137f8b53f895d173ae176fd71015032c2d75c843a0d5981e0184ebe \
  --keep apps,debugfs
```

The preview prints a JSON report listing the selected capabilities and omitted paths. It ends with:

```text
Preview only. Stripping savings are measured with --apply; no output published.
```

Create the installer with this exact command:

```bash
bash ./prepare-fleet.sh \
  --input netdata-armv6l-v2.12.0-2-nightly.gz.run \
  --sha256 4bbb1c330137f8b53f895d173ae176fd71015032c2d75c843a0d5981e0184ebe \
  --keep apps,debugfs \
  --strip-mode all \
  --objcopy llvm-objcopy \
  --apply \
  --output netdata-armv6l-edge.gz.run
```

The following is the complete combined stdout/stderr from the recorded run, using LLVM 23.1.1. The working directory and randomly generated `.netdata-fleet-*` name will reflect your build workspace. The initial JSON is the removal report; the executed commands follow it. Final per-binary sizes are stored in the installed manifest.

Read the [complete ARMv6 preparation output](./armv6l-preparation-output.md).

## Prepare an ARMv7 package

Download the ARMv7 asset from the same release:

```bash
curl -fL -o netdata-armv7l-v2.12.0-2-nightly.gz.run \
  https://github.com/netdata/netdata-nightlies/releases/download/v2.12.0-2-nightly/netdata-armv7l-v2.12.0-2-nightly.gz.run
sha256sum netdata-armv7l-v2.12.0-2-nightly.gz.run
```

Expected checksum:

```text
0994f644457a0c273fdef0a3c05e5e5c8f2fd9ca61843f5a1e1a231b15e74281  netdata-armv7l-v2.12.0-2-nightly.gz.run
```

Create the package:

```bash
bash ./prepare-fleet.sh \
  --input netdata-armv7l-v2.12.0-2-nightly.gz.run \
  --sha256 0994f644457a0c273fdef0a3c05e5e5c8f2fd9ca61843f5a1e1a231b15e74281 \
  --keep apps,debugfs \
  --strip-mode all \
  --objcopy llvm-objcopy \
  --apply \
  --output netdata-armv7l-edge.gz.run
```

Read the [complete ARMv7 preparation output](./armv7l-preparation-output.md).

## Plan package space

The upstream inputs and reduced outputs for this release are:

| Package | ARMv6 | ARMv7 |
|:--|--:|--:|
| Upstream installer, compressed | 173.0 MiB | 254.0 MiB |
| Upstream regular-file payload | 514.7 MiB | 676.4 MiB |
| All components, debug information removed | 338.5 MiB | 496.4 MiB |
| All components, removable symbols removed | 332.1 MiB | 473.9 MiB |
| `apps,debugfs`, removable symbols removed | 54.0 MiB | 40.6 MiB |
| Reduced installer, compressed | 21.8 MiB | 19.3 MiB |

Payload totals sum regular-file lengths. Filesystem allocation, metadata, logs, metric history and application telemetry storage are additional. The reduced installer itself also takes space if retained on the device.

### Individual plugin and executable sizes

All sizes below are **bytes**, for Netdata **v2.12.0-2-nightly**. `debug` and `all` use LLVM 23.1.1. Every ELF executable in these two upstream packages is listed, including shared runtime tools. Shell/Python scripts and data files are covered in the bundle table that follows.

#### ARMv6

| Executable | Original | `debug` | `all` |
|:--|--:|--:|--:|
| `bin/bash` | 1,946,344 | 1,946,344 | 1,946,268 |
| `bin/curl` | 7,743,228 | 7,743,228 | 7,743,152 |
| `bin/log2journal` | 4,519,924 | 1,421,376 | 1,305,560 |
| `bin/nd-mcp` | 6,881,440 | 6,847,144 | 6,847,144 |
| `bin/nd-run` | 486,464 | 87,084 | 67,000 |
| `bin/netdatacli` | 2,574,768 | 446,776 | 367,288 |
| `bin/srv/netdata` | 177,203,036 | 34,752,452 | 31,659,656 |
| `bin/systemd-cat-native` | 11,644,220 | 9,053,632 | 7,703,040 |
| `usr/libexec/netdata/plugins.d/apps.plugin` | 7,328,952 | 1,443,516 | 1,307,800 |
| `usr/libexec/netdata/plugins.d/cgroup-name` | 7,078,048 | 7,020,600 | 7,020,600 |
| `usr/libexec/netdata/plugins.d/cgroup-network` | 2,846,252 | 521,580 | 432,952 |
| `usr/libexec/netdata/plugins.d/debugfs.plugin` | 5,026,844 | 1,015,292 | 900,296 |
| `usr/libexec/netdata/plugins.d/go.d.plugin` | 170,393,760 | 170,384,320 | 170,384,320 |
| `usr/libexec/netdata/plugins.d/ioping` | 113,416 | 113,416 | 113,340 |
| `usr/libexec/netdata/plugins.d/local-listeners` | 3,725,580 | 743,888 | 642,016 |
| `usr/libexec/netdata/plugins.d/ndsudo` | 408,536 | 80,724 | 63,624 |
| `usr/libexec/netdata/plugins.d/network-viewer.plugin` | 21,953,892 | 10,111,576 | 8,782,848 |
| `usr/libexec/netdata/plugins.d/nfacct.plugin` | 2,660,776 | 482,092 | 396,456 |
| `usr/libexec/netdata/plugins.d/perf.plugin` | 2,684,488 | 481,360 | 397,872 |
| `usr/libexec/netdata/plugins.d/scripts.d.plugin` | 15,073,440 | 15,020,880 | 15,020,880 |
| `usr/libexec/netdata/plugins.d/slabinfo.plugin` | 2,633,004 | 460,040 | 379,592 |
| `usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` | 9,699,488 | 9,686,160 | 9,686,160 |

#### ARMv7

| Executable | Original | `debug` | `all` |
|:--|--:|--:|--:|
| `bin/bash` | 1,360,604 | 1,360,604 | 1,360,528 |
| `bin/curl` | 5,699,312 | 5,699,312 | 5,699,236 |
| `bin/log2journal` | 3,988,452 | 1,056,052 | 936,908 |
| `bin/nd-mcp` | 6,815,904 | 6,781,608 | 6,781,608 |
| `bin/nd-run` | 463,212 | 65,932 | 46,508 |
| `bin/netdatacli` | 2,458,772 | 343,292 | 264,876 |
| `bin/srv/netdata` | 166,661,472 | 26,392,392 | 23,086,728 |
| `bin/systemd-cat-native` | 9,557,284 | 6,978,160 | 5,626,356 |
| `usr/libexec/netdata/plugins.d/apps.plugin` | 6,891,880 | 1,084,808 | 943,236 |
| `usr/libexec/netdata/plugins.d/cgroup-name` | 7,078,048 | 7,020,600 | 7,020,600 |
| `usr/libexec/netdata/plugins.d/cgroup-network` | 2,710,152 | 401,780 | 314,156 |
| `usr/libexec/netdata/plugins.d/debugfs.plugin` | 4,778,096 | 780,884 | 662,716 |
| `usr/libexec/netdata/plugins.d/go.d.plugin` | 170,328,224 | 170,318,784 | 170,318,784 |
| `usr/libexec/netdata/plugins.d/ioping` | 84,732 | 84,732 | 84,656 |
| `usr/libexec/netdata/plugins.d/local-listeners` | 3,510,864 | 567,372 | 465,876 |
| `usr/libexec/netdata/plugins.d/ndsudo` | 387,168 | 63,784 | 47,228 |
| `usr/libexec/netdata/plugins.d/netflow-plugin` | 18,063,240 | 18,017,024 | 11,189,700 |
| `usr/libexec/netdata/plugins.d/network-viewer.plugin` | 19,264,304 | 7,690,068 | 6,329,332 |
| `usr/libexec/netdata/plugins.d/nfacct.plugin` | 2,528,364 | 365,976 | 281,756 |
| `usr/libexec/netdata/plugins.d/otel-plugin` | 25,032,160 | 24,968,132 | 15,423,900 |
| `usr/libexec/netdata/plugins.d/perf.plugin` | 2,563,112 | 373,700 | 291,364 |
| `usr/libexec/netdata/plugins.d/scripts.d.plugin` | 15,007,904 | 14,955,344 | 14,955,344 |
| `usr/libexec/netdata/plugins.d/slabinfo.plugin` | 2,511,868 | 352,588 | 273,084 |
| `usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` | 9,699,488 | 9,686,160 | 9,686,160 |
| `usr/libexec/netdata/plugins.d/systemd-journal.plugin` | 8,516,100 | 1,786,324 | 1,501,112 |

### Complete optional bundle sizes

These totals include each capability's dedicated configuration, profiles, scripts and data assets, as well as its executables. Sizes are MiB before filesystem overhead. Shared helpers are retained separately; the `network` row excludes its automatically retained `apps` dependency.

| Capability | ARMv6 original | ARMv6 `all` | ARMv7 original | ARMv7 `all` |
|:--|--:|--:|--:|--:|
| `apps` | 7.00 | 1.26 | 6.58 | 0.91 |
| `debugfs` | 4.79 | 0.86 | 4.56 | 0.63 |
| `network` | 20.94 | 8.38 | 18.37 | 6.04 |
| `containers` | 9.48 | 7.12 | 9.35 | 7.01 |
| `go` | 189.80 | 186.83 | 189.53 | 186.60 |
| `mcp` | 6.56 | 6.53 | 6.50 | 6.47 |
| `journal` | 0.00 | 0.00 | 8.12 | 1.43 |
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

The Go bundle is about **187 MiB after stripping**, including its stock configurations and profiles. Keep it for the collectors you need, then disable unnecessary jobs to reduce runtime work. The ARMv7 NetFlow bundle includes about **132 MiB of IP-intelligence data**; omitting that capability removes the dedicated data along with the plugin.

## Install the prepared package

Use a fresh device image so omitted plugins cannot remain from an earlier installation. The static installer installs at `/opt/netdata`:

```bash
sudo sh ./netdata-armv6l-edge.gz.run --accept -- \
  --dont-start-it --disable-telemetry
```

For ARMv7, use `netdata-armv7l-edge.gz.run`. `--accept` supplies unattended license acceptance; arguments after the bare `--` go to the Netdata post-installer. Do not change the installation prefix with `--target` when installing.

Apply the [Child configuration](./minimize-cpu-and-memory.md) and [streaming credentials](./deployment-and-identity.md) before starting the Agent:

```bash
sudo systemctl start netdata
sudo /opt/netdata/bin/netdatacli version
```

For this release, the CLI reports:

```text
netdata v2.12.0-2-nightly
```

The installer sets the required plugin ownership and Linux capabilities, with setuid fallbacks where necessary. Copying a prepared tree directly into an image requires your image pipeline to assign that ownership and those privileges too; preserving executable mode alone is not sufficient for privileged collectors.

The installed `usr/share/netdata/fleet-manifest.json` records requested/resolved capabilities, omitted files, and before/after sizes for each stripped executable. Archive output includes a `.sha256` sidecar. Store them with the upstream package, preparation-script revision and tool versions in your release inventory.

Run preparation again for each release. Use the [image-controlled update workflow](./updates-and-troubleshooting.md) to retain the device class's component policy: the standard updater installs a full upstream package.
