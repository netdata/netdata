# Bundle map

What each path holds, which platform produces it, and what its absence means. The rationale for
collecting any item is owned by `packaging/installer/SUPPORT-BUNDLE.md#what-is-collected-and-why`;
this file is the routing and the absence semantics, which live nowhere else.

## Read the manifest first

`MANIFEST.json` indexes every file except itself. Per row: `path`, `kind`, `origin`, `title`,
`bytes`, `pii_obfuscated`, `sanitized`. Top level: `schema`, `tool_version`, `generated_utc`,
`runtime_seconds`, `pii_obfuscated`, `secrets_redacted`, `agent_running`, `agent_api_reachable`
and `is_container`. Two are additive and absent from older bundles:
`streaming_api_key_redacted` and the `snmp_diagnostics` object. Treat either as *unknown* when
missing - never as `false`, which would read as "the key was redacted" or "raw SNMP evidence is
present".

Three top-level fields decide how you read everything else:

- `agent_api_reachable` false means the whole runtime area is a single marker file, not evidence of
  a dead agent. See `./false-signals.md`.
- `is_container` true changes where logs live and which host facts are visible.
- `tool_version` ending in `-windows` means the Windows implementation produced this bundle, so the
  POSIX-only rows below are absent by construction, not by failure.

`kind` tells you how to read a file. A `cmd` capture opens with a
`# netdata-support-bundle v<version> | command: ... | captured: <utc>` provenance header, and on
POSIX also ends with an `# exit: N | duration: Ns` trailer - read that trailer before trusting the
body. A `file` or `api` capture carries no header; its provenance is the manifest row only. So a
`.txt` without a `#` header is a copied file, not a command capture.

## Why a file is missing

Resolve every absence through this list before concluding anything:

| Cause | How you recognize it |
|---|---|
| Wrong platform | The row is marked POSIX-only or Windows-only below; check `tool_version` |
| The agent's API was down | The API-backed runtime captures are gone and a "was down" marker replaces them. The binary-derived ones - build information and the build cache - are still there, because they run the binary rather than the API |
| The producer no longer exists | The source was removed upstream; the map below flags these |
| Global deadline | The body reads `SKIPPED: global deadline reached`, and the manifest `origin` is `skipped` |
| Per-command timeout | Windows replaces the body with a timeout line. POSIX only shows a non-zero `# exit:` trailer, which is the wrapped command's own status - an ordinary command failure looks identical, so a non-zero trailer is not by itself evidence of a timeout |
| Cap exceeded | A command capture carries a `### TRUNCATED ###` line; an API body is replaced wholesale by a withheld-body JSON marker; a copied file simply starts mid-source, and only the manifest `origin` says so |
| Sanitizer withheld it | The body states the content was withheld - binary or NUL bytes, a symlinked source, a capped tail with no line break, or a sanitizer failure |
| Genuinely absent | Everything above is excluded, and the host really had no such file |

Two platform asymmetries that change the meaning of absence:

- A **failed** API call on POSIX still writes a file containing a capture-failed JSON marker with the
  exit code. On Windows the file is deleted. So on Windows "no file" conflates "endpoint failed" with
  "never attempted"; on POSIX it does not.
- A successful but **empty** capture is deleted on both.

## Bundle root

| Path | Holds | Platform |
|---|---|---|
| `summary.txt` | Header, agent state, and the fixed READ ORDER FOR TRIAGE. Includes the last exit reason when the status file had one, and an unclean-termination warning when no process exists but the status file still says running | all |
| `README.md` | In-bundle self-documentation, including the streaming-key and SNMP warnings | all |
| `MANIFEST.json` | The index. Written last, indexes everything but itself | all |

The pseudonym map is written **beside** the archive, never inside it. Without it you cannot resolve a
pseudonym to a real identity - ask the customer, and only when the identity is load-bearing.

## `01-system/` - platform context

POSIX: kernel and architecture, OS release, uptime and load, CPU count, memory, filesystem usage,
virtualization and container detection, cgroup version (Linux only, absent when the cgroup mount is
missing), clock and time sync, the mount table, SELinux/AppArmor state, and kernel messages.

Kernel messages are a filtered tail: the kernel journal narrowed to out-of-memory, segfault and
netdata matches, with a `dmesg` fallback. A kill by a container runtime, or a systemd out-of-memory
policy action, does not match that filter and leaves nothing here.

Windows: OS version, computer info, disk usage, clock and time sync, uptime. There is no
kernel-messages equivalent and no Windows Error Reporting scan, so external kills leave no trace.

## `02-install/` - how the agent got here

POSIX: the install-time environment file, the install-type marker, package-manager information, an
inferred install type, and container context (present only when the host was detected as a
container).

