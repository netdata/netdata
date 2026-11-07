# Parent-owned collector jobs

`ownedjobs` is the configuration-owner capability for a component that manages several native collector jobs.
It gives the parent one desired child set while retaining native per-job construction, activation, retries,
dependencies, metrics and cleanup. It does not provide a parent configuration format, persistence, a scheduler or
a DynCfg endpoint.

## Composition and authority

Production composition declares `OwnerOnlyModules` and supplies `OwnedJobServices`. Each service receives a fresh
`Registry` for its Agent run. A service MUST return when its run context is canceled. An owner obtained from a
previous run cannot adopt work in a subsequent run.

The parent binds a stable identity through `Registry.Bind`. The returned `Owner` is an in-process capability;
configuration fields cannot manufacture it. Two live parents cannot bind the same identity or own the same native
job identity. A retired parent can be recreated under the same name, but receives a different lifetime.

Only modules explicitly declared as owner-only accept parent-created jobs. Such modules do not advertise standalone
DynCfg templates, and their jobs reject direct add, update, enable, disable, remove and restart commands. This applies
before a child appears in the native graph and after removal, including daemon echoes and ordinary discovery input.
Other modules retain their existing discovery, configuration and lifecycle behavior.

Composition excludes owner-only modules before standalone inventory records source membership. Withdrawing an ignored
source therefore cannot leave native candidate state that prevents a later parent from using the child identity.

Child edits belong to the parent's canonical configuration. A source group or a label associating jobs with a
parent is not an alternative to this authority boundary.

## Applying a desired child set

1. Bind an owner for the parent lifetime.
2. Call `Prepare` with an increasing parent revision and the complete desired `[]Child`.
3. Apply the returned immutable, single-use `Snapshot`.
4. Read `Status` to observe each child's independent realization.
5. Call `Retire` when the parent lifetime ends.

Preparation clones configurations and validates identities, module policy and collisions before any adoption.
Application checks the parent lifetime and predecessor revision again. `errors.Is(err, ownedjobs.ErrStale)` means
the owner or snapshot is no longer current; the candidate was not adopted. Context cancellation and other errors
must retain their own meaning rather than being treated as stale-edit rejection.

`Apply` waits for the parent adoption command to settle, but can return context cancellation after the desired set was
adopted. The parent MUST coordinate its canonical document with adoption instead of treating every error as rejection.
`Status` is a child realization projection: an empty desired set has no rows and supplies no parent adoption receipt.

A child's stable `Key` is local to its parent. `Config` contains its native module/name and collector configuration;
the native job identity must also be unique. `Enabled: false` retains a desired child without running it. Omitting
a previously present child removes it.

Changed children lose new-work admission before their individual native transitions can block on publication or
predecessor release. Physical cleanup stays with the existing native runtime owner. A successor cannot run before
its predecessor's release is proven. Unchanged children preserve their desired identity, runtime and cadence.

The parent revision is not inserted into every child's effective configuration. Updating one child therefore does
not replace its siblings. A full desired set can exceed the native graph's per-transaction change limit: the parent
adopts desired authority once, then each affected job realizes that intent through its own native transaction.
Acceptance does not promise simultaneous readiness of all children.

## Source provenance and secrets

This capability accepts trusted parent origins: `confgroup.TypeUser`, `TypeStock` and `TypeDyncfg`. It rejects
discovered or unknown origins. Callers MUST NOT relabel untrusted discovery data to bypass that boundary.

Generated native configurations use stable user-trusted provenance, attributed to their owner. The current submitted
parent source type is reported separately by `State.SourceType`. Moving a parent from a file to DynCfg does not
replace unchanged child runtimes solely because its source metadata changed.

Ordinary native secret resolution still runs during child activation. Parent-owned configuration does not carry
resolved credentials into status and does not bypass the configured secret providers or dependency lifecycle.

## Reading status

`Status` returns credential-free state for the current desired children:

| Field | Meaning |
|---|---|
| `Key`, `JobID` | Parent-local child key and native job identity |
| `Revision` | Accepted parent document revision |
| `Desired` | Identity of this child's desired intent; unchanged siblings retain it |
| `Applied` | Child intent applied by native reconciliation |
| `Enabled` | Desired enabled state |
| `SourceType` | Current submitted parent source type |
| `ConfigStatus` | Native configuration status for the applied desired intent |
| `RuntimeStatus`, `Generation` | Current native runtime projection, when a runtime exists |

Desired/applied identity and runtime health answer different questions. A saved parent may contain an activating,
disabled or failed child alongside running siblings. Execution results, freshness and retained-history health belong
to the consumer's observations; this API does not infer them from native readiness.

## Cost and lifetime

Preparation and comparison scale with the submitted configurations. Adoption examines the desired set and revokes
only changed children; native realization is per changed job. Reading status visits the current child set and sorts
by key. It does not return scripts or credential values.

Reservations cover current and retiring child authority. After an owner-only child is removed through native
reconciliation, its identity can be reused by another parent. The module policy continues to reject standalone
replay without retaining every historical child ID for the lifetime of the plugin.

The capability is one level of parent-to-native-jobs ownership. It does not add nested parent graphs, independent
child persistence, fleet ownership or a second runtime cleanup mechanism.
