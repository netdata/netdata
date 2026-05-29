# SOW-0044 - systemd-journal Virtual Functions and Watch Root Ownership

## Status

Status: open

Sub-state: decision-ready. No implementation is approved yet. This SOW exposes the implementation complexity and gives the user a go/no-go decision point before it moves to `current/`.

## Requirements

### Purpose

Make SNMP trap journals visible through a first-class Logs Function without regressing the existing `systemd-journal` Function used by many operators.

The purpose is not just to expose one `snmp-trap` Function. The purpose is to establish a safe reusable model for future "virtual Functions" that share one journal query engine, one watcher, one file registry, and one enrichment pipeline, while each Function has independent defaults, facets, source scope, and DynCfg ownership.

### User Request

The user asked for a separate SOW and review pass before changing `systemd-journal.plugin`, with this specific concern:

- `systemd-journal.plugin` is widely used and must not be broken.
- Each virtual Function should ideally have its own DynCfg.
- The hard problem is the single watcher.
- The user clarified that the watcher is the key remaining problem. For this SOW, "watcher solved" means root ownership, inotify watching, registry masks, deletion, rescan, and query visibility are all correct together.
- A proposed approach is to assign each Function/view a bit in a mask and assign each watched directory/file a matching mask, so every Function can find only its own files.
- Netdata bitmask style must be followed: enum values carry `(1 << n)` directly, instead of adding a helper macro.
- Reviewers must be asked to identify why this may be a bad idea and how the existing `systemd-journal` Function could break.

### Assistant Understanding

Facts:

- `systemd-journal.plugin` currently registers one public Function, `systemd-journal`, in `src/collectors/systemd-journal.plugin/systemd-main.c`.
- The Function event loop already supports multiple Function callbacks and a per-Function `void *data` pointer in `src/libnetdata/functions_evloop/functions_evloop.h`.
- The existing journal file registry and watcher are global to the plugin.
- Current systemd-journal DynCfg updates the same global `journal_directories[]` array that the watcher consumes.
- The current source filtering model is already a bitmask enum using direct `(1 << n)` values in `src/collectors/systemd-journal.plugin/systemd-internals.h`.
- The proposed Function/view ownership mask must be separate from the existing user-facing journal source mask.

Inferences:

- Per-Function DynCfg plus one shared watcher is feasible if directory ownership and file visibility are modeled explicitly.
- A Function/view mask is safer than overloading `SD_JOURNAL_FILE_SOURCE_TYPE`, because source filters are already user-facing query semantics.
- The watcher and registry must be treated as one coupled subsystem. A watcher that watches the right directories is still unsafe if registry insertion, deletion, full rescan, pending events, and query filtering remain path-only or global.
- The most likely regression classes are not packet parsing or SNMP profiles. They are `systemd-journal` Function registration, default facets, source filtering, watched-directory updates, file-registry deletion, and query result compatibility.

Resolved findings:

- The RRD Function lookup strips trailing command words while resolving the registered Function name in `src/database/rrdfunctions.c:269-316`.
- The plugin worker then dispatches callbacks by raw prefix in `src/libnetdata/functions_evloop/functions_evloop.c:158-160`.
- Therefore the dispatcher cannot be changed to plain full-command `strcmp()`: Function commands include arguments after the registered name. The correct fix is registered-name token-boundary matching.
- `logs_query_status.h` is also used by `windows-events` through `src/collectors/windows-events.plugin/windows-events.h`, so the preferred design is to leave it unchanged and instantiate it once per journal Function source file.
- The user proposed an even simpler LQS instantiation shape: move the LQS-dependent journal Function body into a macro-instantiated journal header, move 100% common helpers into a normal `.c` file, then include the header from one source file per public Function after defining the Function name, help text, defaults, source parser, and source-list callback.
- A sibling Netdata worktree contains a compiling prototype of this shape using `systemd-journal-function.h` for LQS prerequisites and `struct lqs_extension`, plus `systemd-journal-execute.h` for the query execution template. That validates the basic header-instantiation direction. After cleanup, the source parser and transformation registration are wrapper-local again.
- The user accepted that internal log prefixes and journal-file errors can keep saying `SYSTEMD-JOURNAL` / "systemd journal file", because the plugin remains `systemd-journal.plugin` and the stored files are systemd journal files. This is no longer a blocker for `snmp-trap`.

Remaining product decisions:

- Whether to implement this now.
- Whether the `snmp-trap` DynCfg node is read-only for the Netdata-owned trap root or writable for operator-supplied journal roots.
- Whether `systemd-journal` continues to expose trap-specific default facets after `snmp-trap` exists.

### Acceptance Criteria

- The SOW is detailed enough that an implementer can start without rediscovering the architecture.
- The SOW exposes the real implementation complexity, affected files, regression risks, and required validation.
- The design keeps one watcher and one journal registry while giving each virtual Function independent DynCfg ownership.
- The design explicitly resolves Function dispatch prefix matching before adding any virtual Function.
- The design explicitly resolves how per-Function names, help text, source lists, and defaults are achieved without changing `logs_query_status.h`.
- The design explicitly resolves mask-aware file registry insertion, update, rescan, deletion, and source enumeration.
- The design states whether `systemd-journal` sees trap journals and trap facets by default.
- Reviewers complete an adversarial pass focused on why the design may be a bad idea and how the existing `systemd-journal` Function could break.
- No implementation starts until the user approves the design and the SOW moves to `current/`.
- If implemented later, existing `systemd-journal` behavior is validated against pre-change behavior for Function registration, help/info output, required params, default facets, source filtering, cancellation, progress updates, and query results.

## Analysis

Sources checked:

- `src/collectors/systemd-journal.plugin/systemd-main.c`
- `src/collectors/systemd-journal.plugin/systemd-internals.h`
- `src/collectors/systemd-journal.plugin/systemd-journal.c`
- `src/collectors/systemd-journal.plugin/systemd-journal-files.c`
- `src/collectors/systemd-journal.plugin/systemd-journal-watcher.c`
- `src/collectors/systemd-journal.plugin/systemd-journal-dyncfg.c`
- `src/libnetdata/functions_evloop/functions_evloop.h`
- `src/libnetdata/functions_evloop/functions_evloop.c`
- `src/libnetdata/facets/logs_query_status.h`
- Existing Netdata bitmask examples under `src/daemon/`, `src/libnetdata/`, and `src/collectors/systemd-journal.plugin/`.

Current state:

- `systemd-main.c` registers only `systemd-journal` through `functions_evloop_add_function()`.
- `functions_evloop_add_function()` stores a callback, default timeout, and `void *data` per Function. This is a local pattern that can support multiple Function instances without changing the event-loop ABI.
- `systemd-journal-dyncfg.c` exposes one DynCfg node, `systemd-journal:monitored-directories`, and an UPDATE replaces `journal_directories[]`.
- `systemd-journal-watcher.c` reads `journal_directories[]`, recursively watches each configured directory, and updates the shared `nd_journal_files_registry`.
- `systemd-journal-watcher.c` currently deletes registry entries under removed directories. With multiple Function owners, that logic would need to recompute visibility masks before deleting.
- `systemd-journal-files.c` classifies each file into `SD_JOURNAL_FILE_SOURCE_TYPE` values such as local, remote, namespace, user, system, and uncategorized.
- `systemd-journal.c` filters files through `jf_is_mine()` by source type and custom source pattern before querying them.
- `logs_query_status.h` initializes source filtering to `LQS_SOURCE_TYPE_ALL`, which maps to `ND_SD_JF_ALL` for systemd-journal.

