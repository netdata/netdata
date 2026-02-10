use journal_registry::Origin;
use std::time::Duration;

/// Controls when journal files should be rotated
///
/// A file rotates when *any* configured limit is exceeded. If all fields are `None`,
/// files never rotate automatically.
#[derive(Debug, Copy, Clone, Default)]
pub struct RotationPolicy {
    /// Maximum file size
    pub size_of_journal_file: Option<u64>,
    /// Maximum duration of head/tail entries
    pub duration_of_journal_file: Option<Duration>,
    /// Maximum number of log entries
    pub number_of_entries: Option<usize>,
}

impl RotationPolicy {
    /// Specifies the maximum journal file size.
    pub fn with_size_of_journal_file(mut self, size_of_journal_file: u64) -> Self {
        self.size_of_journal_file = Some(size_of_journal_file);
        self
    }

    /// Specifies the maximum duration between head/tail entry.
    pub fn with_duration_of_journal_file(mut self, duration_of_journal_file: Duration) -> Self {
        self.duration_of_journal_file = Some(duration_of_journal_file);
        self
    }

    /// Specifies maximum number of entries.
    pub fn with_number_of_entries(mut self, number_of_entries: usize) -> Self {
        self.number_of_entries = Some(number_of_entries);
        self
    }
}

/// Controls when old journal files should be deleted.
///
/// Old files are removed to satisfy *all* configured limits. Removal starts with
/// the oldest files first. If all fields are `None`, files are never deleted.
#[derive(Debug, Copy, Clone, Default)]
pub struct RetentionPolicy {
    /// Maximum number of journal files to keep
    pub number_of_journal_files: Option<usize>,
    /// Maximum total size of all journal files (in bytes)
    pub size_of_journal_files: Option<u64>,
    /// Maximum age of files to keep
    pub duration_of_journal_files: Option<Duration>,
}

impl RetentionPolicy {
    /// Specifies maximum number of journal files.
    pub fn with_number_of_journal_files(mut self, number_of_journal_files: usize) -> Self {
        self.number_of_journal_files = Some(number_of_journal_files);
        self
    }

    /// Specifies maximum size of journal files.
    pub fn with_size_of_journal_files(mut self, size_of_journal_files: u64) -> Self {
        self.size_of_journal_files = Some(size_of_journal_files);
        self
    }

    /// Specifies maximum duration of journal files.
    pub fn with_duration_of_journal_files(mut self, duration_of_journal_files: Duration) -> Self {
        self.duration_of_journal_files = Some(duration_of_journal_files);
        self
    }
}

/// Configuration for a journal log.
#[derive(Debug, Clone)]
pub struct Config {
    pub origin: Origin,
    /// Policy for when to rotate active files
    pub rotation_policy: RotationPolicy,
    /// Policy for when to remove old files
    pub retention_policy: RetentionPolicy,
}

impl Config {
    /// Creates a new log configuration.
    pub fn new(
        origin: Origin,
        rotation_policy: RotationPolicy,
        retention_policy: RetentionPolicy,
    ) -> Self {
        Self {
            origin,
            rotation_policy,
            retention_policy,
        }
    }

    /// Specifies the rotation policy of the log directory
    pub fn with_rotation_policy(mut self, policy: RotationPolicy) -> Self {
        self.rotation_policy = policy;
        self
    }

    /// Specifies the retention policy of the log directory
    pub fn with_retention_policy(mut self, policy: RetentionPolicy) -> Self {
        self.retention_policy = policy;
        self
    }
}
