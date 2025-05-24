use duct::cmd;
use std::collections::HashMap;
use std::io::{self, Error, ErrorKind};
use std::path::Path;

/// A builder for creating systemd journal entries with custom fields
pub struct JournalLogger {
    fields: HashMap<String, String>,
    log2journal_path: String,
    systemd_cat_path: String,
}

impl JournalLogger {
    /// Create a new JournalLogger with paths to the required executables
    pub fn new(log2journal_path: &str, systemd_cat_path: &str) -> Self {
        JournalLogger {
            fields: HashMap::new(),
            log2journal_path: log2journal_path.to_string(),
            systemd_cat_path: systemd_cat_path.to_string(),
        }
    }

    /// Add a field to the journal entry
    pub fn add_field(&mut self, key: &str, value: &str) -> &mut Self {
        self.fields.insert(key.to_string(), value.to_string());
        self
    }

    /// Flush the current fields to the journal and clear the fields
    pub fn flush(&mut self) -> io::Result<()> {
        // Verify that the required executables exist
        if !Path::new(&self.log2journal_path).exists() {
            return Err(Error::new(
                ErrorKind::NotFound,
                format!(
                    "log2journal executable not found at {}",
                    self.log2journal_path
                ),
            ));
        }

        if !Path::new(&self.systemd_cat_path).exists() {
            return Err(Error::new(
                ErrorKind::NotFound,
                format!(
                    "systemd-cat executable not found at {}",
                    self.systemd_cat_path
                ),
            ));
        }

        // Create the JSON string
        let json_data = serde_json::to_string(&self.fields)
            .map_err(|e| Error::new(ErrorKind::InvalidData, e))?;

        // Create the pipeline
        let pipeline = cmd!("echo", json_data)
            .pipe(cmd!(&self.log2journal_path, "json"))
            .pipe(cmd!(&self.systemd_cat_path));

        // Execute the pipeline
        pipeline.run()?;

        // Clear the fields for the next entry
        self.fields.clear();

        Ok(())
    }
}
