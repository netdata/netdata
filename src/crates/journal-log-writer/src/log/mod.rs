mod chain;
use chain::OwnedChain;

mod config;
pub use config::{Config, RetentionPolicy, RotationPolicy};

use crate::{Result, WriterError};
use journal_common::{RealtimeClock, load_boot_id, load_machine_id, monotonic_now};
use journal_core::field_map::{
    FieldMap, REMAPPING_MARKER, extract_field_name, is_systemd_compatible,
};
use journal_core::file::mmap::MmapMut;
use journal_core::file::{JournalFile, JournalFileOptions, JournalWriter};
use journal_registry::repository;
use std::path::{Path, PathBuf};

#[allow(unused_imports)]
use tracing::{debug, error, info, instrument, span, warn};

fn create_chain(path: &Path) -> Result<OwnedChain> {
    let machine_id = load_machine_id()
        .map_err(|e| WriterError::MachineId(format!("failed to load machine ID: {}", e)))?;

    if path.exists() && !path.is_dir() {
        return Err(WriterError::NotADirectory(path.display().to_string()));
    }

    if path.to_str().is_none() {
        return Err(WriterError::InvalidPath(
            "path contains invalid UTF-8".to_string(),
        ));
    }

    let path = PathBuf::from(path).join(machine_id.as_simple().to_string());
    if path.to_str().is_none() {
        return Err(WriterError::InvalidPath(
            "path with machine ID contains invalid UTF-8".to_string(),
        ));
    }

    std::fs::create_dir_all(&path)?;

    path.canonicalize()
        .map_err(|e| WriterError::NotADirectory(format!("failed to canonicalize path: {}", e)))?;
    if path.to_str().is_none() {
        return Err(WriterError::InvalidPath(
            "canonicalized path contains invalid UTF-8".to_string(),
        ));
    }

    OwnedChain::new(path, machine_id)
}

/// Tracks rotation state for size and count limits
struct RotationState {
    size: Option<(u64, u64)>,      // (max, current)
    count: Option<(usize, usize)>, // (max, current)
}

impl RotationState {
    fn new(rotation_policy: &RotationPolicy) -> Self {
        Self {
            size: rotation_policy.size_of_journal_file.map(|max| (max, 0)),
            count: rotation_policy.number_of_entries.map(|max| (max, 0)),
        }
    }

    fn should_rotate(&self) -> bool {
        self.size.is_some_and(|(max, current)| current >= max)
            || self.count.is_some_and(|(max, current)| current >= max)
    }

    fn update(&mut self, journal_writer: &JournalWriter) {
        if let Some((_, ref mut current)) = self.size {
            *current = journal_writer.current_file_size();
        }
        if let Some((_, ref mut current)) = self.count {
            *current += 1;
        }
    }

    fn reset(&mut self) {
        if let Some((_, ref mut current)) = self.size {
            *current = 0;
        }
        if let Some((_, ref mut current)) = self.count {
            *current = 0;
        }
    }
}

/// Groups a journal file and its writer together
struct ActiveFile {
    repository_file: repository::File,
    journal_file: JournalFile<MmapMut>,
    writer: JournalWriter,
}

impl ActiveFile {
    /// Creates a new journal file with the given parameters
    fn create(
        chain: &mut OwnedChain,
        seqnum_id: uuid::Uuid,
        boot_id: uuid::Uuid,
        next_seqnum: u64,
        max_file_size: Option<u64>,
        head_realtime: u64,
    ) -> Result<Self> {
        let head_seqnum = next_seqnum;

        let repository_file = chain.create_file(seqnum_id, head_seqnum, head_realtime)?;

        let options = JournalFileOptions::new(chain.machine_id, boot_id, seqnum_id)
            .with_window_size(8 * 1024 * 1024)
            .with_optimized_buckets(None, max_file_size)
            .with_keyed_hash(true);

        let mut journal_file = JournalFile::create(&repository_file, options)?;
        let writer = JournalWriter::new(&mut journal_file, head_seqnum, boot_id)?;

        Ok(Self {
            repository_file,
            journal_file,
            writer,
        })
    }

