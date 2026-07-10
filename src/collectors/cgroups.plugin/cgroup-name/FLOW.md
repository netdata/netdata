<!-- markdownlint-disable-file -->

# cgroup-name flow

This document is the implementation binding for the Go `cgroup-name` binary.
It maps the legacy `cgroup-name.sh.in` flow by behavior. The binary preserves
branch order, exit codes, and the external output grammar except where this
document explicitly records a correctness, security, or typed-data fix.

## Source ownership

- `main.go`: process entry point only.
- `run.go`: argv, stdout, and exit-code contract.
- `config.go`: environment preparation and immutable invocation configuration.
- `resolver.go`: top-level resolution orchestration and explicit result types.
- `observability.go`: logfmt logging, shared deadline, and call timing.
- `cgroup_resolver.go`: non-Kubernetes dispatch and local cgroup-name heuristics.
- `container_runtime.go`: Docker/Podman command and API resolution.
- `http.go`: bounded HTTP, Unix-socket transport, and Kubernetes TLS policy.
- `json.go` and `labels.go`: shared ordered JSON and label value models.
- `kubernetes.go`: Kubernetes cgroup interpretation and result assembly.
- `kubernetes_metadata.go`: cache/source orchestration for Kubernetes metadata.
- `kubernetes_sources.go`: API server, kubelet, kubectl, and GCP metadata sources.
- `kubernetes_cache.go`: private atomic cache persistence and lookup.
- `kubernetes_model.go`: Kubernetes pod JSON projection.

Tests use the matching `*_test.go` ownership file. `run_test.go` protects the
external process contract across all internal boundaries.

## Startup

- Replace inherited `PATH` before every lookup with the fixed trusted list
  `/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin:/snap/bin`.
- Set `LC_ALL=C` before every lookup and before spawning subprocesses.
- Build `ND_REQUEST` as `'<argv0>' '<arg1>' '<arg2>' `:
  each argument is wrapped in single quotes, there is a trailing space, and
  embedded single quotes are not escaped.
- `PROGRAM_NAME` is the executable basename. The installed binary therefore
  logs `cgroup-name`.
- `NETDATA_LOG_LEVEL` maps `emerg|emergency`, `alert`, `crit|critical`,
  `err|error`, `warn|warning`, `notice`, `info`, and `debug` to the matching
  syslog priorities. Unknown values keep the default info threshold.

## Logging

Each log call writes one logfmt line to stderr, like `go.d.plugin` and
`cgroup-network`:

```text
time=<RFC3339 millis> comm=<program basename> level=<name> request=<quoted argv> msg=<message>
```

The daemon captures helper stderr, so the short-lived helper does not open a
per-message `systemd-cat-native` connection: a wedged journal could otherwise
block name resolution. `request` and `msg` are Go `%q`-quoted, which escapes
embedded quotes and newlines, so the line is always single-line and parseable.

## Timeout And Deadline

`cgroups.plugin` reads `[plugin:cgroups]` `script to get cgroup names`, whose
default is the installed `cgroup-name` binary. It enables rename matching only
when that configured path is executable; builds that omit the helper therefore
retain raw cgroup names without repeated spawn failures. A persisted value
equal to the installed legacy `plugins.d/cgroup-name.sh` path is migrated to
the new binary; all custom helper paths remain unchanged.

`NETDATA_CGROUP_NAME_TIMEOUT_MS` carries the operator budget X (from the
`[plugin:cgroups]` `cgroup-name timeout` option, default 120s). The helper sets
`expires_at = start + X` and shares one deadline `context.Context` across every
external command and HTTP call, so each call gets the remaining budget and
calls after expiry are not attempted. HTTP response bodies are capped (16 MiB
generally, 64 MiB for Kubernetes pod lists); kubectl output uses the same caps.
If the name is still unresolved at the deadline, the helper logs a per-call
time breakdown at error level and exits with the retry code instead of emitting
a fallback name, so discovery's retry ladder runs. `cgroups.plugin` waits X plus
a 2s grace period for a complete response, then kills a silent or partial
helper. After a complete response it allows up to another 2s for process exit;
X=0 keeps the legacy unbounded response wait. The parent incrementally polls
and reads until newline, EOF, the size bound, or the original absolute deadline;
a helper that writes a partial record cannot reset or bypass the timeout.

