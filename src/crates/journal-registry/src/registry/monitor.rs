//! Directory watcher for the journal stack: wraps the `notify` crate's
//! `RecommendedWatcher` and forwards every event it produces to the
//! unbounded tokio channel whose receiver `Monitor::new` returns. Events
//! arrive raw — unfiltered, no debouncing at this layer;
//! [`crate::Registry::process_event`] does the filtering,
//! folding create/remove/rename into the repository and ignoring the rest.
//! notify's own error results are dropped silently by the `Ok` filter in
//! `new`. The notify callback runs on notify's event-loop thread
//! (notify-rs/notify 8.2), not on a tokio worker, so the
//! handoff cannot await and uses the unbounded channel's synchronous,
//! never-blocking `send`: events pile up until a tokio task drains the
//! receiver (`otel-legacy-logs/src/handler.rs`; constructing the
//! monitor needs no runtime). Dropping the receiver turns every later send
//! into the `warn!` in `new`, losing events. All three fallible calls
//! propagate `notify::Error` as [`crate::RegistryError::Notify`].
//! Consumers: otel-legacy-logs drives the
//! watch → process_event → find_files_in_range loop
//! (`otel-legacy-logs/src/handler.rs`) and journal-function
//! re-exports `Monitor` (`journal-function/src/lib.rs`).
use super::error::Result;
use notify::{Event, RecommendedWatcher, RecursiveMode, Watcher};
use std::path::Path;
use tokio::sync::mpsc;
use tracing::warn;

/// Directory watcher for the journal stack; the module docs hold the
/// channel and event semantics.
#[derive(Debug)]
pub struct Monitor {
    watcher: RecommendedWatcher,
}

impl Monitor {
    /// Creates the watcher and its event channel. A watcher-initialization
    /// failure surfaces as `RegistryError::Notify`; afterwards every
    /// watcher event is sent to the returned receiver until it is dropped.
    pub fn new() -> Result<(Self, mpsc::UnboundedReceiver<Event>)> {
        let (event_sender, event_receiver) = mpsc::unbounded_channel();

        let watcher = RecommendedWatcher::new(
            move |res| {
                if let Ok(event) = res {
                    if let Err(e) = event_sender.send(event) {
                        warn!("Failed to send file system event: receiver dropped ({})", e);
                    }
                }
            },
            notify::Config::default(),
        )?;

        Ok((Self { watcher }, event_receiver))
    }

    /// Recursively watches `path`, its subdirectories included. `Registry`
    /// scans the directory first and calls this
    /// ([`crate::Registry::watch_directory`]).
    pub fn watch_directory(&mut self, path: &str) -> Result<()> {
        self.watcher
            .watch(Path::new(path), RecursiveMode::Recursive)?;

        Ok(())
    }

    /// Stops watching `path` (repository cleanup is
    /// [`crate::Registry::unwatch_directory`]'s job).
    pub fn unwatch_directory(&mut self, path: &str) -> Result<()> {
        self.watcher.unwatch(Path::new(path))?;
        Ok(())
    }
}
