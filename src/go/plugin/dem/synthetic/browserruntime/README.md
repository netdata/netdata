# Browser runtime installation and lifetime

This package verifies release descriptors and archives, prepares private executable paths, and coordinates installation,
process ownership and execution lifetime. It does not install host packages, invoke npm or choose component versions.
Release inputs and adapters live in `src/dem-runtime` and `packaging/dem-runtime`. `synthetic/runtimehost` chooses the
key source and adapts this package to the runner and job framework; `cmd/demplugin` is the administrator CLI.

| Files | Contract |
|---|---|
| `manifest.go`, `release.go`, `release_trust.go` | Manifest validation, URL and platform rules, signed releases, keyrings |
| `download.go`, `archive.go`, `payload.go` | HTTPS acquisition, digest and extraction bounds, interpreter binding and bound-payload checks |
| `durable.go`, `root*.go`, `boot*.go` | Record writes and lock files, root bootstrap and permissions, boot identity |
| `ownership_ledger*.go` | Claims, execution receipts, proof inventory and trash |
| `store.go`, `inventory.go`, `retention.go` | Managed store, generations, damage classification, status and retirement |
| `selection.go`, `installation.go`, `prepare.go`, `preload.go` | Selection record and consumers, candidates and published installation, preparation, sealed preloads |
| `recovery.go`, `inputs.go`, `pairing.go` | Prepared/previous pair references, authenticated retained inputs and payload Agent identity |
| `native*.go` | Native package preparation, root admission control and service-UID consumers |

## Trust and preparation

A publisher-authenticated format-2 descriptor identifies its exact Agent version, channel and source commit, plus execution contract, platform, archive
SHA256/size, tar/expanded bounds, entry count, layout and authenticated interpreter slots. The archive is not its own
trust anchor. `FetchRelease` (online discovery) and `ReadRelease` (offline descriptor) both end in `VerifyRelease`;
which keyring they use is the caller's policy. Legacy format-1 descriptors remain readable for inspection and recovery.
The persisted `Manifest` shape stays unchanged; format 2 carries the Agent tuple in the enclosing release.

`Manifest.ValidateMetadata` and `Release.ValidateMetadata` check structural identity and preparation instructions
without requiring this Agent's execution contract. Persisted selections, retained generations and native admission
records remain readable across contract changes. `Manifest.Validate` additionally checks execution eligibility and
returns `ErrIncompatible` for healthy metadata naming another contract. Acquisition and execution still require that
strict check; incompatibility does not justify repairing or deleting ownership evidence. Execution does not compare
the caller’s Agent version/channel/source: it validates the payload’s own provenance and, for managed paired releases,
its binding to the authenticated descriptor. Different or empty plugin release stamps use the same execution checks.
`Store.ValidateRelease` and native/preload installation validators additionally require the exact target Agent pair.

### Signed release descriptors

- **Envelope.** `VerifyRelease` authenticates version 1 of the release envelope. It contains the complete base64-encoded
  release JSON and one detached binary OpenPGP signature over the domain `netdata-dem-runtime-release-v1` followed by a
  NUL and the exact JSON bytes. The release tool uses the existing package GPG signer.
- **Verification.** Verification runs inside Go and requires SHA-256 or stronger signatures from a currently valid,
  unrevoked signing key. The envelope cannot supply keys or a key-discovery URL. `VerifyRelease` returns a
  `VerifiedRelease`, the only release type that `BeginPublishedInstall` and `BeginPublishedEnsure` accept; its zero
  value is refused.
- **What is signed.** The signed release includes the entire `Manifest`, including layout, extraction bounds and
  interpreter-binding instructions, and the source archive's HTTPS URL, digest and size. Signing only the executable
  archive digest would not authenticate the instructions used to prepare it.
- **Limits.** Publisher and verifier enforce a 1 MiB envelope limit; descriptors carry metadata and binding slots, not
  source bodies or complete file inventories. Public keyrings are limited to 1 MiB and detached signatures to 64 KiB
  before parsing. Unknown fields and trailing JSON are rejected.