## Inputs And Exit Codes

- `CGROUP_PATH` is argv[1] exactly as received.
- `CGROUP` is argv[2] with every `/` replaced by `_`.
- Empty `CGROUP` logs fatal `"called without a cgroup name. Nothing to do."`
  and exits `1`.
- Exit codes are:
  - `0`: success or enabled fallback.
  - `1`: fatal missing-argument, oversized-output, or stdout-write failure.
  - `2`: retry.
  - `3`: disable.

`DOCKER_HOST` defaults to `unix:///var/run/docker.sock` when unset or empty.
`PODMAN_HOST` defaults to `unix:///run/podman/podman.sock` when unset or empty.

## Docker And Podman Inspect Parsing

`parse_docker_like_inspect_output` consumes lines from Docker/Podman inspect.
Only these assignment names are recognized:

- `NOMAD_NAMESPACE`
- `NOMAD_JOB_NAME`
- `NOMAD_TASK_NAME`
- `NOMAD_SHORT_ALLOC_ID`
- `CONT_NAME`
- `IMAGE_NAME`

If all four Nomad variables are present, the name is:

```text
<namespace>-<job>-<task>-<short_alloc_id>
```

Otherwise the name is `CONT_NAME` with one leading slash removed.

If `IMAGE_NAME` is non-empty, labels start as:

```text
image="<image>"
```

Every line beginning `LABEL_netdata.cloud/` appends a label. The line is split
only at the first `=`, so additional `=` characters remain part of the value.

The shell helper `eval`ed the inspect output, so exported `NOMAD_*`,
`CONT_NAME`, or `IMAGE_NAME` variables from the helper's own environment could
leak into the result, and container-controlled values were executed by the
shell. The Go helper only parses the output text; this is an intentional fix.

## Docker/Podman API Branch

`docker_like_get_name_api(host_var, id)` treats every outcome as handled: an
empty host variable only logs a warning, and curl/HTTP/JSON failures leave the
name empty so the caller emits the `id[:12]` fallback with the retry exit code.
This mirrors the shell function, which ended with an unconditional `return 0`
whenever `jq` was available. The shell's only real failure mode was a missing
`jq`, which chained to a `podman inspect` CLI fallback; the Go helper parses
JSON natively, so that fallback leg does not exist and the podman flow is
API-only.

Host parsing strips `unix://` for socket access, maps `tcp://` to plain HTTP,
preserves explicit `http://` and `https://`, and rejects unsupported schemes.
Unix-socket paths are detected with a socket stat. Docker/Podman API calls do
not use `--fail`, so HTTP non-2xx bodies are still parsed and only
missing/invalid fields leave the name empty.

## Label Helpers

The Go label serializer escapes quotes and backslashes and replaces control
characters with spaces. This is an intentional safety exception to the shell's
direct interpolation of label values; field order and the external
`name="value"` grammar remain stable.

- `get_lbl_val(labels, name)` splits labels on commas, then on the first `=`.
  It returns the value without the surrounding quotes. Missing or empty labels
  return the literal string `null`.
- `add_lbl_prefix(labels, prefix)` prefixes every comma-separated label and
  trims the final comma.
- `remove_lbl(labels, name)` removes labels whose name matches exactly.

## Kubernetes Flow

`k8s_get_kubepod_name(cgroup_path, id)` runs only for ids containing
`kubepods`.

The first transform removes all `.slice` and `.scope` substrings from the id.
Then it follows this switch:

- `kubepods` -> name is `kubepods`, exit `0`.
- id ending in `besteffort`, `burstable`, or `guaranteed` -> replace `-` with
  `_`, collapse the `kubepods_kubepods` prefix to `kubepods`, exit `0`.
- regex `.+pod[a-f0-9_-]+_(docker|crio|cri-containerd)-([a-f0-9]+)$` ->
  container id is capture group 2.
