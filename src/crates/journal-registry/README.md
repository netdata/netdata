# journal-registry

In-memory registry of systemd journal files: it watches directories for
journal files, parses their metadata from file names, keeps the collection
current as files are created, rotated, or deleted, and answers the query
"which files could contain entries in this time range?". It never opens the
journal files themselves — it tracks paths, rotation status, indexed time
bounds, and filesystem events; reading entries is journal-core's job.

## How to use it

Create a monitor and a registry, watch directories, and pump filesystem
events into the registry from a background task:

```rust
use journal_registry::{Monitor, Registry};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let (monitor, mut event_receiver) = Monitor::new()?;
    let registry = Registry::new(monitor); // the registry owns the watcher

    registry.watch_directory("/var/log/journal")?;

    // Keep the collection current: feed every event back to the registry.
    let registry_clone = registry.clone();
    tokio::spawn(async move {
        while let Some(event) = event_receiver.recv().await {
            // Never fails today: per-file errors are logged and skipped.
            registry_clone.process_event(event).ok();
        }
    });

    Ok(())
}
```

`Registry` is a cheap `Clone` (shared state behind an `Arc<RwLock<..>>`), so
handles can be shared freely across tasks.

Query for files in a time range — a half-open `[start, end)` window in
seconds since the Unix epoch, as `journal_common::Seconds`:

```rust
use journal_common::Seconds;

let files = registry.find_files_in_range(Seconds(1_700_000_000), Seconds(1_700_000_600))?;

for file_info in files {
    println!("{}", file_info.file.path());
}
```

Each result is a `FileInfo`: the `File` plus its indexed `TimeRange`.

After indexing a file, report its coverage so later queries can prune it:

```rust
// start_time/end_time: the file's indexed bounds; indexed_at: when they
// were computed; online: true while the file was still being written
// (Active), false for archived files (Bounded, final bounds). Overwrites
// any previous row for the file.
registry.update_time_range(&file, start_time, end_time, indexed_at, online);
```

`registry.unwatch_directory(path)` stops tracking a root and drops its files
(machine-id subdirectories included). Errors are rare: only `Monitor::new`,
`watch_directory`, and `unwatch_directory` return `RegistryError` (watcher
or initial-scan failures); `process_event` and `find_files_in_range` always
return `Ok` today, and `update_time_range` is infallible.

## How it works

### File names

Journal files follow systemd's naming convention, parsed right to left:

- Active: `system.journal`, `user-1000.journal`, `remote-<host>.journal` —
  basename `system`, `user-<uid>`, or `remote-<host>`; any other basename
  parses as an unknown source.
- Archived (rotated): the basename gains
  `@<seqnum_id>-<head_seqnum>-<head_realtime>` before `.journal`, where
  `seqnum_id` is a UUID and the numbers are hex; `head_realtime` is the
  first entry's timestamp in microseconds since epoch.
- Disposed (corrupted files renamed by journald):
  `@<timestamp>-<number>.journal~`, both hex. The `@` part is required — a
  bare `.journal~` does not parse.

Paths must be absolute. A machine-id directory may sit above the file:
`<machine-id>` or `<machine-id>.<namespace>` for a journal namespace; files
directly in the watched root are tracked with no machine id (a dotted
directory whose machine-id half is not a UUID, e.g. `host.example.com`,
rejects the file). Anything that does not parse is not a journal file for
this crate: the parser returns `None` and such paths are skipped — create
and remove events log a warning, the rename path skips them silently.

### Collection layout

Files are organized directory → origin → chain. The directory key is the
machine-id directory's parent — the journal root in the standard
`<root>/<machine-id>/` layout — or the file's own directory when no machine
id parsed. The origin is machine id + optional namespace + source, and each
chain holds one origin's files sorted: disposed first, then archived files
chronologically (by first-entry time), the active file last. Insertions and
removals binary-search into the sorted chain, so queries can rely on the
order.

### Time-range queries

`find_files_in_range` applies two filters. The chains first, from file-name
timestamps: each archived file covers the span from its first entry to the
next file's first entry, the last archived file and the active file cover to
infinity, and disposed files never match — deliberately over-inclusive.
Then the per-file metadata overlay: files indexed as `Bounded` are kept only
when their bounds overlap the window; `Unknown` (not yet indexed) and
`Active` (still growing, so its recorded end lags behind writes) always
pass. Results are chronological within each chain and unordered across
chains; an empty or inverted range returns nothing. The overlay only ever
drops files — it never resurrects one the chains rejected.

### Filesystem events

`watch_directory` scans the tree recursively (every discovered file starts
at `TimeRange::Unknown`) and registers a recursive `notify` watch, so a file
created between the scan and the watch is picked up only on its next event
(typically the rotation rename). `process_event` folds events in: creates
insert files, removes drop them (their indexed bounds go with them), and
renames — journald rotating the active file to its archived name — remove
the old path and insert the new one as `Unknown`, so a rotated file is
re-indexed instead of inheriting stale bounds. Rename notifications without
both paths are logged and skipped, and every other event (content writes and
alike) leaves the file set untouched. The monitor forwards events unfiltered
and without debouncing.

## Where it fits

The crate depends only on journal-common (the `Seconds` type and the
collection aliases) and is consumed by journal-core (a `repository`
re-export shim), journal-index, journal-engine, journal-function, and
journal-log-writer; netflow-plugin uses the published twin
`journal-sdk-registry` instead of this crate. The only cargo feature is
`allocative`, which adds memory-profiling derives. The precise contracts
live in the module docs: `src/registry/` for the runtime and
`src/repository/` for the data model.