- **Agent pair.** Format 2 requires a validated Agent tuple, the same manifest version, and no publication sequence.
  `ValidateForAgent` requires the exact expected tuple and execution contract. Reusing one tuple for different release
  content is rejected; identical content is a no-op. Agent release/update owners decide ordering and explicit downgrades.
- **Legacy sequence.** Format 1 retains its positive sequence and no Agent tuple. Its old sequence checks remain for
  inspecting/recovering legacy installations; normal paired updates cannot switch back to independent releases.
- **What it does not prove.** These primitives do not detect withheld updates or guarantee freshness for a fresh
  installation. Signature validity also does not establish platform qualification or actual source availability. Key changes
  do not revoke already installed runtime execution.
- **Online keys.** Online trust is fetched from the canonical package-key HTTPS endpoint for each preparation attempt.
  Endpoint control and system TLS trust authorize key replacement; fetching the key does not establish freshness or
  immediate revocation through caches.

### Archive preparation

- **Decompression.** Archives use zstd only. The existing Go decoder limits the frame window and its decoding buffers to
  8 MiB with one decoder worker; this is not a total process-memory limit. No system `zstd` executable is required.
  Compressed size/digest verification precedes extraction, and the signed tar, regular-file and entry bounds still apply
  through the complete decoded stream.
- **Extraction.** Extraction rejects escaping paths, duplicate entries, privileged modes, devices, hard links and
  symlink parents.
- **Binding.** Preparation binds only authenticated interpreter slots to the final generation, generates the private
  browser wrapper/font configuration and verifies the bound layout: plain parents, the entrypoint, and owner-executable
  executables in the preparing account's staging. Retained and preloaded generations also compare the authenticated
  interpreter slots and generated configuration with their expected bound bytes, using bounded reads. Ensure checks
  a selected generation the same way. Native admission
  applies the same layout check but requires a group or other execute bit: root owns package files and the non-root
  service runs them.
- **Publishing.** Preparation syncs staging before publishing. A prepared generation must never be relocated.
- **Workspace.** The store reserves archive plus expanded workspace before opening the source.
- **Downloads.** Explicit downloads use HTTPS with a 60-second network inactivity timeout; continuing network progress
  has no total deadline. Local writes suspend the inactivity timer.
- **Online and offline keys.** Online acquisition fetches a bounded public keyring once from
  `https://repository.netdata.cloud/netdatabot.gpg.key` under the same installation exclusion and consent checks. Failed
  refresh never falls back to local or cached keys. Offline import instead requires an explicit
  administrator-provisioned OpenPGP keyring and performs no network acquisition; a local descriptor combined with
  `--allow-download` remains an online operation. Both routes verify identical archive bounds and digests.
- **Where keys are not used.** Key lookup is not part of execution, status or native package operation, and an update
  lookup failure preserves the installed selection.
- **Qualification.** Ordinary CLI install, update, ensure, repair and preload verify and prepare artifacts without
  executing Node or a browser. `runtime qualify` is an optional diagnostic against the installed release; it runs real
  contract checks, a sandboxed journey and Lighthouse, and reports the caller's account, environment and drainage.
  Its failure does not undo installation. The runtime process has no background release updater or download schedule.

## Ownership

The root is service-owned, mode 0700, on one host with working file locks, rename and directory fsync. Do not copy it to
another host or use network filesystems. This is not isolation against malicious code running as the same service
account.

| Protection | Lifetime |
|---|---|
| Selected installation | Durable until explicit uninstall or replacement selection; survives idle periods and process death |
| Prepared/previous pair | Durable recovery references retained through owner activation or explicit recovery |
| Process consumer | One release acquired atomically at startup; old processes retain it after updates |
| Candidate | Serialized installation transaction, from preparation through validation and selection/abort |
| Execution receipt | Every executable access; only authoritative drainage, helper receipt or verified reboot permits release |