- regex `.+pod[a-f0-9-]+_([a-f0-9]+)$` -> container id is capture group 1.
- regex `.+pod([a-f0-9_-]+)$` -> pod uid is capture group 1 with `_` changed
  to `-`.
- otherwise warn and return `3`.

For container ids, `k8s_is_pause_container` reads the relevant `cgroup.procs`
file from the host cgroup tree, splits all whitespace, requires exactly one
process, then reads `/proc/<pid>/comm` from the running namespace. A single
process named `pause` returns `3`.

The shell helper required `jq` on `PATH` here (absence warned and returned
`1`); the Go helper parses all JSON natively and has no such gate.

The Kubernetes cache lives in a private `0700` state directory:

- `${NETDATA_LIB_DIR}/cgroup-name` under the daemon.
- `${TMPDIR:-/tmp}/netdata-cgroup-name-<euid>` only for standalone invocations
  where `NETDATA_LIB_DIR` is absent.

The files inside it are:

- `netdata-cgroups-k8s-cluster-name`
- `netdata-cgroups-kubesystem-uid`
- `netdata-cgroups-containers`

For container ids, all three files must exist and the containers file must have
a streaming exact match on the final typed `container_id` field to use the
cache. Raw delimiter matching rejects collisions without parsing every
preceding label set; only the matching record is decoded. Metadata records are
capped at 64 KiB, individual container records at 16 MiB, and the complete
container cache at 128 MiB. Scans honor the invocation context.

The directory and files must be owned by the current effective user. The
directory mode is exactly `0700`; files must be single-link `0600` regular files.
Readers reject unsafe mode, ownership, link count, size, symlinks, and inode
replacement. Missing values are discovered and files are rewritten atomically
through an unguessable `0600` same-directory temporary file. Legacy predictable
files directly under `TMPDIR` are never migrated or trusted. (The shell helper
used plain `echo > file 2>/dev/null` writes.)

Kubernetes data source selection is a switch:

- If `KUBERNETES_SERVICE_HOST` and `KUBERNETES_PORT_443_TCP_PORT` are set, use
  the in-cluster API. Read the bearer token from
  `/var/run/secrets/kubernetes.io/serviceaccount/token`. Non-2xx responses are
  failures. TLS verification deliberately differs from the shell's
  `curl --fail -sSk`:
  - API-server calls verify only against the mounted service-account CA. An
    unreadable or invalid CA fails closed and logs the loading error. Setting
    `K8S_TLS_INSECURE` to anything but `0`/`false`/`no` disables verification
    (escape hatch for custom-PKI clusters) and logs a warning.
  - Kubelet calls (`USE_KUBELET_FOR_PODS_METADATA`) never verify: stock
    kubelet serving certificates are self-signed, and even cluster-CA-signed
    ones carry only node-name/IP SANs, so verification of
    `https://localhost:10250` cannot succeed on any cluster.
  - `KUBELET_URL` is a base URL and always gets `/pods` appended, like the
    shell's `${KUBELET_URL:-https://localhost:10250}/pods`.
- Else, if `ps -C kubelet` succeeds and `kubectl` exists, invoke `kubectl`.
  The namespace lookup uses the current `KUBE_CONFIG` value. Before the pod
  lookup only, unset `KUBE_CONFIG` becomes `/etc/kubernetes/admin.conf`; an
  explicitly empty `KUBE_CONFIG=""` remains empty.
- Else warn and return `1`.

GCP cluster metadata calls are the only allowed parallel HTTP chain. Each uses
`Metadata-Flavor: Google`, bypasses proxies, has the existing three-second
timeout, treats non-2xx as failure, and all three values must be non-empty.
Failure produces the cluster name `unknown`. That negative result is cached for
five minutes; after expiry, only the three GCP metadata requests are retried and
cached pod/container labels remain usable.

Pod JSON is converted to one line per container:

```text
namespace="<ns>",pod_name="<pod>",pod_uid="<uid>",<netdata.cloud annotations>,<controller>,node_name="<node>",container_name="<container>",container_id="<id-without-runtime-prefix>"
```

