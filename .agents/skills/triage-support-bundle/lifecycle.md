# Crashes, resource use, retention, and installs

The agent's own lifecycle: why it stopped, why it costs what it costs, why it keeps what it keeps,
and how it got onto the host.

Owners: `src/daemon/README.md#debugging` and `src/daemon/README.md#debugging-crashes` for crash
investigation; `src/database/README.md#tiers` and
`src/database/README.md#retention-size-enforcement` for retention;
`src/database/README.md#monitoring-retention-utilization` for reading utilization;
`src/libnetdata/log/README.md#message-ids` for isolating fatal records.

## Crashes and failures to start

**Read the daemon status file first.** It records the last exit reason and cause, the signal and
fault address, a fatal object naming file, function, line, message and error number, stack traces,
and restart and crash counts - alongside database, memory and disk state at the time. It is the same
data the fleet crash telemetry carries, which is what makes escalation to `triage-agent-events`
cheap once you hold the signature.

Then, in order:

1. **Was it a crash or a kill?** The kernel-message capture distinguishes them. Note its filter is
   narrow - a container-runtime kill or a service-manager memory policy action does not match it and
   leaves nothing, so absence here is weak evidence.
2. **The logs around the exit**, from the journal namespace on systemd or the log files elsewhere.
3. **Restart and crash counts** in the status file, which separate a one-off from a loop.
4. **Coredump metadata**, which proves a dump exists and matches the time. The dump itself is never
   collected; the stack you get is the one the status file recorded.
5. **How it was built**, when the failure looks like a missing or misbehaving component. The build
   cache capture is a superset of build information and shows disabled plugins and custom compiler
   flags, which have caused storage-engine faults.

A distinctive state worth recognising: no process exists, but the status file still says running.
That is unclean termination - the agent never got to update the file. The summary raises it
explicitly.

## Resource use

The bundle is built to remove the "send a screenshot" round trip.

1. **Per-thread CPU** is the key artifact: it names which subsystem is hot rather than reporting one
   undifferentiated process. POSIX only.
2. **The agent's own bounded resource windows** in the runtime area cover the recent past.
3. **Process status, limits and descriptor count** for leak and limit questions. These are derived
   from `/proc`, so they are absent on macOS and BSD as well as Windows - absence there is the
   platform, not a capture failure.
4. **Database size and metric counts**, because cost scales with what is retained and how many
   entities exist.
5. **Machine-learning status**, since training is a recognised cost centre and can be turned down.

Recurring causes, as patterns: a plugin thread spinning; kernel-level bugs in polling primitives that
present as agent CPU; metadata database growth; database pressure on parents receiving many children;
and a plugin lacking privileges retrying continuously.

Without per-thread evidence every cause looks the same, so on Windows - which has no per-thread
capture - resist attributing cost to a subsystem.

## Retention

Retention is bounded by **both** time and size per tier, and the size bound is a soft target enforced
by deleting whole files, so the finest tier overshoots most. Very small configured sizes are silently
raised to a floor.

1. **Per-tier disk usage** against the effective database configuration, never against the on-disk
   file or the reporter's expectation.
2. **Per-tier retention** from the runtime information capture.
3. **Disk space and the filesystem** the database lives on.
4. On a parent, remember that children share the parent's per-tier budget.

The most common gap is an expectation gap rather than a fault: users expect time-based retention and
the defaults are bounded by size. Say that plainly rather than treating it as user error.

## Install and update

1. **Install type**, which decides every subsequent troubleshooting and update path.
2. **The install-time environment**, carrying the method, flags, release channel and any custom
   compiler flags.
3. **Package manager state** for version skew.
4. **The updater log**, systemd only. On other init systems the updater keeps no persistent log, so
   update failures leave no evidence at all.
5. **Build information**, which the bug template requires and which works with the daemon down.

Recurring causes, as patterns: repository metadata failures; an unwritable or missing temporary
directory; distribution detection gaps and end-of-life releases; missing prerequisite repositories;
signing keys not imported; and updater environment files that no longer match the install.

A frequent misreading: a reporter on the nightly channel who believes they are on the latest stable.
The install-time environment settles it.

## Traps

- A normal startup line about memory protection reads like a crash cause and is not - see
  `./false-signals.md`.
- Expected database growth is not a leak; compare against configured limits before calling it one.
- The absence of kernel messages does not exclude an external kill, because the capture is filtered.
- Retention questions answered from the on-disk configuration rather than the effective one are
  usually answered wrongly.
