# DEM process runtime owner

`Open` turns the administrator's `runtime:` configuration (`Config`) into the
process `Handle`. The mode decides who owns the files: `managed` releases live
in the DEM cache, `native` uses the package-manager payload, and `preloaded`
borrows a sealed administrator preload. Packaging sets `DefaultMode` to native
for DEB/RPM builds. Managed and native modes get a `Host`; a preload is a
runner source without one, outside managed retention. Setup problems never fail
`Open`: the handle's source reports them to browser jobs while RUM keeps
working. The `Host` is the runner source of managed and native modes
(`host.go`); the borrowed-claim, preload and unavailable sources and the
execution-use adapter live in `source.go`.

`Host` acquires the explicitly installed selection and its process protection
atomically at construction. That immutable release is used for the entire plugin
process. Enabling, disabling, updating or removing a monitor never downloads or
prepares runtime files. An installation made after an empty process start becomes
available after restarting the plugin; replacing an installation does not change
an existing process's running release.

`Run` sweeps storage at startup and hourly, even with no browser jobs. Selected
installations and active process/execution protection govern retention. There is
no per-monitor resource demand or first-use download.

`Ready` and `Dependencies` are pure memory lookups. Installation changes do not
publish readiness changes to the fixed process selection. Missing setup leaves
browser jobs waiting with installation and restart
guidance while RUM continues. Every runner probe and invocation obtains `BeginUse`
from the held process consumer, passes its completion handle to supervision, and
finishes with actual drainage proof. A failed execution admission can retry
against the same held release without permanently poisoning readiness.

`Status` distinguishes the process's running release identity from the store's
current selection. Setup and maintenance errors are separate. Cleanup failures do
not invalidate a usable runtime or alter a completed website measurement. Each
maintenance operation retains its error until its own successful retry; a healthy
invocation cannot erase an earlier execution-retirement failure.

`Close` follows joined jobs, joins the attached updater and the maintenance
worker, and releases process protection. The handle then closes the store.
Uncertain executions remain protected by their independent completion receipts.

`Host.NewUpdater` attaches the `Updater` to a managed host. It is a separately
run process service whose progress and errors `Status` reports. After explicit
signed installation consent, it checks latest within the first five minutes and
then every 23–25 hours. Installation exclusion precedes consent and discovery;
manual installations and an empty store perform no network access. Failed checks
retry on the next scheduled attempt. Download/qualification does not block
hourly storage maintenance.

Candidate qualification shares the monitoring engine's admission and fatal owner,
with a separate source and temporary work store. It never changes the startup
consumer or emits monitor history. Successful preparation changes next-process
selection and reports restart required; the updater never restarts the Agent.
Its `Close` cancels and joins update work; `Host.Close` calls it before
releasing runtime ownership. Unknown child drainage reaches the existing process
fail-stop path and keeps its receipts.
