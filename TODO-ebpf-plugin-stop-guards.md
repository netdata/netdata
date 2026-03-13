# TODO: Propagate ebpf_plugin_stop() Guards to All eBPF Modules

## TL;DR
The VFS module received comprehensive `ebpf_plugin_stop()` checks inside data collection loops
(accumulators, map reads, cgroup iterations, apps loops) in commit c7bf36c. The same pattern
must be propagated to all other eBPF collector modules for consistent, fast shutdown behavior.

## Requirements
> "Propagate the guards to other files, like it is in VFS."

## Facts (from code analysis)

### Pattern applied to VFS (reference):
1. **Accumulator loops** (`for (i = 1; i < end; i++)`) → `if (ebpf_plugin_stop()) break;` at top of loop body
2. **Map read loops** (`while (bpf_map_get_next_key(...))`) → `if (ebpf_plugin_stop()) break;` at top of loop body
3. **Cgroup iteration loops** (`for (ect = ebpf_cgroup_pids; ect; ect = ect->next)`) → same
4. **Apps target loops** (`for (w = root; w; w = w->next)`) → same
5. **Collector/reader loops** → extra `if (ebpf_plugin_stop()) break;` after `mutex_unlock(&lock)` before acquiring `ebpf_exit_cleanup`
6. **Data send functions** → `if (ebpf_plugin_stop()) return;` before expensive operations

### Missing guards per file:

| File | Missing locations |
|------|------------------|
| ebpf_dcstat.c | accumulator (~542), create_apps_charts (~766) |
| ebpf_fd.c | accumulator (~730), update_cgroup (~847), send_apps (~940) |
| ebpf_hardirq.c | map read loop in hardirq_reader (~366) |
| ebpf_shm.c | accumulator (~520), update_cgroup (~546), send_apps (~694) |
| ebpf_socket.c | send_apps (~1100), map read loop (~1805), resume_apps (~1900), update_cgroup (~1940) |
| ebpf_swap.c | accumulator (~504), update_cgroup (~523), resume_apps (~575), send_apps (~738), systemd_charts (~805) |
| ebpf_process.c | send_apps (~378), update_cgroup (~456), accumulator (~1467), map read loop (~1527) |
| ebpf_cachestat.c | accumulator (~769), update_cgroup (~860), create_apps_charts (~1062) |
| ebpf_mdflush.c | map read loop (~225) |
| ebpf_oomkill.c | write_data (~164), map read loop (~383) |

## User Made Decisions
- Propagate the same pattern as VFS to all other modules

## Implied Decisions
- Only add `ebpf_plugin_stop()` guards — no other changes
- Follow the exact same code style as VFS

## Pending Decisions
None.

## Plan
1. ebpf_dcstat.c
2. ebpf_fd.c
3. ebpf_hardirq.c
4. ebpf_shm.c
5. ebpf_socket.c
6. ebpf_swap.c
7. ebpf_process.c
8. ebpf_cachestat.c
9. ebpf_mdflush.c
10. ebpf_oomkill.c

## Testing Requirements
- Build the ebpf.plugin and verify it compiles without errors
- Manual test: run eBPF plugin and send SIGTERM — verify fast, clean shutdown

## Documentation Updates
None required (internal shutdown behavior change only).
