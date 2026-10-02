# No data, collectors, and "my config is ignored"

A chart that is missing, a collector that produces nothing, a job that fails, or a configuration the
agent appears to ignore. These share evidence, so they share a guide.

Owners: `src/collectors/REFERENCE.md#troubleshoot-a-collector` for the debug-mode recipe in every
language; `src/go/plugin/go.d/README.md#debugging-a-specific-collector` for the Go plugin;
`src/go/plugin/agent/jobmgr/ARCHITECTURE.md#candidate-lifecycle` and
`src/go/plugin/agent/jobmgr/ARCHITECTURE.md#autodetection-retries` for how a job reaches or fails to
reach a running state; `src/go/plugin/agent/jobmgr/ARCHITECTURE.md#service-discovery` for discovered
targets; `src/daemon/config/README.md#configuration-section-details` and
`src/daemon/dyncfg/README.md#api-access` for the configuration model;
`src/collectors/README.md#collector-privileges` for what a plugin needs in order to work at all.

## Order of checks

1. **Job state.** The dynamic configuration tree in the runtime area is the authoritative record of
   which jobs exist and what state each is in - running, failed, or an accepted template that never
   became a job. Start here; it converts a vague "no data" into a specific job with a specific state.
2. **Distinguish the three shapes.** "No chart at all" means the job never produced data. "A chart
   with gaps" means it produced data and stopped. "A chart with wrong values" is a data question the
   bundle cannot answer at all (`./evidence-limits.md`). They have different causes; establish which
   one the reporter means.
3. **The collector log.** Narrow to the collector and the job rather than reading the whole file.
   Job-scoped fields identify both. An empty collector log is not a reason to stop - on systemd the
   agent logs to its own journal namespace, captured separately.
4. **Configuration, effective first.** The effective running config is authoritative and annotates
   unrecognized options, which resolves most "my config is ignored" reports outright. Then the
   dynamic configuration area, because a UI- or API-created entry **overrides** the plugin file for
   that job - reading the file first is how triage blames a setting that is not in effect. Read the
   plugin's own config last.
5. **Privileges.** A plugin that lost a capability or a setuid bit produces nothing and commonly logs
   nothing useful. Go to `./environment.md` - this is one of the most common causes of "the collector
   is broken" and it is invisible in a directory listing.
6. **Visibility.** In containers and restricted namespaces the agent may be unable to see what it is
   asked to collect. The mount table, cgroup version and virtualization detection settle that; see
   `./environment.md`.
7. **Build-time exclusion.** A plugin disabled when the agent was built never appears at runtime. The
   build cache capture is the only artifact that shows this, and it is the answer when a collector is
   missing on one host and present on another with the same configuration.

## What the bundle settles well

- Which jobs exist and which failed, when the API answered.
- Whether the configuration in effect matches the configuration on disk.
- Whether a plugin has the privileges it needs.
- Whether the plugin was built in at all.

## What it does not settle

- **The failing request or response.** There is no per-job protocol capture for any collector except
  SNMP. The bundle gives you the job's state and the log text; the next step is asking the reporter
  to run the collector in debug mode using the recipe in the owner document.
- **Metric correctness.** No sample values are collected. See `./evidence-limits.md`.
- **Anything about the target system** - the database being monitored, the exporter being scraped.

## Recurring causes

Written as patterns, not as a measured ranking.

- **Job configuration or reachability** - the target refused, timed out, or required credentials the
  job did not have.
- **Missing privileges on the monitored system**, not on the agent - a monitoring user without the
  grants the module needs.
- **Certificate validation** against a self-signed endpoint, where the module needs to be told to
  accept it.
- **Plugin privileges lost** on the agent side, commonly after a restore, a manual copy, or a
  container rebuild.
- **Upgrade regressions**, where a module changed its configuration shape or its defaults.
- **Discovery finding nothing**, which is a discovery-configuration question rather than a collector
  question.

## Traps

- **A job that is absent is not the same as a job that failed.** A template with no job means nothing
  was configured or discovered; a failed job means something was tried. The tree distinguishes them;
  the chart does not.
- **"It works when I run it by hand."** Running a plugin interactively as an administrator changes
  both the privileges and the environment. That it works by hand is evidence *for* a privilege or
  environment cause, not against one.
- On **older bundles**, a path named for go.d job statuses actually contains python.d state. go.d
  stopped writing job state to disk; it only exists in the dynamic configuration tree now. Do not
  read that older path as go.d.
- **Windows carries no dynamic configuration area**, so step 1 has no evidence there and UI-created
  jobs are invisible. See `./windows.md`.
- When the API was down, the dynamic configuration tree is absent. That is not "no jobs" - it is no
  evidence. Fall back to the plugin configuration and the logs, and say what you could not check.