Windows: MSI registry information, the install tree, and the install-type marker. `Win32_Product` is
deliberately never queried because it triggers MSI reconfiguration.

The install type decides which update and troubleshooting paths apply, and the environment file
carries install flags and the release channel - including custom compiler flags, which have caused
storage-engine faults.

## `03-process/` - the running agent

POSIX: the netdata process tree with CPU and memory; per-thread CPU; process status, limits and file
descriptor count; the agent's own environment; and a zombie-process check. Everything keyed on the
process id is absent when no netdata process was found, and the `/proc`-derived rows are additionally
absent on macOS and BSD.

The agent's environment matters because the environment a service sees differs from the environment
in the reporter's shell - proxy and claiming variables in particular. When it cannot be read, the
capture says so and falls back to a filtered view of the bundle shell's own environment, which is a
different fact.

Windows: the netdata process list and the service definition - start mode, the account it runs as,
the binary path, state and exit code. Windows is the only platform where the service definition is
captured; POSIX has no unit or drop-in capture at all.

## `04-config/` - configuration

| Path | Holds |
|---|---|
| `effective-netdata.conf` | The merged running config the agent actually uses, with unrecognized options annotated. **Authoritative over every on-disk file.** Absent when the API was unreachable |
| `config-tree.txt` | A listing of the config directory. Files present here are user-customized. TLS material is suppressed |
| `netdata.conf`, `stream.conf`, `go.d.conf` | The on-disk files |
| `exporting.conf`, `cloud.conf` | POSIX only |
| `claim.conf` | Windows only |
| user config files | Every config under the config directory on POSIX, path-mirrored. Windows sweeps only a fixed set of plugin subdirectories, so anything outside them is invisible there |

The sweep stops after a bounded number of files, and user configs get a smaller cap than the main
files. TLS key material is excluded everywhere by path and by extension.

`stream.conf` is the one file whose streaming API key survives verbatim, so a child's key can be
compared against a parent's. Every other secret in it is still redacted.

## `05-logs/` - history

POSIX: the systemd journal for the unit, **and separately the netdata journal namespace** - on
systemd installs the agent logs to its own namespace, so the unit journal alone shows almost nothing.
Then the on-disk log files, the access log on a smaller cap, the updater journal, and coredump
metadata. Most journal captures are bounded by the run's log window and then tailed. The updater journal is
the exception - it is tail-capped only, so it can carry events from outside the window.

In a container the log files are symlinks to standard output, so no history exists on disk; the
bundle writes a marker naming the command that retrieves it from the host instead. A marker is not
logs.

Coredump metadata proves a dump exists and matches the crash time. The dump itself is never
collected.

Windows: one merged event-log file covering the Netdata ETW channels plus the legacy channel and
Netdata records from the Application log, ordered for triage with the highest-volume channel last and
smallest. It also records **channel state**, because a disabled channel is otherwise indistinguishable
from the agent having logged nothing. It carries no maximum size or retention mode, so it cannot
confirm a full channel - ask for that separately. Any on-disk log files are collected too, but
a default Windows install logs to the channels, not to files.

There is no updater log and no coredump metadata on Windows.

## `06-state/` - persistent state

| Path | Holds | Notes |
|---|---|---|
| `status-file.json` | The daemon status file: last exit reason and cause, signal, fault address, a fatal object with file/function/line/message/errno, stack traces, restart and crash counts, plus database, memory and disk state | The single most valuable crash artifact. POSIX picks the newest of several trusted locations and excludes shared temporary directories; Windows reads one fixed path |
| `state-tree.txt` | POSIX: aggregate file count and byte total only, because a state filename can itself be a live credential. Windows: a listing with bearer-token filenames withheld and counted | The POSIX form is strictly stricter |
| `cloud-state.txt` / `claimed-id.txt` | The claim id only. POSIX captures a listing plus the id; Windows copies the id file | The claim token and private key are never collected |
| `db-disk-usage.txt` | Per-tier disk usage and database file sizes. POSIX additionally reports corruption sentinels | Read against the effective config, not against expectations |
| `health-silencers.json` | Persisted alert silencers | POSIX only. **Absent on Windows**, so "why are alerts silent" has no evidence there |
| `pythond-job-statuses.json` | python.d job state | POSIX only, only when that plugin wrote a dump, and only in bundles produced after the collector change that renamed it. Before that change the same content was published at `go.d-job-statuses.json` under a go.d title |
| `dyncfg/` | Configs created or modified through the UI or API | POSIX only. **Absent on Windows**, so UI-created jobs and UI-edited alerts are invisible there |
| `snmp-diagnostics-status.txt` | Always present; states whether SNMP evidence was requested and what happened | Hand SNMP questions to `triage-snmp-diagnostics` |
| `snmp-diagnostics/` | The opt-in, **unsanitized** SNMP store | Owned by `triage-snmp-diagnostics` |

