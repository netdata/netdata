# Native Linux host-to-container Java monitoring

The experiment runs a native monitor against newly created Java applications on an authorized, shared Debian VM.
Its purpose is to remove the shared-PID-namespace assumption from the preceding Docker Desktop study while preserving
existing VM services. The application fixture, upstream OTel agent and Hikari extension are unchanged.

## Environment and containment

- Debian 13.7, Linux 6.12.63+deb13-amd64, x86_64; systemd 257, Docker with systemd cgroup v2, Yama `ptrace_scope=0`.
- No host Java installation was present. The runner extracted the pinned Temurin 21 helper runtime into a private
  root-owned task bundle. No packages, accounts, persistent services, sysctl settings or existing applications changed.
- One native application uses UID 61001. Two plain JRE containers use UIDs 61002 and 61003, ordinary bridge networking,
  and their default independent PID namespaces. Container application images contain no monitoring jar or helper.
- Native units and container cgroups live beneath a randomly named task slice. Before inspecting executable/arguments,
  the scanner requires exact slice membership or a descendant. It receives no target PID list. A fourth matching
  application, also task-owned, runs outside that slice to check exclusion.
- Each transient service is bounded to two CPUs, 768 MiB, 256 tasks and 15 minutes; containers have two-CPU/768-MiB
  limits. Native fixtures use private temporary directories and a read-only binding of the runtime bundle. The monitor
  remains in the host PID namespace and performs no `nsenter`, `setns`, host-PID container operation or Docker API call.

The cgroup admission boundary is protection for this shared VM experiment. It is not a proposed user setup requirement,
nor proof that production service discovery and selection are solved. `host_checks.py` orchestrates the lab through
Docker/systemd; `Scout` discovers candidate processes directly through `/proc` inside its admitted scope.

## Tested behavior

The live run and complete evidence replay passed on 2026-10-09:

| Check | Observed result |
|---|---|
| Initial discovery and attachment | One native JVM and two container JVMs, three UIDs and three independent PID namespaces |
| Monitor restart | No repeated attachment to the three already monitored processes |
| Host application restart | Automatically attached; all required contexts have fresh samples under the new process identity |
| Rolling container replacement | New container ID and independently live PID namespace; automatic attachment and fresh samples under the new identity |
| Empty-capability control | Access denied for all three admitted JVMs; no candidate discovered or attached |
| Matching application outside scope | Served 500 requests; no attachment attempt or OTLP metrics |

Each of the five monitored phases served 2,500 measured requests: 2,250 HTTP 200 responses and 250 deliberately generated
HTTP 503 responses, with zero transport failures or unexpected statuses. Raw HTTP route, status, count and histogram
checks pass. All five phases have actual stored samples for JVM memory, HTTP count/buckets, pool active/idle connections,
pending requests and pool limit. Recovery checks require each context on the new instance and timestamps after its
restart/replacement boundary; delayed samples from the old instance cannot satisfy them.

There were five successful attachments and five unique attempts. Both container applications run as PID 1 in their
own namespaces; the native application's PID namespace matches the host monitor. No application monitoring flags,
exporters, remote JMX configuration or preinstalled monitoring artifacts were required.

## Permissions and delivery

The working native monitor has `NoNewPrivileges=yes` and only these capabilities in its bounding set:

- `CAP_SYS_PTRACE` for inspecting different-user target process paths.
- `CAP_SETUID` and `CAP_SETGID` for switching each attachment child to the target credentials.

The child clears supplementary groups and performs artifact delivery and Attach API operations as the application user.
It repeats the cgroup/process identity check before delivery and before attachment. Jars go into a freshly created
directory in the target's `/tmp`, reached through `/proc/<host-pid>/root`. The bundled JDK handles the target's inner
PID and Attach socket path. Target containers need neither a JDK nor shared host directories or monitoring JVM options.

The empty-capability scanner is a read-only negative control. This is a tested sufficient configuration on this VM,
not a claim about the universal minimum permissions. Existing SELinux/AppArmor policy, user namespaces, restricted
procfs mounts, other Yama settings, disabled JVM attachment/loading and target storage policy can change the result.
None of those policies was relaxed for the test.

The exact tested Attach implementation is linked in [the preceding experiment](AUTOMATION_RESULTS.md). systemd's
[execution environment documentation](https://manpages.debian.org/trixie/systemd/systemd.exec.5.en.html) describes the
capability bound, `NoNewPrivileges` and private temporary-directory behavior used by the fixture.

## Harness corrections

The initial native fixture failed before monitoring began because systemd attempted NSS resolution for an unused
numeric UID (`217/USER`). `setpriv` now selects these numeric fixture credentials without creating accounts.
`PrivateTmp` also hides `/var/tmp`; explicitly binding the bundle read-only keeps it available in the fixture's private
mount namespace. Neither correction adds monitoring configuration to the Java application.

A first replacement check assumed namespace inode numbers would never repeat. After the old container stopped, the
replacement received the same number. The final test performs rolling replacement: compare namespace identities while
both processes are alive, then stop the old target before the replacement workload. Process recovery always requires
PID plus start time and fresh metric samples under the new service-instance identity. Namespace inode numbers are not
persistent instance identifiers.

## What this establishes

The deployment path remains a Netdata-owned process monitor and privilege boundary, a bundled Java helper and upstream
agent plus extension, and existing OTLP ingestion. Application owners should not need to install exporters or configure
JMX for the tested stack. The next production design needs explicit ownership of discovery, admission, identity,
attachment outcomes, upgrades, artifact cleanup and user-visible coverage states.

The prototype still uses fixture jar matching, UID-based service names and a per-run attempt journal. It does not
implement unrestricted discovery, a production security boundary, multiple-monitor coordination, whole-host reboot
recovery, in-place agent upgrades or hardened/rootless/Kubernetes deployment. Readiness traffic accounts for at most one
additional successful request in restart phases. Hikari discovery still requires a borrow after attachment.

This was a deployment/lifecycle experiment, not an overhead benchmark. The preceding repeated measurement still found
approximately 101 MiB extra RSS per JVM with the narrow configuration; this experiment does not reduce or generalize it.

## Reproduction and evidence

See [the authorized host procedure](README.md#authorized-native-linux-host-experiment). `host_checks.py --verify`
performs read-only replay of saved raw OTLP, chart/data captures, lifecycle events, process namespace/cgroup evidence
and cleanup/pre-existing-container checks. The same updated scanner also passes the previous isolated-container test.
No production Netdata collector, privilege helper, installer or public metric contract is changed.

The replay output is committed as [host-evidence.json](host-evidence.json). Raw evidence is retained locally under
`.local/java-monitoring-spike/host-verified`; both earlier harness attempts are retained separately as diagnostics.
All 89 evidence files across the three attempts were copied and checksum-verified before deleting the remote workspace.

All owned transient workloads, containers and the bridge network were removed, and the task slice is inactive. The
three pre-existing containers retained their running state, process IDs, start times and restart counts. The exact task
directory, private runtime and three unique task image tags were removed after evidence preservation. Downloaded base
images and Docker build cache remain; no global prune was performed.
