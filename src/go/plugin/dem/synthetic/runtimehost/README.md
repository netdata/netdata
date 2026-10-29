# DEM process runtime owner

`Host` acquires the explicitly installed selection and its process protection
atomically at construction. That immutable release is used for the entire plugin
process. Enabling, disabling, updating or removing a monitor never downloads or
prepares runtime files. An installation made after an empty process start becomes
available after restarting the plugin; replacing an installation does not change
an existing process's running release.

`Run` sweeps storage at startup and hourly, even with no browser jobs. Selected
installations and active process/execution protection govern retention. There is
no per-monitor resource demand, first-use download, preparation retry or polling
for a new selection.

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

`Close` follows joined jobs, joins the maintenance worker, and releases process
protection. The caller then closes the Store. Uncertain executions remain protected
by their independent completion receipts.