**go.d job state is not in this area.** go.d stopped writing job state to disk when its job manager
moved to a single-owner command kernel; it now lives in the dynamic configuration tree, captured in
the runtime area. A bundle taken while the API was down therefore carries no go.d job state at all.
Older bundles may contain a `go.d-job-statuses.json` path whose contents are actually python.d state
- treat that path in any older bundle as python.d, not go.d.

## `07-runtime/` - live agent state

The API captures here are present only when the agent's API answered; otherwise a single marker file
replaces them, and its own text lists three causes - see `./false-signals.md`. The binary-derived
captures are the exception: build information and the build cache run the binary rather than the API,
so they survive a dead daemon and are the fallback identity evidence when everything else here is
missing.

| Path | Holds |
|---|---|
| `info-v3.json` | The best single call: build information, features, cloud status, per-tier retention. Works even under bearer protection |
| `info-v1.json`, `node-instances.json` | Children, streaming state, database size, metric counts |
| `stream-info.json`, `aclk.json` | Streaming and cloud-connection diagnostics |
| `alerts-active.json`, `alerts-all.json` | Current alert state - a snapshot, never a history |
| `functions.json` | Which plugins expose what |
| `dyncfg-tree.json` | **The authoritative collector job state**: every go.d and scripts.d job with its status (`accepted`, `running`, `failed`, `disabled`, `orphan`, `incomplete`, `none`), plus service discovery, vnodes, secret stores and alert prototypes. Added by the collector change that removed the dead on-disk job-state file, so bundles produced before it do not carry this path |
| `ml-info.json` | Machine-learning status only; model state is never collected |
| `self-cpu.csv`, `self-memory.csv`, `self-api-clients.csv` | The agent's own bounded resource windows |
| `buildinfo.txt`, `buildinfo.json` | Required by the bug template; **works with the daemon down**, because it runs the binary |
| `cmakecache.txt` | How the agent was built - pinpoints a disabled plugin or a custom flag that build information alone misses |
| `aclk-state.json` | The command-line cloud state |
| `perflib.json` | Windows only. When it cannot run it holds a marker stating why - not elevated, timed out, or an access error - so the reason is visible instead of a missing file |

The build-information and cache captures come from the binary, not the API, so they survive a dead
daemon. That makes them the fallback identity evidence when the runtime area is otherwise empty.

The dynamic configuration tree is large. On an install with very many discovered jobs it can exceed
the API cap, in which case the file holds the withheld-body marker rather than a truncated tree.

## `08-network/` - connectivity

POSIX: the socket inventory for the whole netdata process tree - not a host-wide listing - with
several tool fallbacks and an explicit unavailable note when none is usable; resolver configuration
with search domains withheld unless obfuscation was disabled; the bundle shell's proxy variables; and
a Cloud reachability probe that validates the certificate.

The probes are deliberately asymmetric: local API reads clear proxy variables and target the loopback
address, while the Cloud probe **retains** the real proxy configuration so it represents the
installation's actual network path.

Windows: the socket inventory filtered to netdata processes, DNS configuration, proxy configuration
from both the system and the user hive, and a Cloud probe that is TCP and TLS only. The Windows DNS
capture carries interface aliases and server addresses only - there are no search domains in it to
withhold, and none to read.

## `09-permissions/` - why the agent cannot read or execute something

Mode bits alone explain almost nothing here, which is why this area exists.

POSIX: for every discovered plugin directory - mode, ownership, setuid and setgid entries, and
per-file capabilities. For every netdata path - mode, owner, extended attributes, security context,
access control lists, and non-default filesystem flags. An immutable flag on a state directory
silently blocks the agent's own writes, and a security-context mislabel fails collectors with nothing
in the agent log.

Plugin directories are derived from the binary prefix, the known install prefixes and the custom
plugin directory. A configured plugin-directory override is **deliberately not read**, so a host that
relocated its plugins may show a directory the agent does not actually use.

Tools are feature-detected and a missing one is reported rather than skipped - the extended-attribute
tool is not installed by default on Debian and Ubuntu, so its absence is common and not a finding.

Windows: access control lists with inheritance state, protected-ACL detection, integrity labels, and
alternate data streams. A download-marker stream records that the file arrived from another machine -
provenance, not proof that Windows blocked it; a protected
list means inheritance was disabled, a common post-restore breakage.

Owner for what these privileges are supposed to be:
`src/collectors/README.md#collector-privileges` and
`src/collectors/README.md#file-permissions-and-ownership`.