Risks:

- A shared DynCfg node can let one Function change watched directories for another Function. This is not acceptable for virtual Functions.
- Multiple watchers can duplicate inotify watches and race while updating or deleting shared registry entries. This is not acceptable.
- Overloading source type for Function ownership would mix internal ownership with user-facing filtering semantics.
- Prefix matching in Function dispatch can cause future name collisions if virtual Function names share prefixes.
- Changing `logs_query_status.h` for one virtual Function may accidentally affect Windows Events or other future log explorers that use the same header. The preferred design avoids this by leaving `logs_query_status.h` unchanged and using one source file per Function.
- Any default-facet changes to `systemd-journal` can affect Cloud Logs UX for existing users.

Reviewer-confirmed blockers from adversarial pass:

- `functions_evloop.c` dispatches by prefix using `strncmp(function, we->function, we->function_length)`, so future virtual Function names with shared prefixes can route to the wrong callback.
- `logs_query_status.h` is a macro-instantiated header included once by `systemd-journal.c`; `void *data` alone cannot make Function name, help text, source-list callback, default source type, and default facets runtime-configurable. This does not require changing LQS if each Function gets its own source file and macro instantiation.
- `function_systemd_journal()` is monolithic and currently ignores callback `data`; it hardcodes facet lists, request defaults, histogram default, and transformations.
- `remove_directory_watch()` deletes all registry entries under a removed path. With overlapping or shared roots, one Function can remove files still visible to another Function.
- `nd_journal_files_registry_update()` deletes registry entries not seen in the current full scan. With per-Function roots, "not seen" must be mask-aware or files from another Function can disappear.
- `available_journal_file_sources_to_json_array()` enumerates sources from the global registry. A `snmp-trap` info response would expose system journal sources unless source enumeration becomes Function/view-aware.
- Trap fields currently live in `SYSTEMD_KEYS_INCLUDED_IN_FACETS`; leaving them there can pollute `systemd-journal`, while removing them without a separate `snmp-trap` facet set can hide useful trap fields.

## Pre-Implementation Gate

Status: ready for user go/no-go. Implementation remains blocked until the user approves moving this SOW to `current/` and records the open decisions.

Problem / root-cause model:

- SNMP traps are written to compact journal files under Netdata cache directories, not under the default systemd journal roots.
- The existing `systemd-journal` Function can read journal files only from watched/scanned directories.
- Simply adding the trap directory to the current global directory list would make traps visible, but it would also mix trap files into the existing `systemd-journal` Function and global DynCfg ownership.
- The root design problem is ownership: multiple virtual Functions need independent defaults and DynCfg, but scanning and watching must remain shared to avoid duplicated inotify state and registry races.

Evidence reviewed:

- `src/collectors/systemd-journal.plugin/systemd-main.c:78-81`: one `systemd-journal` Function registration and one DynCfg init call.
- `src/libnetdata/functions_evloop/functions_evloop.h:88-95`: Function callback receives `void *data`.
- `src/libnetdata/functions_evloop/functions_evloop.c:158-160`: worker dispatch currently uses raw prefix matching.
- `src/database/rrdfunctions.c:269-316`: RRD Function lookup strips trailing command words and can resolve a registered Function from a command with arguments.
- `src/collectors/systemd-journal.plugin/systemd-journal-dyncfg.c:82-104`: current DynCfg UPDATE mutates `journal_directories[]` directly and restarts the watcher.
- `src/collectors/systemd-journal.plugin/systemd-journal-dyncfg.c:164-176`: current DynCfg node is `systemd-journal:monitored-directories`.
- `src/collectors/systemd-journal.plugin/systemd-journal-files.c:9`: current root list is one global `journal_directories[]` array.
- `src/collectors/systemd-journal.plugin/systemd-journal-files.c:357-426`: registry insert classifies source by filename and has no Function/view ownership.
- `src/collectors/systemd-journal.plugin/systemd-journal-files.c:537-575`: source enumeration walks the global registry and is not Function/view-aware.
- `src/collectors/systemd-journal.plugin/systemd-journal-files.c:727-785`: full scan deletes registry entries not seen in the latest global scan.
- `src/collectors/systemd-journal.plugin/systemd-journal-watcher.c:257-280`: removed directory watches delete every registry entry under the path.
- `src/collectors/systemd-journal.plugin/systemd-journal-watcher.c:450-477`: pending file processing deletes or inserts by path with no Function/view ownership recomputation.
- `src/collectors/systemd-journal.plugin/systemd-journal-watcher.c:561-565`: watcher reads the global `journal_directories[]` list.
- `src/collectors/systemd-journal.plugin/systemd-journal.c:77-87`: LQS is macro-configured for one `systemd-journal` Function.
- `src/collectors/systemd-journal.plugin/systemd-journal.c:106-207`: trap fields currently live in the generic systemd default facet list on this branch.
- `src/collectors/systemd-journal.plugin/systemd-journal.c:724-902`: query selects files from the global registry through `jf_is_mine()`.
- `src/collectors/systemd-journal.plugin/systemd-journal.c:1136-1201`: callback ignores `data` and hardcodes facets, histogram, transformations, info response, and query path.
- `src/libnetdata/facets/logs_query_status.h:169-180`: help text uses macro-defined Function name and description.
- `src/libnetdata/facets/logs_query_status.h:546-581`: source parsing uses macro-defined source conversion.
- `src/libnetdata/facets/logs_query_status.h:663-696`: info response uses macro-defined source-list callback and Function description.
- `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:50-57`: Go SNMP trap journals are written under `<cache>/traps/<job>`.

Affected contracts and surfaces:

- Public Function registry: `systemd-journal` must remain available and compatible.
- New public Function registry: `snmp-trap` may be added only if it does not alter existing `systemd-journal` semantics.
- Function UI schema and Cloud Logs expectations for info/help/table responses.
- DynCfg tree path and node ids for monitored directories.
- Inotify watcher behavior and restart behavior.
- Journal file registry lifetime and deletion behavior.
- Source filter options exposed via `__logs_sources`.
- Default facets, histogram selection, row transformations, row severity mapping, and enrichments.
- Local Agent API, Cloud Function execution, streaming Function metadata, and MCP Function discovery.

Existing patterns to reuse:

- Netdata bitmask enum style with direct bit values, as used by `SD_JOURNAL_FILE_SOURCE_TYPE`, `DYNCFG_CMDS`, dictionary options, and daemon service flags.
- `functions_evloop_add_function()` per-Function `void *data`.
- Existing single watcher and registry architecture in `systemd-journal.plugin`.
- Existing source filtering as a user-facing mask, kept separate from internal Function/view ownership.

Risk and blast radius:

- Blast radius is high because `systemd-journal` is a general-purpose logs Function.
- The implementation risk is medium if the change is kept to wrapper-local Function configuration, root ownership mask, and early file-visibility filter.
- The implementation risk becomes high if progress/cancellation semantics, the low-level query loops, or shared watcher/registry lifetime are changed.
- The LQS risk is lower if `logs_query_status.h` remains unchanged, but the risk shifts to safely splitting `systemd-journal.c` into per-Function wrappers plus a shared journal core.
- The operational risk is high if DynCfg updates can remove another Function's roots or if watcher deletion removes files still visible to another Function.
- The UX risk is high if `systemd-journal` starts showing trap-specific facets or trap files by default.

