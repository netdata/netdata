# Minimize the Disk Footprint of Fleet Devices

Prepare reduced static Netdata packages on the build host. The [fleet preparation script](https://github.com/netdata/netdata/blob/master/packaging/makeself/prepare-fleet.sh) strips ELF debugging information and omits explicitly unselected stock components. It leaves the input unchanged and writes a fresh output when `--apply` is supplied.

## Requirements and supported inputs

The standalone script requires Bash, Python 3.9 or newer, and an `objcopy` implementation supporting the target architecture when stripping. Installer output also requires POSIX `cksum`. These tools belong on the build host, not the device. A native x86 `objcopy` may not support ARM; choose an ARM cross-toolchain implementation or LLVM `llvm-objcopy` and pass it with `--objcopy`.

Supported inputs are:

- An official unsigned, unencrypted, single-gzip static `.gz.run` installer, supplied with its full-file published SHA-256 through `--sha256`.
- An offline static package tree, supplied with `--source`. Treat this as trusted image-build input; it has no upstream archive checksum.

Use `--input` for an installer file and `--output` for a new derived installer. Use `--source` for an offline tree and `--output` for a new tree. Never use a live installation. Inputs must include the static wrapper, daemon and installation scripts. Unsupported archive layouts, special files and unsafe links are rejected.

Keep the upstream installer and checksum, the preparation script revision, the selected policy, and the derived artifact checksum together in your release records. Verify upstream downloads before preparation. A derived installer has its own checksum and is not the original Netdata release artifact.

## Select capabilities explicitly

`--keep` is required. It accepts `all`, `none`, or comma-separated names. Run `--list-capabilities` to see the exact stock paths. Unselected components are omitted from the new output; recognized dedicated assets and stock configuration are omitted with them.

| Capability | Retained functionality |
|:--|:--|
| `apps` | Process collection and process Functions |
| `debugfs` | Debugfs-based hardware metrics |
| `network` | Network inspection; also retains `apps` for process enrichment |
| `containers` | Container naming and network attribution helpers |
| `go` | The complete Go collector bundle, its stock configuration, discovery helper and SNMP trap profile generator |
| `mcp` | The bundled local MCP tool |
| `journal`, `otel`, `netflow` | Journal inspection, OpenTelemetry ingestion, or NetFlow ingestion and IP-intelligence assets |
| `dashboard` | Device-local dashboard files |
| `python`, `charts`, `scripts` | The corresponding external collector framework and stock assets |
| `perf`, `slabinfo`, `nfacct`, `ipmi`, `xen`, `cups`, `ebpf`, `systemd-units`, `ioping` | The named optional plugin or helper |
| `log2journal` | Log-to-journal conversion tool and stock rules |

Explicitly requesting an unavailable capability fails. `all` retains everything available in the input without requiring every optional plugin to exist. There is no universal best minimal selection: `none` keeps built-in host collection and shared files, but removes these optional capabilities.

The script preserves unknown paths, user files under `etc/`, state under `var/`, and shared runtime/install helpers, including Bash, curl, privilege and logging helpers. It does not reduce compiled-in collectors, DBengine, ML or Go submodules. Configuration alone cannot shrink the monolithic Go binary; retaining one Go collector retains the bundle. Custom collectors may depend on optional components: include those capabilities explicitly and test the custom workload.

## Preview, prepare and inspect

Use `--help` for the command interface. First supply your input, checksum and `--keep` selection without `--apply`. The JSON preview lists requested and resolved capabilities, omitted paths and their regular-file byte total. Preview does not measure stripping savings or publish an output.

For the preparation run, add `--apply`, a fresh `--output`, the appropriate `--objcopy`, and the stripping policy:

- `--strip-mode debug` is the default. It removes debugging information while retaining more symbols useful for diagnosis.
- `--strip-mode all` additionally removes removable symbols. Crash diagnosis becomes harder; retain matching original binaries off-device.
- `--strip-mode none` omits components without stripping.

Commands are printed before execution, with failure status and working-directory context. Every stripped ELF is checked for unchanged allocated sections and identity. This is structural verification, not proof that the target runs correctly.

The output includes `usr/share/netdata/fleet-manifest.json` with the capability policy, omitted paths, measured stripping results and source installer hash when available. Installer outputs get a `.sha256` sidecar and rebuilt Makeself checksums, payload length and uncompressed-space declarations. Original modes, ownership metadata and symlinks are retained in installer output. Tree output retains modes and links; assign required ownership in the image pipeline.

Workspaces are retained beside the output, or in the current directory for archive previews. Budget for the upstream file, decompressed tar, staged tree and derived archive. Inspect failure output before retrying with a fresh destination. Do not deploy an incomplete tree left by a failed copy. Clean up only your pipeline's own intermediate files according to its retention policy.

## Measured 32-bit ARM package sizes

The following offline experiment used checksum-verified [v2.12.0-2-nightly installers](https://github.com/netdata/netdata-nightlies/releases/tag/v2.12.0-2-nightly), published October 1, 2026, and LLVM objcopy 23.1.1. MiB means 1,048,576 bytes. Payload is the sum of regular-file lengths, excluding filesystem block rounding and runtime databases.

| Experiment | ARMv6 payload MiB | ARMv7 payload MiB |
|:--|--:|--:|
| Original | 514.7 | 676.4 |
| Debug information stripped | 338.5 | 496.4 |
| Removable symbols also stripped | 332.1 | 473.9 |
| Host-metrics sizing candidate, retaining apps and debugfs | 55.2 | 41.5 |

The last row is a package-sizing experiment, not a universal profile or measured running installation. It omitted the Go bundle, local dashboard, network viewer, container helpers, optional collection frameworks and selected specialized plugins. Actual script output depends on your policy and version. ARMv7 contained journal, OpenTelemetry, NetFlow and about 131.8 MiB of IP-intelligence assets absent from ARMv6. Both included about 162.5 MiB of already stripped Go collector binaries.

The preparation script was also run against both installers with `--keep apps,debugfs --strip-mode all`. Its resulting regular-file payloads, before the small manifest, were 54.0 MiB for ARMv6 and 40.6 MiB for ARMv7. The difference from the exploratory host candidate comes from the script's exact dedicated-stock-asset policy. These output archives passed structural and checksum checks; device execution still requires image commissioning.

Original installer downloads were 173.0 and 254.0 MiB. Experimental host-candidate gzip payloads were about 22.3 and 19.7 MiB. Compression size differs from installed size; the installer also needs temporary extraction space. No ARM binaries were executed for these sizing measurements, and no CPU or RAM saving is inferred from them.

Finally, configure [local retention and logs](./minimize-cpu-and-memory.md): reducing package files does not bound future database and log growth. Use the [image-controlled update workflow](./updates-and-troubleshooting.md), because the stock updater downloads the full package again.
