# DEM performance and sizing

DEM capacity depends on browser service time, RUM identity and payload volume, retained detail, and query traffic.
Job count or beacon rate alone is not a sizing rule. Browser attempts share one process allowance; RUM processing
does not consume browser slots.

## Qualification workload

The qualification uses a development build with shared browser admission, independent check and chart cadences,
and exact time-based RUM retention. It runs the native Agent, DEM plugin, real Chromium executions, HTTP receiver,
and retained-history queries together. The browser and RUM implementation was subsequently merged in
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

## Measured outcome

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

## Choosing browser concurrency

Set the process allowance in `dem.conf`, and restart the plugin to apply it:

```yaml
browser:
  concurrency: 12
```

This is the tested setting for the workload above, not a new default. The default remains 1. Choose it together
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

## Measurement boundaries

The test uses Debian 13.7, Linux 6.12.111, x86_64 and an exposed AMD EPYC 7713 CPU model. The VM has 16 virtual
CPUs and about 64 GiB of RAM; the test unit is limited to 16 CPUs and 32 GiB. The virtual CPU model does not identify
an allocation of exclusive physical cores.
The Linux test places the native Agent, plugin, browsers, fixture server, HTTP generator, observers and offline
audits in one cgroup. Its memory includes charged file cache and kernel memory; it is not the Go heap or a sum of
process RSS. An additional conservative catalog accounts for immutable files that could have been warmed outside
the cgroup. CPU and memory measurements therefore include test overhead. Peak charged memory was 16,410,107,904 bytes;
the additional immutable-file bound was 1,184,989,141 bytes, not a measurement of externally charged pages.
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

The tested development source snapshot is `dd7e27768d`; the Linux plugin SHA-256 is
`a6bcdee58e2575d474d1b075c72809085b0749a4f56f17a2722dcf958fcccd08`. The managed runtime generation is
`79bb9a948aa20f006d000edbe92081e910c1a8cd3538cc7d1a3be66a2aa1d0c2`. Revalidate after material source or runtime changes.