Sensitive data handling plan:

- Do not copy live journal messages, trap payloads, device hostnames, public IPs, SNMP communities, USM secrets, customer identifiers, or private endpoints into this SOW, specs, docs, skills, comments, or tests.
- Use sanitized paths and placeholders in examples.
- Reviewer prompts must reference code paths and design risks, not local live trap payloads.

Ready-to-implement design:

1. Scope boundary:
   - Implement the minimum reusable internal model for two views: `systemd-journal` and `snmp-trap`.
   - Do not build a public generic virtual-Function framework in this SOW.
   - Keep the shared query engine, watcher, registry, progress reporting, cancellation, enrichments, and journal parsing code.
   - Make per-Function behavior explicit through separate wrapper source files and view masks.

2. Watcher critical-path definition:
   - The hard problem is considered solved only when all of these are true:
     - directory roots have explicit owner/view metadata;
     - DynCfg mutates only roots owned by the targeted Function;
     - the single watcher consumes a snapshot of the union of active roots;
     - recursive directory scanning carries the root view mask down to every discovered file;
     - same-file discoveries through overlapping roots OR their view masks;
     - registry insert and conflict paths preserve and update view masks;
     - registry deletion removes a file only when it no longer exists or no view owns it;
     - full rescans reduce, increase, or clear view masks based on the current root snapshot;
     - pending inotify events cannot insert stale-mask entries after root changes;
     - query selection filters by view mask before applying user-facing source filters;
     - source enumeration is view-aware.
   - Adding a second watched directory without the registry and query-mask work is not a valid implementation.

3. Function dispatch:
   - Add a helper in `functions_evloop.c` that matches a registered Function name only when the command starts with that name and the next byte is end-of-string or whitespace.
   - Replace raw `strncmp(function, we->function, we->function_length) == 0` with this token-boundary helper.
   - Break out of the expectations loop after the first successful dispatch attempt. The current loop has no `break`, so prefix collisions can continue scanning and produce duplicate-transaction behavior.
   - Preserve existing command strings with arguments, because the registered Function name is only the first command token.
   - Add a same-file or narrow unit test if this subsystem has an existing test harness; otherwise add a focused test-only helper or scripted validation covering `systemd-journal`, `systemd-journal-x`, `snmp-trap`, `snmp-trap info`, and `config`.
   - This is a shared library change, so validate every existing `functions_evloop_add_function()` caller found by `rg`, including `systemd-journal`, `systemd-list-units`, `windows-events`, DynCfg `config`, and any other registered Function names in the tree.

4. LQS header-template per Function:
   - Do not change `logs_query_status.h` for this SOW.
   - Create `systemd-journal-function.h` for LQS prerequisites and the shared `struct lqs_extension`.
   - Create `systemd-journal-execute.h` for the static query execution template that depends on `LOGS_QUERY_STATUS`.
   - Keep source parsers, default facets, default histogram, Function name, Function description, and transformation registration in wrapper source files.
   - Keep `systemd-journal.c` as the existing `systemd-journal` wrapper.
   - Create `snmp-traps.c` in `src/collectors/systemd-journal.plugin/` by copying `systemd-journal.c` and adapting only the wrapper-local parts.
   - Instantiate `logs_query_status.h` plus the journal execution template once for `systemd-journal` in `systemd-journal.c` with the existing systemd macros.
   - Instantiate `logs_query_status.h` plus the journal execution template once for `snmp-trap` in `snmp-traps.c` with trap-specific Function name, description, source parser, source-list callback, default facets, default histogram, and transformations.
   - Keep `windows-events` untouched.
   - The shared journal core must not depend on Function-specific LQS macros for info/help/source-list output. Those stay in the per-Function wrapper files.
   - Because `logs_query_status.h` and the journal execution template are macro-instantiated, they must not be included twice with different macros in the same translation unit.
   - Before adding `snmp-trap`, make sure the reusable headers do not force the systemd source parser, default facets, default histogram, Function name/help text, or transformation registration onto the trap Function.

5. Journal Function wrapper model:
   - Keep one wrapper source file per public Function.
   - `systemd-journal.c` owns the existing `systemd-journal` Function.
   - `snmp-traps.c` owns the new `snmp-trap` Function.
   - Wrapper-local responsibilities:
     - Function name and description macros.
     - Function tags and timeout passed at registration.
     - default facet include/exclude/always-visible strings.
     - default histogram field.
     - source parser and source-list callback.
     - DynCfg id/path/capabilities.
     - root owner id for the root manager.
     - transformation registration.
   - Shared header-template responsibilities:
     - `LOGS_QUERY_STATUS`-dependent journal query execution.
     - progress/cancellation handling.
     - row parsing and sampling integration.
   - The wrappers may ignore callback `data` if all Function-specific configuration is static in the wrapper source file.

6. View mask:
   - Add a separate internal view mask type, not a new `SD_JOURNAL_FILE_SOURCE_TYPE` value.
   - Proposed shape:
     - `typedef uint64_t ND_JOURNAL_VIEW_MASK;`
     - `ND_JOURNAL_VIEW_NONE = 0`
     - `ND_JOURNAL_VIEW_SYSTEMD = (1ULL << 0)`
     - `ND_JOURNAL_VIEW_SNMP_TRAP = (1ULL << 1)`
   - `1ULL` is required if the design keeps the user's 64-bit capacity goal; using plain `(1 << n)` would overflow for high bits on common C targets.
   - Add `ND_JOURNAL_VIEW_MASK view_mask` to `struct nd_journal_file`.
   - Add `ND_JOURNAL_VIEW_MASK view_mask` to the root record that replaces `struct journal_directory`.

7. Root manager:
   - Replace direct mutation of `journal_directories[]` with a root manager.
   - The root manager stores records with path, owner id, view mask, and root kind.
   - The root manager exposes immutable snapshots to scanner and watcher code.
   - DynCfg updates validate and replace only the roots owned by that DynCfg node, then bump a generation and restart the single watcher.
   - Watcher startup consumes the union of all active root paths from a snapshot.
   - `journal_data_directories_exist()` becomes a root-manager startup policy check: the plugin starts when at least one enabled view has an existing root, and a missing trap root must not disable normal `systemd-journal`.
   - Trap-only startup support requires the trap root to exist before plugin startup. If trap-only systems must work before any trap job runs, packaging or startup code must create the trap cache parent directory.

8. DynCfg ownership:
   - Keep `systemd-journal:monitored-directories` at `/logs/systemd-journal`, with current GET/UPDATE behavior and validation.
   - Add a separate `snmp-trap:monitored-directories` node at `/logs/snmp-trap`.
   - Recommended first implementation: `snmp-trap` DynCfg is SCHEMA/GET only and reports the Netdata-owned trap root, because Go SNMP trap jobs own journal creation under `<cache>/traps/<job>`.
   - If writable `snmp-trap` roots are required, that is a product decision and expands validation because operator-supplied roots can overlap system roots and expose arbitrary compact journals as traps.
   - The C plugin must derive the trap root from the same cache-dir contract as Go (`<cache>/traps`) and validate fallback behavior when the environment does not export an explicit cache directory.

