//! Multi-file journal session for sequential entry iteration.
//!
//! Provides a cursor-based API for iterating journal entries across multiple
//! journal files in a directory. Files are discovered, sorted, and traversed
//! sequentially — one at a time — with transparent cross-file transitions.
//!
//! # Example
//!
//! ```no_run
//! use journal_session::{JournalSession, Direction};
//!
//! let session = JournalSession::open("/var/log/journal/machine-id").unwrap();
//! let mut cursor = session.cursor(Direction::Forward).unwrap();
//! while cursor.step().unwrap() {
//!     let ts = cursor.realtime_usec();
//!     let mut payloads = cursor.payloads().unwrap();
//!     while let Some(data) = payloads.next().unwrap() {
//!         // data: &[u8] — raw FIELD=VALUE payload
//!     }
//! }
//! ```

mod cursor;

pub use cursor::{Cursor, CursorBuilder, Payloads};
pub use journal_core::Direction;

use journal_registry::repository::file::scan_journal_files;

/// Errors from session operations.
#[derive(Debug, thiserror::Error)]
pub enum SessionError {
    #[error("journal error: {0}")]
    Journal(#[from] journal_core::JournalError),

    #[error("repository error: {0}")]
    Repository(#[from] journal_registry::repository::error::RepositoryError),
}

/// Default mmap window size (8 MiB).
const DEFAULT_WINDOW_SIZE: u64 = 8 * 1024 * 1024;

/// A session representing a set of journal files to query.
///
/// Created from a directory path (scans for `.journal` files) or from an
/// explicit list of paths. Use [`cursor()`](JournalSession::cursor) or
/// [`cursor_builder()`](JournalSession::cursor_builder) to iterate entries.
pub struct JournalSession {
    files: Vec<journal_registry::repository::File>,
    window_size: u64,
    load_remappings: bool,
}

impl JournalSession {
    /// Open all journal files found under `path` (recursive scan).
    pub fn open(path: &str) -> Result<Self, SessionError> {
        let files = scan_journal_files(path)?;
        Ok(JournalSession {
            files,
            window_size: DEFAULT_WINDOW_SIZE,
            load_remappings: true,
        })
    }

    /// Start a [`SessionBuilder`] for custom configuration.
    pub fn builder() -> SessionBuilder {
        SessionBuilder::default()
    }

    /// Create a session from an explicit list of file paths.
    ///
    /// Paths that don't parse as journal files are silently skipped.
    pub fn from_files(paths: Vec<std::path::PathBuf>) -> Result<Self, SessionError> {
        let mut files = Vec::with_capacity(paths.len());
        for path in &paths {
            if let Some(f) = journal_registry::repository::File::from_path(path) {
                files.push(f);
            }
        }

        Ok(JournalSession {
            files,
            window_size: DEFAULT_WINDOW_SIZE,
            load_remappings: true,
        })
    }

    /// Create a cursor with the given direction and default settings.
    pub fn cursor(&self, direction: Direction) -> Result<Cursor, SessionError> {
        CursorBuilder::new(self.files.clone(), self.window_size, self.load_remappings)
            .direction(direction)
            .build()
    }

    /// Create a [`CursorBuilder`] for advanced configuration (filters, time bounds).
    pub fn cursor_builder(&self) -> CursorBuilder {
        CursorBuilder::new(self.files.clone(), self.window_size, self.load_remappings)
    }
}

/// Builder for creating a [`JournalSession`] with custom configuration.
#[derive(Default)]
pub struct SessionBuilder {
    directory: Option<String>,
    paths: Option<Vec<std::path::PathBuf>>,
    window_size: Option<u64>,
    load_remappings: Option<bool>,
}

impl SessionBuilder {
    /// Set the journal directory to scan.
    pub fn directory(mut self, path: &str) -> Self {
        self.directory = Some(path.to_string());
        self
    }

    /// Set explicit file paths instead of scanning a directory.
    pub fn files(mut self, paths: Vec<std::path::PathBuf>) -> Self {
        self.paths = Some(paths);
        self
    }

    /// Set the mmap window size in bytes (default: 8 MiB).
    pub fn window_size(mut self, size: u64) -> Self {
        self.window_size = Some(size);
        self
    }

    /// Whether to load ND_ field remappings (default: true).
    pub fn load_remappings(mut self, load: bool) -> Self {
        self.load_remappings = Some(load);
        self
    }

    /// Build the session.
    pub fn build(self) -> Result<JournalSession, SessionError> {
        let window_size = self.window_size.unwrap_or(DEFAULT_WINDOW_SIZE);
        let load_remappings = self.load_remappings.unwrap_or(true);

        let files = if let Some(dir) = self.directory {
            scan_journal_files(&dir)?
        } else if let Some(paths) = self.paths {
            let mut files = Vec::with_capacity(paths.len());

            for path in &paths {
                if let Some(f) = journal_registry::repository::File::from_path(path) {
                    files.push(f);
                }
            }

            files
        } else {
            Vec::new()
        };

        Ok(JournalSession {
            files,
            window_size,
            load_remappings,
        })
    }
}
