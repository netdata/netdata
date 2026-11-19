# DEM process runtime owner

`Open` turns the administrator's `runtime:` configuration (`Config`) into the process `Handle`. Setup problems never
fail `Open`: the handle's source reports them to browser jobs while RUM keeps working. Store internals (trust,
protection, repair, preloads, native packages) belong to [`browserruntime`](../browserruntime/README.md); the
cross-package rules are in [ARCHITECTURE](../../ARCHITECTURE.md#browser-runtime-ownership).

## Modes

The mode decides who owns the runtime files. Packaging sets `DefaultMode` to `native` for DEB/RPM builds.

| Mode | Files owned by | Runner source | Pair update owner |
|---|---|---|---|
| `managed` | The service account, in its private runtime store | `Host` | Agent installer or explicit deployment helper |
| `native` | The package manager (`netdata-dem-runtime`) | `Host` | Package manager |
| `preloaded` | The administrator (a sealed preload) | A borrowed preload source, outside managed retention | Deployment owner |

| File | Contents |
|---|---|
| `config.go` | `Config`, modes, validation, release loading, store configuration |
| `open.go` | `Open` and the process `Handle` |
| `host.go` | `Host`, the runner source of managed and native modes |
| `source.go` | The borrowed-claim, preload and unavailable sources and the execution-use adapter |
| `health.go` | Maintenance and execution-completion health logging |

## Fixed startup selection

- **Atomic acquisition.** `Host` acquires the explicitly installed selection and its process protection atomically at
  construction. That immutable release is used for the entire plugin process.
- **No per-monitor acquisition.** Enabling, disabling, updating or removing a monitor never downloads or prepares
  runtime files. There is no per-monitor resource demand or first-use download.
- **Restart to change.** An installation made after an empty process start becomes available after restarting the
  plugin; replacing an installation does not change an existing process's running release.
- **Readiness.** `Ready` and `Dependencies` are pure memory lookups. Installation changes do not publish readiness
  changes to the fixed process selection. Missing setup leaves browser jobs waiting with installation and restart
  guidance while RUM continues.
- **Uses.** Every runner probe and invocation obtains `BeginUse` from the held process consumer, passes its completion
  handle to supervision, and finishes with actual drainage proof. A failed execution admission can retry against the
  same held release without permanently poisoning readiness.
- **Reporting.** `Handle.Identity` reports the release the process runs, and `Handle.NativeStatus` the native package's
  ownership proof, for the runtime CLI.

## Maintenance and errors

- **Sweeps.** `Run` sweeps storage at startup and hourly, even with no browser jobs. Selected, prepared and previous
  pair references and process/execution protection govern retention. It performs no release discovery or downloads.
- **Separate errors.** A setup error keeps `Ready` failing, while cleanup and execution-completion failures are logged
  with their recovery. They do not invalidate a usable runtime or alter a completed website measurement.
- **Sticky errors.** Each maintenance operation retains its error until its own successful retry; a healthy invocation
  cannot erase an earlier execution-retirement failure.

## Pair preparation and recovery

The runtime host holds its startup selection and performs storage maintenance. It has no background release updater
or download schedule. Initial browser installation remains explicit; an Agent without an enrolled runtime acquires no
browser through ordinary startup or monitor changes.

The Agent's installer or deployment owner acquires its exact signed companion. Managed `prepare` retains a candidate
without selecting it; the owner stops consumers, replaces the Agent payload and calls `activate` before restarting.
One recovery selection remains available for explicit `rollback`, which does not execute the old runtime or restore
Agent files. Beginning B→C retains B and may retire the older A pair, even when C fails. An unchanged B preparation
verifies the payload and preserves A without creating a transaction; the owner can still call `activate` to verify the
selected pair. Existing pending transitions are reconciled rather than treated as unchanged. The owner repairs or
restores the matching Agent payload or image through its normal installation workflow. The compatibility `runtime update`
command provides this owner guidance without changing runtime storage.

The persisted `automatic_updates` bit remains acquisition consent for that installation owner. It grants no runtime
process permission to discover or advance a release independently. Manual installations require explicit matching
inputs or acquisition authority. Native package operations and protected preload deployments retain their own owners.

Structural metadata stays readable across execution-contract changes. A healthy incompatible selection remains
inventory and recovery evidence. Execution requires a compatible contract, matching architecture and intact payload,
including its own release provenance. It does not compare the plugin’s version/channel/source stamp; ordinary
development builds follow the same checks. Exact release pairing belongs to runtime acquisition and published
Agent updates. Unstamped source updates retain and validate the existing runtime through the installation owner;
they do not acquire a companion or relax execution checks.
See [installation and recovery](../browserruntime/README.md#installation-update-and-repair) for the store primitives.

## Shutdown

- `Close` follows joined jobs, joins the maintenance worker and releases process protection. The handle then closes
  the store.
- Uncertain executions remain protected by their independent completion receipts. Unknown child drainage reaches the
  existing process fail-stop path and keeps its receipts.