    /// Creates a successor file, inheriting settings from this file
    fn rotate(
        self,
        chain: &mut OwnedChain,
        max_file_size: Option<u64>,
        head_realtime: u64,
    ) -> Result<Self> {
        let next_seqnum = self.writer.next_seqnum();
        let boot_id = self.writer.boot_id();

        let head_seqnum = next_seqnum;

        let seqnum_id = uuid::Uuid::from_bytes(self.journal_file.journal_header_ref().seqnum_id);
        let repository_file = chain.create_file(seqnum_id, head_seqnum, head_realtime)?;

        let mut journal_file = self
            .journal_file
            .create_successor(&repository_file, max_file_size)?;
        let writer = JournalWriter::new(&mut journal_file, head_seqnum, boot_id)?;

        Ok(Self {
            repository_file,
            journal_file,
            writer,
        })
    }

    /// Writes a journal entry
    fn write_entry(&mut self, items: &[&[u8]], realtime: u64, monotonic: u64) -> Result<()> {
        self.writer
            .add_entry(&mut self.journal_file, items, realtime, monotonic)?;
        Ok(())
    }

    /// Gets the current file size
    fn current_file_size(&self) -> u64 {
        self.writer.current_file_size()
    }
}

pub struct Log {
    chain: OwnedChain,
    config: Config,
    active_file: Option<ActiveFile>,
    rotation_state: RotationState,
    boot_id: uuid::Uuid,
    seqnum_id: uuid::Uuid,
    current_seqnum: u64,
    remapping_registry: FieldMap,
    clock: RealtimeClock,
}

impl Log {
    /// Captures both realtime and monotonic timestamps, similar to systemd's dual_timestamp_now().
    ///
    /// Returns (realtime_usec, monotonic_usec) where:
    /// - realtime: microseconds since Unix epoch (CLOCK_REALTIME), monotonically increasing
    /// - monotonic: microseconds since boot (CLOCK_MONOTONIC)
    fn capture_dual_timestamp(&self) -> Result<(u64, u64)> {
        let realtime = self.clock.now().get();
        let monotonic = monotonic_now().map_err(|e| WriterError::Io(e))?.get();
        Ok((realtime, monotonic))
    }

    /// Creates a new journal log.
    pub fn new(path: &Path, config: Config) -> Result<Self> {
        let chain = create_chain(path)?;

        let current_seqnum = chain.tail_seqnum()?;
        let boot_id = load_boot_id()?;
        let seqnum_id = uuid::Uuid::new_v4();
        let rotation_state = RotationState::new(&config.rotation_policy);

        // Initialize clock with last entry timestamp if available
        let clock = if let Some(tail_realtime) = chain.tail_realtime()? {
            RealtimeClock::with_initial(tail_realtime)
        } else {
            RealtimeClock::new()
        };

        Ok(Log {
            chain,
            config,
            active_file: None,
            rotation_state,
            boot_id,
            seqnum_id,
            current_seqnum,
            remapping_registry: FieldMap::new(),
            clock,
        })
    }

