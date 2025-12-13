use super::error::Result;
use notify::{Event, RecommendedWatcher, RecursiveMode, Watcher};
use std::path::Path;
use tokio::sync::mpsc;
use tracing::warn;

/// File system watcher that sends events through an async channel
#[derive(Debug)]
pub struct Monitor {
    /// The watcher instance
    watcher: RecommendedWatcher,
}

impl Monitor {
    /// Create a new monitor with an event receiver channel
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

    /// Start watching a directory recursively
    pub fn watch_directory(&mut self, path: &str) -> Result<()> {
        self.watcher
            .watch(Path::new(path), RecursiveMode::Recursive)?;

        Ok(())
    }

    /// Stop watching a directory
    pub fn unwatch_directory(&mut self, path: &str) -> Result<()> {
        self.watcher.unwatch(Path::new(path))?;
        Ok(())
    }
}
