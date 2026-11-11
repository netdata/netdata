# Managed browser reuse validation

Measured on 2026-11-11 with the production owner/attempt protocol and real Agent
execution pool. The initial browser implementation was `0281faf140`, with harness
`dec2d53522`; follow-up source versions are identified below. These are controlled
loopback comparisons, not capacity guarantees
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
- The initial four workload runs started all jobs at one barrier. The serial
  comparison used 105 journeys and one slot, crossing the 100-loan retirement boundary. The mixed
  comparison used 300 journeys, 100 Lighthouse audits and 12 shared slots.

Unit totals include startup, every attempt, required cleanup and replacement,
final browser retirement and deferred storage closure. CPU is whole-unit systemd
accounting; peak memory includes the complete unit. The harness also reports its
narrower execution interval, first result, per-kind slot time and runtime-use
count. Mixed admission order may vary. Each initial comparison row is one run, not
a repeated-trial estimate or statistical confidence interval. The follow-up
sections report repeated trials separately. First-result timings start at the
workload barrier, after common preflight. The serial pair ran fresh then reuse;
the mixed pair ran reuse then fresh.

## Matched concurrency comparison

A follow-up comparison held the workload constant: 120 distinct journeys, no
Lighthouse audits, and 1, 4 or 12 slots. At each concurrency, trial order was
fresh, reuse, reuse, fresh. All 1,440 journeys passed. Agent/runtime source was
`c4c2550f9d`; browser execution code and the runtime fixture above were unchanged.
The exact original harness was built from that commit for these trials.

The following values are means of two runs per mode. Percentage changes compare
those means; two runs are repeatability evidence, not a statistical confidence
interval. Memory averages are approximately one-second cgroup memory.current
samples over each complete unit, including its startup and shutdown. Peak and
average memory include anonymous memory, kernel accounting and charged file cache.

| Slots | CPU reduction | Elapsed reduction | Mean memory, fresh → reuse | Peak memory, fresh → reuse |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 17.1% | 24.4% | 216 → 358 MiB | 448 → 566 MiB |
| 4 | 14.7% | 26.1% | 761 → 1265 MiB | 1668 → 1871 MiB |
| 12 | 9.9% | 31.9% | 1971 → 2997 MiB | 4530 → 5039 MiB |

CPU savings occurred at every tested concurrency. The 12-slot journey-only result
must not be confused with the earlier 12-slot mixed batch: Lighthouse work and
warm-owner retirement also affect that comparison. Lower total CPU does not mean
lower instantaneous CPU demand; completing the batch sooner concentrates work.

All individual trials are retained below. Each mode completed exactly 120
journeys. Fresh used 121 runtime leases. Reuse used 123, 125 and 133 at 1, 4 and
12 slots respectively, corresponding to two, four and twelve browser owners.
The one-slot case includes the 100-loan owner rollover.

| Slots | Pair | Mode | Unit elapsed (s) | Unit CPU (s) | Peak memory (MiB) | Mean memory (MiB) |
| ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 1 | Fresh | 317.185 | 442.198 | 449.0 | 220.0 |
| 1 | 1 | Reuse | 232.301 | 361.627 | 570.2 | 360.9 |
| 1 | 2 | Reuse | 240.611 | 365.495 | 562.3 | 355.3 |
| 1 | 2 | Fresh | 308.475 | 434.879 | 446.6 | 211.9 |
| 4 | 1 | Fresh | 93.826 | 449.864 | 1688.7 | 757.1 |
| 4 | 1 | Reuse | 69.810 | 384.017 | 1881.2 | 1263.4 |
| 4 | 2 | Reuse | 71.566 | 379.489 | 1860.7 | 1267.1 |
| 4 | 2 | Fresh | 97.514 | 445.390 | 1646.5 | 765.7 |
| 12 | 1 | Fresh | 62.354 | 468.565 | 4511.9 | 1931.7 |
| 12 | 1 | Reuse | 43.256 | 422.056 | 4806.4 | 3024.0 |
| 12 | 2 | Reuse | 41.348 | 410.252 | 5271.9 | 2971.0 |
| 12 | 2 | Fresh | 61.837 | 455.304 | 4547.7 | 2011.1 |

Every trial left zero runtime claims/pins and browser-owner workspaces, with the
installed Agent and runtime selection unchanged. The first coordinator run
incorrectly treated its final live PID sample as evidence of leftover processes
after systemd had removed the empty cgroup. The observer was corrected to inspect
the final cgroup state; the successful workload was retained and not rerun.

## Paced target cadence comparison

The paced comparison used 300 persistent journey jobs every five minutes and
100 persistent Lighthouse jobs every fifteen minutes, sharing 12 slots. Each
trial delivered 900 journey occurrences and 100 audit occurrences over an actual
15-minute window. Per-job FNV phases matched the production monitor; all four
trials had identical scheduled arrivals. Each pool started cold. Trial order was
fresh, reuse, reuse, fresh.