For container cgroups, append `kind="container"`, `qos_class`, optional
`cluster_id`, and optional `cluster_name`; then remove `container_id` and
`pod_uid`, prefix labels with `k8s_`, and build:

```text
cntr_<namespace>_<pod_name>_<container_name> <labels>
```

For pod cgroups, take the first matching pod record and retain typed fields up
to the first actual `container_*` field, then append pod labels, remove
`pod_uid`, prefix labels, and build:

```text
pod_<namespace>_<pod_name> <labels>
```

This typed boundary is an intentional safety exception to the shell's raw
`,container_` substring truncation: an annotation value containing that text
does not corrupt the pod label record.

Required namespace, pod-name, and container-name fields are validated by typed
presence and non-empty value before constructing the result. Missing/empty
fields warn and return `2` with `USE_KUBELET_FOR_PODS_METADATA`, otherwise `1`.
A literal Kubernetes name equal to `null` is a valid value and succeeds.

The function returns the built name and code `0` when the name is non-empty, or
code `1` when it is empty. `k8s_get_name` prefixes successful output with
`k8s_` and `run()` writes stdout. Return `1` enables `k8s_<id>`, return `2`
retries with `k8s_<id>`, and every other code disables with `k8s_<id>`.

## Main Dispatch

The main dispatcher is a switch, not a fallback chain. Kubernetes has priority:
ids containing `kubepods` call the Kubernetes flow before every other branch.

If no name was set, these branches run in order:

1. Docker regex `^.*docker[-_/\.][a-fA-F0-9]+[-_\.]?.*$`.
   Extraction intentionally uses only `docker[-_/]`.
2. ECS regex `^.*ecs[-_/\.][a-fA-F0-9]+[-_\.]?.*$`.
   Extraction is `^.*ecs[-_/].*[-_/]([a-fA-F0-9]+)[-_\.]?.*$`.
3. Containerd regex `system.slice_containerd.service_cpuset_[a-fA-F0-9]+[-_\.]?.*$`.
   Extraction preserves the script typo:
   `^.*ystem.slice_containerd.service_cpuset_([a-fA-F0-9]+)[-_\.]?.*$`.
4. Podman regex `^.*libpod-[a-fA-F0-9]+.*$`.
   Extraction is `^.*libpod-(conmon-)?([a-fA-F0-9]+).*$`.
5. systemd-nspawn `machine.slice[_/].*\.service`.
6. libvirt/LXC `machine.slice_machine.*-lxc`.
7. libvirt/qemu `machine.slice_machine.*-qemu`.
8. libvirt qemu `machine_.*\.libvirt-qemu`.
9. Proxmox qemu `qemu.slice_([0-9]+).scope` when
   `${NETDATA_HOST_PREFIX}/etc/pve` is a directory.
10. Proxmox LXC `lxc_([0-9]+)` when `${NETDATA_HOST_PREFIX}/etc/pve` is a
    directory.
11. LXC 4.0 `lxc.payload.*`.

Docker accepts ids with length 64 or 12. Podman accepts length 64 only. Invalid
ids log errors but do not change `EXIT_CODE`; the final fallback still exits 0.
Podman preserves the legacy error text:

```text
a podman id cannot be extracted from docker cgroup '<cgroup>'.
```

If the switch still produced no name, use `CGROUP`.

The 100-byte truncation applies only inside this non-Kubernetes block. K8s
names set by the first block skip truncation. The final space-to-underscore
replacement applies to every name.

Final stdout is either:

```text
<NAME>
```

or:

```text
<NAME> <LABELS>
```

The payload before the newline is at most 8190 bytes, so the C consumer's
8192-byte buffer receives the complete record plus newline and terminator.
Oversized records and short/failed writes produce fatal exit `1`; the C
consumer rejects any incomplete record, so partial stdout is never applied.
After the normal retry ladder, monitoring continues with the raw cgroup name.
Kubernetes-prefixed `k8s_netdata.cloud/cgroup.name` and
`k8s_netdata.cloud/ignore` are control annotations at that boundary; other
`k8s_netdata.cloud/*` annotations remain ordinary labels.
