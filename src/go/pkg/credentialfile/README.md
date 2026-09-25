# Configured credential file reads

The package exposes operation-scoped functions. Unix operations each start one installed
`buildinfo.NetdataBinDir/nd-run --file-reader` helper with reduced authority. There is no persistent process, shared
reader, credential cache, queue or worker pool. File access stays implicit inside ordinary HTTP helpers; requests
without credential files start no helper.

- `Read(ctx, path)` preserves safefile's descriptor-validated regular-file and 1 MiB policy. Symlinks to regular files
  are accepted. No partial contents are returned after an error.
- `ReadAll(ctx, path)` retains unbounded file/stream semantics. It also discards partial contents on failure.
- `Open(ctx, path)` exposes an unbounded `io.ReadCloser`. File-open errors may surface on the first read. The caller
  MUST close the stream, including after scanner/parser failure. EOF is successful only after terminal validation.
- `Stat(ctx, path)` returns modification time for reload decisions. It uses a separate helper invocation.

Only an open stream has a lifecycle. Canceling its context or calling its idempotent `Close` closes its pipe and
terminates its exact child, interrupting blocked Go reads. One goroutine owns `Wait` and eventual reaping. Close and
cancellation do not wait for a child delayed in uninterruptible kernel I/O, and cannot terminate another operation.
Completed buffered operations have already waited for their helper to exit.

The private contract is owned by `src/collectors/utils/nd-file-reader.h`: fixed argv, raw read stdout, textual stat
stdout, and one numeric `NDFILE` stderr record. The client accepts success only with exit zero and exactly
`NDFILE 0 0\n`. Native errors require positive errno and exit one; policy errors require zero errno and exit one.
Malformed, missing, duplicate or oversized records, signals, and exit/result disagreement are transport failures.
Result and stat buffers are bounded by their numeric field formats. Streaming reads report terminal failures rather
than clean EOF; buffered reads discard all bytes on failure. There is no version negotiation or fallback protocol.

Native file errors preserve `errors.Is` for `safefile.ErrFile`, policy errors and OS causes. Helper launch and
transport errors match `safefile.ErrFile` but never unwrap an OS missing-file cause. This prevents optional-token
handling from hiding a missing helper. No error exposes raw helper stderr or file contents. Failures never fall back
to elevated direct reads.

Windows deliberately retains local service-account reads, native stream behavior and its existing authority. It
starts no helper and provides no Unix-style active-I/O cancellation. The boundary covers explicit credential-file
options, not SDK default credential acquisition or database DSNs.

HTTP consumers use ordinary `web.NewHTTPClient(ctx, cfg)` and `web.NewHTTPRequest(ctx, cfg)` APIs, receive standard
HTTP objects and manage only HTTP connections. Bearer-token files are reread per request. TLS construction invokes
one read per configured CA/certificate/key file; cookie loading retains its mtime cache and invokes Stat followed by
Open when a reload is needed. No caller retains a credential reader.

## Validation

Unit tests use a synthetic subprocess peer to exercise argv, terminal validation, file policies and process ownership.
They do not prove the C privilege transition. For actual C interoperability, supply a prebuilt merged helper at a path
traversable by its reduced identity; package tests do not invoke a compiler:

```sh
NETDATA_TEST_ND_RUN=/absolute/path/nd-run go test -race -run '^TestCReader' ./pkg/credentialfile
NETDATA_TEST_ND_RUN=/absolute/path/nd-run go test -run '^$' -bench '^BenchmarkCredentialRead256$' -benchmem ./pkg/credentialfile
```

Run from `src/go`. These tests use synthetic fixtures in a traversable temporary directory, including a root-owned
mode-0600 denial fixture when run as root. The benchmark compares safefile with a complete one-shot helper invocation
for each 256-byte read and reports allocations. Timings describe that machine and are not CI thresholds. The Go Credential File Tests workflow runs the real-helper
checks on macOS/Linux and adds Linux elevated-parent checks. Full setuid and Linux capability launch-shape validation
belongs to the helper's C tests.