9. Registry insertion and conflict handling:
   - The scan temporary dictionary must store path plus discovered view mask, not only path.
   - Recursive scan receives the root view mask; if the same file is found through multiple roots, conflict handling ORs view masks.
   - Registry insert initializes `view_mask` and then performs existing source classification.
   - Registry conflict handling ORs `view_mask`, updates timestamps and size, and preserves existing header-cache behavior.
   - For trap files, default source classification must not make `snmp-trap info` list system journal sources. Recommended first implementation exposes only `all` as the trap source selector; trap-specific filtering should happen through `TRAP_*` facets.

10. Registry deletion and rescan semantics:
   - Directory watch removal must remove inotify watches only; it must not directly delete all registry entries under the path.
   - File deletion can delete the registry entry only when `stat()` confirms the file path is gone from the filesystem. If the file still exists and only root ownership changed, recompute the view mask instead.
   - Root removal or overlap changes must recompute the file's view mask from the current root snapshot.
   - A registry item is deleted only when the recomputed view mask is zero or the file no longer exists.
   - Full rescan treats the discovered mask as authoritative for the current root snapshot. Files not discovered by any root get deleted; files discovered by fewer roots have their mask reduced, not blindly removed.
   - Root-manager updates trigger watcher restart and full rescan, matching the existing restart-heavy behavior rather than introducing incremental root mutation in the first implementation.
   - Watcher pending events carry the snapshot generation that discovered them. If root generation changes before pending events are processed, discard pending entries and force the post-restart full rescan instead of inserting stale-mask files.

11. Query path:
    - Do not add a journal instance pointer to the shared `LOGS_QUERY_STATUS` struct and do not change the `windows-events` `struct lqs_extension`.
    - Move the journal-specific `struct lqs_extension` to a journal-only header used by both journal Function wrapper source files before including `logs_query_status.h`.
    - Keep `LQS_SOURCE_TYPE` and the journal-specific `struct lqs_extension` layout identical in both wrapper source files so any shared query core sees one compatible journal query shape.
    - Preferred first implementation uses `systemd-journal-function.h` and `systemd-journal-execute.h` as private static journal Function templates included by both wrapper source files after the Function-specific macros are defined.
    - Pure helpers that do not depend on LQS or Function macros may move to a normal `.c` file later, but the first implementation can keep them in the static template if they are immutable and low risk.
    - If a compiled shared journal core is used instead, it must not call macro-bound LQS helpers such as `lqs_info_response()`, `lqs_function_help()`, or source-list callbacks. Those stay in the wrapper source files.
    - Add an early `view_mask` check in `jf_is_mine()` before existing source-type and custom-source filtering.
    - Keep existing `SD_JOURNAL_FILE_SOURCE_TYPE` semantics unchanged for `systemd-journal`.
    - `systemd-journal all` means all files visible to the systemd view, not all files in the global registry.
    - `snmp-trap all` means all files visible to the trap view.
    - Progress and cancellation stay in the common query loop.

12. Source enumeration and info/help output:
    - Replace `available_journal_file_sources_to_json_array()` with a view-aware implementation plus a compatibility wrapper for `systemd-journal`.
    - `systemd-journal info` lists only systemd-view sources.
    - `snmp-trap info` lists only trap-view sources.
    - LQS info/help output comes from the per-Function wrapper source file that instantiated `logs_query_status.h` with that Function's macros.

13. Facet defaults:
    - Recommended first implementation:
      - Move `TRAP_*` default facets out of `SYSTEMD_KEYS_INCLUDED_IN_FACETS`.
      - Add `SNMP_TRAP_KEYS_INCLUDED_IN_FACETS` with trap fields from the Go writer: `TRAP_REPORT_TYPE`, `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_PDU_TYPE`, `TRAP_VERSION`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS`, `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, and bounded `TRAP_TAG_*` behavior where supported by facets.
      - Use `TRAP_SEVERITY` as the default histogram for `snmp-trap`.
      - Keep `PRIORITY` as the default histogram for `systemd-journal`.
    - If the user wants generic `systemd-journal` to keep trap facets, record that as an explicit product choice because it pollutes a widely used Function's default UI.

14. Compatibility baseline:
    - Before implementation, capture current `systemd-journal info` and a representative query response from a temporary test journal directory.
    - After each implementation batch, compare shape and defaults for `systemd-journal`.
    - Any intentional difference must be recorded in this SOW before shipping.
    - Update the debug path in `systemd-main.c` so direct test/debug calls use the correct wrapper callback for `systemd-journal` and do not depend on callback `data`.
    - Keep `systemd-list-units` out of the journal-view refactor, but validate its Function registration and dispatch still work after the shared dispatcher change.

Implementation batches:

1. Shared safety batch:
   - Function dispatch token-boundary fix.
   - Duplicate Function registration guard, if accepted by implementation constraints.
   - Verify every existing Function command with arguments still dispatches.

2. Watcher and root ownership batch:
   - Root manager with owner/view metadata and immutable snapshots.
   - Watcher startup from the snapshot union of roots.
   - DynCfg owner replacement that restarts the single watcher.
   - Preserve current `systemd-journal` behavior with only the systemd view active.
   - Pre/post compatibility capture for `systemd-journal`.

3. Registry mask batch:
   - Per-file view masks.
   - Mask-aware scan, conflict, deletion, and watcher pending processing.
   - Overlapping-root tests.
   - No `snmp-trap` public registration yet unless `systemd-journal` compatibility holds.

4. Function wrapper batch:
   - Per-Function journal wrapper source files that instantiate `logs_query_status.h`, `systemd-journal-function.h`, and `systemd-journal-execute.h` without modifying `logs_query_status.h`.
   - Wrapper-local source parser, default facets, default histogram, Function name/help text, and transformation registration.
   - `systemd-journal` wrapper wired with current defaults.
   - Verify `windows-events` remains untouched by the LQS strategy.

5. `snmp-trap` Function batch:
   - `snmp-traps.c`, trap facets, source enumeration, registration, and read-only DynCfg.
   - Trap root discovery from cache dir.
   - Query validation against compact trap journal files.

6. Product artifact batch:
   - Specs, docs, operator skill updates, SOW validation, reviewer iteration, and final cleanup.

Complexity estimate:

- Overall risk: high, because this touches a widely used Logs Function, shared Function dispatch, the journal watcher, and the journal file registry.
- Expected source touch: approximately 9-12 source/header files before docs and tests.
- Highest-risk code: `systemd-journal.c` splitting/core extraction, `systemd-journal-files.c`, and `systemd-journal-watcher.c`.
- Medium-risk code: `functions_evloop.c`, `systemd-journal-dyncfg.c`, `systemd-main.c`, new journal Function wrapper files, and build-system source lists.
- Lower-risk code: docs/spec/operator skill updates after behavior is stable.
- This is not a small "add one watched directory" change. It is a controlled refactor of Function identity, root ownership, registry lifetime, and query visibility.

Additional risks after choosing the two-wrapper LQS design:

