# Reviewing Coverity Findings

Use current source, the event trace and callers to establish reachability, ownership and consequences. The snippets in
a saved bundle may be stale. Apply the relevant domain skills for the affected code and the review's scope; a review
does not itself authorize implementing fixes or applying Coverity classifications.

## Verdicts

The vocabulary below is supported by `./scripts/finalize-defect.sh`. Reviews MAY reuse it or use another vocabulary;
translate custom labels to the helper's supported names or explicit attributes before an authorized update.
Classification describes the finding. Action describes its disposition, including whether a fix has been submitted.

| Verdict | Evidence / meaning |
|---|---|
| `TRUE_BUG_MEMORY_CORRUPTION` | Reachable OOB read/write, UAF, double-free, type confusion or stack-escape UAF. |
| `TRUE_BUG_CRASH` | Reachable NULL dereference, division by zero, assertion or fatal without memory corruption. |
| `TRUE_BUG_RESOURCE_LEAK` | Reachable accumulating fd, memory, lock or reference leak. |
| `TRUE_BUG_LOGIC` | Wrong result, metric, stored or transmitted data without a crash. |
| `TRUE_BUG_UB` | Language-level undefined behavior such as signed overflow or aliasing violations, even if it compiles. |
| `FALSE_POSITIVE_GUARD_EXISTS` | An actual guard the analyzer failed to follow prevents the reported defect. |
| `FALSE_POSITIVE_UNREACHABLE` | The flagged path is unreachable under the actual caller/state contracts. |
| `FALSE_POSITIVE_TRUSTED_INPUT` | The specific tainted-data claim is disproved by the real source and validation boundary. |
| `FALSE_POSITIVE_TOOL_MODEL` | The analyzer's model of the relevant primitive is wrong. |
| `IMPOSSIBLE_CONDITIONS` | Existing invariants prohibit the reported combination of states. |
| `COSMETIC` | A verified harmless unused value, dead branch or redundant expression. |
| `CODE_GONE` | The flagged file/function was removed; verify source, not merely a missing scan instance. |
| `NEEDS_HUMAN` | Material uncertainty remains after reasonable investigation. |

True bugs map to Bug / Fix Required unless a submitted-fix reference is provided. The false-positive group maps to
False Positive / Ignore; cosmetic findings map to Intentional / Ignore. The two bookkeeping verdicts do not update
Coverity. See `./SKILL.md#apply-decisions` for authorization and the submitted-fix argument contract.

## Netdata Idioms To Check

These are investigation pointers, not automatic false-positive verdicts. Confirm the primitive actually used, its
inputs and the reachable path; analyzer behavior varies by checker and build configuration. Paths below are
repository-relative.

- Allocation: `src/libnetdata/memory/nd-mallocz.c` and its header own `mallocz`, `callocz`, `reallocz`, `strdupz` and
  `strndupz`, including tracing builds. They terminate on allocation failure rather than return an OOM NULL.
  This does not validate their inputs, requested sizes or subsequent pointer ownership. Check the actual allocator,
  including flexible-allocation helpers, before dismissing an OOM path. `freez(NULL)` is a no-op.
- STRING: `src/libnetdata/string/string.c` owns interned, reference-counted strings. `string_strdupz` accepts C text
  and acquires an interned STRING reference; `string_dup` acquires another reference from an existing STRING.
  Both may legitimately appear in the same code, as the unit checks demonstrate. Verify input types and balanced
  releases through `string_freez`; NULL/empty text can produce NULL, and `string_freez(NULL)` is safe.
- Lists: `src/libnetdata/linked_lists/linked_lists.h` owns `DOUBLE_LINKED_LIST_*` prev/next invariants. Raw pointer
  manipulation inside these macros is expected; verify caller membership and lifecycle before judging it.
- Buffers: `src/libnetdata/buffer/` owns reserve/growth and write behavior. Check the specific API's capacity handling
  and caller lengths; direct indexing into `buffer->buffer[N]` requires its own bounds evidence.
- ARAL: `src/libnetdata/aral/` reuses freed slab objects. A stale pointer that still appears to work after
  `aral_freez` may refer to a different object at the same address.
- Dictionary: `src/libnetdata/dictionary/` owns acquired-item and traversal lifetimes, including
  `dictionary_get_and_acquire_item` / `dictionary_acquired_item_release`. Determine which API holds a reference or lock;
  do not assume a raw pointer remains valid after that protection ends.
- Locks: `src/libnetdata/locks/` owns `spinlock_lock` and `rw_spinlock_*`. These are custom primitives, so a
  MISSING_LOCK report may reflect modeling limitations; verify the actual protected access and lock ordering.
- Portability: check glibc and musl assumptions, including `strerror_r` signatures. Changes MUST compile with both
  GCC and Clang. The long-running Agent and its plugin processes make accumulating leaks and stale lifetimes
  significant.

## Input Boundaries

A source's origin, transport and authorization do not by themselves prove a value safe for a particular operation.
Before using `FALSE_POSITIVE_TRUSTED_INPUT`, trace the field to the operation and establish the relevant constraint.

| Input / source owner | Review boundary |
|---|---|
| `/proc`, `/sys`; `src/collectors/proc.plugin/`, `src/collectors/cgroups.plugin/` | Kernel-provided records can contain workload-controlled names/values. Check the actual field, namespace and parsing. |
| Plugin protocol; `src/plugins.d/`, `src/collectors/` | The Agent launches plugins using line-oriented stdin/stdout. A trusted plugin executable can relay monitored-system data; validate the field's origin. |
| Streaming; `src/streaming/` | Remote peer data is untrusted; inspect protocol validation and connection policy. |
| HTTP; `src/web/api/`, `src/web/server/` | Requests are untrusted. Do not assume localhost: default binding is wildcard in `web_server.c`. Check route ACL/auth and deployment. |
| MCP; `src/web/mcp/` | Requests are untrusted. `src/daemon/config/netdata-conf-web.c` initializes MCP allow rules from the dashboard default; do not infer localhost-only access. |
| Cloud; `src/aclk/` | TLS/session authentication identifies transport/peer, not the validity of every payload. Trace forwarded inputs and operation-specific checks. |
| Configuration; `src/daemon/config/`, `src/health/` | Establish actual file ownership, write access, provisioning path and field constraints before treating a value as trusted. |