- **Installation and selection.** `BeginPublishedInstall` holds a separate installation lock while
  preparation and any caller-owned checks run outside inventory coordination. `Candidate.Select` publishes the verified
  release without requiring a browser smoke test. Pair owners use `PreparePair` instead: preparation leaves selection
  unchanged until the owner has installed the matching Agent and drained consumers.
  `AcquireSelected` reads that selection and creates its process claim under the same coordination lock. There is no
  per-monitor claim or automatic download. A missing startup selection requires a restart after installation.
- **Uses.** Each consumer/candidate `BeginUse` publishes an execution record before returning paths. Pass its
  `Completion` to exactly one supervised invocation, join the helper, then `Finish`. Version probes follow the same
  rule. An owner that never launched can report verified-empty completion; recovery cannot infer this from owner death.
  Closing a consumer blocks future uses but does not manufacture child completion.
- **Selection durability.** Selection uses synced temporary metadata, rename and a parent-directory sync. Failure after
  rename is explicitly uncertain, not rollback. Before executable admission, another mutation or retirement, the store
  syncs and reads the visible selection under coordination. A failing durability barrier blocks those actions; a
  successful one reconciles the authoritative state, including after restart.
- **Reboot recovery.** Reboot recovery requires stored and current boot identities with verified kernel provenance.
  Container replacements and bind mounts of the per-read `uuid` source are rejected. Missing provenance disables reboot
  recovery, not ordinary receipts.

## Store initialization

- **Initialization.** A fresh or empty store publishes an initialization record, completes its private layout under
  coordination, then durably marks itself ready before admission. Interrupted initialization is retryable. A ready store
  never recreates missing safety namespaces or locks, and a missing selection record or field is damage, not an
  uninstall.
- **Precreated roots.** Precreated roots, including mountpoints, initialize in place without parent write access.
  Creating an absent path stages its missing suffix and records the parent durability barrier in each created directory;
  new stores sharing those ancestors reconcile that barrier too, and successful setup retires the publication records.
  Interrupted publication can leave a small unpublished sibling with initial metadata only, never payloads. Published
  roots and lock identities are preserved. Preload readers require ready state without modifying it.

## Installation, update and repair

The compatibility `runtime update` command only directs callers to the Agent installer, package manager or deployment
workflow. It performs no discovery, acquisition or storage mutation.

- **Serialization.** `BeginPublishedInstall` serializes explicit release discovery, candidate preparation and selection
  with installation and uninstall. It takes installer exclusion before reading the selection or fetching metadata, but
  never holds inventory exclusion across the network. Preparing an absent selection fails before discovery. Cancellation
  releases installer exclusion; a progressing download has no total duration limit, so another installer or uninstall
  may wait. The store schedules no discovery or downloads itself.
- **Selection record.** The selected descriptor and update-consent policy share one atomic selection record. Failed
  candidates preserve them. A matching Agent tuple is a no-op only for identical complete metadata;
  explicit reinstallation can change its policy without downloading unchanged bytes. Identical `PrepareRelease`
  verifies the selected payload and provenance, returns unchanged and preserves the previous recovery point. An existing
  pending transition still requires reconciliation, including when its target is already selected. Legacy sequences
  remain readable metadata for old installations; they no longer drive an independent runtime update.
- **Update policy.** The existing `automatic_updates` selection field records acquisition consent for the Agent's
  installation owner; no process service consumes it to update the runtime independently. Pair preparation preserves
  this field. Install and repair set it, including offline imports: acquisition for one invocation and future owner consent
  are separate decisions. Protected preloads remain manually deployed.