- Static-core extraction risk: many current helpers in `systemd-journal.c` are static and locally ordered. Splitting them can accidentally duplicate state, expose private symbols, or change initialization order.
- LQS type-coupling risk: the two wrapper source files must agree on `LQS_SOURCE_TYPE` and `struct lqs_extension` layout. A silent divergence can make a shared journal core unsafe.
- Macro-bound helper leakage risk: shared core code that calls `lqs_info_response()`, `lqs_function_help()`, source parsing, or source-list helpers can emit the wrong Function name, help text, or source list.
- Shared-header risk: `LOGS_QUERY_STATUS` is produced by `logs_query_status.h`, so common headers should not expose fields or callbacks that require `LOGS_QUERY_STATUS` unless they are included after the wrapper's LQS instantiation.
- Header-template risk: using `systemd-journal-function.h` plus `systemd-journal-execute.h` avoids an LQS API change, but duplicates generated static functions in each wrapper translation unit and can duplicate static state if mutable state is left in the headers.
- Copy-adapt wrapper risk: `snmp-traps.c` should duplicate only wrapper-local configuration from `systemd-journal.c`. Copying too much systemd-specific facet/default/source behavior would make the new Function look separate while still behaving like generic system logs.
- Prototype-specific risk: the cleaned prototype still has one important macro-coupling point: `systemd-journal-execute.h` uses `ND_SD_JOURNAL_FUNCTION_DESCRIPTION` for response help. The `snmp-trap` wrapper must either define this macro to the trap description before including the template, or the template should use a neutral wrapper-provided macro. Log prefixes and "systemd journal file" errors are accepted by user decision.
- Build-system risk: new wrapper/core source files must be added to every relevant build list, not only the local CMake path.
- Registration risk: both Functions need explicit `PLUGINSD_KEYWORD_FUNCTION`, `functions_evloop_add_function()`, callback, timeout, access, tags, and DynCfg registration. Missing one path can make the Function visible in one surface but unusable in another.
- Registry lifecycle risk remains the biggest operational risk: any root removal, watcher event, or full rescan that deletes by path instead of recomputing the view mask can hide files from the other Function.
- Source-version risk: keeping one shared `versions.sources` can cause harmless cross-view refreshes. Per-view versions would reduce refresh noise but add state and invalidation complexity.
- Trap source semantics risk: exposing only `all` for `snmp-trap` keeps source UX simple, but operators must use `TRAP_*` facets for device/source filtering.
- Startup/package risk: trap-only startup depends on the trap cache parent directory existing. If packaging/startup does not create it, `snmp-trap` can be unavailable on systems without normal systemd journal roots.

Concrete implementation contracts:

1. Dispatcher helper contract:
   - Add a local helper equivalent to:
     - `static inline bool function_command_matches_name(const char *cmd, const char *name, size_t name_len)`
     - return true only when `strncmp(cmd, name, name_len) == 0` and `cmd[name_len] == '\0' || isspace((uint8_t)cmd[name_len])`.
   - Dispatch semantics are exact command-token matching, not longest-prefix matching. A registered name matches only the first token exactly.
   - `function` and `function_ab` registered together must dispatch `function_ab info` to `function_ab`, because `function` does not match when the next byte is `_` or another non-whitespace byte.
   - `functions_evloop_add_function()` should detect duplicate registered names, log an error, and ignore the later duplicate registration. Existing behavior for non-duplicates must remain unchanged.
   - In `worker_add_job()`, after a matching expectation queues or rejects a job for that transaction, stop scanning the expectation list with `break`.
   - If no expectation matches by token boundary, keep the current "No function with this name found" behavior.

2. LQS header-template contract:
   - Do not edit `src/libnetdata/facets/logs_query_status.h` for this design.
   - The prototype change from `const char *value` to `char *value` inside `logs_query_status.h` may be harmless because GET parsing mutates the keyword buffer while splitting comma-separated sources, but it is not required for the header-template design. Keep it only if a compiler warning or correctness issue proves it necessary.
   - Create or keep one wrapper source file for `systemd-journal` that:
     - defines `LQS_FUNCTION_NAME` as `ND_SD_JOURNAL_FUNCTION_NAME`;
     - defines the existing systemd description, source parser, source-list callback, defaults, and source type macros;
     - includes `logs_query_status.h` exactly once;
     - includes `systemd-journal-execute.h` exactly once after `logs_query_status.h`;
     - owns the `systemd-journal` instantiation of info/help handling and data queries.
   - Create one wrapper source file for `snmp-trap` that:
     - defines `LQS_FUNCTION_NAME` as `snmp-trap`;
     - defines the trap description, trap source parser, trap source-list callback, trap defaults, and the same journal source type macros;
     - includes `logs_query_status.h` exactly once;
     - includes `systemd-journal-execute.h` exactly once after `logs_query_status.h`;
     - owns the `snmp-trap` instantiation of info/help handling and data queries.
   - Keep `windows-events` untouched. It should continue using its existing macro instantiation path.
   - Both journal wrappers must use the same `LQS_SOURCE_TYPE` and the same journal-specific `struct lqs_extension` layout. If this cannot be maintained cleanly, stop before implementation because the common journal query core will be type-fragile.
   - `systemd-journal-execute.h` may call macro-bound LQS helpers, because it is instantiated separately per Function after the macros are set.
   - `systemd-journal-execute.h` and `systemd-journal-function.h` must not define mutable `static` process state. Mutable shared state belongs in normal `.c` files with explicit symbols, locks, or registries.
   - Header-template functions that emit Function help/description must use Function-specific macros or neutral wording. Internal logs and journal-file errors may keep systemd-journal wording by user decision.
   - Transformation registration must stay wrapper-specific, because trap rows should not inherit only systemd severity/facet transformations.
   - Function-specific source parsers should stay in the wrapper source file before `logs_query_status.h` inclusion, as in the cleaned prototype.
   - Runtime-configurable LQS remains a fallback only if the copied-wrapper/template approach proves unmaintainable or maintainers reject the static header-template pattern.

3. Journal Function wrapper contract:
   - Keep Function-specific configuration static in each wrapper source file.
   - Minimum wrapper-local items:
     - Function name macro.
     - Function description macro.
     - Function tags, timeout, and access at registration.
     - `ND_JOURNAL_VIEW_MASK view_mask` or equivalent view ownership value for file filtering.
     - always-visible/facet/non-facet key strings.
     - default histogram.
     - source parser and source-list callback.
     - transformation registration function.
     - `JOURNAL_ROOT_OWNER root_owner`.
   - The existing callback `void *data` does not need to carry Function configuration if the wrapper source file provides all Function-specific behavior statically.
   - Keep LQS info/help metadata in the wrapper source files, unless the implementation later chooses the runtime-config fallback.
   - Hardcoded internal log prefixes and "systemd journal file" query errors may remain systemd-journal oriented by user decision.

