# Evidence limits

What a bundle structurally cannot answer. Reaching one of these is a result, not a failure: say which
limit you hit and name the live query, the second bundle, or the human context that would settle it.

The hard exclusions are owned by
`packaging/installer/SUPPORT-BUNDLE.md#what-is-never-collected`; this file is what those exclusions
mean for an investigation.

## Excluded by design

- **Metric values.** The database files are never collected. The only sample values present are the
  agent's own bounded self-monitoring windows. "The chart shows the wrong number", "there is a gap at
  03:00", "that spike is impossible" have **no evidence at all** in a standard bundle. Resolve with
  `query-netdata-agents` or `query-netdata-cloud`, or ask for the time window and a screenshot.
- **Machine-learning state.** Status only; no model data. "The anomaly rates look wrong" cannot be
  settled.
- **Registry state.** Excluded because it holds person identifiers and dashboard locations, so
  registry and dashboard-sync symptoms have configuration but no state.
- **Core dumps.** Metadata only - you learn a dump exists and matches the time. The stack available
  is whatever the status file recorded.
- **Anything outside netdata's own scope.** No full system journal, no other services' logs, no
  packet captures. Cross-service causality is out of reach.
- **Secrets.** Never collected from *standard* captures, with the documented streaming-key exception.
  This guarantee does **not** extend to a bundle carrying raw SNMP evidence: that store bypasses
  sanitization entirely and can contain whatever the devices returned, including credentials. The
  manifest says which case you have.

## Single host, single moment

- **The other end of a streaming pair.** A bundle is one node. "Child not on parent" needs the
  child's configuration and attempt *and* the parent's decision. Ask for both bundles - the streaming
  key is kept verbatim precisely so the two can be compared.
- **Cloud's own view.** Only the agent's half is present. "The agent says connected, Cloud says
  offline" is half-answerable at best.
- **Everything in front of the agent.** No reverse-proxy configuration or logs, no browser view, no
  certificate chain as the client sees it. A broken proxy leaves the bundle looking healthy.
- **The monitored target.** Nothing about the database, exporter, or device a collector talks to.
- **Fleet context.** Whether this crash signature is known, fixed, or widespread needs
  `triage-agent-events`.

## Window and cap limits

- **The collection window is a flag on the run**, and it bounds most *journal* captures, which are
  then additionally tailed. The updater journal is an exception: it is tail-capped only, so it can
  carry events from well outside the window. It does **not** bound the on-disk log files: those are copied with their
  own size cap and no time filter, so an older incident can still survive in a log tail on a quiet
  host. Configurations and API responses have their own caps. Treat the window as decisive for
  journal evidence and as a hint elsewhere.
- **The configuration sweep stops after a bounded number of files** on POSIX, and covers only a fixed
  set of subdirectories on Windows.
- **Caps cut at line boundaries**, and a capped tail with no line break is withheld entirely - so an
  empty file can mean "withheld", never assume "nothing happened".
- **A global deadline** can skip a collector. On POSIX a skipped command capture says so in its body
  and its manifest origin, but a skipped file copy or API read can return without writing either. On
  Windows the command helper also returns early, leaving no marker. So a deadline-skipped artifact is
  often indistinguishable from one that was never attempted.

Always state the window alongside a negative finding. "No errors in the log" means "none inside the
window that survived the caps".

## Missing structured history

- **Alert transitions** exist only as log text inside the window; there is no structured alert history
  capture. "Why did this alert fire last Tuesday" is usually unanswerable.
- **Per-collector protocol evidence** exists only for SNMP. Every other collector bottoms out at job
  state plus log text; going deeper requires the reporter to run the collector in debug mode.
- **Service-manager configuration on POSIX.** No unit or drop-in capture, so a restrictive sandbox is
  invisible except indirectly. Windows does capture its service definition.
- **Updater evidence on non-systemd installs**, where the updater keeps no persistent log.

## Human and account context

The bundle answers "what is this machine doing". It cannot answer:

- **Which account, space, room or plan** this node belongs to. Reported "unsupported" or "locked"
  states are commonly plan conditions presenting as technical failures.
- **What the reporter expected**, what changed recently, or what they already tried.
- **When the incident happened.** The bundle records its own generation time and carries plenty of
  event timestamps - log lines, the status file, capture headers - but nothing saying which moment
  the reporter means. Correlate with the timestamps it does carry; do not discard them.

Ask for these; do not infer them. A bundle without an incident timestamp and a symptom statement
cannot be triaged, only described.

## Windows coverage

A Windows bundle is materially thinner. `./windows.md` carries the verified list; consult it before
reading any Windows absence as a finding.

## Sanitization effects on analysis

- Pseudonyms are stable within one bundle only, and stop correlating past a large number of
  identities. The resolving map is never inside the archive.
- Differently sanitized files do not share literal identifiers, so joining standard captures against
  raw SNMP evidence on a device name does not work.
- Content can be withheld deliberately - binary input, a symlinked source, a sanitizer failure - and
  each says so in the file.