- **Paired activation.** `PreparePair` records pending before/after selections in one separate `recovery.json` record
  and makes the current selection the recovery point. Beginning B→C may retire older A; a failed transition
  retains B, and repeated rollback stays on B. An unchanged B preparation preserves A. A changed target retains both
  pending generations without publishing the target selection. `ActivatePair` requires drained consumers and the exact
  target Agent, selects the target, then clears the pending transition while retaining its recovery point. A retry
  reconciles interruption between these writes. With no pending transition, activation verifies the exact selected pair without
  changing recovery state, so owners can also call it after unchanged preparation. `RollbackPair` restores pending-before
  or previous selection without executing it; preparation-only cancellation is a no-op when no pending transition exists.
  Agent files/images belong to the caller.
- **Retained inputs.** `VerifiedRelease.SaveInputs` saves the authenticated descriptor, compressed archive and exact
  verification public keyring outside managed storage, after pair/digest checks and without changing selection. Retry
  supplies that retained keyring explicitly. Earlier binaries ignore pair recovery references and can reclaim a future
  generation after rollback; owners retain inputs for unresolved attempts and re-import on retry. Successful
  source/static installations release their temporary inputs. Before first migration imports, stop the
  old consumer and its GC. No background keeper or physical-retention promise is added for old readers.
- **Errors.** CLI install and pair preparation or validation errors keep the previous selection. Repair errors keep a
  healthy different selection but cannot restore a cleared one; retrying repair is supported once the reported problem
  is resolved, and a successful repair needs a DEM restart.
- **Repair.** Explicit repair can select an older authenticated release after all consumers/executions drain, with a
  newly specified update policy. Repair retires damaged generations plus the requested generation, then uses ordinary
  import or download, integrity validation and selection. It durably clears a damaged selection, or the one being rebuilt,
  before retirement. A healthy different selection stays protected; other unselected generations follow ordinary
  retention. A retained pair can be rebuilt only with its exact executable manifest and Agent identity, even after a
  failed import removed its generation. Its recovery record remains intact. Pending pair operations must be resolved
  through their owner first; repair never bypasses that ownership guard. Repair reports candidate identity and progress
  separately from the final selected installation.
- **Uninstall.** Uninstall serializes against installation and refuses any live consumer/candidate, preparation or
  unresolved execution across all generations. Refusal preserves selection. Successful uninstall clears selection before
  retiring bytes. No timeout, PID disappearance or process lock can substitute for an execution receipt.
- **Damaged metadata.** Repair and uninstall both refuse live consumers, candidates, preparation and unresolved
  executions across all generations. Complete ownership evidence permits removing generations even when executable
  metadata is damaged. Missing safety structure and operational I/O errors still block removal, and neither command
  recreates lost ownership evidence or replaces the store root or its locks.

## Retirement and status

- **Allowance.** The default allowance is 4 GiB of logical regular-file bytes, including archives, workspace, lifecycle
  records and trash. Active preparation reserves its unwritten remainder using the same observation as byte accounting.
  This is not a physical quota; filesystem metadata and protected preloads are additional.
- **Protection and grace.** Selected, prepared/previous-pair, process-owned, candidate and execution-protected
  generations never yield to pressure. The most recently used unprotected old generation has a seven-day soft grace;
  others can retire immediately.
  Pressure can shorten grace. A selected release stays installed even without monitors. Unknown executions may retain
  files indefinitely and prevent a later installation.
- **Inspect.** `Inspect` is read-only: selected descriptor, protected/reclaimable totals, preparation, trash, and
  overlapping selected/process/candidate/execution byte categories. Do not sum overlapping categories as extra disk use.
  Reclaimable failed-deletion trash is not free space.
- **Diagnostic inventory.** Inventory separates executable metadata validity from independent ownership evidence. Status
  reports selection and generation damage with the available bytes and owners, and an incomplete ownership or I/O
  inspection is explicit (unknown). A damaged old generation does not make a healthy selection unavailable. Execution
  still requires a compatible selected release; ordinary installation and automatic retirement do not repair metadata.
  Healthy incompatible metadata remains visible in inventory. CLI status preserves its identity and reports
  `incompatible` with a nonzero exit status, rather than classifying it as `damaged`.
