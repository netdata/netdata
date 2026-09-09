<!-- markdownlint-disable MD013 -->

# Process runtime operator model

## Operator goal

The profile answers five process-level resource questions for the exporter process represented by one collector job:

- CPU consumption;
- resident and virtual memory;
- open file descriptors; and
- the file-descriptor limit.

Each question remains a separate view because the measurements have different meaning and are not additive peers.

## Identity and composition

The source exposes no subordinate process identity label. One chart therefore represents the process selected by the
exporter's process collector, and the collector job supplies the deployment identity.

The profile deliberately omits `app`. When it composes with an application profile, that profile supplies application
identity; when it is selected alone, the job name supplies the fallback identity.

## Source boundary

Python platform metadata may accompany the process metrics. It is source evidence, but `python_info` is an information
family and does not enter the numeric writer.

The process start metric is a Unix timestamp, not elapsed uptime. Answering “how long has the process been running?”
requires subtracting the timestamp from current wall-clock time, which the chart template cannot express. The profile drops
that exact family instead of presenting a raw epoch or a misleading age.

## Display conventions

- A cumulative CPU-seconds counter becomes a per-second rate; seconds per second are displayed as consumed CPU cores.
- File-descriptor counts use the established compact `fds` unit.
