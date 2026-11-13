# DEM performance and sizing

DEM capacity depends on browser service time, RUM identity and payload volume, retained detail, and query traffic.
Job count or beacon rate alone is not a sizing rule. Browser attempts share one process allowance; RUM processing
does not consume browser slots.

The original mixed qualification met the tested target. A current-master repeat completed the offered-load interval
but shed 43 requests in the generator and coalesced five occurrences during activation. It did not meet the same
lossless-load and zero-coalescing targets. Both runs use fresh browsers. Browser reuse remains deferred, and the production default remains one shared browser slot.

## Qualification workload

The mixed runs use shared browser admission, independent check and chart cadences, and exact time-based RUM
retention. They run the native Agent, DEM plugin, real Chromium executions, HTTP receiver and retained-history
queries together. The browser admission and RUM retention changes merged in
[PR #24674](https://github.com/netdata/netdata/pull/24674) and
[PR #24675](https://github.com/netdata/netdata/pull/24675). The target below applies to one Agent and its DEM plugin process.

| Work | Configuration |
|---|---|
| Sites | 100 configured vnodes, each with independent RUM, journey and Lighthouse jobs |
| Journeys | 300 jobs, one short Playwright test each, `check_every: 300` |
| Lighthouse | 100 desktop audit jobs, `check_every: 900` |
| Chart publication | `update_every: 10` |
| Shared browser allowance | `browser.concurrency: 12` |
| RUM ingress | 1,000 HTTP beacons/second for 35 minutes; half to one site |
| RUM measurement | Five-minute window, measurement sample rate 1 |
| RUM investigation | 10% session sampling, no always-keep overrides |
| Retained history | Shared 1 GiB budget, 30-day age policy |
| Observation | Six-minute RUM cooldown after traffic stops; browser schedules continue |
| Resource envelope | 16 virtual CPUs and a 32 GiB memory limit |

Each site has browse/search, checkout and form-submission journeys. Each test navigates, interacts with the page,
and asserts a visible result. Each execution creates a fresh browser. The Lighthouse page includes CSS, JavaScript,
an image, a delayed hero, a long task, a layout shift and a fetch. These are deterministic local pages; network
latency, third-party services and long application workflows are outside this measurement.

Each RUM document sends two reports, and four documents share a session. A report contains three revised Web Vital
identities and a stable first-party resource identity, with 20 page paths and three application versions per site.
Thus 1,000 beacons/second is approximately 500 new document experiences/second, not 1,000. The hot site receives half
the traffic. The expected five-minute population is about 150,000 documents across all sites.

Geolocation and frustration capture, event-log exports, tracing, failure screenshots and Lighthouse HTML reports
are disabled. Receiver admission settings explicitly allow the single-address load generator. Those settings are
test inputs, not recommended public ingress limits. The history age setting is subject to the shared byte budget;
this test does not establish 30 days of retention at this rate.

## Original mixed result — 2026-11-10

On 2026-11-10, this workload completed with the following results. The result covers the specific fixture and
resource envelope above; it is not a capacity guarantee for arbitrary websites or beacon contents.

| Observation | Result |
|---|---:|
| RUM requests planned / attempted / HTTP 202 | 2,100,000 / 2,100,000 / 2,100,000 |
| Unoffered, shed or failed requests | 0 |
| Distinct document experiences / resources | 1,050,003 / 1,050,003 |
| Expected sampled history records / missing records | 209,369 / 0 |
| Concurrent session-history queries / failures / skipped ticks | 328 / 0 / 0 |
| Initial browser checks completed successfully | 400 / 400 |
| Last initial check completed after traffic start | 281.1 seconds |
| Recurring journeys / Lighthouse audits due during the load, all successful | 1,800 / 136 |
| Coalesced synthetic occurrences | 0 |
| Peak persisted browser-attempt overlap | 12 |
| Average CPU during offered load, including the test harness | 7.21 cores |
| Peak charged cgroup memory | 15.28 GiB |
| Peak plus conservative externally warmed immutable-file bound | 16.39 GiB |
| OOM or memory-limit events | 0 |

Every full five-minute steady due cohort contained 300 successful journeys. All 100 first recurring Lighthouse
checks completed; two finished just after the common 30-minute boundary, within their individually phased cadence.
All 2,336 initial and recurring attempts due through the load end have successful terminal records. Later cooldown
work is separate: 414 additional successes and 12 cancellations at controlled shutdown. Every admitted run has a
terminal record, including eight explicitly diagnosed cancellations before start-history append.

All 166 rolling RUM observation polls passed their count/percentile bounds, final cumulative counters matched,
and the five-minute live population became exactly empty after cooldown. Closed history matched every independently
selected investigation record. These checks do not turn sampled history into a lossless event archive.

The offered average was 999.99999 beacons/second. Actual HTTP starts varied from 242 to 1,801 per one-second bin;
rolling 60-second rates were 984.2–1,015.8/second. The generator's dispatch-to-HTTP-start delay had a p99 upper bound
of 580.2 ms and a 1.94-second maximum. HTTP response latency had a p99 upper bound of 18.5 ms and a 1.25-second
maximum. Histogram percentile bounds have 2% resolution. All requests started within the nominal 35-minute window,
without shedding or extending the window to finish dispatch. This establishes sustained throughput with measured short-term jitter,
not a per-second pacing or latency SLO.

| Recurring browser work | Median execution | p95 execution | p95 admission delay | Maximum admission delay |
|---|---:|---:|---:|---:|
| Short journey | 3.44 s | 5.23 s | 3.05 s | 8.86 s |
| Lighthouse audit | 12.66 s | 13.82 s | 3.40 s | 10.01 s |

Execution duration excludes earlier runtime acquisition/setup and later history synchronization. Admission delay
is measured separately from the selected due time. First-check queueing is substantially longer: the maximum was
276.6 seconds during simultaneous activation.

Lighthouse scores were 85–86 for recurring audits and 80–86 during activation, compared with 85–86 in three serial
control audits of the same page. Maximum total blocking time was 85 ms for recurring audits and 191.2 ms during
activation, versus 51–71 ms in the serial control. These observed tails matter when interpreting lab measurements;
the test does not establish equivalence to an unloaded runner.

## Current-master repeat — 2026-11-11

The same fixture was repeated on master `d2c037a448`, including the merged history-query, snapshot,
bot-classification and retained-state changes. **This repeat did not qualify the complete target.** The generator's
1,024-entry hot-site queue shed 43 requests before sending them, and five initial journey occurrences were replaced
by newer due occurrences while waiting for a browser slot. All requests actually sent received HTTP 202, with no
transport errors. The failed generator and browser qualification verdicts are retained.

| Observation | Original run | Current-master repeat |
|---|---:|---:|
| Planned RUM requests | 2,100,000 | 2,100,000 |
| Requests attempted / HTTP 202 | 2,100,000 / 2,100,000 | 2,099,957 / 2,099,957 |
| Generator-shed requests | 0 | 43 |
| Original initial occurrences completed | 400 / 400 | 395 / 400 |
| Time until all 400 jobs had a first result | 281.1 s | 353.1 s |
| Coalesced occurrences | 0 | 5 |
| Successful recurring journeys / audits through load end | 1,800 / 136 | 1,800 / 136 |
| Average whole-test CPU during offered load | 7.21 cores | 7.45 cores |
| Peak charged cgroup memory, including final audits | 15.28 GiB | 14.51 GiB |
| Peak plus conservative external immutable-file bound | 16.39 GiB | 15.59 GiB |
| OOM or memory-limit events | 0 | 0 |

The repeat retained every expected sampled RUM record: 209,369 expected, zero missing or invalid. All 166 rolling
observation polls passed, final counters matched the accepted population, cooldown reached exact zero, and all 328
history queries succeeded without skipped ticks. The accepted population contained 1,049,982 distinct document and
resource experiences. This verifies accepted-data correctness; it does not erase the generator's offered-load gap.

The 43 missing request ordinals fall at load offsets 1,442.302–1,442.386 seconds, after CPU profiling ended.
HTTP starts ranged from 275 to 1,678 per one-second bin and 984.4–1,014.9/second over rolling 60-second windows.
Dispatch-to-HTTP-start delay had a p99 upper bound of 476 ms and a 2.13-second maximum; response latency had a p99
upper bound of 17.8 ms and a 1.23-second maximum. One HTTP start occurred 0.268 ms beyond the nominal window.
These observations do not satisfy the original lossless, within-window delivery criterion.

Closed synthetic history contains 2,331 successful attempts due through load end: 395 original initial occurrences
and all 1,936 recurring occurrences. The five replacement first attempts belong to that recurring count. A separate
cadence reconciliation accounts for the five missing initial anchors; it does not change the failed qualification
verdict. Cooldown adds 416 successes and nine controlled-shutdown cancellations. Every persisted run has one terminal
record; four shutdown cancellations occurred before start-history append and satisfy the existing explicit drain
exception.

| Recurring browser work in repeat | Median execution | p95 execution | p95 admission delay | Maximum admission delay |
|---|---:|---:|---:|---:|
| Short journey | 3.96 s | 5.92 s | 9.36 s | 43.01 s |
| Lighthouse audit | 12.86 s | 13.72 s | 6.42 s | 9.51 s |

These recurring timings include the five replacement first attempts and activation spillover. Recurring Lighthouse
scores were 84–86, with maximum total blocking time 113 ms. No new serial-control trial was run, and these values do
not establish equivalence to an unloaded runner.

The runtime, fixture, normalized workload configuration and browser execution source matched the original run.
First-attempt time increased both inside child execution and around it. I/O pressure was observed after activation,
but there is no matched old pressure trace or per-stage timing proof that establishes its cause. These are separate
dated runs, not a randomized before/after comparison. The repeat shows lower observed memory and higher whole-test
CPU; it does not establish a causal aggregate CPU improvement or a code regression.

The test-only build exposed the three existing Function families for observation and added the bounded CPU capture
below. Profiling ran for three of the 35 load minutes; aggregate counters include its overhead. The Linux plugin
SHA-256 is `92ad1006cd112847631f7dff16000d90b723dd573703ba06e76bfbf5a6d068b5`. No production configuration,
runtime or default changed. The private Agent drained successfully, all 43,477 tracked process identities exited,
and the installed Agent/runtime remained unchanged.

## Current DEM CPU attribution — 2026-11-11

A 180-second CPU profile at minutes 20–23 of the current-master repeat recorded 118.22 sampled CPU-seconds in the
DEM Go process, about **0.66 core**. The profile PID matched the native Agent's DEM child; it completed without
interruption. These are cumulative stack costs, so nested rows overlap.

| Profile path | Sampled CPU time | Share of DEM profile |
|---|---:|---:|
| RUM request handler | 40.86 s | 34.56% |
| Rolling-window reads | 17.74 s | 15.01% |
| Background GC marking | 13.65 s | 11.55% |
| Faro decoding, within request handling | 17.12 s | 14.48% |
| Bot classification, within request handling | 9.71 s | 8.21% |
| Aggregation ingestion, within request handling | 8.12 s | 6.87% |

Window reads include chart snapshots and Pages queries from the qualification observer. Background marking is not
all GC work. This profile excludes the native Agent, Node, Chromium and load generator, and does not measure blocked
I/O time. It precedes the 30-minute replay-retention turnover; the full workload covers that turnover, but this
profile does not attribute its mature expiry costs. No original-run CPU profile exists, so these figures establish
current attribution, not a causal CPU reduction.

## Choosing browser concurrency

Set the process allowance in `dem.conf`, and restart the plugin to apply it:

```yaml
browser:
  concurrency: 12
```

This is the setting used in both mixed runs, not a new default or a guarantee of lossless activation. The repeat
coalesced five initial occurrences at this allowance. The default remains 1. Choose it together
with CPU, memory and Lighthouse measurement quality. A browser slot can own several child processes and is not
a hard memory limit. Rehearsals and in-process runtime qualification share the allowance with monitoring.

Measure both activation and recurring work. Activation queues all first checks immediately; recurring checks are
spread over their configured cadence. In the eight-slot comparison, 12 queued initial journeys were replaced by
newer due occurrences. Its later steady throughput did not make that startup result acceptable.

Use scheduled, admitted and coalesced counts, admission delay and observation age to detect insufficient capacity.
Changing `update_every` only changes chart publication; `check_every` changes browser execution frequency. A
coalesced occurrence never ran and is not evidence of website failure. For workload estimates, combine checks per
second with observed admission-to-completion time, then allow for startup bursts and variation. That time excludes
final history synchronization and slot release, so it is a lower bound on slot occupancy.

Higher concurrency can increase contention and change Lighthouse measurements. The small serial fixture control
does not establish statistical equivalence with concurrent audits. Recheck the workload and measurement quality
when changing scripts, audited pages, runtime, hardware, frequency or concurrency.

## RUM and query costs

Size RUM using new document, session, vital and resource identities as well as bytes and beacons per second.
Canonical measurements retain their receipt window; replay identities remain for 30 minutes. Restarting or replacing
the site job, or changing its vnode association, resets these in-memory windows. Repeated reports and revisions have
a different memory cost from entirely new documents. Session detail sampling controls investigation history, not the
population used for live measurement.

The qualification reads live sites/pages and two concurrent, bounded recent session-history ranges every 15 seconds,
while also observing browser status/history. This is a specific query mix. Broad history ranges, more concurrent
readers, richer payloads, always-kept errors, exports and captures require their own measurements.

## Subsequent RUM component measurements

After the original mixed qualification, [PR #24677](https://github.com/netdata/netdata/pull/24677) reduced history-query
string allocations and [PR #24680](https://github.com/netdata/netdata/pull/24680) reduced snapshot and Pages read
costs. Both are merged. The following Linux VM results compare six balanced baseline/candidate pairs for each
component; they are not a rerun of the full mixed workload.

| Component fixture | Median time, before → after | Allocated bytes per operation, before → after |
|---|---:|---:|
| Selected-site history query, repeated metadata | 416.1 → 304.2 ms (-26.9%) | 171.14 → 8.33 MiB (-95.1%) |
| Dense chart snapshot | 538.0 → 364.8 ms (-32.2%) | 120.00 → 33.63 MiB (-72.0%) |
| Dense Pages read | 131.7 → 119.7 ms (-9.1%) | 14.87 → 3.77 MiB (-74.6%) |
| High-cardinality chart snapshot | 57.00 → 21.98 ms (-61.4%) | 18.40 → 7.67 MiB (-58.3%) |

The history fixture contains 100,000 rich records across 100 interleaved sites and 24 receipt hours. Metadata
repetition determines the benefit: two deliberately varying-metadata controls showed no statistically significant
timing change, and their allocated bytes fell only about 1.3%. The ordinary varying-metadata median was 3.5% slower,
so this is not a universal speedup claim. The tested history implementation is `7560ac8f48`, compared with `e8fb39b24d`.

The dense snapshot fixture retains 150,000 documents with three varied vitals and a resource; the high-cardinality
fixture has 10,000 pages/versions. The tested snapshot implementation is `96712049ae`, compared with `b7c4db7de2`.
Exact-output, revision, expiry and vnode-reset checks passed. Ingestion controls had unchanged allocated bytes and
allocation counts, with no significant timing change. A separate concurrent ingestion/read fixture retained about
794 MB of heap in both versions. Lower allocation volume per read is not an equivalent reduction in resident memory.

Further merged changes reduced receiver bot-classification CPU in [PR #24691](https://github.com/netdata/netdata/pull/24691)
and retained RUM state in [PR #24692](https://github.com/netdata/netdata/pull/24692). On the VM, the mixed-fixture
User-Agent benchmark fell from 109.1 to 21.5 microseconds, and the desktop-browser collect-handler benchmark fell
from 322.1 to 75.8 microseconds. These are six balanced baseline/candidate pairs; the handler benchmark excludes the
HTTP socket path. The short mixed-fixture User-Agent saving corresponds to about 0.09 core at 1,000 calls/second.

The retained-state probe used one site with 450,000 documents over 29 simulated minutes and forced GC. Retained heap
fell from 916 to 630 MB, about 31%, with identical outputs and unchanged allocation counts on the measured operations.
This is retained Go heap, not whole-Agent resident memory. The tested candidates were `993d743dcc` for classification
and `12cf967cd1` for retained state, both compared with `5993198826`.

Component percentages do not translate directly into whole-Agent savings. Keep their controlled comparisons
separate from the dated mixed-workload observations, which include browser processes and the test harness.

## Optional browser error summary costs

`browser_error_summary` defaults to `false`. A 2026-11-13 controlled candidate-A observer probe compared matched
fresh-process executions with observation off and on, using Node 24.20.0, Playwright 1.63.0 and Chromium on the same
otherwise lightly loaded Linux VM. Each workload had one warmup pair and five measured pairs, alternating order.
The table reports medians of paired increments, not differences between independently computed mode medians.

| Workload | Added CPU seconds | Added peak charged memory | Added elapsed time |
|---|---:|---:|---:|
| Quiet page | 0.405 | 6.4 MiB | 277 ms |
| 10,000 non-error console messages | 1.075 | 146.6 MiB | 488 ms |
| 300 successful subresources | 0.478 | 24.2 MiB | 257 ms |
| 10,000 console errors | 0.870 | 156.9 MiB | 625 ms |

Console subscription incurs provider dispatch and handle costs before severity filtering, even when no sampling
calls occur. Sampling quotas bound owned sampling work and retained detail; they do not bound provider-owned
console dispatch, handles or total browser memory. Counts continuing past sample limits does not make observation
free. Retain capacity and deadline margin for the monitored pages when enabling the option.

These measurements include Node, worker and browser descendants, a synthetic loopback server and probe logging.
Peak charged cgroup memory includes charged cache and kernel memory; it is not RSS or Go heap. Fresh cgroups may
exclude previously charged immutable file pages. All 48 executions and 44 exact core/failure-evidence comparisons
passed, with no outcome failures, OOM events or surviving task processes. The fixtures qualify these controlled
workloads, not universal timing neutrality or mixed-load capacity. They measured the observer prototype before
production producer, Go decoding and history integration, so they do not establish the full production Engine's
incremental cost. The mixed qualification above did not enable browser error summaries.

The final implementation was also measured through production Engine, nd-run, artifact finalization and journal
reopen on the same pinned VM. Quiet and 10,000-error journeys each had one excluded warmup pair and three measured
off/on pairs in alternating order, with fresh cgroups and scratch directories. The console-error median paired
increment was **1.469 CPU seconds, 152.8 MiB peak charged memory and 508 ms elapsed**. CPU increments ranged from
1.065 to 1.695 seconds. Quiet paired medians were -0.033 CPU seconds, -6.4 MiB and -303 ms; the negative differences
and elapsed-time variability indicate measurement noise, not a claimed speed improvement. All 16 executions passed
outcome, process-drainage and live/reopened journal-equality checks, without OOM events or surviving task cgroups.
This includes the Go fixture server and journal work, while previously charged immutable pages can still be excluded.
It qualifies these final-path fixtures, not broad mixed-workload capacity or a universal overhead percentage.

## Browser reuse experiment — deferred

On 2026-11-11, [PR #24684](https://github.com/netdata/netdata/pull/24684) compared fresh browsers with supervised
Chromium reuse while retaining fresh Playwright Test processes and isolated contexts per attempt. This experiment
is deferred and is not a shipped configuration option. The implementation and its
[detailed validation report](https://github.com/netdata/netdata/blob/58f052c313/src/dem-runtime/tests/browser-reuse/RESULTS.md)
are preserved for reference.

The browser-only tests used the same 16-CPU/32-GiB unit limits, with no swap, on the same Debian VM. They ran real
Engine, runtime, supervisor, history and artifact paths against deterministic loopback pages. They excluded native
Agent bootstrap, chart publication and RUM traffic. Source `c4c2550f9d` supplied the browser implementation; paced
harness `02854d067c` added scheduled arrivals. Both modes used the same experimental contract-2 runtime
`2026.11.11-contract2-proof2` (Node 24.20.0, Playwright 1.63.0 and Chromium 154.0.8037.92).

Matched batches held the workload at 120 journeys with no audits. At each slot count, the order was fresh, reuse,
reuse, fresh. The table compares means of two runs per mode. CPU, elapsed time and mean memory cover the complete
unit, including startup, cleanup, owner replacement and shutdown.

| Shared slots | CPU reduction with reuse | Elapsed reduction | Mean memory, fresh → reuse |
|---:|---:|---:|---:|
| 1 | 17.1% | 24.4% | 216 → 358 MiB |
| 4 | 14.7% | 26.1% | 761 → 1265 MiB |
| 12 | 9.9% | 31.9% | 1971 → 2997 MiB |

The paced comparison used 300 persistent journey jobs every five minutes and 100 audit jobs every fifteen minutes,
sharing 12 slots. Four full 15-minute windows, in the same balanced mode order, each completed 900 journeys and
100 audits. All four used identical per-job phases matching production scheduling and began with a cold pool.

| Paced browser measurement | Fresh | Reuse |
|---|---:|---:|
| Mean whole-unit CPU per workload | 4203.4 CPU-seconds | 3585.6 CPU-seconds |
| Mean memory during the arrival window | 1.19 GiB | 2.27 GiB |
| Mean journey response | 4.84 s | 2.85 s |
| Journey p95 response | 10.34 s | 6.10 s |

Reuse saved 14.7% CPU: 618 CPU-seconds per 15-minute workload, equivalent to about 0.69 CPU cores. Average resident
memory increased by 1.08 GiB, or about 90%. Response time runs from the scheduled due time through attempt cleanup;
the response statistics pool both trials per mode. Both modes met cadence. Fixed-window elapsed time is not a
throughput comparison, and these response timings differ from the narrower execution timings in the mixed test.

Whole-unit CPU includes a further 75-second idle observation and final shutdown. Memory means use approximately
one-second cgroup samples over the exact arrival window. Every response finished before its next same-job occurrence;
monitor coalescing was not implemented by this harness. Short CPU-quota throttling bursts occurred, with no OOM or
memory-limit events. All 5,480 checks across 12 matched trials, two short smokes and four full paced trials passed.
Task processes and runtime ownership drained; the installed Agent and runtime were unchanged. Two runs per mode
demonstrate repeatability, not statistical confidence or arbitrary-website capacity.

Reuse remains deferred based on that browser-only experiment, where fresh execution met the paced target. The
measured CPU and response gains do not currently justify maintaining long-lived browser owners, cleanup
acknowledgments, separate runtime leases, retirement/replacement logic and browser-API restrictions. Making reuse opt-in would retain that maintenance cost.
The experiment also retains browsers for up to one idle minute, so a sparse five-minute check can start cold every
time and gain no reuse benefit. A demonstrated CPU-budget or cadence shortfall with fresh execution would provide
a stronger reason to reconsider this trade-off. The 12-slot test setting does not change the production default of 1.

## Mixed-qualification measurement boundaries

The test uses Debian 13.7, Linux 6.12.111, x86_64 and an exposed AMD EPYC 7713 CPU model. The VM has 16 virtual
CPUs and about 64 GiB of RAM; the test unit is limited to 16 CPUs and 32 GiB. The virtual CPU model does not identify
an allocation of exclusive physical cores.
The Linux test places the native Agent, plugin, browsers, fixture server, HTTP generator, observers and offline
audits in one cgroup. Its memory includes charged file cache and kernel memory; it is not the Go heap or a sum of
process RSS. An additional conservative catalog accounts for immutable files that could have been warmed outside
the cgroup. CPU and memory measurements therefore include test overhead. In the original run, peak charged memory
was 16,410,107,904 bytes; the additional immutable-file bound was 1,184,989,141 bytes, not a measurement of externally
charged pages.
The sum remained within 32 GiB. Ordinary Go GC policy was unchanged; no `GOGC` or `GOMEMLIMIT` override was used.

Runtime 2026.10.29.1 supplies the browser tools. Chromium sandboxing stays enabled. The installed Agent service and
runtime remain untouched; the test uses private state, HOME, temporary storage and a runtime clone mounted at its
required absolute path. No public network load is generated.

The generator records every offered request without application retries. Independent oracles check actual HTTP
acceptance, rolling populations and percentiles, final cumulative counters, and sampled retained identities.
Closed synthetic history checks actual initial and recurring runs, captured vnode/runtime identity and terminal
outcomes. An HTTP 202 response alone is not proof of durable history.

The original live observer reported five admission/history mismatches. Review and raw-record reconciliation
confirmed that admission counters preceded start-history publication for five cooldown attempts. The observer was
corrected to defer its upper-bound check to complete closed history; the lower bound and all target-work checks
remain enforced. The strict closed audit passed before and after that correction. The original failed observer
verdict is preserved with the test evidence; no workload result was fabricated or retried to obtain the result.

Shutdown validation signals and joins the private DEM plugin while its Agent protocol pipes remain open, verifies
browser/runtime ownership is drained, then stops the private Agent. It does not qualify ordinary Agent-initiated
shutdown. A 35-minute load plus cooldown is a bounded capacity result, not a long-duration soak or a guarantee for
arbitrary site behavior.

The original tested development source snapshot is `dd7e27768d`; its Linux plugin SHA-256 is
`a6bcdee58e2575d474d1b075c72809085b0749a4f56f17a2722dcf958fcccd08`. The managed runtime generation is
`79bb9a948aa20f006d000edbe92081e910c1a8cd3538cc7d1a3be66a2aa1d0c2`. Revalidate after material source or runtime changes.
