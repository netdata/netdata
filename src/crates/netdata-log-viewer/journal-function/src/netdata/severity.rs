//! Severity levels for log entries.
//!
//! This module provides severity classification based on syslog PRIORITY values,
//! matching the C implementation in systemd-journal-annotations.c.

use serde::{Deserialize, Serialize};

/// Log entry severity level.
///
/// Maps syslog PRIORITY values to severity levels for UI display.
/// Implementation matches systemd-journal-annotations.c:256-281.
#[derive(Default, Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Severity {
    /// Critical errors (PRIORITY <= 3, LOG_ERR)
    Critical,

    /// Warnings (PRIORITY == 4, LOG_WARNING)
    Warning,

    /// Notices (PRIORITY == 5, LOG_NOTICE)
    Notice,

    /// Debug messages (PRIORITY >= 7, LOG_DEBUG)
    Debug,

    /// Normal/Info messages (PRIORITY == 6, LOG_INFO, or missing)
    #[default]
    Normal,
}

impl Severity {
    /// Convert syslog PRIORITY value to severity level.
    ///
    /// Implements the same logic as `syslog_priority_to_facet_severity()`
    /// in systemd-journal-annotations.c:256-281.
    ///
    /// Priority mapping (from syslog.h):
    /// - 0 = LOG_EMERG
    /// - 1 = LOG_ALERT
    /// - 2 = LOG_CRIT
    /// - 3 = LOG_ERR
    /// - 4 = LOG_WARNING
    /// - 5 = LOG_NOTICE
    /// - 6 = LOG_INFO (default)
    /// - 7 = LOG_DEBUG
    ///
    /// # Arguments
    ///
    /// * `priority` - The PRIORITY field value (as string), or None if missing
    ///
    /// # Examples
    ///
    /// ```
    /// use journal_function::netdata::Severity;
    ///
    /// assert_eq!(Severity::from_priority(Some("0")), Severity::Critical);
    /// assert_eq!(Severity::from_priority(Some("3")), Severity::Critical);
    /// assert_eq!(Severity::from_priority(Some("4")), Severity::Warning);
    /// assert_eq!(Severity::from_priority(Some("5")), Severity::Notice);
    /// assert_eq!(Severity::from_priority(Some("6")), Severity::Normal);
    /// assert_eq!(Severity::from_priority(Some("7")), Severity::Debug);
    /// assert_eq!(Severity::from_priority(None), Severity::Normal);
    /// ```
    pub fn from_priority(priority: Option<&str>) -> Self {
        let priority_num = priority.and_then(|s| s.parse::<i32>().ok()).unwrap_or(6); // Default to LOG_INFO if missing or invalid

        // Matches C code logic exactly
        if priority_num <= 3 {
            // LOG_ERR and below (EMERG, ALERT, CRIT, ERR)
            Severity::Critical
        } else if priority_num <= 4 {
            // LOG_WARNING
            Severity::Warning
        } else if priority_num <= 5 {
            // LOG_NOTICE
            Severity::Notice
        } else if priority_num >= 7 {
            // LOG_DEBUG
            Severity::Debug
        } else {
            // LOG_INFO (6) or anything else
            Severity::Normal
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_severity_from_priority() {
        // Critical: 0-3 (EMERG, ALERT, CRIT, ERR)
        assert_eq!(Severity::from_priority(Some("0")), Severity::Critical);
        assert_eq!(Severity::from_priority(Some("1")), Severity::Critical);
        assert_eq!(Severity::from_priority(Some("2")), Severity::Critical);
        assert_eq!(Severity::from_priority(Some("3")), Severity::Critical);

        // Warning: 4
        assert_eq!(Severity::from_priority(Some("4")), Severity::Warning);

        // Notice: 5
        assert_eq!(Severity::from_priority(Some("5")), Severity::Notice);

        // Normal: 6 (INFO)
        assert_eq!(Severity::from_priority(Some("6")), Severity::Normal);

        // Debug: 7
        assert_eq!(Severity::from_priority(Some("7")), Severity::Debug);

        // Default: missing or invalid → Normal
        assert_eq!(Severity::from_priority(None), Severity::Normal);
        assert_eq!(Severity::from_priority(Some("invalid")), Severity::Normal);
        assert_eq!(Severity::from_priority(Some("")), Severity::Normal);
    }

    #[test]
    fn test_severity_serialization() {
        // Test that severity serializes to lowercase strings
        assert_eq!(
            serde_json::to_string(&Severity::Critical).unwrap(),
            "\"critical\""
        );
        assert_eq!(
            serde_json::to_string(&Severity::Warning).unwrap(),
            "\"warning\""
        );
        assert_eq!(
            serde_json::to_string(&Severity::Notice).unwrap(),
            "\"notice\""
        );
        assert_eq!(
            serde_json::to_string(&Severity::Debug).unwrap(),
            "\"debug\""
        );
        assert_eq!(
            serde_json::to_string(&Severity::Normal).unwrap(),
            "\"normal\""
        );
    }
}