4. Root manager contract:
   - Replace the public global array as the source of truth with a root manager protected by a mutex or rwlock.
   - Use static owner ids:
     - `JOURNAL_ROOT_OWNER_SYSTEMD`
     - `JOURNAL_ROOT_OWNER_SNMP_TRAP`
   - Use static root kinds:
     - default system root
     - host-prefixed system root
     - DynCfg root
     - Netdata trap cache root
   - Root record fields:
     - `STRING *path`
     - `JOURNAL_ROOT_OWNER owner`
     - `ND_JOURNAL_VIEW_MASK view_mask`
     - root kind
     - `bool required_for_startup`
   - Snapshot record fields:
     - generation id
     - number of roots
     - array copy of root records with retained path strings.
   - Pseudo-C shape:
     - `typedef enum { JOURNAL_ROOT_OWNER_SYSTEMD, JOURNAL_ROOT_OWNER_SNMP_TRAP } JOURNAL_ROOT_OWNER;`
     - `typedef enum { JOURNAL_ROOT_KIND_DEFAULT_SYSTEM, JOURNAL_ROOT_KIND_HOST_PREFIXED_SYSTEM, JOURNAL_ROOT_KIND_DYNCFG, JOURNAL_ROOT_KIND_TRAP_CACHE } JOURNAL_ROOT_KIND;`
     - `struct journal_root { STRING *path; JOURNAL_ROOT_OWNER owner; JOURNAL_ROOT_KIND kind; ND_JOURNAL_VIEW_MASK view_mask; bool required_for_startup; };`
     - `struct journal_roots_snapshot { uint64_t generation; size_t used; struct journal_root *roots; };`
   - Required APIs:
     - initialize default roots.
     - replace all roots for one owner after DynCfg validation.
     - acquire and release a snapshot.
     - recompute a path view mask from the current root set.
     - test whether any enabled view has an existing startup root.
   - Suggested API names:
     - `nd_journal_roots_init_defaults()`
     - `nd_journal_roots_replace_owner(JOURNAL_ROOT_OWNER owner, const struct journal_root *roots, size_t count)`
     - `nd_journal_roots_snapshot_acquire()`
     - `nd_journal_roots_snapshot_release(struct journal_roots_snapshot *snapshot)`
     - `nd_journal_roots_mask_for_path(const char *path)`
     - `nd_journal_roots_any_startup_root_exists(ND_JOURNAL_VIEW_MASK enabled_views)`
   - The old `journal_directories[]` array can remain temporarily as a compatibility wrapper only if all mutations go through the root manager.

5. Scanner contract:
   - Change recursive scan signature to include a view mask:
     - `nd_journal_directory_scan_recursively(files, dirs, dirname, depth, view_mask)`.
   - Update every call site in the same batch, including the full-scan caller in `systemd-journal-files.c` and the watcher recursion path that currently calls this helper while discovering subdirectories.
   - The temporary `files` dictionary value must hold at least:
     - discovered view mask.
     - stat-derived size and modification time if already available.
   - The temporary dictionary conflict callback ORs discovered view masks when the same path appears through multiple roots.
   - Full scan builds discovered masks from all roots in one snapshot, then updates each registry item with the discovered mask.
   - Registry conflict callback must explicitly OR masks:
     - `njf_old->view_mask |= njf_new->view_mask;`
   - Timestamp, size, and header update behavior remains otherwise equivalent to the current `files_registry_conflict_cb()`.

6. Watcher contract:
   - Watcher startup takes a root snapshot and watches the union of paths in that snapshot.
   - Watcher startup must not read `journal_directories[]` directly. It must call `nd_journal_roots_snapshot_acquire()`, iterate the snapshot, and release the snapshot when the watcher session exits.
   - Pending file events store the path and the snapshot generation.
   - `process_pending()` recomputes view mask from the current root manager before inserting or updating a registry item.
   - If pending event generation is stale, discard pending entries and let the post-restart full scan repair the registry.
   - `remove_directory_watch()` removes inotify watches only; registry cleanup is handled by recompute/full scan.

7. DynCfg registration contract:
   - Register both Function metadata and DynCfg metadata explicitly:
     - `functions_evloop_add_function()` for `systemd-journal`.
     - `functions_evloop_add_function()` for `snmp-trap`.
     - `PLUGINSD_KEYWORD_FUNCTION` line for `systemd-journal`.
     - `PLUGINSD_KEYWORD_FUNCTION` line for `snmp-trap`.
     - `functions_evloop_dyncfg_add()` for `systemd-journal:monitored-directories`.
     - `functions_evloop_dyncfg_add()` for `snmp-trap:monitored-directories`.
   - `systemd-list-units` is outside the journal-view refactor and should keep its existing Function metadata and callback.

8. Trap source contract:
   - `snmp-trap` uses a dedicated source-to-mask callback that recognizes only `all` for the first implementation.
   - `snmp-trap` source enumeration returns only `all` for the first implementation.
   - Existing path-based source classification can still classify trap files internally as local/other, but that classification must not be exposed by `snmp-trap info` and must not affect trap queries because the view mask is applied first.
   - Operators should use `TRAP_*` facets, not `__logs_sources`, for trap device/source filtering.

9. Startup semantics contract:
   - Missing trap root while normal systemd roots exist: plugin starts and `systemd-journal` behaves as today; `snmp-trap` returns empty/no data until trap files exist.
   - Existing trap root while no systemd roots exist: plugin starts if the user approves trap-only startup in Open Decision 6.
   - No systemd roots and no trap root: plugin keeps existing `DISABLE` behavior.
   - If trap-only startup is required before any job creates `<cache>/traps`, installer/packaging must create the trap cache parent directory.

10. View versioning contract:
   - Current `buffer_json_journal_versions()` reports one global `versions.sources` based on `systemd_journal_session + dictionary_version(nd_journal_files_registry)`.
   - Recommended first implementation keeps this shared version. A trap-only file change may cause a spurious `systemd-journal` source-version refresh, but this is safer than missing updates and avoids adding per-view version bookkeeping to the first refactor.
   - If the user requires no cross-view source-version refreshes, add per-view source versions as a separate explicit design choice before implementation.

Validation plan:

- Adversarial external review of this SOW before implementation.
- Static same-failure searches around Function registration, prefix dispatch, DynCfg path/node ownership, and watcher deletion.
- Build checks for Linux `systemd-journal.plugin` and a no-diff/no-touch check for `logs_query_status.h` and `windows-events`.
- Cross-plugin scan of all `functions_evloop_add_function()` registrations and validation that token-boundary dispatch does not regress existing Function commands.
- Dispatch validation must include exact-token cases: `function`, `function_ab`, `function info`, `function_ab info`, and duplicate registration handling.
- Automated or scripted compatibility capture for current `systemd-journal` info/help output before code changes.
- Build `systemd-journal.plugin`.
- Compare pre-change and post-change `systemd-journal info` output for required params, table config, default facets, help text, source list, and response structure.
- Compare pre-change and post-change `systemd-journal` query output against a test journal directory.
- Verify cancellation and progress still work by exercising a query over multiple files.
- Verify DynCfg updates for `systemd-journal` do not affect `snmp-trap`, and vice versa.
- Verify overlapping roots OR masks and deletion only removes a file when no Function/view bit remains.
- Verify a missing trap directory does not disable `systemd-journal`.
- Verify a `snmp-trap` DynCfg update during an active `systemd-journal` query does not crash, corrupt JSON, hide system journal files, or change `systemd-journal` results outside documented directory changes.
- Verify source enumeration for each Function only includes sources visible to that Function/view.
- Verify shared `versions.sources` behavior is accepted: trap-only file changes may bump the source version visible to `systemd-journal`, but must not expose trap files or trap source names in `systemd-journal`.
- Verify Function names with shared prefixes are rejected or dispatched exactly, depending on the approved dispatcher design.
- Verify `git diff -- src/libnetdata/facets/logs_query_status.h src/collectors/windows-events.plugin` is empty. If a build path is available, also verify `windows-events` still compiles and can still produce info output with its unchanged macro-default LQS path.
- Verify `systemd-journal-execute.h` is included only by wrapper source files after the required LQS macros and `logs_query_status.h` are included.
- Verify `systemd-journal-function.h` and `systemd-journal-execute.h` contain no mutable `static` process state.
- Verify reusable headers do not hardcode systemd-only Function help text, default histogram, default facets, or transformation registration into `snmp-trap`; source parsers should remain wrapper-local. Internal log prefixes and journal-file error wording are allowed to remain systemd-journal oriented by user decision.
- Verify any `logs_query_status.h` change is independently justified. The preferred design still leaves that shared header unchanged unless a narrow warning/correctness fix is required.
- Verify no production directories are used by tests; test journals and trap journals must live under temporary directories.
- Verify debug mode in `systemd-main.c` calls the correct `systemd-journal` wrapper and does not require callback data.
- Verify `systemd-list-units` Function metadata and dispatch still work after the shared dispatcher change.
- Verify `snmp-trap info` returns the `snmp-trap` Function name, trap description, only trap-view sources, and the `TRAP_SEVERITY` default histogram.
- Verify `snmp-trap help` returns trap-specific help text rather than `systemd-journal` text.
- Verify `snmp-trap` query against temporary compact trap journal files returns trap entries with `TRAP_*` facets.
- Verify `snmp-trap:monitored-directories` DynCfg is read-only in the recommended first implementation and reports the expected trap root.
- Verify `TRAP_*` defaults are removed from generic `systemd-journal` if Open Decision 4 selects option A.
- Overlapping-root scenarios:
  - same temporary directory registered for both views gives files both view bits;
  - removing one root keeps the other bit;
  - deleting a file from disk removes the registry entry;
  - updating systemd DynCfg during a trap query does not crash or corrupt JSON;
  - missing trap root at startup does not disable `systemd-journal`;
  - existing trap root without systemd roots follows the approved startup decision.