- **Sweep.** `Sweep` reconciles and retires at startup, before preparation and hourly; cleanup health is separate from
  monitor outcomes. Cleanup health survives coordination failures and cannot be cleared by an older sweep.
- **Deletion.** Objects retire to trash under coordination. Parent directories are synced before physical deletion
  outside coordination.
- **Lock and record release.** Completed preparation releases its local claim and staging locks before coordinated
  record reclamation. Failed local lock closes stay owned by the store for maintenance or final-close retry; released
  records are reclaimed by the dead-owner inventory. A failed execution release never supplies completion evidence.

## Protected preloads

`Candidate.SealPreload` finishes a fresh, integrity-verified root and records its trusted identity/final location. Managed
operations then refuse the root, including stores opened before sealing. `ReadPreload` supports read-only mounts without
state writes and checks the bound payload, compatible contract and intrinsic release provenance. Installation owners
supply an expected Agent identity for an additional exact-pair check; ordinary execution supplies none. It uses sealed
metadata without a separate installed manifest. The administrator must keep it private, service-account-owned and immutable, and stop every consumer before
removal/replacement. Deployment prepares a new final root and retains the old one; sealed roots cannot be relocated.
The deployment owner publishes/restores configuration and controls service intent. Source/static preload updates
use ordinary Agent installation and leave the pair stopped; `runtime preload-validate` checks the target pair
before and after replacement, independently of ordinary execution/status; they invoke no managed selection operation or automatic
configuration rewrite.

## Native packages

A native package installs a prebound payload at the fixed prefix `/usr/lib/netdata/dem-runtime` and keeps admission
state in `/var/lib/netdata-dem-runtime`. Native package and repository authentication is its trust boundary.

- Build: `PrepareNativePackage` verifies the archive in a spool beside a new or empty stage, extracts it, binds
  interpreter slots and generated configuration to the installed prefix, normalizes modes to 0755/0644 and publishes
  `native.json` only after syncing the stage. Package recipes assign root ownership.
- Admission (root, package hooks): a root-owned gate and `admission.json` move through `initializing`, `closed`,
  `qualifying` and `ready`. `CloseAdmission` refuses while service-UID proof shows consumers or executions, and closes
  admission before the package manager changes files. `ValidateAndReady` records the installed payload as the candidate
  target and verifies it as the service UID without executing the browser. The service acquires and closes an exact
  candidate claim while the root gate is released; admission opens only if the payload and target are unchanged and no
  proof remains. The persisted `qualifying` phase name is retained for older readers and interrupted transactions; it
  now denotes candidate validation. Established state never bootstraps a missing gate, ledger or namespace.
- Consumers (service UID): `Native` reads admission under the shared gate and admits a payload only if its whole tree is
  root-owned and immutable to the service, its bound layout is intact with a group or other execute bit, and its
  interpreter slots and generated configuration name the installed prefix. Claims and execution receipts use the same
  ownership ledger as managed stores, in a service-owned proof root. `Inspect` and `Sweep` reconcile proof without
  touching package files.

## Validation

Tests cover archive bounds/path handling, interpreter binding, selection/activation, idle and pressure protection,
serialization, refused uninstall, publication failure, process death, uncertain uses, receipt recovery and boot
provenance. Native admission tests need root and a `netdata` account and otherwise skip. Their helper processes
re-execute the test binary as that account, which cannot enter `go test`'s private build directory: build it with `go
test -c` into a directory the account can traverse and run it from the package directory. Runtime, host, CLI and
release-tool tests share one synthetic archive builder, `internal/browserruntimetest`. `DEM_RUNTIME_TEST_HELPER` enables
real supervisor receipt tests; run under the same unprivileged identity as the helper. Real CLI qualification covers
journeys and Lighthouse. Filesystem fault tests do not simulate physical power loss or establish native SELinux/AppArmor
policy compatibility.
