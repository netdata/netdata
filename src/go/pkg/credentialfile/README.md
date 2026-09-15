# Configured credential file reader

`New()` creates a lazy reader with explicit ownership. Unix requests use the installed
`buildinfo.NetdataBinDir/nd-run --read-file-server-v1` helper. There is no privileged
fallback, process-global reader, credential cache, or worker pool. Close the reader
when its collector/discovery owner ends. Package `Read` and `ReadAll` are scoped
conveniences for isolated operations, such as native secret-store resolution.

- `Read(ctx, path)` preserves safefile's regular-file and 1 MiB policy. No partial
  contents are returned after an error.
- `ReadAll(ctx, path)` retains unbounded stream semantics.
- `Open(ctx, path)` exposes a stream and holds the reader's request slot until EOF
  or Close. File-open errors may surface on the first stream read. The caller MUST
  close it, including after scanner/parser failure. Early Close retires the helper.
- `Stat(ctx, path)` returns modification time for reload decisions.
- `Close()` is terminal and idempotent and interrupts active Unix I/O.

A request waiting for the slot observes its context. Canceling an active request
retires only that request's session; a completed request's late cancellation cannot
stop a replacement. A child is independent of the first request's context. One
Wait goroutine owns reaping. A retired child must exit before a replacement starts;
uninterruptible kernel I/O can delay reap without accumulating replacement children.

The private wire contract is owned by `src/collectors/utils/nd-file-reader.h`.
Native file errors preserve `errors.Is` for `safefile.ErrFile`, policy errors, and
OS causes. Helper launch and malformed/truncated protocol errors are distinct from
file errors: they match `safefile.ErrFile` but never unwrap an OS missing-file cause.
This prevents optional-token handling from hiding a missing helper. Neither helper
stderr nor file contents are used in transport diagnostics.

Windows deliberately retains local service-account reads, native stream behavior,
and its existing authority. It does not start this helper or provide Unix active-I/O
cancellation. The boundary covers explicit credential-file options; it makes no
claim about SDK default credential acquisition or database DSNs.

Tests use a subprocess protocol peer to exercise framing and client lifecycle.
They do not prove the C helper's privilege transition; that requires separate
integration validation using the actual helper on the target operating system.

For actual C interoperability, supply a prebuilt helper at a path traversable by
its reduced identity (no compiler is implicitly invoked by package tests):

```sh
NETDATA_TEST_ND_RUN=/absolute/path/nd-run go test -race -run '^TestCReader' ./pkg/credentialfile
NETDATA_TEST_ND_RUN=/absolute/path/nd-run go test -run '^$' -bench '^BenchmarkCredentialRead256$' -benchmem ./pkg/credentialfile
```

Run from `src/go`. These cases use synthetic fixtures in a traversable temporary
directory, including a root-owned mode-0600 denial fixture when run as root. The
benchmark compares safefile with warm persistent 256-byte reads and reports
allocations. Timings describe that machine and are not CI thresholds. Full setuid
and Linux capability launch-shape validation belongs to the helper's C tests.

HTTP consumers bind the reader once with `web.NewHTTPClient`; request methods receive the current context and config.
The bound client borrows the reader, so closing idle HTTP connections leaves it usable until the job owner closes it.
There is no bearer-token cache. `web.WrapHTTPClient` explicitly binds custom standard clients and test readers.

TLS-only consumers use `tlscfg.NewTLSConfig` or `web.NewTransportClient`. These scope one reader across the complete
CA/certificate/key construction and close it before returning; they retain no reader in the returned TLS/HTTP object.