    /// Writes a journal entry.
    ///
    /// If `source_realtime_usec` is provided, a `_SOURCE_REALTIME_TIMESTAMP` field will be added
    /// to record the original timestamp from the source (in microseconds since Unix epoch).
    /// This is useful when ingesting logs from external sources that have their own timestamps.
    pub fn write_entry(&mut self, items: &[&[u8]], source_realtime_usec: Option<u64>) -> Result<()> {
        if items.is_empty() {
            return Ok(());
        }

        if self.should_rotate() {
            self.rotate()?;
            self.remapping_registry.clear();
        }

        // Collect new incompatible field names that need remapping
        let mut new_mappings: Vec<(Vec<u8>, String)> = Vec::new();

        for item in items {
            if let Some(field_name) = extract_field_name(item) {
                // Skip if already systemd-compatible
                if is_systemd_compatible(field_name) {
                    continue;
                }

                // Skip if already in registry
                if self.remapping_registry.contains_otel_name(field_name) {
                    continue;
                }

                // Generate remapped name and add to list
                let remapped_name = rdp::encode_full(field_name);
                new_mappings.push((field_name.to_vec(), remapped_name));
            }
        }

        // Write remapping entry if we have new mappings
        if !new_mappings.is_empty() {
            self.write_remapping_entry(&new_mappings)?;

            // Update registry
            for (otel_name, systemd_name) in new_mappings.iter() {
                self.remapping_registry
                    .add_otel_mapping(otel_name.clone(), systemd_name.clone());
            }
        }

        // Inject _BOOT_ID field - this is required for journalctl boot filtering to work
        let boot_id_field = format!("_BOOT_ID={}", self.boot_id.as_simple());

        // Transform items to use remapped field names, prepending _BOOT_ID
        let mut transformed_items: Vec<Vec<u8>> = Vec::with_capacity(items.len() + 2);
        let mut items_refs: Vec<&[u8]> = Vec::with_capacity(items.len() + 2);

        // Prepend _BOOT_ID field first
        transformed_items.push(boot_id_field.into_bytes());

        // Add _SOURCE_REALTIME_TIMESTAMP if provided
        if let Some(timestamp_usec) = source_realtime_usec {
            let source_timestamp_field = format!("_SOURCE_REALTIME_TIMESTAMP={}", timestamp_usec);
            transformed_items.push(source_timestamp_field.into_bytes());
        }

        for item in items {
            if let Some(field_name) = extract_field_name(item) {
                if let Some(remapped_name) = self.remapping_registry.get_systemd_name(field_name) {
                    // Need to remap: create new item with remapped field name
                    let equals_pos = item.iter().position(|&b| b == b'=').unwrap();
                    let value = &item[equals_pos..]; // includes '='
                    let mut new_item = Vec::with_capacity(remapped_name.len() + value.len());
                    new_item.extend_from_slice(remapped_name.as_bytes());
                    new_item.extend_from_slice(value);
                    transformed_items.push(new_item);
                } else {
                    // No remapping needed, use original
                    transformed_items.push(item.to_vec());
                }
            } else {
                // No field name (shouldn't happen with valid items)
                transformed_items.push(item.to_vec());
            }
        }

        // Build references for the underlying write
        for item in &transformed_items {
            items_refs.push(item.as_slice());
        }

        let (realtime, monotonic) = self.capture_dual_timestamp()?;

        let active_file = self.active_file.as_mut().unwrap();
        active_file.write_entry(&items_refs, realtime, monotonic)?;

        self.rotation_state.update(&active_file.writer);
        self.current_seqnum += 1;

        Ok(())
    }

    /// Writes a remapping entry containing field name mappings.
    ///
    /// Format:
    /// _BOOT_ID=<boot_id>
    /// ND_REMAPPING=1
    /// ND_<md5_1>=<otel_key_1>
    /// ND_<md5_2>=<otel_key_2>
    /// ...
    fn write_remapping_entry(&mut self, mappings: &[(Vec<u8>, String)]) -> Result<()> {
        let mut remapping_items: Vec<Vec<u8>> = Vec::with_capacity(mappings.len() + 2);

        // Inject _BOOT_ID field first
        let boot_id_field = format!("_BOOT_ID={}", self.boot_id.as_simple());
        remapping_items.push(boot_id_field.into_bytes());

        // Add marker field
        remapping_items.push(REMAPPING_MARKER.to_vec());

        // Add each mapping as ND_<md5>=<otel_key>
        for (otel_name, systemd_name) in mappings {
            let mut item = Vec::with_capacity(systemd_name.len() + 1 + otel_name.len());
            item.extend_from_slice(systemd_name.as_bytes());
            item.push(b'=');
            item.extend_from_slice(otel_name);
            remapping_items.push(item);
        }

        // Build references
        let items_refs: Vec<&[u8]> = remapping_items.iter().map(|v| v.as_slice()).collect();

        let (realtime, monotonic) = self.capture_dual_timestamp()?;

        let active_file = self.active_file.as_mut().unwrap();
        active_file.write_entry(&items_refs, realtime, monotonic)?;

        self.rotation_state.update(&active_file.writer);
        self.current_seqnum += 1;

        Ok(())
    }

    /// Syncs all written data to disk, ensuring durability.
    ///
    /// This should be called after writing a batch of log entries to ensure
    /// they are persisted to disk before acknowledging the request.
    pub fn sync(&mut self) -> Result<()> {
        if let Some(active_file) = &mut self.active_file {
            active_file.journal_file.sync()?;
        }
        Ok(())
    }

    fn should_rotate(&self) -> bool {
        self.active_file.is_none() || self.rotation_state.should_rotate()
    }

