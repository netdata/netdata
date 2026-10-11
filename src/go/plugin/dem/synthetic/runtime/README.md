# Managed browser runtimes

This package owns the portable DEM runtime format, preparation and lifetime. It is
not wired into job admission yet; the Q11 integration stage supplies that process
owner and installation workflows. It does not install host packages or run npm.

## Trust and preparation

An Agent-installed format-1 `Manifest` pins architecture, compressed SHA256 and
size, tar size, entry count, expanded regular-file bytes, layout and interpreter
bindings. An archive cannot provide its own trust anchor. Downloads use the
installed HTTPS URL; offline import verifies the identical digest and bounds.
The independent release pipeline must supply real pinned artifacts before an
installed download manifest is enabled.

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

- `Demand` protects a generation selected by accepted configuration, including
  one that is not prepared yet. A process-lifetime lock releases demand after
  owner exit. Reconcile startup demand before the first `Sweep`.
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
Soft graces yield to acquisition pressure. Demand and execution pins never do.
An uncertain pin can retain files indefinitely and prevent a later acquisition.

Archives, staging, lifecycle records and trash count as managed bytes. Active
preparation reserves its unwritten remainder; byte accounting uses the same
observation for used and reserved bytes while writers are active. This is an
allowance, not a physical-disk quota; filesystem metadata/allocation and external
preloads are separate. ENOSPC still needs handling.

`Inspect` is read-only and never launches or prepares a runtime. `Sweep` reconciles
ownership and cleans eligible state; the process owner must run it at startup,
before preparation and hourly, including when no browser jobs remain. Retired
objects move to owned trash. Source and trash parents are synced before deletion,
including successor recovery. Deletion runs outside coordination exclusion and
failed bytes stay accounted. Cleanup health is separate from executable readiness.

External protected preloads are not owned by this store and must never be sent to
its retirement paths. The integration stage provides their trusted resolution.

## Validation

Unit tests cover archive integrity/bounds, extraction types and path handling,
binding/encoding, demand/graces, pressure and uncertain-use protection, interrupted
preparation using real exiting processes, read-only status and boot provenance.
`DEM_RUNTIME_TEST_HELPER` enables the real `nd-run` persistent-receipt recovery
integration test. Run Linux tests with `-race`; tests do not simulate power loss
or establish native AppArmor/browser support.
