# Browser runtime installation and lifetime

This package verifies release descriptors and archives, prepares private executable
paths, and coordinates installation, process ownership and execution lifetime.
It does not install host packages, invoke npm or choose component versions.
Release inputs and adapters live in `src/dem-runtime` and `packaging/dem-runtime`.

## Trust and preparation

An administrator-trusted format-1 descriptor identifies execution contract 1,
release version, platform, archive SHA256/size, tar/expanded bounds, entry count,
layout and authenticated interpreter slots. The archive is not its own trust anchor.
Production authentication and release discovery are separate delivery work.

Extraction rejects escaping paths, duplicate entries, privileged modes, devices,
hard links and symlink parents. Preparation validates plain executables and the
entrypoint, binds only authenticated interpreter slots to the final generation,
and generates the private browser wrapper/font configuration. It syncs staging
before publishing. A prepared generation must never be relocated.

The store reserves archive plus expanded workspace before opening the source.
Explicit downloads use HTTPS with a 60-second network inactivity timeout; continuing
network progress has no total deadline. Local writes suspend the inactivity timer.
Offline import verifies the identical bounds and digest. Browser qualification is
separate from preparation and belongs to the CLI caller before selection.

## Ownership

The root is service-owned, mode 0700, on one host with working file locks, rename
and directory fsync. Do not copy it to another host or use network filesystems.
This is not isolation against malicious code running as the same service account.

| Protection | Lifetime |
|---|---|
| Selected installation | Durable until explicit uninstall or replacement selection; survives idle periods and process death |
| Process consumer | One release acquired atomically at startup; old processes retain it after updates |
| Candidate | Serialized installation transaction, from preparation through qualification and selection/abort |
| Execution receipt | Every executable access; only authoritative drainage, helper receipt or verified reboot permits release |

`BeginInstall` holds a separate installation lock while preparation/qualification
runs outside inventory coordination. `Candidate.Select` publishes the qualified
release. `AcquireSelected` reads that selection and creates its process claim under
the same coordination lock. There is no per-monitor claim or automatic download.
A missing startup selection requires a restart after installation.

Each consumer/candidate `BeginUse` publishes an execution record before returning
paths. Pass its `Completion` to exactly one supervised invocation, join the helper,
then `Finish`. Version probes follow the same rule. An owner that never launched
can report verified-empty completion; recovery cannot infer this from owner death.
Closing a consumer blocks future uses but does not manufacture child completion.

Uninstall serializes against installation and refuses any live consumer/candidate,
preparation or unresolved execution across all generations. Refusal preserves
selection. Successful uninstall clears selection before retiring bytes. No timeout,
PID disappearance or process lock can substitute for an execution receipt.

Selection uses synced temporary metadata, rename and a parent-directory sync.
Failure after rename is explicitly uncertain, not rollback. Before executable
admission, another mutation or retirement, the store syncs and reads the visible
selection under coordination. A failing durability barrier blocks those actions;
a successful one reconciles the authoritative state, including after restart.

Reboot recovery requires stored and current boot identities with verified kernel
provenance. Container replacements and bind mounts of the per-read `uuid` source
are rejected. Missing provenance disables reboot recovery, not ordinary receipts.

## Retirement and status

The default allowance is 4 GiB of logical regular-file bytes, including archives,
workspace, lifecycle records and trash. Active preparation reserves its unwritten
remainder using the same observation as byte accounting. This is not a physical
quota; filesystem metadata and protected preloads are additional.

Selected, process-owned, candidate and execution-protected generations never yield
to pressure. The most recently used unprotected old generation has a seven-day
soft grace; others can retire immediately. Pressure can shorten grace. A selected
release stays installed even without monitors. Unknown executions may retain files
indefinitely and prevent a later installation.

`Inspect` is read-only: selected descriptor, protected/reclaimable totals, preparation,
trash, and overlapping selected/process/candidate/execution byte categories.
Do not sum overlapping categories as extra disk use. Reclaimable failed-deletion
trash is not free space. `Sweep` reconciles and retires at startup, before
preparation and hourly; cleanup health is separate from monitor outcomes.

Objects retire to trash under coordination. Parent directories are synced before
physical deletion outside coordination. Failed lock releases remain owned by the
store for retry; a failed execution release never supplies completion evidence.

## Protected preloads

`Candidate.SealPreload` finishes a fresh, qualified root and records its trusted
identity/final location. Managed operations then refuse the root, including stores
opened before sealing. `ReadPreload` supports read-only mounts without state writes.
The administrator must keep it private, service-account-owned and immutable, and
stop every consumer before removal/replacement. Native package transaction safety
requires separate payload/coordination ownership and is deferred to delivery work.

## Validation

Tests cover archive bounds/path handling, interpreter binding, selection/activation,
idle and pressure protection, serialization, refused uninstall, publication failure,
process death, uncertain uses, receipt recovery and boot provenance.
`DEM_RUNTIME_TEST_HELPER` enables real supervisor receipt tests; run under the same
unprivileged identity as the helper. Real CLI qualification covers journeys and
Lighthouse. Filesystem fault tests do not simulate physical power loss or establish
native SELinux/AppArmor policy compatibility.
