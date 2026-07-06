# Job Manager Architecture

This document describes the current `jobmgr` architecture for maintainers.
It is intentionally human-oriented: it explains the moving parts, command
flows, state ownership, and test model without requiring the reader to
reconstruct the system from individual review notes.

Read it top to bottom as a journey: the plain-language model first, then the
building blocks (loop, lanes, claims, effects), then how real commands flow,
and finally the precise contracts and the test model.

## What jobmgr Does

`jobmgr` owns collector jobs at runtime. It accepts configuration changes
from discovery and Dynamic Configuration (DynCfg), starts and stops jobs,
coordinates secret stores and virtual nodes, and publishes Function routes.

The central rule is simple:

> The manager loop owns orchestration state. Blocking module work runs
> outside the loop, then returns to the loop to commit.

## Job Concurrency In Plain Words

`jobmgr` is not one goroutine doing all job work, and it is not many
goroutines freely mutating job state. It is a small concurrency model. The
table below introduces the vocabulary used throughout the rest of this
document; later sections make each part precise.

| Part | Plain meaning | What it means for jobs |
| --- | --- | --- |
| Inputs | Concurrent inputs enter manager channels:<br/>• discovery configs appeared/disappeared<br/>• DynCfg user actions<br/>• effect completions<br/>• shutdown | Inputs do not freely mutate job state. The manager loop consumes them first. |
| Manager loop | One goroutine makes one orchestration decision at a time. | It chooses:<br/>• domain/key<br/>• inline vs wait vs effect<br/>• visible state commit |
| Per-key lane | One lane serializes one object. | For the same collector config:<br/>• actions do not overtake<br/>• discovery and DynCfg meet in the same lane |
| Claims | Shared dependencies are reserved before work runs. | Jobs can block across different collector keys when they share:<br/>• secret stores<br/>• vnodes<br/>Unrelated jobs can proceed concurrently. |
| Effect pool | Blocking work runs outside the manager loop. | The lane stays occupied while effects run:<br/>• validation<br/>• detection/start<br/>• stop waits<br/>• backend work |
| Running job | A committed job runs separately from command orchestration. | The collector can collect/emit independently. `jobmgr` still owns:<br/>• lifecycle<br/>• routing<br/>• dependencies<br/>• cleanup |
| Shutdown | Shutdown switches to the one-rule path. | During shutdown:<br/>• new non-terminal work does not start<br/>• unfinished DynCfg commands answer 503<br/>• unfinished work publishes no CONFIG state |

```mermaid
flowchart LR
    discovery("Discovery<br/>config appeared/disappeared") --> input("Manager input channels")
    dyncfg("DynCfg<br/>user action") --> input
    finished("Effect completion") --> input
    shutdown("Shutdown") --> input

    input --> loop("Manager loop<br/>one decision at a time")
    loop --> lane("Per-key lane<br/>same object order")
    lane --> claims("Dependency claims<br/>stores/vnodes/jobs")
    claims --> inline("Inline answer<br/>cheap or rejection-only")
    claims --> effect("Effect pool<br/>blocking work")
    effect --> loop

    loop --> commit("Commit visible state")
    commit --> running("Running job<br/>collects independently")
    commit --> output("DynCfg terminal<br/>CONFIG records")

    classDef loop fill:#cfe4ff,stroke:#1f6feb,stroke-width:3px,color:#0b2948;
    classDef input fill:#eef1f4,stroke:#8b949e,color:#24292f;
    classDef sched fill:#ece2ff,stroke:#8250df,color:#3b1f6b;
    classDef effect fill:#ffe6cc,stroke:#e36209,color:#5a2a00;
    classDef commit fill:#d8f3e0,stroke:#2da44e,color:#0b3d1f;
    classDef hard fill:#ffdcdc,stroke:#cf222e,color:#5a0b0b;

    class loop loop;
    class discovery,dyncfg,input input;
    class shutdown hard;
    class finished,effect effect;
    class lane,claims sched;
    class inline,commit,running,output commit;
```

Typical job lifecycle:

