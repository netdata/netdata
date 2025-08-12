use error::{JournalError, Result};
use journal_file::{
    load_boot_id, BucketUtilization, JournalFile, JournalFileOptions, JournalWriter,
};
use memmap2::MmapMut;
use std::cmp::Ordering;
use std::ffi::OsStr;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct JournalFileInfo {
    pub path: PathBuf,
    pub timestamp: SystemTime,
    pub counter: u64,
    pub size: Option<u64>,
}

impl JournalFileInfo {
    pub fn from_path(path: impl AsRef<Path>) -> Result<Self> {
        let path = path.as_ref();
        let filename = path
            .file_name()
            .and_then(OsStr::to_str)
            .ok_or(JournalError::InvalidFilename)?;

        let (timestamp, counter) = Self::parse_filename(filename)?;

        Ok(Self {
            path: path.to_path_buf(),
            timestamp,
            counter,
            size: None,
        })
    }

    pub fn from_parts(
        timestamp: SystemTime,
        counter: u64,
        size: Option<u64>,
    ) -> Result<JournalFileInfo> {
        let duration = timestamp
            .duration_since(UNIX_EPOCH)
            .map_err(|_| JournalError::SystemTimeError)?;
        let micros = duration.as_secs() * 1_000_000 + duration.subsec_micros() as u64;
        let path = PathBuf::from(format!("journal-{}-{}.journal", micros, counter));

        Ok(Self {
            path,
            timestamp,
            counter,
            size,
        })
    }

    /// Parse timestamp and counter from filename
    /// Expected format: "journal-{timestamp_micros}-{counter}.journal"
    fn parse_filename(filename: &str) -> Result<(SystemTime, u64)> {
        let name = filename.strip_suffix(".journal").unwrap_or(filename);

        if let Some(stripped) = name.strip_prefix("journal-") {
            let parts: Vec<&str> = stripped.split('-').collect();
            if parts.len() == 2 {
                let timestamp_micros: u64 = parts[0]
                    .parse()
                    .map_err(|_| JournalError::InvalidFilename)?;
                let counter: u64 = parts[1]
                    .parse()
                    .map_err(|_| JournalError::InvalidFilename)?;

                let timestamp = UNIX_EPOCH + Duration::from_micros(timestamp_micros);
                return Ok((timestamp, counter));
            }
        }

        Err(JournalError::InvalidFilename)
    }

    /// Get file size, loading from filesystem if not cached
    pub fn get_size(&mut self) -> Result<u64> {
        if let Some(size) = self.size {
            Ok(size)
        } else {
            let metadata = std::fs::metadata(&self.path)?;
            let size = metadata.len();
            self.size = Some(size);
            Ok(size)
        }
    }
}

// Implement ordering based on counter (for detecting duplicates and ordering)
impl Ord for JournalFileInfo {
    fn cmp(&self, other: &Self) -> Ordering {
        // Order by counter only - this enables duplicate detection
        self.counter.cmp(&other.counter)
    }
}

impl PartialOrd for JournalFileInfo {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

/// Determines when an active journal file should be sealed
#[derive(Debug, Copy, Clone, Default)]
pub struct RotationPolicy {
    /// Maximum file size before rotating (in bytes)
    pub size_of_journal_file: Option<u64>,
    /// Maximum duration that entries in a single file can span
    pub duration_of_journal_file: Option<Duration>,
}

impl RotationPolicy {
    pub fn with_size_of_journal_file(mut self, size_of_journal_file: u64) -> Self {
        self.size_of_journal_file = Some(size_of_journal_file);
        self
    }

    pub fn with_duration_of_journal_file(mut self, duration_of_journal_file: Duration) -> Self {
        self.duration_of_journal_file = Some(duration_of_journal_file);
        self
    }
}

/// Retention policy - determines when files should be removed
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
    pub fn with_number_of_journal_files(mut self, number_of_journal_files: usize) -> Self {
        self.number_of_journal_files = Some(number_of_journal_files);
        self
    }

    pub fn with_size_of_journal_files(mut self, size_of_journal_files: u64) -> Self {
        self.size_of_journal_files = Some(size_of_journal_files);
        self
    }

    pub fn with_duration_of_journal_files(mut self, duration_of_journal_files: Duration) -> Self {
        self.duration_of_journal_files = Some(duration_of_journal_files);
        self
    }
}

/// Configuration for journal directory management
#[derive(Debug, Clone)]
pub struct JournalDirectoryConfig {
    /// Directory path where journal files are stored
    pub directory: PathBuf,
    /// Policy for when to rotate active files
    pub rotation_policy: RotationPolicy,
    /// Policy for when to remove old files
    pub retention_policy: RetentionPolicy,
}