    #[tracing::instrument(skip_all, fields(active_file))]
    fn rotate(&mut self) -> Result<()> {
        use journal_core::file::JournalState;

        // Update chain with current file size before rotating
        if let Some(active_file) = &self.active_file {
            self.chain.update_file_size(
                &active_file.repository_file,
                active_file.current_file_size(),
            );
        }

        // Respect retention policy
        self.chain.retain(&self.config.retention_policy)?;

        // Create new file (either initial or rotated)
        let max_file_size = self.config.rotation_policy.size_of_journal_file;
        let head_realtime = self.clock.now().get();
        let new_file = if let Some(mut old_file) = self.active_file.take() {
            // Set the old file's state to ARCHIVED before creating successor
            old_file.journal_file.journal_header_mut().state = JournalState::Archived as u8;
            old_file.journal_file.sync()?;

            old_file.rotate(&mut self.chain, max_file_size, head_realtime)?
        } else {
            ActiveFile::create(
                &mut self.chain,
                self.seqnum_id,
                self.boot_id,
                self.current_seqnum + 1,
                max_file_size,
                head_realtime,
            )?
        };

        tracing::Span::current().record("new_file", new_file.repository_file.path());

        self.active_file = Some(new_file);
        self.rotation_state.reset();

        Ok(())
    }

    /// Writes a journal entry from a serializable value.
    ///
    /// This method serializes the value to JSON, flattens it, and writes it to the journal.
    /// The flattened structure converts nested JSON into KEY=VALUE pairs suitable for journal entries.
    ///
    /// # Example
    ///
    /// ```no_run
    /// use serde::Serialize;
    /// use journal_log_writer::{Log, Config, RotationPolicy, RetentionPolicy};
    /// use journal_registry::Origin;
    /// use std::path::Path;
    ///
    /// #[derive(Serialize)]
    /// struct LogEntry {
    ///     message: String,
    ///     level: String,
    ///     user: User,
    /// }
    ///
    /// #[derive(Serialize)]
    /// struct User {
    ///     id: u64,
    ///     name: String,
    /// }
    ///
    /// # fn main() -> Result<(), Box<dyn std::error::Error>> {
    /// let origin = Origin {
    ///     machine_id: None,
    ///     namespace: None,
    ///     source: journal_registry::Source::System,
    /// };
    /// let config = Config::new(origin, RotationPolicy::default(), RetentionPolicy::default());
    /// let mut log = Log::new(Path::new("/tmp/test-journal"), config)?;
    ///
    /// let entry = LogEntry {
    ///     message: "User logged in".to_string(),
    ///     level: "INFO".to_string(),
    ///     user: User {
    ///         id: 42,
    ///         name: "alice".to_string(),
    ///     },
    /// };
    ///
    /// // This will write fields like:
    /// // MESSAGE=User logged in
    /// // LEVEL=INFO
    /// // USER_ID=42
    /// // USER_NAME=alice
    /// log.write_structured(&entry)?;
    /// # Ok(())
    /// # }
    /// ```
    #[cfg(feature = "serde-api")]
    pub fn write_structured<T: serde::Serialize>(&mut self, value: &T) -> Result<()> {
        use flatten_serde_json::flatten;

        // Serialize to JSON value
        let json_value = serde_json::to_value(value).map_err(|e| {
            WriterError::Serialization(format!("failed to serialize to JSON: {}", e))
        })?;

        // Flatten the JSON structure - requires a JSON object (Map)
        let flattened = if let serde_json::Value::Object(map) = json_value {
            flatten(&map)
        } else {
            // If not an object, return error
            return Err(WriterError::Serialization(
                "value must be a JSON object, not a primitive or array".to_string(),
            ));
        };

        // Convert to journal field format (KEY=VALUE)
        let mut fields: Vec<Vec<u8>> = Vec::with_capacity(flattened.len());

        for (key, value) in flattened.iter() {
            // Convert key to uppercase and replace dots with underscores
            // (journal convention)
            let journal_key = key.to_uppercase().replace('.', "_");

            // Format as KEY=VALUE
            let field = match value {
                serde_json::Value::String(s) => {
                    format!("{}={}", journal_key, s)
                }
                serde_json::Value::Number(n) => {
                    format!("{}={}", journal_key, n)
                }
                serde_json::Value::Bool(b) => {
                    format!("{}={}", journal_key, if *b { "true" } else { "false" })
                }
                serde_json::Value::Null => {
                    format!("{}=", journal_key)
                }
                // Arrays and objects should be flattened already, but just in case
                _ => {
                    format!("{}={}", journal_key, value)
                }
            };

            fields.push(field.into_bytes());
        }

        // Convert Vec<Vec<u8>> to Vec<&[u8]> for write_entry
        let field_refs: Vec<&[u8]> = fields.iter().map(|f| f.as_slice()).collect();

        self.write_entry(&field_refs, None)
    }
}