| Step | What happens | Concurrency rule |
| --- | --- | --- |
| 1 | Discovery or DynCfg introduces a config. | The input enters the manager loop through a channel. |
| 2 | The loop records the config and chooses:<br/>• start<br/>• wait<br/>• rejection | The decision is serialized by the config's lane. |
| 3 | Starting a job runs validation/detection in an effect worker. | Blocking work leaves the loop, but the lane stays occupied. |
| 4 | The effect result returns to the loop. | Only the loop commits the result. |
| 5 | The loop publishes one terminal state:<br/>• running<br/>• failed<br/>• disabled<br/>• deleted<br/>• rejected | Visible state changes happen at commit. |
| 6 | A running collector works until another lifecycle event arrives:<br/>• stop/restart/update/remove<br/>• dependency restart<br/>• shutdown | Collection is independent; lifecycle remains controlled by `jobmgr`. |

## Architecture Layers

With the plain-language model in mind, the same system in structural terms
has four layers:

1. Inputs:
   discovery add/remove, DynCfg functions, effect completions, shutdown.
2. Manager loop:
   the single goroutine that owns executor state, claim state, and commits.
3. Executor:
   per-key lanes plus a multi-key claim table.
4. Effects:
   blocking work on a bounded worker pool, with deadline abandon and late
   return handling.

```mermaid
flowchart TD
    discovery("Discovery add/remove") --> loop("Manager.run")
    dyncfg("DynCfg function") --> loop
    done("Effect completion") --> loop
    stop("Shutdown") --> loop

    loop --> exec("Executor")
    exec --> lanes("Per-key lanes")
    exec --> claims("Claim table")
    exec --> domains("Domain handlers")

    domains --> collector("Collector config/job handler")
    domains --> stores("Secretstore controller")
    domains --> vnodes("Vnode controller")

    collector --> effects("Effect pool")
    stores --> effects
    effects --> done

    collector --> commit("Loop-side commit")
    stores --> commit
    vnodes --> commit
    commit --> wire("DynCfg output and CONFIG records")
    commit --> jobs("runningJobs / deps / funcctl / fileStatus")

    classDef loop fill:#cfe4ff,stroke:#1f6feb,stroke-width:3px,color:#0b2948;
    classDef input fill:#eef1f4,stroke:#8b949e,color:#24292f;
    classDef sched fill:#ece2ff,stroke:#8250df,color:#3b1f6b;
    classDef effect fill:#ffe6cc,stroke:#e36209,color:#5a2a00;
    classDef commit fill:#d8f3e0,stroke:#2da44e,color:#0b3d1f;
    classDef hard fill:#ffdcdc,stroke:#cf222e,color:#5a0b0b;

    class loop loop;
    class discovery,dyncfg input;
    class stop hard;
    class done,effects effect;
    class exec,lanes,claims,domains,collector,stores,vnodes sched;
    class commit,wire,jobs commit;
```

## Main Objects

This is the cast of characters: the plain concepts above mapped to the code
types that implement them.

| Object | Owner | Purpose |
| --- | --- | --- |
| `Manager.run` | One goroutine | Routes every accepted event and executes commits. |
| `executor` | Manager loop | Serializes same-key work, manages effects, shutdown drain, and wedged keys. |
| `keyState` | Manager loop | State for one domain key: FIFO, wait park, busy phase, grant, wedge. |
| `claimTable` | Manager loop | Reserves cross-key dependencies before work can run. |
| `dyncfg.Handler` | Manager loop plus callbacks | Generic collector config state machine. |
| `secretsctl.Controller` | Manager loop plus effects | Secret store CRUD, validation, and dependent restarts. |
| `vnodectl.Controller` | Manager loop | Vnode CRUD and validation. |
| `runningJobs` | Locked helper | Registered runtime jobs. |
| `secretStoreDeps` | Locked helper | Mapping from stores to dependent collector configs. |
| `funcctl.Controller` | Locked helper plus reconciler | Function route publication. |
| `emissionGates` | Locked helper | Output gate used to quarantine stopping or dropped jobs. |
| `fileStatus` | Locked helper | File status persistence for dyncfg-managed jobs. |

The loop owns orchestration decisions, but several helpers are locked
because effects and auxiliary goroutines also need safe access.

## Event Domains And Keys

Every executor event has:

- kind: discovery add, discovery remove, or DynCfg command;
- domain: collector, secretstore, vnode, or unknown;
- key: the object identity inside the domain.

The lane key is domain-namespaced as `<domain>|<key>`, so collector job
keys, store keys, and vnode names cannot collide.

| Domain | Key | Examples |
| --- | --- | --- |
| Collector | Exposed config key | `mysql_local`, `go.d_job` |
| Secretstore | Store key | `vault:vault_prod` |
| Vnode | Vnode name | `db-primary` |
| Unknown | none | Rejected immediately. |

