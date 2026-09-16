# Orientation

The minimum causal model needed to read bundle evidence. Each subsystem's depth belongs to its owner
document, cited inline. Read this once; return to the owners when a specific result needs explaining.

## What produces what

The daemon starts, reads configuration, starts internal collectors in-process and external plugins as
child processes, writes samples into the database, evaluates health against those samples, optionally
streams everything to a parent, and optionally connects to Cloud. Almost every bundle artifact is a
by-product of one of those stages, which is why locating the stage locates the evidence.

- **Configuration** is merged, not read from one file. On-disk files, environment overrides and
  dynamic configuration combine into an effective running config, and unrecognized options are
  annotated rather than rejected. This is why the effective config beats any on-disk file, and why a
  user insisting "I set that" is usually right about the file and wrong about the effect.
  Owner: `src/daemon/config/README.md`, and `src/daemon/dyncfg/README.md#api-access` for the dynamic
  layer.
- **Collectors** are internal plugins, external plugins speaking a line protocol over standard
  output, and the Go plugin hosting most modern modules. External plugins are separate processes with
  their own privileges, which is why a capability or setuid bit decides whether a collector produces
  anything. Owner: `src/plugins.d/README.md` for the protocol,
  `src/collectors/README.md#collector-privileges` for what each plugin needs.
- **Jobs** are a collector module bound to a configured or discovered target. A job has a lifecycle -
  candidate, accepted, running, failed - and that state now lives in dynamic configuration, captured
  in the bundle's runtime area. Owner: `src/go/plugin/go.d/README.md#troubleshooting` for the
  operator view, `src/collectors/REFERENCE.md#troubleshoot-a-collector` for the debug-mode recipe.
- **The database** stores samples in tiers of decreasing resolution. Retention is bounded by both
  time and size per tier, and the size bound is a soft target enforced by deleting whole files, so
  the finest tier overshoots most. Owner: `src/database/README.md#tiers` and
  `src/database/README.md#retention-size-enforcement`.
- **Health** evaluates alerts against collected data. An alert is enabled only when the chart it
  targets starts collecting, so an alert aimed at a context that never appears is never enabled - and
  that is indistinguishable from "not firing" unless you check. Owner:
  `src/health/README.md` for the lifecycle, `src/health/REFERENCE.md#troubleshooting-decision-trees`
  for the decision trees.
- **Streaming** sends everything from a child to a parent, authenticated by an API key, with the
  parent deciding acceptance. Rejections are explicit and named in the parent's log. Owner:
  `src/streaming/README.md#troubleshooting`.
- **Cloud** is a separate connection from streaming. A node can stream fine and be offline in Cloud,
  or the reverse. Identity is a persisted machine identifier plus a claim id. Owner:
  `src/claim/README.md#connection-troubleshooting`.
- **Logs** go to a dedicated journal namespace on systemd, to files elsewhere, and to dedicated
  event-log channels on Windows. Fields identify the source, the collector and the job. Owner:
  `src/libnetdata/log/README.md#log-fields`, and
  `src/libnetdata/log/README.md#linux-using-journalctl-to-query-netdata-logs` for the namespace.

## Consequences that decide how you read a bundle

- **Config on disk is not config in effect.** Dynamic configuration is stored separately from the
  config directory, so a job or alert created in the UI appears in neither the config files nor the
  config tree listing. Where the bundle carries the dynamic layer, read it; where it does not
  (Windows), say the evidence is missing rather than concluding the job does not exist.
- **A collector failing and a collector lacking privileges look identical from the chart.** The chart
  alone is ambiguous; job state and the collector log usually separate them first. Reach for the
  permissions area when those are absent or inconclusive - it is the only artifact carrying
  capabilities and security contexts rather than mode bits.
- **Streaming and Cloud fail independently.** Establish which one the reporter actually means before
  choosing evidence; "offline" is used for both.
- **Health state is a snapshot; health history is log text.** The bundle captures current alert state
  as structured data, but transitions only as log records bounded by the collection window.
- **The database is excluded.** No sample values are in a standard bundle beyond the agent's own
  bounded self-monitoring, so questions about metric values are unanswerable here by construction.
  See `./evidence-limits.md`.
- **A dead agent still yields evidence.** The status file, the logs, the permissions area and the
  build information run from disk and from the binary, so a crash investigation does not need a
  living API.
