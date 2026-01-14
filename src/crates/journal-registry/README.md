# journal-registry

This crate watches directories for journal files, parses their metadata from
filenames, organizes them efficiently, and keeps the collection updated as
files are created, rotated, or deleted.

## How to use it

Start by creating a monitor and registry, then watch directories:

```rust
use journal_registry::{Registry, Monitor};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let (monitor, mut event_receiver) = Monitor::new()?;
    let registry = Registry::new(monitor);

    registry.watch_directory("/var/log/journal")?;

    // Process filesystem events in the background
    let registry_clone = registry.clone();
    tokio::spawn(async move {
        while let Some(event) = event_receiver.recv().await {
            registry_clone.process_event(event).ok();
        }
    });

    Ok(())
}
```

Query for files in a time range:

```rust
let files = registry.find_files_in_range(start_sec, end_sec)?;

for file_info in files {
    println!("{}", file_info.file.path());
}
```

Update metadata after indexing:

```rust
registry.update_time_range(&file, start_time, end_time, indexed_at, online);
```

## How it works

Journal files follow systemd's naming convention. Active files are named
like `system.journal` or `user-1000.journal`. Archived files append
metadata: `system@<seqnum_id>-<head_seqnum>-<head_realtime>.journal`.
Corrupted files end with `.journal~`.

The registry organizes files into a three-level hierarchy:
directories contain origins (system, user, remote), and each origin has a
chain of files sorted by status and time. Disposed files come first,
followed by archived files in chronological order, with the active file
last. This ordering makes time-range queries efficient.

Files start with unknown time ranges. After you index them and call
`update_time_range`, the registry uses this metadata to filter queries
appropriately. Files with bounded ranges are included only if they overlap
the requested time window, while unknown and active files are always included.

The monitor watches directories recursively and sends create, delete, and
rename events through an async channel. The registry processes these to keep
the collection current.