Underivable commands keep a domain fallback key and execute the existing
handler rejection path. They are rejection-only by construction and do not
claim dependencies.

## Manager Loop

`Manager.run` is the only consumer of:

- discovery add/remove channels;
- DynCfg command channel;
- effect completion channel.

It checks shutdown before every normal receive and again after the select
chooses a work item. That bounds the shutdown race to the single event
already being handled.

```mermaid
flowchart TD
    start("Loop tick") --> pre("Check manager context")
    pre -->|done| drain("Executor shutdown drain")
    pre -->|active| receive("Receive one event")
    receive --> add("Discovery add")
    receive --> remove("Discovery remove")
    receive --> fn("DynCfg command")
    receive --> result("Effect result")

    add --> activeCheck("Context still active?")
    remove --> activeCheck
    fn --> activeCheck
    result --> resultCheck("Context still active?")

    activeCheck -->|yes| dispatch("executor.dispatch")
    activeCheck -->|no| reject("drop discovery or answer DynCfg 503")
    resultCheck -->|yes| commit("executor.onEffectDone")
    resultCheck -->|no| shutdownResult("executor.onEffectDoneShutdown")

    classDef loop fill:#cfe4ff,stroke:#1f6feb,stroke-width:3px,color:#0b2948;
    classDef input fill:#eef1f4,stroke:#8b949e,color:#24292f;
    classDef sched fill:#ece2ff,stroke:#8250df,color:#3b1f6b;
    classDef effect fill:#ffe6cc,stroke:#e36209,color:#5a2a00;
    classDef commit fill:#d8f3e0,stroke:#2da44e,color:#0b3d1f;
    classDef hard fill:#ffdcdc,stroke:#cf222e,color:#5a0b0b;

    class start,pre,receive,activeCheck,resultCheck loop;
    class add,remove,fn input;
    class result effect;
    class dispatch sched;
    class commit commit;
    class drain,reject,shutdownResult hard;
```

The loop must not block on module code, external I/O, or unbounded channel
sends. Blocking work must go through a `StepRunner` and return to the loop
for commit.

## Executor Lanes

Each key has a lane. A lane may be:

- idle;
- occupied by claim acquisition;
- occupied by an effect;
- occupied by a held claim after inline work;
- wait-parked awaiting an enable/disable decision;
- wedged after a deadline abandon.

Same-key events never overtake each other. Different keys can run
concurrently unless the claim table finds a dependency conflict.

Wait-park is special: it applies to discovery events for a config awaiting
the user's enable/disable decision. DynCfg commands against the same key do
not wait-park; they execute and answer the state machine's current outcome.

Effect completion, deadline abandon, and late-return ordering are planned in
`executor_transition.go` and pinned by `executor_transition_test.go`. The
diagram below is the conceptual lane shape, not the policy table.

```mermaid
stateDiagram-v2
    [*] --> Idle
    Idle --> AcquiringClaims: mutating event
    Idle --> InlineCommit: claimless event
    Idle --> WaitParked: discovered config awaiting decision
    WaitParked --> AcquiringClaims: enable or disable decision
    AcquiringClaims --> BusyEffect: grant plus blocking phase
    AcquiringClaims --> InlineCommit: grant plus inline command
    BusyEffect --> Commit: effect completion
    BusyEffect --> Wedged: deadline abandon
    Wedged --> Commit: late return
    InlineCommit --> Idle
    Commit --> Idle

    classDef input fill:#eef1f4,stroke:#8b949e,color:#24292f;
    classDef sched fill:#ece2ff,stroke:#8250df,color:#3b1f6b;
    classDef effect fill:#ffe6cc,stroke:#e36209,color:#5a2a00;
    classDef commit fill:#d8f3e0,stroke:#2da44e,color:#0b3d1f;
    classDef hard fill:#ffdcdc,stroke:#cf222e,color:#5a0b0b;

    class Idle input
    class AcquiringClaims,WaitParked sched
    class BusyEffect effect
    class Wedged hard
    class InlineCommit,Commit commit
```

## Claim Table

Claims reserve cross-key dependencies before work runs.

| Command class | Claims |
| --- | --- |
| Collector mutation | Own collector key as write; referenced stores as read; referenced vnode as read. |
| Collector update/replace | Union of old and new store/vnode references. |
| Collector disable/remove | Old store/vnode references from the exposed config. |
| Secretstore mutation | Store key as write; restartable dependent jobs as write. |
| Secretstore test | Store key as read. |
| Vnode mutation | Vnode name as write. |
| Rejection-only command | No claims. |

