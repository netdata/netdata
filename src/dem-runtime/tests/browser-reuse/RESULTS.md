# Managed browser reuse validation

Measured on 2026-11-11 with the production owner/attempt protocol and real Agent
execution pool. The implementation is `0281faf140`; the integration harness is
`dec2d53522`. These are controlled loopback comparisons, not capacity guarantees
for real websites or a complete Agent workload with RUM traffic.

## Environment and method

- Debian 13, Linux 6.12.111, x86-64 VM with 16 vCPUs on AMD EPYC 7713.
- Each isolated unit had a 16-CPU quota, 32 GiB memory limit, no swap and 4,096-task
  limit. The installed Agent and its runtime were preserved.
- Node 24.20.0, Playwright 1.63.0 and Chromium 154.0.8037.92; identical components
  and contract-2 runtime for fresh and reuse modes.
- Runtime fixture `2026.11.11-contract2-proof2`, archive SHA-256
  `0f6f4efef80dd6c2e64586287b9b8615bdeb3c9119ea2f31d37979f3e7ffb8b6`.
- `engine-proof.go` used real runtime leases, `nd-run`, history and artifact stores.
  Each journey navigated to deterministic local HTML and asserted its heading.
  Lighthouse used the same local page with the production desktop audit path.
- All workload jobs became ready at one barrier. The serial comparison used
  105 journeys and one slot, crossing the 100-loan retirement boundary. The mixed
  comparison used 300 journeys, 100 Lighthouse audits and 12 shared slots.

Unit totals include startup, every attempt, required cleanup and replacement,
final browser retirement and deferred storage closure. CPU is whole-unit systemd
accounting; peak memory includes the complete unit. The harness also reports its
narrower execution interval, first result, per-kind slot time and runtime-use
count. Mixed admission order may vary. Each row is one run, not a repeated-trial
estimate or statistical confidence interval. First-result timings start at the
workload barrier, after common preflight. The serial pair ran fresh then reuse;
the mixed pair ran reuse then fresh.

## Journey-only comparison

| Mode | Whole-unit elapsed | Whole-unit CPU | Peak memory | First result |
| --- | ---: | ---: | ---: | ---: |
| Fresh | 275.164 s | 388.186 s | 449.7 MiB | 2.477 s |
| Reuse | 204.409 s | 318.966 s | 576.4 MiB | 2.975 s |

All 105 journeys passed in each mode. Reuse reduced elapsed time by 25.7% and CPU
by 17.8%, with 28.2% more peak memory. Its first cold result was about half a second
later; the benefit comes from subsequent attempts. Fresh used 106 runtime leases
(one probe and 105 attempts). Reuse used 108 (the probe, 105 attempts and exactly
two owners), confirming that the measured run included busy-owner replacement.

## Mixed journey and Lighthouse comparison

| Mode | Whole-unit elapsed | Whole-unit CPU | Peak memory | First result |
| --- | ---: | ---: | ---: | ---: |
| Fresh | 261.593 s | 1,967.788 s | 4.6 GiB | 5.526 s |
| Reuse | 244.106 s | 1,906.309 s | 5.1 GiB | 12.759 s |

All 400 checks passed in each mode. Reuse reduced elapsed time by 6.7% and CPU by
3.1%, with about 0.5 GiB more peak memory. Journey slot time fell from 1,656.0 to
1,417.0 seconds; Lighthouse slot time was similar at 1,396.5 and 1,399.7 seconds.
This explains the smaller overall benefit when the workload includes audits.
The first mixed result was later with reuse; admission order varies, and these
measurements do not establish faster cold-start latency.

Fresh used 401 runtime leases; reuse used 487: one probe, 400 attempts and 86
owners. Lighthouse and explicit fresh work retire a slot's idle owner before
running, so this comparison includes repeated owner startup and retirement,
not just a permanently warm journey pool.

## Cleanup, isolation and retirement

- The real-browser foundation passed 20 cases; the production owner/attempt
  protocol passed 31 cases. These include pending creation, original close
  completion, caught cleanup errors, worker restart, hooks/fixtures, module
  formats, download copying, storage/service-worker isolation and forced exit.
- All 51 Node tests passed. The Agent integration proof passed repeated
  retire/replace cycles, same-digest qualification source isolation, fresh and
  Lighthouse handoff, retained history, a fetched PNG, timeout, cancellation,
  peer progress and shutdown with zero execution pins. Separate Store
  installation qualification exercised the candidate path.
- The existing real-runtime integration suite passed configuration Test, secret
  resolution/redaction, outcome classification, artifacts and pidfd-observed
  process completion. Go race/vet, Linux build, runtime packaging and integration
  metadata checks passed for the affected code.
- A different UID could not connect to the private owner socket (`EACCES`), and
  the owner had no TCP listener. Browser profiles, artifacts and registry files
  are inside the owner's private work directory.
- All 1,010 workload checks completed successfully. Final inspection found no
  remaining test cgroups, browser-owner workspaces or claims/pins in either
  isolated runtime store. The installed Agent PID/start time and runtime selection
  digest matched preflight.

A separate 1,000-connection provider probe found retained owner state: anonymous
memory grew from 83.3 MiB initially to 119.7 MiB after 100 connections, 192.6 MiB
after 500 and 212.8 MiB after 1,000. This led to retirement after 100 loans in
addition to one-minute idle expiration. The probe used real context cleanup but
did not include fresh Test processes, so its timing is not journey throughput.
The rollover policy bounds connection count, not all possible browser memory use.

## Scope

Reuse keeps fresh Playwright Test processes and isolated contexts for each
attempt. Explicit `browser_mode: fresh` remains available for direct browser
control and local browser-file APIs. Lighthouse reuse and UI changes are outside
this implementation. A compatible contract-2 runtime is required; this validation
did not publish a production runtime or replace the installed service.

The earlier startup spike used a different fixture and omitted the complete
cleanup barrier. Its 281-second mixed result is not a directly comparable baseline.
Use the paired measurements from this harness instead.