Artifact impact plan:

- AGENTS.md: no expected update unless this design establishes a new project-wide SOW/process rule.
- Runtime project skills: update `project-writing-collectors` only if implementation exposes a reusable Function-authoring workflow rule.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` if `snmp-trap` becomes a first-class Logs Function.
- End-user/operator docs: update SNMP trap docs if the `snmp-trap` Function ships.
- End-user/operator skills: update `query-snmp-traps` if the operator query path changes from `systemd-journal` to `snmp-trap`.
- SOW lifecycle: this SOW remains pending/open until approved. If it becomes required for SOW-0039 merge readiness, record the dependency explicitly in both SOWs before implementation.

Open-source reference evidence:

- None checked for this SOW creation. This design is specific to Netdata's plugin Function event loop, DynCfg, and systemd-journal watcher implementation.

Open decisions:

1. Go/no-go:
   - A. Implement this before SNMP trap merge readiness.
   - B. Defer this and keep trap querying through the current `systemd-journal` route for the first release.
   - Recommendation: A only if the user accepts a high-risk, multi-batch refactor. B is simpler, but gives a weaker operator UX and leaves trap facets in a generic logs Function.

2. Trap visibility in generic `systemd-journal`:
   - A. Exclude trap roots from the `systemd-journal` view once `snmp-trap` exists.
   - B. Include trap roots in `systemd-journal all` as well as `snmp-trap all`.
   - Recommendation: A. It preserves the existing generic systemd logs mental model and avoids surprising operators.

3. Trap DynCfg:
   - A. Add a separate read-only `snmp-trap:monitored-directories` DynCfg node showing the Netdata-owned trap root.
   - B. Add a writable `snmp-trap:monitored-directories` node for operator-supplied compact journal roots.
   - Recommendation: A for the first implementation. B expands the security and validation surface.

4. Trap facets in generic `systemd-journal`:
   - A. Move `TRAP_*` defaults to the `snmp-trap` Function only.
   - B. Keep `TRAP_*` defaults in generic `systemd-journal`.
   - Recommendation: A. Generic `systemd-journal` should not get trap-specific default UI.

5. Framework scope:
   - A. Implement the minimum reusable internal shape for `systemd-journal` and `snmp-trap`.
   - B. Build a fully generic virtual Function framework now.
   - Recommendation: A. B increases risk without a second real Function family to prove the abstraction.

6. Trap-only startup:
   - A. Start `systemd-journal.plugin` when only the trap root exists, even if no systemd journal roots exist.
   - B. Keep the current plugin startup model: if no normal systemd journal roots exist, the plugin disables even if trap support is configured.
   - Recommendation: A if `snmp-trap` is intended to work on systems where Netdata can receive traps but systemd journals are absent. This requires installer/packaging or startup code to ensure the trap cache parent directory exists.

7. Source versioning:
   - A. Keep one shared `versions.sources` value from the shared registry.
   - B. Add per-view source versions so trap-only changes do not refresh `systemd-journal` source metadata.
   - Recommendation: A for the first implementation. It may cause harmless extra refreshes, but it avoids subtle per-view version bookkeeping in the same refactor.

## Implications And Decisions

1. Function/view mask style:
   - Proposed decision: use Netdata's existing bitmask enum pattern with direct bit values and no helper macro.
   - Rationale: this matches `SD_JOURNAL_FILE_SOURCE_TYPE`, `DYNCFG_CMDS`, dictionary options, and other local patterns.
   - Implication: do not introduce a separate `BIT(id)` helper macro for this design. Because the proposed view mask is 64-bit, use direct `1ULL << n` values for the view enum to avoid overflow.

2. DynCfg ownership:
   - Proposed decision: each virtual Function owns its own DynCfg node.
   - Benefit: prevents `systemd-journal` directory changes from implicitly rewriting `snmp-trap` roots, and vice versa.
   - Risk: requires a shared root manager so independent DynCfg nodes can still feed one watcher safely.

3. Watcher ownership:
   - Proposed decision: keep exactly one watcher and one file registry.
   - Benefit: avoids duplicate inotify watches and registry races.
   - Risk: watcher delete/update paths must become mask-aware.

4. `systemd-journal` compatibility:
   - Proposed decision: existing `systemd-journal` behavior must be treated as a compatibility contract.
   - Benefit: protects existing users.
   - Risk: increases validation scope and makes implementation slower, but this is necessary.

## Plan

1. Keep this SOW pending/open until the user gives a go/no-go decision.
2. Do not start implementation until the open decisions are recorded.
3. If approved, move this SOW to `current/` and implement in the batches listed above.
4. Run external reviewers only after a complete SOW or complete implementation batch is finished, not while drafting partial sections.

## Execution Log

### 2026-05-28

- Created SOW for virtual Function design review.
- Recorded user constraint to follow Netdata direct `(1 << n)` bitmask enum style.
- No implementation started.
- Ran adversarial read-only review with `glm`, `kimi`, `minimax`, and `qwen`.
- Reviewers were explicitly asked to identify why the design is a bad idea and how existing `systemd-journal` behavior may break.
- Consolidated reviewer blockers into this SOW:
  - prefix Function dispatch must be resolved;
  - `logs_query_status.h` macro-template reuse must be resolved;
  - monolithic callback configuration must become explicit;
  - registry insertion/update/delete/rescan must be mask-aware;
  - source enumeration must be Function/view-aware;
  - `systemd-journal all` and generic default facets must not be polluted by trap data unless explicitly approved.
- Expanded the SOW into a ready-to-implement design with implementation batches, complexity estimate, validation plan, and user decision points.
- Ran a follow-up read-only reviewer pass with `glm`, `kimi`, `minimax`, and `qwen` against the ready-to-implement draft.
- Follow-up reviewers agreed the direction is viable but found the draft was not yet concrete enough.
- Folded follow-up findings into the SOW:
  - exact dispatcher boundary helper and required `break`;
  - LQS per-Function source-file contracts, with runtime config kept only as a fallback;
  - root manager structs, snapshot API, scanner signature, and watcher generation rules;
  - startup semantics and trap-only startup decision;
  - explicit Function and DynCfg streaming registration tasks;
  - debug path and `systemd-list-units` validation;
  - trap source behavior, trap-specific acceptance criteria, and overlapping-root tests.

### 2026-05-29

- Stopped external reviewer looping based on user direction.
- Adopted the workflow rule for this SOW: work independently until a whole SOW or whole implementation batch is complete, then run one full external review pass if needed.
- Added the final scanner call-site clarification: both full scan and watcher recursion call sites must be updated when the recursive scan signature gains a view mask.
- Updated the SOW after the user challenged the `logs_query_status.h` runtime-config assumption: preferred design now leaves `logs_query_status.h` unchanged and uses one wrapper source file per journal Function.
- Added the shifted risks from that decision: static-core extraction, LQS type coupling, macro-bound helper leakage, build-system coverage, Function registration completeness, and trap-only startup packaging.
- Recorded the user-proposed simplification: `systemd-journal-function.h` and `systemd-journal-execute.h` as macro-instantiated static journal Function templates, with wrapper-local configuration in `systemd-journal.c` and future `snmp-traps.c`.
- Checked the current worktree. The proposed header refactor is not visible in `src/collectors/systemd-journal.plugin`; the only visible tracked plugin source diff is adding `TRAP_*` fields to `SYSTEMD_KEYS_INCLUDED_IN_FACETS` in `systemd-journal.c`.
- Attempted to build `systemd-journal.plugin`, but the existing `build/` directory is owned by root and Ninja could not create `.ninja_lock` or update `.ninja_log`.
- Rechecked the sibling prototype after cleanup. Source parsing moved back to the wrapper source file, which matches the desired template boundary. `systemd-journal.c` passes a non-writing `-fsyntax-only` compile using the recorded `compile_commands.json` flags, but full Ninja build remains blocked by root-owned build files.
- Recorded the refined implementation direction: create `snmp-traps.c` in `systemd-journal.plugin` by copying `systemd-journal.c` and adapting wrapper-local Function identity, source parser, facets, histogram, and transformations. Internal `SYSTEMD-JOURNAL` log prefixes and "systemd journal file" errors are explicitly accepted because the plugin and files remain systemd-journal based.
- Marked this SOW decision-ready and left it pending/open because implementation is not yet approved.

## Validation

Acceptance criteria evidence:

- Ready-to-implement design drafted, revised after adversarial review, and finalized for user go/no-go. No implementation has started.

Tests or equivalent validation:

- Not run. This is a design-review SOW; no source code implementation has started.

Real-use evidence:

- No real-use evidence yet because no implementation has started. Real-use validation is listed in the validation plan for any future implementation.

Reviewer findings:

- `glm`: Blocked implementation until exact Function dispatch or enforced non-overlapping names is designed; identified the LQS macro-template pattern and path-destructive watcher deletion as mandatory design problems.
- `kimi`: Blocked implementation until dispatcher prefix matching, mask-aware registry deletion, `logs_query_status.h` reuse, and DynCfg node ownership are specified. Recommended concrete tests for prefix routing, missing trap directory behavior, and independent DynCfg updates.
- `minimax`: Blocked implementation until Function dispatch, `logs_query_status.h` instantiation, monolithic callback configuration, and registry deletion lifecycle are resolved. Recommended scoping down from a generic framework to the minimum safe SNMP trap Function.
- `qwen`: Blocked implementation until dispatcher prefix matching, unconditional watcher deletion, LQS macro reuse, `systemd-journal all` trap inclusion, and full-rescan deletion semantics are resolved. Recommended adding an explicit per-file Function/view mask and mask-aware deletion.
- Follow-up `glm`: Not ready until dispatcher `break`, startup semantics, `lqs_extension` placement, Function streaming registration, watcher restart race, trap source classification, and trap facet acceptance criteria are explicit. These were folded into the concrete contracts and validation plan.
- Follow-up `kimi`: Not ready until concrete struct definitions, LQS reuse strategy, root manager snapshot API, scanner signature, dispatch validation, startup behavior, and generic query error messages are specified. These were folded into the concrete contracts.
- Follow-up `minimax`: Not ready until the LQS help/info strategy, token-boundary function details, and macro-default compatibility are concrete. These were folded into the dispatcher and LQS contracts.
- Follow-up `qwen`: Not ready until `journal_data_directories_exist()`, `process_pending()`, full scan mask data, `systemd-list-units`, debug path, and `snmp-trap` acceptance criteria are explicit. These were folded into the contracts and validation plan.
- Final partial review pass before user stopped reviewer looping: available completed final reviews reported no remaining blockers; one review was stopped per user direction and no further reviewer looping is required for this decision-ready SOW.

Same-failure scan:

- `rg` found current bitmask style uses direct `(1 << n)` enum values in `src/collectors/systemd-journal.plugin/systemd-internals.h`, `src/libnetdata/inicfg/dyncfg.h`, dictionary options, and daemon service flags.
- `rg` found existing Function registration paths using `functions_evloop_add_function()` in several plugins; the shared dispatcher behavior must be treated as cross-plugin risk, not only a systemd-journal detail.

Sensitive data gate:

- SOW contains no raw secrets, credentials, bearer tokens, SNMP communities, customer names, personal data, public customer-identifying IPs, private endpoints, or live trap payloads.

Artifact maintenance gate:

- AGENTS.md: no update needed for SOW creation.
- Runtime project skills: no update needed before design approval.
- Specs: no update needed before design approval.
- End-user/operator docs: no update needed before design approval.
- End-user/operator skills: no update needed before design approval.
- SOW lifecycle: new pending/open SOW created at `.agents/sow/pending/SOW-0044-20260528-systemd-journal-virtual-functions.md`.

Specs update:

- No spec update yet; no behavior changed.

Project skills update:

- No project skill update yet; no workflow changed.

End-user/operator docs update:

- No docs update yet; no behavior changed.

End-user/operator skills update:

- No operator skill update yet; no behavior changed.

Lessons:

- The bitmask owner model may be viable, but it is not the hard part. The hard parts are dispatch exactness, LQS instantiation, and registry lifecycle under overlapping roots.
- Separate wrapper source files are the preferred way to carry Function-specific configuration while keeping macro-generated LQS pieces in the correct translation unit.

Follow-up mapping:

- No implementation follow-up is created yet; this SOW remains pending/open until the user decides whether to proceed.

## Outcome

Pending.

## Lessons Extracted

- Treat `systemd-journal` behavior as a compatibility contract, not as an implementation detail.
- For virtual Functions, source filtering and Function/view ownership must remain separate concepts.
- Reviewer pass confirmed that a simple "add a mask" implementation would underestimate the blast radius.

## Followup

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