Claim rules:

- reads share;
- writes exclude;
- acquisition walks one global lexicographic order, including the primary;
- a request holds the acquired prefix and parks at the first blocked key;
- per-key waiter FIFO prevents barging;
- claim sets recompute at every restage;
- if the recomputed set changes, the old prefix is released and the new set
  is acquired from the start;
- claim waiters are pumped before lane release hooks can settle parked lane
  events.

This is the deadlock-avoidance core: no command waits for a later key while
holding keys in an order that another command can invert.

## Effects, Deadlines, And Wedges

Blocking work runs as an effect with the manager context plus a flat
deadline. Blocking work includes:

- collector validation and detection;
- job stop waits;
- secretstore backend validation and activation;
- dependent restarts.

If an effect returns before the deadline, the result is committed on the
loop. If the deadline fires first, the worker commits the deadline outcome
and the leaked call keeps running in the background. The key is then wedged
until the leaked call returns.

`executor_transition.go` is the loop-side owner for completion, abandon, late
return, warm continuation, late replay, and shutdown-late ordering. The prose
below records the cross-file invariants that stay with the manager, effect
worker, claim table, and domain controllers.

While wedged:

- the lane remains busy;
- same-key events park;
- read claims release at the abandon commit;
- write claims remain held until late return;
- claim waiters at the wedged key are re-attempted so commands that can
  skip wedged keys do not wait for an unbounded leaked call.

At late return:

- late replay work runs before remaining write claims release;
- a warm start resumes only if the config is still current, no stop intent
  is queued, the manager is not shutting down, and referenced stores are
  unchanged/not write-held;
- dropped warm starts dispose silently behind a closed emission gate;
- shutdown late returns only release state and publish nothing.

```mermaid
sequenceDiagram
    participant L as Manager loop
    participant E as Effect worker
    participant M as Module call

    L->>E: dispatch effect
    E->>M: run blocking work
    alt returns before deadline
        rect rgba(46, 160, 67, 0.15)
        M-->>E: result
        E-->>L: normal completion
        L->>L: commit, release claims, settle lane
        end
    else deadline fires first
        rect rgba(248, 81, 73, 0.15)
        E-->>L: abandoned result
        L->>L: abandon transition, wedge key
        M-->>E: late result
        E-->>L: late completion
        L->>L: late-return transition, release writes, settle lane
        end
    end
```

## Domain Flows

### Collector Configs And Jobs

Collector commands use the generic DynCfg handler plus jobmgr callbacks.

| Command | Main flow |
| --- | --- |
| `add` | Validate payload, expose config, optionally replace old job, publish create/status. |
| `enable` | Start an accepted/failed/disabled config and publish running or failed. |
| `disable` | Stage stop, wait in effect, publish disabled. |
| `remove` | Stage stop if needed, remove exposed/seen state, publish delete. |
| `update` | For same-source update: stop old job, start replacement. For conversion: activate dyncfg config over file/user config. |
| `restart` | Stop then start the same config. |
| `get` / `schema` / `userconfig` | Read-only or cheap response paths. |
| `test` | Keyless interactive validation on its own bounded pool. |

Collector mutations claim their own key and referenced store/vnode keys.
Update-shaped commands claim both old and new references.

Function routing follows committed state:

- stop withdrawal happens at stage time, before the stop effect reaches a
  worker;
- start publication happens at commit time, when the config is running.

### Discovery

Discovery does not mutate manager state directly. It sends add/remove
intents to the loop.

Discovery add:

1. Validate identity.
2. Remember the discovered config.
3. If replacing an exposed config, stage the old stop.
4. Publish create/status.
5. If auto-enable applies, chain an enable command through the lane.
6. Otherwise wait-park the key for an enable/disable decision.

Discovery remove:

1. Stage stop if a matching exposed job exists.
2. Remove active job state and exposed config at commit.
3. Publish delete.

Discovery replace/remove stops are final-phase staged stops. Their stop wait
claims completion before disarming the deadline fence, so a completed stop
cannot be misclassified as an unfenced timeout.

### Secretstores

Secretstore commands run through `secretsctl.Controller.StepExec`.

| Command | Main flow |
| --- | --- |
| `add` | Validate/activate store in effect, publish store config, restart dependents if needed. |
| `update` | Validate/activate replacement, restart dependents. |
| file/user conversion `update` | Activate dyncfg override in place, then restart dependents. |
| `remove` | Reject if referenced; otherwise remove store. |
| `test` | Validate candidate or stored config; read claim only. |
| `get` / `schema` / `userconfig` | Cheap response paths. |