impl JournalDirectoryConfig {
    pub fn new(directory: impl Into<PathBuf>) -> Self {
        Self {
            directory: directory.into(),
            rotation_policy: RotationPolicy::default(),
            retention_policy: RetentionPolicy::default(),
        }
    }

    pub fn with_sealing_policy(mut self, policy: RotationPolicy) -> Self {
        self.rotation_policy = policy;
        self
    }

    pub fn with_retention_policy(mut self, policy: RetentionPolicy) -> Self {
        self.retention_policy = policy;
        self
    }
}

/// Manages a directory of journal files with automatic cleanup and sealing
#[derive(Debug)]
pub struct JournalDirectory {
    config: JournalDirectoryConfig,
    /// Files ordered by counter (oldest counter first)
    files: Vec<JournalFileInfo>,
    /// Next counter value for new files
    next_counter: u64,
    /// Cached total size of all files
    total_size: u64,
}

impl JournalDirectory {
    /// Scan the directory and load existing journal files
    pub fn with_config(config: JournalDirectoryConfig) -> Result<Self> {
        // Create directory if it does not already exist.
        if !config.directory.exists() {
            std::fs::create_dir_all(&config.directory)?;
        } else if !config.directory.is_dir() {
            return Err(JournalError::NotADirectory);
        }

        let mut journal_directory = Self {
            config,
            files: Vec::new(),
            next_counter: 0,
            total_size: 0,
        };

        // Read all .journal files from directory
        for entry in std::fs::read_dir(&journal_directory.config.directory)? {
            let entry = entry?;
            let file_path = entry.path();

            if file_path.extension() != Some(OsStr::new("journal")) {
                continue;
            }

            match JournalFileInfo::from_path(&file_path) {
                Ok(mut file_info) => {
                    // Load the actual file size from filesystem
                    let file_size = file_info.get_size().unwrap_or(0);
                    journal_directory.total_size += file_size;
                    journal_directory.next_counter =
                        journal_directory.next_counter.max(file_info.counter + 1);
                    journal_directory.files.push(file_info);
                }
                Err(_) => {
                    // Skip files with invalid names
                    continue;
                }
            }
        }

        // Sort files by counter to maintain order
        journal_directory.files.sort();

        Ok(journal_directory)
    }

    pub fn directory_path(&self) -> &Path {
        &self.config.directory
    }

    pub fn get_full_path(&self, file_info: &JournalFileInfo) -> PathBuf {
        if file_info.path.is_absolute() {
            file_info.path.clone()
        } else {
            self.config.directory.join(&file_info.path)
        }
    }

    // Get information about all the files in the journal directory
    pub fn files(&self) -> Vec<JournalFileInfo> {
        self.files.clone()
    }

    /// Add a new journal file to the directory representation
    pub fn new_file(&mut self, existing_file: Option<JournalFileInfo>) -> Result<JournalFileInfo> {
        let timestamp = SystemTime::now();
        let new_file = JournalFileInfo::from_parts(timestamp, self.next_counter, None)?;

        self.files.push(new_file.clone());
        self.next_counter += 1;

        if let Some(existing_file) = existing_file {
            self.total_size += existing_file.size.unwrap_or(0);
        }

        Ok(new_file)
    }

    /// Remove the oldest file (by counter) from both filesystem and tracking
    fn remove_oldest_file(&mut self) -> Result<()> {
        if let Some(oldest_file) = self.files.first() {
            let file_path = self.get_full_path(oldest_file);
            let file_size = oldest_file.size.unwrap_or(0);

            // Remove from filesystem
            if let Err(e) = std::fs::remove_file(&file_path) {
                // Log error but continue cleanup - file might already be deleted
                eprintln!(
                    "Warning: Failed to remove journal file {:?}: {}",
                    file_path, e
                );
            }

            // Remove from tracking and update total size
            self.files.remove(0);
            self.total_size = self.total_size.saturating_sub(file_size);
        }

        Ok(())
    }

    /// Remove files older than the specified cutoff time
    fn remove_files_older_than(&mut self, cutoff_time: SystemTime) -> Result<()> {
        // Find files older than cutoff time
        let mut files_to_remove = Vec::new();
        for (index, file) in self.files.iter().enumerate() {
            if file.timestamp <= cutoff_time {
                files_to_remove.push(index);
            }
        }

        // Remove files in reverse order to maintain indices
        for &index in files_to_remove.iter().rev() {
            let file = &self.files[index];
            let file_path = self.get_full_path(file);
            let file_size = file.size.unwrap_or(0);

            // Remove from filesystem
            if let Err(e) = std::fs::remove_file(&file_path) {
                // Log error but continue cleanup
                eprintln!(
                    "Warning: Failed to remove journal file {:?}: {}",
                    file_path, e
                );
            }

            // Remove from tracking and update total size
            self.files.remove(index);
            self.total_size = self.total_size.saturating_sub(file_size);
        }

        Ok(())
    }

