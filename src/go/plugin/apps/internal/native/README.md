# Local native apps backend

This package is a Linux procfs backend with a POSIX fixture build on macOS. It is
part of the apps Go/C proof of concept. It does not link libnetdata, include the
original collector, execute a helper, or call Go from C.

## Provenance and license

`native.c` adapts algorithms and field mappings from Netdata Agent commit
`3bd129555e`, under **GPL-3.0-or-later**, retaining the original SPDX license:

| Original source under `src/collectors/apps.plugin/` | Local adaptation |
| --- | --- |
| `apps_os_linux.c` | stat/status/io/limits/cmdline/PSS fields, exclusive guest CPU, global CPU, procfs no-follow opens, inode-aware readlink cache |
| `apps_incremental_collection.c` | separate monotonic timestamps for stat and IO rates, failed-read handling |
| `apps_pid.c` | parent-first sampling, process incarnation lifetime, subtracting observed exited-child CPU/faults from the surviving ancestor, one extra collection of reconciliation state |
| `apps_pid_files.c` | FD target classification and unique per-group FD target counting |
| `apps_aggregations.c` | group-change/exit feedback into PSS refresh priority |
| `apps_plugin.c` | host CPU normalization is implemented by Go grouping, outside this native package |

The dependency closure is replaced locally: libc/POSIX reads replace procfile and
ARL; owned C strings replace Netdata string helpers; a sorted PID vector replaces
the PID index; and sorted `(group ID, link target)` pairs replace the global FD
registry and dense group-by-registry tables. These are adaptations, not verbatim
copies of complete original translation units. The repository root `LICENSE`
contains the GPL text. Existing source copyright ownership is unchanged.

## Boundary and lifetime

`Scan` and `Finalize` each perform one batched operation. C owns all mutable
sampling state. Go copies complete strings/values before returning a snapshot.
Command lines cross as length-delimited NUL-separated argv bytes; Go renders matching text separately so script paths containing spaces keep their identity. No Go pointer is retained by C. `Finalize` accepts exactly one PID/start-time
assignment per current row and rejects wrong generations, stale incarnations,
missing rows and duplicates before changing assignment state. Group IDs MUST be
unique across aggregation axes. Go owns ordered matching and aggregation policy.

`Close` is idempotent. A mutex serializes all operations, including Close.
Cancellation is checked before and after the synchronous C scan; a pending
kernel procfs read is not interruptible by context cancellation. Callers SHOULD
use the local procfs mount. There are no mutable globals or static reader buffers.

A process row requires valid stat and UID/GID identity. Optional observations have
independent validity bits. Counter warmup and resets produce gaps. Missing reads
preserve the last successful counter timestamp, so recovery spans the actual
measurement interval. A start-time change resets the replacement's baselines and
caches. The old incarnation's accepted CPU/fault lifetime and accepted parent
PID/start-time identity move into a separate compact retirement record before
that reset. Ordinary disappearance uses the same retirement path. Pending debt
therefore survives reuse of either the child PID or its ancestors, and retirement
records never own FD strings or sampling caches. A second stat read detects PID
replacement between per-process file reads; sampled PSS also validates the
incarnation. Only live readable rows leave C.

Retirement debt is eligible during the detecting scan and one additional scan,
matching the original collector's grace. Expired ancestors may still supply
ancestry during reconciliation; younger retirement records bypass those ancestors
before their records are removed. This preserves ancestry without extending old
debt's lifetime. Read gaps do not create retirements: ancestor resolution stops at
a matching parent that is still present but unreadable, leaving debt pending for
that parent's recovery within the grace window.

## POC sampling differences

PSS uses the original `PSS / RSS` ratio to estimate memory from current RSS. A
successful sample exports its age in seconds. A failed attempted refresh makes
PSS unavailable. The POC replaces alternating shared-memory-delta/age priority
with oldest-attempt priority and a ten-second refresh target; group changes and
exits mark affected groups for priority. Sampling is spread with a per-cycle
budget derived from elapsed time and live process count (minimum one). This is an
accuracy/cost tradeoff, not equivalence to the original configurable scheduler.

The inode-aware FD link cache increases successful stable-read spacing up to the
original 60-second cache horizon. Cached strings transfer ownership between
vectors without per-FD duplication. Like the source collector, link targets can
remain cached until recheck when a descriptor changes without a changed procfs
inode. Group FD counts deduplicate link target strings; regular file names are
not inode-level open-file-description identities. An incomplete member makes a
group FD result invalid rather than reporting a complete-looking partial count.

C exports unnormalized own/child CPU rates and independent host CPU observations.
Go grouping owns normalization with preference for live-process CPU. C retains
exited-child reconciliation, so previously observed child lifetimes are removed
before the grouping policy runs. Reconciliation and sampled estimates remain
heuristic observations of a changing process tree. Reaping
later than the retained reconciliation grace can include previously observed
child lifetime in the ancestor's child metrics. Linux eBPF, cgroup and NetIPC
integration and non-Linux live collection are outside this POC.

## Complexity and validation

For P live/recent PID slots, R retired incarnations, F open descriptors, A nonzero
assignments (at most 3P), and ancestry depth D: current PID lookup is O(log P),
retired incarnation lookup is O(log R), and the retirement index sorts in
O(R log R). Parent-first traversal visits each known PID once. Reconciliation and
ancestry cleanup are O(R × D log(P+R)); neither scans all PIDs for each retirement.
FD deduplication sorts at most 3F+A pairs in O((F+P) log(F+P)). Active data occupies
O(P+F+R), with retirement debt limited to the detection scan and one extra scan.
Backing arrays may retain previous peaks: the PID pointer vector resizes on later
PID additions, while the retirement vector retains capacity until a retirement-free
window, when it is released. Thus allocated bytes can exceed the current logical
O(P+F+R) population. There is no group-by-FD-registry product allocation. FD cache
lookups are O(log descriptors-in-process), and cached target strings are reused.
Per-process file buffers and FD vectors remain ordinary short-lived allocations.

Run from `src/go`:

```sh
go test -count=1 ./plugin/apps/internal/native
go vet ./plugin/apps/internal/native
CGO_ENABLED=0 go test ./plugin/apps/internal/native
go test -race -count=1 ./plugin/apps/internal/native
go test -run '^$' -bench 'BenchmarkScan' -benchmem ./plugin/apps/internal/native
# On Linux with a sanitizer-capable C compiler:
go test -asan -count=1 ./plugin/apps/internal/native
CGO_CFLAGS='-O1 -g -fsanitize=undefined' CGO_LDFLAGS='-fsanitize=undefined' go test -count=1 ./plugin/apps/internal/native
```

Fixtures exercise rates, gaps, malformed names/numbers, identity replacement
between stat/status, reuse and disappearance, child accounting and delayed reaps,
shared FDs, link caching, PSS freshness, assignment validation and lifecycle.
`BenchmarkScanWarmFDs200x20` exercises 200 PIDs with 20 shared FD targets each and
reports actual file reads/readlinks as well as Go allocations. Go `B/op` and
`allocs/op` exclude C/libc allocations; they are not total native-memory metrics.
Timing results are machine/workload trends, not CI thresholds or performance
parity claims. The original executable comparison belongs to the complete POC.
