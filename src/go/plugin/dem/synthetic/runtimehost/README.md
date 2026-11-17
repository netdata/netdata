# DEM process runtime owner

`Open` turns the administrator's `runtime:` configuration (`Config`) into the process `Handle`. Setup problems never
fail `Open`: the handle's source reports them to browser jobs while RUM keeps working. Store internals (trust,
protection, repair, preloads, native packages) belong to [`browserruntime`](../browserruntime/README.md); the
cross-package rules are in [ARCHITECTURE](../../ARCHITECTURE.md#browser-runtime-ownership).

## Modes

The mode decides who owns the runtime files. Packaging sets `DefaultMode` to `native` for DEB/RPM builds.

| Mode | Files owned by | Runner source | Automatic updater |
|---|---|---|---|
| `managed` | DEM, in its managed store (the DEM cache) | `Host` | Yes, `Host.NewUpdater` |
| `native` | The package manager (the `netdata-dem-runtime` payload) | `Host` | No |
| `preloaded` | The administrator (a sealed preload) | A borrowed preload source, outside managed retention | No |

| File | Contents |
|---|---|
| `config.go` | `Config`, modes, validation, release loading, store configuration |
| `open.go` | `Open` and the process `Handle` |
| `host.go` | `Host`, the runner source of managed and native modes |
| `source.go` | The borrowed-claim, preload and unavailable sources and the execution-use adapter |
| `health.go` | Maintenance and execution-completion health logging |
| `update.go` | The automatic `Updater` |

## Fixed startup selection

- **Atomic acquisition.** `Host` acquires the explicitly installed selection and its process protection atomically at
  construction. That immutable release is used for the entire plugin process.
- **No per-monitor acquisition.** Enabling, disabling, updating or removing a monitor never downloads or prepares
  runtime files. There is no per-monitor resource demand or first-use download.
- **Restart to change.** An installation made after an empty process start becomes available after restarting the
  plugin; replacing an installation does not change an existing process's running release.
- **Readiness.** `Ready` and `Dependencies` are pure memory lookups. Installation changes do not publish readiness
  changes to the fixed process selection. Missing setup leaves browser jobs waiting with installation and restart
  guidance while RUM continues.
- **Uses.** Every runner probe and invocation obtains `BeginUse` from the held process consumer, passes its completion
  handle to supervision, and finishes with actual drainage proof. A failed execution admission can retry against the
  same held release without permanently poisoning readiness.
- **Reporting.** `Handle.Identity` reports the release the process runs, and `Handle.NativeStatus` the native package's
  ownership proof, for the runtime CLI.

## Maintenance and errors

- **Sweeps.** `Run` sweeps storage at startup and hourly, even with no browser jobs. Selected installations and active
  process/execution protection govern retention.
- **Separate errors.** A setup error keeps `Ready` failing, while cleanup and execution-completion failures are logged
  with their recovery. They do not invalidate a usable runtime or alter a completed website measurement.
- **Sticky errors.** Each maintenance operation retains its error until its own successful retry; a healthy invocation
  cannot erase an earlier execution-retirement failure.

## Automatic updater

`Host.NewUpdater` attaches the `Updater` to a managed host. It is a separately run process service that logs check
failures and their recovery.

- **Consent and schedule.** After explicit signed installation consent, it checks latest within the first five minutes
  and then every 23–25 hours. Installation exclusion precedes consent and discovery; manual installations and an empty
  store perform no network access.
- **Failures.** Failed checks retry on the next scheduled attempt. Download and qualification do not block hourly
  storage maintenance.
- **Qualification.** Candidate qualification shares the monitoring engine's admission and fatal owner, with a separate
  source and temporary work store. It never changes the startup consumer or emits monitor history.
- **Activation.** Successful preparation changes the next process's selection, which a restart activates; the updater
  never restarts the Agent.

## Shutdown

- `Close` follows joined jobs, joins the attached updater and the maintenance worker, and releases process protection.
  The handle then closes the store.
- The updater's `Close` cancels and joins update work; `Host.Close` calls it before releasing runtime ownership.
- Uncertain executions remain protected by their independent completion receipts. Unknown child drainage reaches the
  existing process fail-stop path and keeps its receipts.