Dependent restarts are one multi-key effect:

1. The loop snapshots the restart plan after the store command has its
   grant.
2. The effect restarts dependents sequentially.
3. Each completed dependent restart buffers a loop-side CONFIG STATUS replay.
4. The store command flushes completed replay work before its terminal
   response at normal commit or deadline-abandon commit.
5. Restarts that finish only after a deadline abandon are replayed at the
   late return, before the remaining write claims release.
6. Wedged dependents are excluded from the claim set and reported as
   skipped.

The terminal message belongs to the command's own effect context, so
overlapping store commands cannot cross-attribute messages.

### Vnodes

Vnode commands are loop-synchronous stage+commit operations under a vnode
write claim.

| Command | Main flow |
| --- | --- |
| `add` | Validate name, payload, GUID, and uniqueness; commit a versioned vnode snapshot. |
| `update` | Validate payload/GUID/uniqueness; commit a new versioned vnode snapshot. |
| `remove` | Reject if missing, non-dyncfg, or referenced; otherwise remove and publish delete. |
| `test` | Validate candidate inline, no claim. |
| `get` / `schema` / `userconfig` | Cheap response paths. |

The vnode store is the authoritative config source. Lookups return cloned
snapshots with:

- a store revision that advances on every committed vnode config write;
- a metadata revision that advances only when runtime-visible vnode metadata
  changes.

Jobs bind to an explicit vnode name and consume snapshots from the store:

- job creation gets the current snapshot from the factory;
- job registration reconciles the current snapshot before `Start`;
- V1 jobs refresh after collection and before emission;
- V2 jobs refresh before collection and emission;
- cleanup never re-reads the live vnode store:
  - V1 cleanup uses the job's committed local snapshot;
  - V2 module-owned cleanup samples `VirtualNode()` before module cleanup;
  - V2 jobmgr-owned cleanup uses the last successfully emitted HOST_DEFINE
    cleanup info for owner and stale-suppression metadata.

Runtime-equivalent vnode commits are still consumed by revision, but do not
force redundant HOSTINFO/HOST_DEFINE output. Module-owned vnodes keep their
runtime-specific precedence and are not overwritten by jobmgr snapshots.

Collector commands that reference a vnode hold a read claim on the vnode.
That prevents vnode removal/update from racing a collector stop/start window.

Every job registration reconciles the job's vnode baseline before `Start`.
This covers dependent restarts and warm resumes that were created before a
vnode update but registered after it.

## Command Planning And Rejection-Only Commands

Every command is planned before it runs; the plan decides whether it reserves
claims. There are three plan classes:

| Plan class | Claims | Behavior |
| --- | --- | --- |
| Claimless | none | Answers inline, no claim-table serialization (rejection-only and read-only). |
| Hold-aware claimless | none, normally | Answers inline, but parks behind a foreign write hold on its key. |
| Claimed | full set | Acquires its claims before the first claim-protected access. |

**Rejection-only** is the common claimless case — a command that answers before
its first claim-protected access, so it claims nothing and never parks behind a
foreign write hold. Examples: invalid identity; unknown or unsupported command;
missing object or payload; most source/type gates; parse gates before state
access.

**The one exception is status-derived collector gates.** Enable, restart, and
disable can answer from `Entry.Status`, which secretstore dependent-restart
plans mutate — so they are *hold-aware*: while the collector key is
foreign-write-held they park and answer after the hold resolves.

Ownership:

- each domain owns its plan — `dyncfg.Handler`, `secretsctl.Controller`, and
  `vnodectl.Controller` each expose `CommandPlan`;
- the executor wraps that plan in an event plan and owns claim-key computation:
  `NeedsClaims(false)` drives intrinsic acquisition, `NeedsClaims(true)` drives
  the foreign-write-hold bypass, and computation re-runs at every restage.

Claim computation is dynamic: a parked command can become claimless or change
its dependencies before it acts.

## Shutdown

Shutdown has one rule:

> Every non-terminal DynCfg command answers 503, publishes nothing, and
> disposes everything.

Shutdown drain handles all places work can be stuck:

- commands still in `dyncfgCh`;
- pending effects that were not picked by a worker;
- effect tasks sitting in the worker channel;
- claim-parked commands;
- in-flight effects that finish inside the bounded drain window;
- still-busy keys after the drain window expires;
- lane FIFO and wait FIFO entries.