    /// Enforce the retention policy by removing old files
    pub fn enforce_retention_policy(&mut self) -> Result<()> {
        let policy = self.config.retention_policy;

        // 1. Remove by file count limit
        if let Some(max_files) = policy.number_of_journal_files {
            while self.files.len() > max_files {
                self.remove_oldest_file()?;
            }
        }

        // 2. Remove by total size limit
        if let Some(max_total_size) = policy.size_of_journal_files {
            while self.total_size > max_total_size && !self.files.is_empty() {
                self.remove_oldest_file()?;
            }
        }

        // 3. Remove by entry age limit
        if let Some(max_entry_age) = policy.duration_of_journal_files {
            let cutoff_time = SystemTime::now()
                .checked_sub(max_entry_age)
                .unwrap_or(SystemTime::UNIX_EPOCH);
            self.remove_files_older_than(cutoff_time)?;
        }

        Ok(())
    }
}

fn generate_uuid() -> [u8; 16] {
    uuid::Uuid::new_v4().into_bytes()
}

/// Configuration for JournalLog
#[derive(Debug, Clone)]
pub struct JournalLogConfig {
    /// Directory where journal files are stored
    pub journal_dir: PathBuf,
    /// Policy for when to rotate active files
    pub rotation_policy: RotationPolicy,
    /// Policy for when to remove old files
    pub retention_policy: RetentionPolicy,
}

impl JournalLogConfig {
    pub fn new(journal_dir: impl Into<PathBuf>) -> Self {
        Self {
            journal_dir: journal_dir.into(),
            rotation_policy: RotationPolicy::default()
                .with_size_of_journal_file(100 * 1024 * 1024) // 100MB
                .with_duration_of_journal_file(Duration::from_secs(2 * 3600)), // 2 hours
            retention_policy: RetentionPolicy::default()
                .with_number_of_journal_files(10) // 10 files
                .with_size_of_journal_files(1024 * 1024 * 1024) // 1GB
                .with_duration_of_journal_files(Duration::from_secs(7 * 24 * 3600)), // 7 days
        }
    }

    pub fn with_rotation_policy(mut self, policy: RotationPolicy) -> Self {
        self.rotation_policy = policy;
        self
    }

    pub fn with_retention_policy(mut self, policy: RetentionPolicy) -> Self {
        self.retention_policy = policy;
        self
    }
}

pub struct JournalLog {
    directory: JournalDirectory,
    current_file: Option<JournalFile<MmapMut>>,
    current_writer: Option<JournalWriter>,
    current_file_info: Option<JournalFileInfo>,
    machine_id: [u8; 16],
    boot_id: [u8; 16],
    seqnum_id: [u8; 16],
    previous_bucket_utilization: Option<BucketUtilization>,
}

/// Calculate optimal bucket sizes based on previous file utilization or rotation policy
fn calculate_bucket_sizes(
    previous_utilization: Option<&BucketUtilization>,
    rotation_policy: &RotationPolicy,
) -> (usize, usize) {
    if let Some(utilization) = previous_utilization {
        let data_utilization = utilization.data_utilization();
        let field_utilization = utilization.field_utilization();

        let data_buckets = if data_utilization > 0.75 {
            (utilization.data_total * 2).next_power_of_two()
        } else if data_utilization < 0.25 && utilization.data_total > 4096 {
            (utilization.data_total / 2).next_power_of_two()
        } else {
            utilization.data_total
        };

        let field_buckets = if field_utilization > 0.75 {
            (utilization.field_total * 2).next_power_of_two()
        } else if field_utilization < 0.25 && utilization.field_total > 512 {
            (utilization.field_total / 2).next_power_of_two()
        } else {
            utilization.field_total
        };

        (data_buckets, field_buckets)
    } else {
        // Initial sizing based on rotation policy max file size
        let max_file_size = rotation_policy
            .size_of_journal_file
            .unwrap_or(8 * 1024 * 1024);

        // 16 MiB -> 4096 data buckets
        let data_buckets = (max_file_size / 4096).max(1024).next_power_of_two() as usize;
        let field_buckets = 128; // Assume ~8:1 data:field ratio

        (data_buckets, field_buckets)
    }
}

