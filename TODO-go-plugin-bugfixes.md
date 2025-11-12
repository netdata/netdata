# TL;DR
- Fix seven regressions reported in go.d/scripts.d layers: chart TypeOverride guard, vnode label copying, job ID sanitization, perfdata unit scaling, resource usage scaling, scheduler state exposure, and long-output whitespace handling.
- Root causes confirmed via source review; each change needs clear expectations around behavior (e.g., bit-vs-byte units, underscore handling) before coding.
- Costa approved Option A for every decision, so implementation can proceed per the plan below.

# Analysis
## 1. Chart creation guard ignores TypeOverride (src/go/plugin/go.d/agent/module/job.go)
- `processMetrics` builds `typeID := fmt.Sprintf("%s.%s", j.FullName(), chart.ID)` around line 460 to enforce `NetdataChartIDMaxLength`.
- `createChart` later emits charts via `TypeID: getChartType(chart, j)` and `ID: getChartID(chart)`, which respect `TypeOverride` and custom ID segments.
- Because the guard relies on `FullName()+chart.ID`, overridden `Type` values that would shorten the prefix still trigger the length warning and mark `chart.ignore = true`, leaving valid charts unused.

## 2. VirtualNode.Copy nils Labels (src/go/plugin/go.d/agent/vnodes/vnodes.go)
- Copy allocates the `labels` map only when `len(v.Labels) > 0`; otherwise it returns `nil`.
- The YAML/JSON struct tags for `Labels` lack `omitempty`, so marshaling expects an empty map `{}` for “no labels”. Copying a vnode with `Labels == nil` yields a nil map, leading JSON output to become `null` instead of `{}`, changing the API surface.

## 3. Job ID sanitization trims distinguishing underscores (src/go/plugin/scripts.d/pkg/ids/ids.go)
- `Sanitize` lowercases input, emits underscores for delimiters/punctuation, then runs `strings.Trim(_, "_")` at the end.
- Names that differ only by leading/trailing punctuation (e.g., "Disk usage" vs. "Disk usage /") become indistinguishable once the trailing underscore is trimmed, so `JobKey` collisions merge unrelated jobs/charts/perfdata.

## 4. Bit-vs-byte perfdata scaling conflated (src/go/plugin/scripts.d/pkg/units/scale.go)
- `NewScale` lowercases the unit before calling `byteScale` → `byteMultiplier`.
- Case is the only distinction between megabit ("Mb") and megabyte ("MB"), so both collapse to `multiplier=1_000_000` bytes. Nagios plugins that emit bits/sec are interpreted as bytes/sec, inflating reported throughput by ×8.

## 5. MaxRSS scaling misses BSD (src/go/plugin/go.d/pkg/ndexec/resource_usage.go)
- `convertMaxRSS` multiplies by 1024 only on Linux/Android, assuming BSD/Darwin already report bytes.
- According to the bug report, at least FreeBSD/OpenBSD/NetBSD/DragonFly also emit KiB in `ru_maxrss`, so our value stays 1024× too small on those OSes.

## 6. Soft state overwritten by hard state (src/go/plugin/scripts.d/pkg/runtime/scheduler.go)
- `jobState.recordResult` stores the normalized plugin state in `softState`, updates hard-state tracking when attempts exhausted, then unconditionally sets `js.state = js.hardState` (line ~737).
- Telemetry metrics, macros, and `currentState` read from `js.state`, so during retries the UI keeps reporting the prior hard state (often OK) instead of the current soft failure. Soft-state-only errors stay invisible until they turn hard.

## 7. Long-output trimming removes intentional indentation (src/go/plugin/scripts.d/pkg/output/parser.go)
- Each long-output line already has trailing space removed via `strings.TrimRightFunc`. However, the final `LongOutput` field is wrapped in `strings.TrimSpace` (around line 85).
- That extra trim strips leading whitespace from the concatenated block, flattening indentation on the first line and deviating from the plugin’s intended formatting.

# Decisions (confirmed with Costa)
1. **TypeOverride guard** – Use `getChartType/getChartID` in the guard before enforcing the length limit so overrides are respected.
2. **VirtualNode labels** – Always return at least an empty map for `Labels` in `Copy()` (leave `Custom` untouched) to keep JSON output as `{}`.
3. **IDs.Sanitize disambiguation** – Preserve leading/trailing underscores; only hash when no alphanumerics exist.
4. **Perfdata units** – Preserve case to distinguish bits vs bytes, setting canonical units appropriately (e.g., `bits/s` vs `bytes/s`) and adjusting multipliers accordingly.
5. **MaxRSS scaling** – Treat `freebsd`, `openbsd`, `netbsd`, and `dragonfly` like Linux/Android by multiplying `ru_maxrss` by 1024.
6. **Soft-state exposure** – Let `js.state` reflect the latest soft state while `hardState` keeps tracking the last hard transition.
7. **Long-output whitespace** – Only trim trailing whitespace on the combined long-output payload so leading indentation stays intact.

# Plan
1. Update `processMetrics` to compute the guard key via `getChartType/getChartID`, ensuring TypeOverride/IDSep logic matches the chart creation path.
2. Adjust `VirtualNode.Copy` to always initialize `Labels` to an empty map before copying entries, preventing `nil` JSON output; leave `Custom` untouched unless Costa prefers similar handling.
3. Rework `ids.Sanitize` so it preserves boundary underscores for disambiguation, tracks whether any alphanumeric characters were emitted, and only falls back to hashed IDs when none exist.
4. Revise `units.NewScale`/`byteScale`/`byteMultiplier` to inspect the original unit string, distinguish bits vs bytes based on letter case (and potentially suffixes like `bit`), set canonical units appropriately, and add unit tests covering Mbps vs MBps et al.
5. Extend `convertMaxRSS` to treat BSD `GOOS` values as KiB-based and update the comment accordingly; consider adding a unit test to guard behavior.
6. Change `jobState.recordResult` so `js.state` reflects the current soft state while `hardState` keeps capturing the last hard transition; verify downstream `currentState`/telemetry/macros now represent soft failures.
7. Modify the output parser to only trim trailing whitespace from `LongOutput` (e.g., using `strings.TrimRightFunc` on the joined string) so leading indentation is preserved.
8. Add/adjust unit tests where feasible (ids, units, parser) and rerun targeted Go test suites for affected packages.

# Implied decisions / considerations
- Units package likely needs new tests plus potential updates to canonical-unit strings (`bits/s`?).
- No doc updates seem necessary unless Costa wants the behavior documented for plugin authors.

# Testing requirements
- `go test ./src/go/plugin/go.d/agent/...` for module/job and vnode/resource usage changes.
- `go test ./src/go/plugin/scripts.d/pkg/...` to cover ids/units/output/runtime packages.
- Rerun any existing Nagios plugin integration tests Costa recommends.

# Documentation updates
- None identified so far; please advise if we should document these fixes for plugin authors/operators.