Late completions after the drain window are dropped through `lateDrop`.
They must not publish state.

## Function Publication

Function routing follows committed state, asynchronously — separate from job
start/stop mechanics:

- stop withdrawal happens when a stop stages;
- start publication happens when a running status commits;
- a reconciler goroutine performs the actual publish outside the manager loop;
- the manager loop may request reconciliation, but must not publish directly.

This keeps routing aligned to committed state without blocking the loop on
publication work.

## Output Ordering Rules

Important ordering contracts:

- Collector shared-handler commands emit the terminal result before their
  same-command CONFIG records.
- Secretstore dependent restart CONFIG STATUS records that completed by the
  command commit are replayed before the store command terminal. Restarts
  completing after deadline abandon are replayed at the late return, before
  the remaining write claims release.
- Vnode remove emits CONFIG delete before its terminal.
- Shutdown publishes no CONFIG records for non-terminal work.
- Deadline abandon keeps the permanent deadline classification, not the
  shutdown one-rule.

These contracts are pinned by characterization and flow tests.

## Testing Model

The test suite should be read as a set of matrices, not as isolated tests.

### Matrix Axes

| Axis | Values to cover |
| --- | --- |
| Domain | collector, secretstore, vnode |
| Object state | missing, accepted, running, failed, disabled, dyncfg, file/user, stock/internal |
| Command class | read-only, rejection-only, stage+commit, effect, chained effect, keyless test |
| Dependencies | no refs, store refs, vnode refs, dependent jobs, busy dependent, wedged dependent |
| Ordering | same-key FIFO, wait-parked discovery, foreign write hold, read/read sharing, read/write conflict |
| Failure boundary | validation failure, start failure, stop timeout, effect deadline, late return, shutdown |
| Expected result | terminal code, CONFIG records, claim behavior, publication timing, retry behavior |

Do not try to test the full Cartesian product. The useful target is
equivalence-class coverage: one test per behavior boundary, plus parity
tests that fail when a command gate moves without updating the command plan.

### Existing Coverage Anchors

| Area | Primary tests |
| --- | --- |
| Claim table ordering and fairness | `claims_test.go` |
| Per-key dispatch, derivation, keyless collector test | `executor_test.go` |
| Generic DynCfg command state machine | `plugin/framework/dyncfg/handler_test.go` |
| Collector callbacks and command basics | `dyncfg_collector_test.go`, `manager_test.go` |
| Discovery wait parking and wire order | `characterization_test.go` |
| Secretstore flow and conversion | `secretstore_flow_test.go` |
| Secretstore effects and dependent restart edge cases | `secretstore_effect_test.go`, `effect_deadline_test.go` |
| Vnode and cross-domain claim interactions | `vnode_claims_test.go`, `dyncfg_vnode_test.go` |
| Deadline, wedge, shutdown one-rule | `effect_deadline_test.go`, `effect_test.go`, `executor_test.go`, `executor_transition_test.go` |
| Function publication timing | `manager_process_test.go`, `funcdispatch_test.go`, `funcctl/*_test.go` |
| Command-plan parity | `handler_test.go`, `secretsctl/commandplan_test.go`, `vnodectl/commandplan_test.go`, `executor_test.go` |

### Known Test-Gap Classes

These are not known runtime defects. They are places where future test
hardening should focus.

| Gap class | Current state | Suggested next test shape |
| --- | --- | --- |
| Human-readable matrix | Coverage exists but is spread across many files. | Keep this section current when adding a command or state. |
| Unsupported/inline command parity | End-to-end rejection pins exist for representative unsupported store/vnode commands; per-domain parity tables are not a full unsupported-command census. | Extend parity tables when command support changes, especially for unsupported commands that should remain claimless. |
| Pairwise cross-domain interleavings | Representative read/write conflicts are pinned; every command pair is not enumerated. | Add pairwise tests only when a new claim mode or new cross-key writer is introduced. |
| Shutdown matrix by every command | Shutdown one-rule is pinned at the main chokepoints and representative commands. | Add command-specific shutdown rows only when a command adds a new effect phase or publication path. |

When adding tests, prefer:

- table-driven gate parity tests for deterministic command plans;
- property tests for claim-table ordering rules;
- end-to-end choreography only for cross-domain ordering or publication
  outcomes that cannot be proven at the plan/unit layer.