impl JournalLog {
    pub fn new(config: JournalLogConfig) -> Result<Self> {
        let journal_config = JournalDirectoryConfig::new(&config.journal_dir)
            .with_sealing_policy(config.rotation_policy)
            .with_retention_policy(config.retention_policy);

        let mut directory = JournalDirectory::with_config(journal_config)?;

        // Enforce retention policy on startup to clean up any old files
        directory.enforce_retention_policy()?;

        let machine_id = journal_file::file::load_machine_id()?;
        let boot_id = load_boot_id()?;
        // TODO: Use NETDATA_INVOCATION_ID
        let seqnum_id = generate_uuid();

        Ok(JournalLog {
            directory,
            current_file: None,
            current_writer: None,
            current_file_info: None,
            machine_id,
            boot_id,
            seqnum_id,
            previous_bucket_utilization: None,
        })
    }

    fn ensure_active_journal(&mut self) -> Result<()> {
        // Check if rotation is needed before writing
        if let Some(writer) = &self.current_writer {
            if self.should_rotate(writer) {
                self.rotate_current_file()?;
            }
        }

        if self.current_file.is_none() {
            // Create a new journal file
            let file_info = self.directory.new_file(None)?;

            // Get the full path for the journal file
            let file_path = self.directory.get_full_path(&file_info);

            // Calculate optimal bucket sizes based on previous file utilization
            let (data_buckets, field_buckets) = calculate_bucket_sizes(
                self.previous_bucket_utilization.as_ref(),
                &self.directory.config.rotation_policy,
            );

            let options = JournalFileOptions::new(
                self.machine_id,
                self.boot_id,
                self.seqnum_id,
                generate_uuid(),
            )
            .with_window_size(8 * 1024 * 1024)
            .with_data_hash_table_buckets(data_buckets)
            .with_field_hash_table_buckets(field_buckets)
            .with_keyed_hash(true);

            let mut journal_file = JournalFile::create(&file_path, options)?;
            let writer = JournalWriter::new(&mut journal_file)?;

            self.current_file = Some(journal_file);
            self.current_writer = Some(writer);
            self.current_file_info = Some(file_info);

            // Enforce retention policy after creating new file to account for the new file count
            self.directory.enforce_retention_policy()?;
        }

        Ok(())
    }

    /// Checks if we have to rotate. Prioritizes file size over file creation
    /// time.
    fn should_rotate(&self, writer: &JournalWriter) -> bool {
        let policy = self.directory.config.rotation_policy;

        // Check if the file size went over the limit
        if let Some(max_size) = policy.size_of_journal_file {
            if writer.current_file_size() >= max_size {
                return true;
            }
        }

        // Check if the time span between first and last entries exceeds the limit
        let Some(file) = &self.current_file else {
            return false;
        };
        let Some(max_entry_span) = policy.duration_of_journal_file else {
            return false;
        };
        let Some(first_monotonic) = writer.first_entry_monotonic() else {
            return false;
        };

        let header = file.journal_header_ref();
        let last_monotonic = header.tail_entry_monotonic;

        // Convert monotonic timestamps (microseconds) to duration
        let entry_span = if last_monotonic >= first_monotonic {
            Duration::from_micros(last_monotonic - first_monotonic)
        } else {
            return false;
        };

        if entry_span >= max_entry_span {
            return true;
        }

        false
    }

    fn rotate_current_file(&mut self) -> Result<()> {
        // Capture bucket utilization before closing the file
        if let Some(file) = &self.current_file {
            self.previous_bucket_utilization = file.bucket_utilization();
        }

        // Update the current file's size in our tracking before closing
        if let (Some(file_info), Some(writer)) = (&mut self.current_file_info, &self.current_writer)
        {
            let current_size = writer.current_file_size();
            file_info.size = Some(current_size);

            // Update the size in the directory's file list
            if let Some(tracked_file) = self
                .directory
                .files
                .iter_mut()
                .find(|f| f.counter == file_info.counter)
            {
                let old_size = tracked_file.size.unwrap_or(0);
                tracked_file.size = Some(current_size);

                // Update total size tracking
                self.directory.total_size = self
                    .directory
                    .total_size
                    .saturating_sub(old_size)
                    .saturating_add(current_size);
            }
        }

        // Close current file
        self.current_file = None;
        self.current_writer = None;
        self.current_file_info = None;

        // Next call to ensure_active_journal() will create new file
        Ok(())
    }

    pub fn write_entry(&mut self, items: &[&[u8]]) -> Result<()> {
        if items.is_empty() {
            return Ok(());
        }

        self.ensure_active_journal()?;

        let journal_file = self.current_file.as_mut().unwrap();
        let writer = self.current_writer.as_mut().unwrap();

        let now = SystemTime::now();
        let realtime = now
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_micros() as u64;

        // Use realtime for monotonic as well for simplicity
        let monotonic = realtime;

        writer.add_entry(journal_file, items, realtime, monotonic, self.boot_id)?;

        Ok(())
    }
}
