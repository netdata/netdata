# False signals

Evidence that reads as a diagnosis and is not. Check this file before reporting any conclusion; each
entry below has cost real investigations time.

## A normal startup line that reads like a crash cause

The daemon logs a line about out-of-memory protection during normal startup, reporting the system
memory it observed. It is informational. It appears in every healthy agent's log, and it appears
directly before a crash for the same reason every startup line does - because the agent started.

Do not cite it as a cause. The exit reason lives in the daemon status file; memory pressure is
established from the kernel messages and the status file's own memory and disk fields, never from
this line.

A related pattern: an informational line about the process out-of-memory score read as evidence of an
imminent kill, when the real fault was an architecture mismatch between the installed binary and the
host.

## Capabilities that appear set and do not work

Setting file capabilities inside a container that lacks the privilege to grant them **succeeds
silently**. The command exits zero, and a subsequent read shows the capabilities present. They are
inert.

Neither a directory listing nor a capability read reveals this. What reveals it is the combination
the permissions area captures - mode, capabilities, extended attributes and mandatory access control
state together, read against the container's own capability set from the container context.

This is the highest-cost trap in the permissions class: every surface an investigator would normally
check reports success.

## A Windows service reporting "running"

After sleep or hibernate the service state can report running while the agent is not functioning.
The service manager's view is not a health check.

Corroborate with the event-log channels, the socket inventory, and whether the API answered at all
before accepting "the service is up" as evidence that the agent is up.

## Reinstalling does not change node identity

The machine identifier persists in the agent's state directory, outside the installation. Reinstalling
preserves it by design.

The consequence: cloned images and restored virtual machines share one identity and replace each other
in Cloud indefinitely, presenting as nodes flapping, disappearing, or "coming back after removal". The
streaming receiver names this case explicitly when a known machine identifier arrives under a
different hostname.

So "have you tried reinstalling" is not a remedy here, and a reporter who has already reinstalled
several times has not ruled the cause out - they have confirmed it.

## The runtime area being empty

A marker replaces the runtime captures whenever the agent's API did not answer. The marker itself only
says the API was unreachable and points at the logs and the status file; the three possible causes -
the agent was down, the API was bound somewhere other than the loopback address, or it required
authentication - are spelled out in the bundle's own `README.md`. Read that before concluding.

Only the first is "the agent was down". Separate them with the process capture, the socket inventory
and the effective web configuration before reporting an outage.

## A missing file

The single most common analysis error in this whole skill. A file can be missing for at least seven
reasons, and only one of them is "the thing did not exist". Resolve every absence through
`./bundle-map.md` before it becomes a finding.

On Windows this is worse: a failed API call leaves no file at all, where POSIX leaves a marker with
the failure code.

## An empty file

An empty or withheld body is not an empty source. A capped tail with no line break is withheld whole;
a file with NUL bytes is withheld whole; a sanitizer failure withholds content deliberately. Each
writes a stating line rather than failing silently - read it.

## A "complete" status that is not a snapshot

Where a status reports that evidence collection completed, that means the selected files were copied
- not that they describe one simultaneous moment, and not that every source produced evidence. Files
have independent timestamps and can rotate mid-copy.

## Pseudonyms that look like they correlate

Pseudonyms are stable **within one bundle**. Across two bundles from the same customer they are
unrelated, and past a large number of distinct identities they stop correlating even within one
bundle, becoming non-correlating placeholders.

Never join two bundles on a pseudonym. The map that resolves them is written beside the archive and
is not inside it.

## "It works when I run it by hand"

Running a plugin interactively changes the privileges and the environment it gets. That it works by
hand is evidence *for* a privilege or environment cause, not against one - the two runs are not
comparable.

## Rankings in these guides

Every "recurring cause" list in this skill is drawn from support-corpus analysis and is written as a
pattern, not as a measured ranking. Do not harden them into frequency claims, and do not present them
to a reporter as established fact about their case.
