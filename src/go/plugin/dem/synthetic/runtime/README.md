# Managed browser runtimes

This package owns the portable DEM runtime format, preparation and lifetime. It is
used by the process owner in `../runtimehost` for accepted enabled configuration
and asynchronous preparation. It does not install host packages or run npm.

## Trust and preparation

An administrator-supplied format-1 `Manifest` pins architecture, compressed SHA256 and
size, tar size, entry count, expanded regular-file bytes, layout and interpreter
bindings. An archive cannot provide its own trust anchor. Downloads use the
installed HTTPS URL; offline import verifies the identical digest and bounds.
HTTP acquisition and each response-body read have a 60-second network inactivity
timeout. Followed redirects refresh acquisition progress; ongoing body progress
has no overall transfer deadline. The timer is stopped during local archive writes
and preparation. Inactivity is an acquisition failure and uses the existing retry
backoff; voluntary demand cancellation remains distinct and permits immediate
preparation when demand returns. Closing the response cancels outstanding network
I/O and stops its timer.
Bundle construction, release selection and manifest installation are outside this
experimental foundation. Development deployments must supply a trusted manifest and
matching archive; no production download source is configured by this tree.

Extraction rejects escaping paths, duplicate entries, privileged modes, devices,
hard links and symlink parents. Internal symlinks are allowed. Before publication,
preparation validates and fills only the authenticated interpreter slots, naming
the final generation directory. It encodes generated shell/XML paths without
expanding user path characters. A prepared generation must not be relocated.

Files are prepared in an owned staging directory and synced before atomic
publication. The allowance reserves both the archive and expanded output before
opening the source. A failed preparation cannot replace a valid generation.
Prepared files establish integrity and layout, not browser sandbox availability;
qualification and actual monitoring must test their execution context.

## Ownership

The store is service-owned, mode 0700, on a host-local filesystem with working
locks, rename and directory fsync. It must never be copied to another host or
shared over a network filesystem. The same trusted-account boundary as `nd-run`
applies: this is not isolation against malicious service-account code.

- `ReserveDemand` protects a pending configuration without downloading or changing
  generation timestamps. `Claim.Commit` promotes it to accepted demand; an I/O
  failure retains the claim for retry. `Close` on an unused reservation does not
  change idle timestamps. Pending protection is excluded from retirement and
  reported separately from accepted demand.
- `Demand` immediately reserves and commits demand for explicit preparation.
  Accepted configuration uses the transaction above. A process-lifetime lock
  releases these claims after owner exit; it is never execution evidence.
- `BeginUse` publishes a unique durable execution pin before returning paths.
  Pass its `Completion` to **one** `ndexec` tree invocation and join that helper
  before `Finish`. Version probes use the same protocol. A caller that never
  launched may finish with a verified-empty result; recovery cannot guess this.
- Preparation owns an operation lock until all in-process readers/writers have
  joined. It never detaches an installer. A free operation lock is enough to
  reclaim abandoned preparation, but is **never** proof for an execution pin.
- Constructors publish fully initialized ownership records atomically. An
  interrupted unpublished initialization cannot have started an execution.

A matching helper receipt releases an execution pin even after plugin loss.
Missing or malformed proof retains its generation. Live authoritative drainage
also permits release; failed retirement can be retried while that proof remains
in memory. No PID disappearance, timeout or lock state releases execution use.
Manually launching from managed generation directories is unsupported.

Reboot recovery requires both stored and current boot identities to have verified
kernel provenance. The opened procfs file's mount ID and mount root must identify
`/sys/kernel/random/boot_id`. Container-specific replacements and a bind of the
per-read `uuid` source are rejected. Missing provenance disables reboot recovery;
it does not disable ordinary receipt recovery. Host locality remains a deployment
contract, not something PID namespaces or machine-id can prove.

## Storage policy

Defaults are 4 GiB logical owned regular-file bytes, seven days for one most
recently used previous generation while browser demand exists, and thirty days
for the last generation without demand. Grace deadlines persist across restarts.
Soft graces yield to acquisition pressure. Pending/accepted demand and execution
pins never do.
An uncertain pin can retain files indefinitely and prevent a later acquisition.

Archives, staging, lifecycle records and trash count as managed bytes. Active
preparation reserves its unwritten remainder; byte accounting uses the same
observation for used and reserved bytes while writers are active. This is an
allowance, not a physical-disk quota; filesystem metadata/allocation and protected
preloads are separate. ENOSPC still needs handling.

`Inspect` is read-only and never launches or prepares a runtime. It reports
protected and reclaimable bytes, written preparation/trash bytes, and overlapping
generation byte totals for pending/accepted demand, execution and uncertain
execution. These categories explain ownership; they must not be added together as extra disk use.
Allowance errors report needed, used, reserved, protected and reclaimable bytes.
Reclaimable bytes may still occupy failed-deletion trash; they are not free space.

Cached generations may be reclaimed at startup before saved enabled configuration
arrives, including after a long shutdown or under shared-store acquisition pressure.
A later accepted job can need another download; offline cache hits are not guaranteed.
Use administrator-managed protected preloads for controlled offline availability.
There is no global replay barrier or process-selection pin.

`Sweep` reconciles ownership and cleans eligible state; the process owner runs it at startup,
before preparation and hourly, including when no browser jobs remain. Retired
objects move to owned trash. Source and trash parents are synced before deletion,
including successor recovery. Deletion runs outside coordination exclusion and
failed bytes stay accounted. Cleanup health is separate from executable readiness.

`runtime preload` uses the same verified preparation, then seals its fresh root
with a `preload.json` record identifying the bundle and final location. Managed
operations refuse sealed roots, including through stores opened before sealing.
`ReadPreload` checks that record and prepared identity against the installed manifest
without locks, claims, writes or browser launches. It supports read-only mounts.
The administrator manages these files, but their private root must be owned by
the consuming service account. Stop all consumers before replacing or removing them. Component paths and ordinary unsealed caches cannot
be selected as preloads. Neither mode promises integrity against malicious code
running as the service account; verification of the archive occurs at preparation.

## Validation

Unit tests cover archive integrity/bounds, extraction types and path handling,
binding/encoding, demand/graces, pressure and uncertain-use protection, interrupted
preparation using real exiting processes, read-only status and boot provenance.
`DEM_RUNTIME_TEST_HELPER` enables the real `nd-run` persistent-receipt recovery
integration test. `DEM_RUNTIME_TEST_MANIFEST` and `DEM_RUNTIME_TEST_ARCHIVE`
add real-bundle import, inventory timing and supervised Node/browser/re-execution
qualification. Run Linux tests with `-race`; tests do not simulate power loss
or establish native AppArmor/browser support.
