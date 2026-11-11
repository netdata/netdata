# Managed browser cleanup proof

`proof.mjs` exercises the actual managed-browser guard with the release's pinned
Playwright and Chromium, including fresh Playwright Test CLI/worker processes.
Run on Linux, as the installed runtime's unprivileged service identity, with the
Chromium sandbox enabled. It uses only loopback content and task-owned processes.

Build `supervise.go` from the `src/go` module. Pass absolute `-store`, `-helper`,
`-script` and a **new** `-work` directory. `-helper` names the installed `nd-run`;
`-script` names this directory's `proof.mjs` in a copy retaining its relative path
to `runner/managed-browser.mjs`. The helper acquires the selected runtime, holds
one execution use for the complete cohort, and releases it only after authoritative
supervisor drainage. Use an isolated systemd unit with explicit resource limits.
It neither installs nor selects a runtime and does not restart the Agent.

The proof covers pending creation, unawaited page creation, original concurrent
close promises, caught creation/close failures, early context event delivery,
late creation, disconnection during acknowledgment, owner crash and retirement,
HAR flush failures, downloads, tracing, screenshots, service-worker/storage
isolation, real Test hooks/fixtures, worker restart, option-based fresh fallback,
caught unsupported operations and hard CLI termination. Controlled delays extend
real creation/close operations; they do not replace Chromium cleanup with a fake.

Successful cases emit JSON lines; failure exits nonzero. The helper's final record
separately reports process status, physical drainage and runtime-use release.
Retain both outcomes: an expected failure inside a fixture is not a failed proof,
and verified drainage does not make a failed assertion successful.

`runtime-proof.mjs` exercises the production owner/attempt protocol, all entry
formats, policy failures during discovery, reused-context fresh fallback, downloads,
EOF/early stop and idle browser death. The owner uses a Unix socket and private
profile/artifact/registry paths. `-script-arg=--inspect` pauses with a private
`access-probe.json` until an external cross-UID/no-TCP check creates `access-checked`
(within 45 seconds). `--soak` measures owner memory across 1,000 clean connections;
it is a provider retention probe, not full journey throughput. `unit-suite.mjs` runs
all source Node tests with the installed dependencies.

These cohort receipts do not prove per-owner Agent admission, epoch checking,
runtime upgrade or independent-slot progress. `engine-proof.go` covers the actual
Agent pool through a selected contract-2 runtime, real `nd-run`, history and
artifact stores. Build it from `src/go` and run as the service identity against an
**idle, isolated** runtime store:

```sh
go build -o /tmp/dem-engine-proof ../dem-runtime/tests/browser-reuse/engine-proof.go
/tmp/dem-engine-proof -store /absolute/test/store -helper /absolute/nd-run \
  -work /absolute/new/proof-output -mode proof
```

The proof checks warm reuse, same-digest qualification source isolation, repeated retirement,
fresh and Lighthouse handoff, outcome/history identity, a real screenshot,
cancellation, timeout, independent-slot progress and shutdown with zero execution
pins. An unverified process tree fails the proof and keeps ownership evidence.
It does not modify the selected runtime or restart an installed Agent.

For a controlled comparison, use `-mode fresh` and `-mode reuse` in separate runs
with the same `-journeys`, `-lighthouse` and `-slots` values and resource limits.
Every workload job uses deterministic loopback content; all jobs become ready at
one barrier. Report the complete batch and shutdown times, CPU and memory, rather
than just browser launch or reporter completion. Slot time includes execution,
cleanup and required browser replacement. Admission order can vary between runs.
Use more than 100 journeys on one slot to exercise busy-owner rollover. These
synthetic timings are comparative evidence, not a capacity guarantee for real
websites or a long-running mixed RUM workload.
See [the validation results](RESULTS.md) for the measured implementation, runtime,
resource limits, cleanup evidence and fresh/reuse trade-offs.

Managed mode preserves real Test behavior within its browser-control contract.
Explicit `browser_mode: fresh` provides the unrestricted SDK throughout. Automatic
fresh fallback for worker restart or launch options permits ordinary browser APIs,
but the managed Test shim still restricts custom selector registration. Remote Playwright downloads use `saveAs()`;
`download.path()` and `video.path()` require a local fresh browser. Mode errors
must remain executor errors even if user code catches the original exception.