The harness (`02854d067c`) drove the real Engine, runtime, supervisor, history and
artifact stores. Native Agent bootstrap, monitor coalescing, charts and RUM traffic
were excluded. Every response completed before its next same-job occurrence;
otherwise the simplified arrival model would fail validation. After the arrival
window and all completions, each trial waited another 75 seconds and verified
natural owner expiry with zero execution pins before Engine shutdown.

CPU and peak memory below cover the complete unit, including preflight, late
completions, idle retirement and final storage closure. Mean memory covers the
exact 15-minute arrival window using approximately one-second cgroup samples.
Fixed-window elapsed time is not a throughput improvement. All 4,000 checks
passed, and all final process/runtime ownership checks passed. Two 20-attempt
paced smoke runs also passed, bringing follow-up validation to 5,480 successful
checks across 18 trials. Final inspection found no task units, cgroups or browser
owner workspaces; runtime claims/pins were empty and the installed Agent identity
and runtime selections were unchanged.

| Trial order | Mode | Unit CPU (s) | Unit elapsed (s) | Arrival-window mean memory (GiB) | Peak memory (GiB) | Browser owners started |
| ---: | --- | ---: | ---: | ---: | ---: | ---: |
| 1 | Fresh | 4204.780 | 987.542 | 1.133 | 4.059 | 0 |
| 2 | Reuse | 3574.703 | 988.257 | 2.226 | 3.974 | 20 |
| 3 | Reuse | 3596.543 | 987.889 | 2.315 | 4.772 | 20 |
| 4 | Fresh | 4201.975 | 989.253 | 1.251 | 4.146 | 0 |

Across the two runs per mode, reuse reduced CPU by 14.7%. The saving was 618
CPU-seconds per 15-minute workload,
equivalent to about 0.69 CPU cores over that arrival window, with all cleanup
costs charged. Arrival-window mean memory increased from 1.19 to 2.27 GiB, an
additional 1.08 GiB. This is a resident-memory trade-off: anonymous-memory samples show the same increase, not merely file cache.
Peak memory can remain similar even when average memory rises substantially.

Response time below is measured from the scheduled due time through complete
attempt execution and cleanup. Percentiles combine the two trials per mode; they are
observed distributions for this fixture, not confidence intervals or website SLOs.

| Kind | Mode | Mean response (s) | p95 response (s) | p95 admission delay (s) |
| --- | --- | ---: | ---: | ---: |
| Journey | Fresh | 4.841 | 10.339 | 1.814 |
| Journey | Reuse | 2.846 | 6.101 | 0.007 |
| Lighthouse | Fresh | 13.531 | 17.986 | 1.942 |
| Lighthouse | Reuse | 12.350 | 15.049 | 0.007 |

Individual mean journey response times were 4.22 and 5.46 seconds fresh, versus
2.77 and 2.93 seconds reused; latency varied more between trials than CPU cost.
Both modes met the scheduled cadence. Reuse did not eliminate occasional queues:
the largest observed admission delays were 7.768 seconds fresh and
5.423 seconds reused. CPU-quota throttling counters recorded short bursts
under the configured 16-CPU limit; no memory-limit or OOM events occurred.

This steady arrival mix has nine journey executions per audit, compared with
three per audit in the initial startup batch. The first paced reuse trial started
20 owners for 900 journeys; the initial mixed burst started 86 for 300 journeys.
These counts demonstrate different amortization, but do not identify individual
retirement reasons. Neither comparison establishes production website capacity.
Sparse checks can still receive no reuse benefit: an owner expires after one idle
minute, so an isolated journey every five minutes starts cold on every occurrence.

## Initial journey-only comparison

| Mode | Whole-unit elapsed | Whole-unit CPU | Peak memory | First result |
| --- | ---: | ---: | ---: | ---: |
| Fresh | 275.164 s | 388.186 s | 449.7 MiB | 2.477 s |
| Reuse | 204.409 s | 318.966 s | 576.4 MiB | 2.975 s |

All 105 journeys passed in each mode. Reuse reduced elapsed time by 25.7% and CPU
by 17.8%, with 28.2% more peak memory. Its first cold result was about half a second
later; the benefit comes from subsequent attempts. Fresh used 106 runtime leases
(one probe and 105 attempts). Reuse used 108 (the probe, 105 attempts and exactly
two owners), confirming that the measured run included busy-owner replacement.

## Initial mixed journey and Lighthouse comparison

| Mode | Whole-unit elapsed | Whole-unit CPU | Peak memory | First result |
| --- | ---: | ---: | ---: | ---: |
| Fresh | 261.593 s | 1,967.788 s | 4.6 GiB | 5.526 s |
| Reuse | 244.106 s | 1,906.309 s | 5.1 GiB | 12.759 s |

All 400 checks passed in each mode. Reuse reduced elapsed time by 6.7% and CPU by
3.1%, with about 0.5 GiB more peak memory. Journey slot time fell from 1,656.0 to
1,417.0 seconds; Lighthouse slot time was similar at 1,396.5 and 1,399.7 seconds.
This comparison changes both workload composition and concurrency relative to
the initial serial run; it does not isolate a concurrency-only effect.
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
- All 1,010 initial workload checks completed successfully. Final inspection found no
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
