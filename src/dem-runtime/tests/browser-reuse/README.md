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

This is the first runtime cleanup proof. Its one cohort receipt does not prove
per-owner Agent admission, epoch checking, runtime upgrade or independent-slot
progress. Those require integration tests through the actual Agent pool. Timings
here are diagnostics, not a capacity benchmark.

Managed mode preserves real Test behavior within its browser-control contract.
Explicit `browser_mode: fresh` provides the unrestricted SDK throughout. Automatic
fresh fallback for worker restart or launch options permits ordinary browser APIs,
but the managed Test shim still restricts custom selector registration. Remote Playwright downloads use `saveAs()`;
`download.path()` and `video.path()` require a local fresh browser. Mode errors
must remain executor errors even if user code catches the original exception.
