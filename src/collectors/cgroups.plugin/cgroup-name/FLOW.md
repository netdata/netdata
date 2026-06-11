<!-- markdownlint-disable-file -->

# cgroup-name flow

This document is the implementation binding for the Go `cgroup-name` binary.
It maps the legacy `cgroup-name.sh.in` flow line-by-line by behavior. The
binary must preserve the script's branch order, exit codes, output format,
logging payload, environment handling, and known bugs.

## Startup

- Extend `PATH` before every lookup: existing `PATH` plus
  `/sbin:/usr/sbin:/usr/local/sbin:<sbindir>`.
- Set `LC_ALL=C` before every lookup and before spawning subprocesses.
- Build `ND_REQUEST` as `'<argv0>' '<arg1>' '<arg2>' `:
  each argument is wrapped in single quotes, there is a trailing space, and
  embedded single quotes are not escaped.
- `PROGRAM_NAME` is the executable basename. The installed binary therefore
  logs `cgroup-name`.
- `NETDATA_LOG_LEVEL` maps to syslog priorities. Unknown values keep the
  default info threshold.

## Logging

Every log call invokes:

```text
systemd-cat-native --log-as-netdata
```

The payload is written on stdin with these fields:

```text
INVOCATION_ID=<NETDATA_INVOCATION_ID>
SYSLOG_IDENTIFIER=<program basename>
PRIORITY=<level>
THREAD_TAG=cgroup-name
ND_LOG_SOURCE=collector
ND_REQUEST=<quoted argv with trailing space>
MESSAGE=<message with literal LF bytes replaced by backslash+n>

```

The final blank line is intentional. `MESSAGE` uses the shell transform
`${*//$'\n'/\\n}`, so Go uses `strings.ReplaceAll(msg, "\n", "\\n")`.

## Inputs And Exit Codes

- `CGROUP_PATH` is argv[1] exactly as received.
- `CGROUP` is argv[2] with every `/` replaced by `_`.
- Empty `CGROUP` logs fatal `"called without a cgroup name. Nothing to do."`
  and exits `1`.
- Exit codes are:
  - `0`: success or enabled fallback.
  - `1`: fatal no-argument path only.
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

## Docker/Podman API Branch

`docker_like_get_name_api(host_var, id)` returns failure only when:

- the selected host variable is empty; or
- `jq` is not found on `PATH`.

Every other outcome returns success, even when curl/HTTP/JSON parsing fails and
no name is found. This is required because the shell function ends with
unconditional `return 0`.

Host parsing preserves the shell regex `^([a-z]+)://(.*)`: lowercase schemes
are stripped before constructing the request target. Unix-socket paths are
detected with a socket stat. Docker/Podman API calls do not use `--fail`, so
HTTP non-2xx bodies are still parsed and only missing/invalid fields leave the
name empty.

## Label Helpers

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

`jq` must be present on `PATH`; absence warns and returns `1`.

The Kubernetes cache files are:

- `${TMPDIR:-/tmp}/netdata-cgroups-k8s-cluster-name`
- `${TMPDIR:-/tmp}/netdata-cgroups-kubesystem-uid`
- `${TMPDIR:-/tmp}/netdata-cgroups-containers`

For container ids, all three files must exist and the containers file must have
a grep match for the container id to use the cache. Otherwise the binary reads
any existing cluster/system uid cache, discovers missing values, then overwrites
the cache files with non-atomic writes equivalent to `echo > file 2>/dev/null`.

Kubernetes data source selection is a switch:

- If `KUBERNETES_SERVICE_HOST` and `KUBERNETES_PORT_443_TCP_PORT` are set, use
  the in-cluster API. Read the bearer token from
  `/var/run/secrets/kubernetes.io/serviceaccount/token`. Non-2xx responses are
  failures. TLS verification deliberately differs from the shell's
  `curl --fail -sSk`:
  - API-server calls verify against the system pool plus the mounted
    service-account CA (the in-cluster default of client-go and of netdata's
    go.d collectors). Setting `K8S_TLS_INSECURE` to anything but
    `0`/`false`/`no` disables this verification (escape hatch for custom-PKI
    clusters) and logs a warning.
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
Failure produces the cluster name `unknown`.

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

For pod cgroups, take the first matching pod line, strip from
`,container_` to the end, append pod labels, remove `pod_uid`, prefix labels,
and build:

```text
pod_<namespace>_<pod_name> <labels>
```

Names containing `_null` followed by `_` or end warn. With
`USE_KUBELET_FOR_PODS_METADATA` set they return `2`; otherwise they return `1`.

The function prints the built name, then returns `0` if it is non-empty and `1`
if it is empty. `k8s_get_name` prefixes successful output with `k8s_`.
Return `1` enables `k8s_<id>`, return `2` retries with `k8s_<id>`, and every
other code disables with `k8s_<id>`.

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
