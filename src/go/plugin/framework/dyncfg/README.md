# Dynamic configuration commands

`Handler` implements the shared ADD, ENABLE, DISABLE, REMOVE and UPDATE state
machine. Service discovery supplies structural parsing and runtime ownership via
`Callbacks`. Read-only commands continue through the ordinary Function registry.

## Acceptance and activation

| Command | Preparation | Successful adoption |
|---|---|---|
| ADD | Structural validation, including the raw requested name | 202, passive Accepted configuration |
| ENABLE | Existing configuration lookup | 202, enabled Accepted intent; construction follows publication |
| UPDATE of enabled configuration | Structural validation and component preflight | 202, enabled Accepted replacement |
| UPDATE of disabled configuration | Structural validation | 200, disabled replacement |
| UPDATE with identical content of a running DynCfg configuration | Structural validation | 200, unchanged running configuration |
| DISABLE / REMOVE | Existing configuration lookup and command policy | 200, logical revocation |

ENABLE of an already running configuration returns 200. ENABLE of enabled
Accepted intent returns 202 without restarting it. Passive Accepted configurations
must receive an ENABLE or DISABLE decision before UPDATE. Converting a file
configuration to DynCfg uses the same UPDATE preparation and ownership boundary;
identical content does not bypass conversion.

Accepted intent does not establish runtime health. `Entry.Enabled` distinguishes
accepted enabled work from passive ADD. Runtime status can become Running or
Failed only after acceptance has been published.

## Prepared command ownership

Managed callers register a `CommandPreparer` through `PreparedRegistry` and use
these phases:

1. `Prepare` performs rejection-capable work against an immutable exposed entry.
   `ParseAndValidate` is structural; only enabled UPDATE calls `PrepareUpdate`
   to materialize a candidate. Preparation cannot change caches, stop an
   incumbent or publish frames. It may run concurrently with the state owner.
2. `Apply` runs on the component's serialized state owner. It checks cancellation
   before entering adoption and checks the exact exposed predecessor snapshot.
   A changed predecessor returns 409 without adopting the candidate. Successful
   adoption updates desired state and revokes prior output authority without
   waiting for physical cleanup.
3. The caller publishes the returned `Result` and `Notifications`, in that order,
   before invoking `Published` once. The component must keep state publication
   serialized across this interval so activation health cannot precede Accepted.
   A failed publication must fail the component closed without invoking the
   activation continuation.

The caller must apply or dispose a prepared command exactly once. Canceled Apply,
stale predecessors and explicit Dispose release unaccepted preflight resources.
`PreparedActivation.Accept` transfers ownership only on success. An error must
leave the incumbent unchanged and the candidate available for Dispose. Successful
acceptance transfers cleanup responsibility to the component, including when
later activation or publication fails. `Enable` follows the same no-mutation-on-error
rule. Component callbacks own actual physical lifetimes beyond acknowledgment.

`ParseAndValidate` and `PrepareUpdate` turn callback errors into ordinary DynCfg
rejection results, honoring valid `CodedError` overrides. `Enable` and
`PreparedActivation.Accept` perform adoption only. Their errors propagate
through `Apply` to the managed caller as ownership failures for fail-closed
handling; they do not produce ordinary DynCfg rejection results.

The direct `Cmd*` wrappers are synchronous conveniences for standalone callers
and tests. They publish through `Output`, which cannot report write failures, and
then invoke `Published`; they do not provide the managed fail-closed publication
guarantee. Raw `Prepare` or `Apply` errors produce a 500 reply in these wrappers.
Production managed Function calls, including daemon echoes, use the
prepared boundary so the composition layer owns framing and failure.

## Cache and status ownership

Exposed entries and their configurations are immutable after insertion. Writers
replace snapshots; readers and concurrent preparations may retain the old pointer.
Cache locks protect lookup and replacement, not whole command transactions. All
Apply calls, discovered configuration mutations and status updates must therefore
share the component's serialized state owner.

`SetStatus` checks configuration UID and content hash and returns true only for
an actual status change. Duplicate events retain the current snapshot. The component must first
fence the event using its exact runtime generation: configuration identity alone
cannot distinguish a restarted runtime with the same input. Stale generations
must not update the exposed status or forward discovered output.

The seen cache retains underlying file configurations when a DynCfg replacement
is adopted, preserving later source selection. Rejected preparation leaves both
the exact exposed entry and seen-cache contents unchanged.

Accepted ADD transfers a pending ENABLE/DISABLE decision to the replacement
configuration, including when replay changes a file-origin key to a DynCfg key.
Removing that replacement releases the wait. Unrelated commands and rejected
preparation do not change the pending decision.
