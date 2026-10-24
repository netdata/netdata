# Windows bundles

A Windows bundle is produced by a different implementation with different capabilities. Establish
that you have one - the manifest tool version carries a Windows suffix - before applying any POSIX
expectation.

Owners: `src/libnetdata/log/README.md#using-event-tracing-for-windows-etw` and
`src/libnetdata/log/README.md#channels` for where the agent logs;
`src/libnetdata/log/README.md#windows-using-event-viewer-to-view-netdata-logs` for reading it
interactively; `packaging/windows/WINDOWS_INSTALLER.md#working-with-netdata-on-windows` for the
installed layout and `packaging/windows/WINDOWS_INSTALLER.md#where-the-claim-settings-are-stored`
for claim persistence; `docs/install/windows-release-channels.md#troubleshooting-common-issues` for
install and channel problems; `src/collectors/windows-events.plugin/README.md#faq` for the event
collector.

## Logs are the first difference

The agent logs into dedicated event-log channels, not to files. One merged capture covers those
channels plus the legacy channel and Netdata records from the Application log, ordered for triage
with the highest-volume channel placed last on the smallest budget.

The capture also records **channel state**, and that matters: a disabled channel is otherwise
indistinguishable from the agent having logged nothing. Check it before reading absence as silence.
The capture does not carry maximum size or retention mode, so it identifies a disabled channel but
cannot confirm a full one - ask for that separately when it matters.

A default install produces no daemon records in the legacy channel at all, so a report of "nothing in
the event log" usually means the wrong channel was inspected interactively. A host can be configured
to log to files instead, and the bundle collects those when present - check before concluding the
agent logged nothing.

## Windows-only evidence

- **The service definition** - start mode, the account the service runs as, the binary path, state
  and exit code. POSIX has no equivalent capture at all, so this is the one area where Windows is
  better covered.
- **The performance-library dump**, the most common Windows collector surface. When it cannot run it
  holds a marker stating why - not elevated, timed out, or an access error - so the reason is visible
  rather than the file simply being missing.
- **Access control detail** - lists with inheritance state, protected-list detection, integrity
  labels, and alternate data streams. A download-marker stream records that the file arrived from another
  machine; it is provenance, not proof that Windows blocked execution, so do not conclude a failure
  from the stream alone. A protected list means inheritance was disabled, a common post-restore
  breakage.
- **Installer registry information** and the install tree.
- **The claim configuration file**, which POSIX does not have in that form.

## What a Windows bundle does not contain

Verified absent from the Windows implementation. Each of these turns a normal line of investigation
into "no evidence", so state it rather than concluding from silence:

| Missing | Consequence |
|---|---|
| The dynamic configuration area | UI-created jobs and UI-edited alerts are invisible |
| The persisted silencers | "Why are alerts silent" has no evidence |
| Arbitrary user config sweep | Only a fixed set of plugin subdirectories is collected; anything outside them is unseen |
| The agent's process environment | Proxy and claiming variables as the service sees them are unavailable |
| Per-thread CPU | Resource cost cannot be attributed to a subsystem |
| Descriptor count, process limits, zombie check | Leak and reaping questions have no evidence |
| Coredump metadata | No confirmation that a dump exists |
| The updater log | Update failures leave no trace |
| A kernel-message equivalent | An external kill leaves no trace |

Two further behavioural differences that change how you read absence:

- **A failed API call leaves no file at all**, where POSIX writes a marker carrying the failure. So
  on Windows a missing runtime capture conflates "the endpoint failed" with "it was never attempted".
- **Command captures carry a provenance header but no exit trailer**, because the launch mechanism
  does not surface a meaningful exit code. You cannot check whether a Windows command capture
  succeeded the way you can on POSIX.

Some state captures are also less strict than their POSIX counterparts - the state listing shows
filenames with only credential-bearing names withheld, where POSIX reports aggregate counts only, and
the resolver capture does not withhold search domains. Treat a Windows bundle as marginally more
identifying when deciding how to handle it.

## Traps

- **A service reporting "running" is not proof the agent is working**, particularly after sleep or
  hibernate. See `./false-signals.md`.
- A node reported as unsupported or locked is commonly a licensing or plan condition presenting as a
  technical failure, not an agent fault. That is account context the bundle cannot carry at all - see
  `./evidence-limits.md`.
- Claim state not surviving a reinstall is a known Windows pattern; the installer owner document
  states where those settings live.
- Do not compare a Windows bundle's coverage against a POSIX one and read the difference as a broken
  collection. Check the table above first.
