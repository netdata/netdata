//! Severity levels for the systemd-journal logs UI: the `Severity` each
//! rendered row carries in its rowOptions `{"severity": ...}` object,
//! derived per row from the journal `PRIORITY` field. Mirrors the C
//! implementation `syslog_priority_to_facet_severity()` in
//! `src/collectors/systemd-journal.plugin/systemd-journal-annotations.c`.
//! Sole consumer (grep-verified):
//! [`crate::netdata::response::table_to_netdata_response`].

use serde::{Deserialize, Serialize};

/// Log entry severity level, serialized lowercase
/// ("critical", "warning", "notice", "debug", "normal").
#[derive(Default, Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Severity {
    /// Critical: PRIORITY <= 3 (EMERG, ALERT, CRIT, ERR)
    Critical,

    /// Warning: PRIORITY == 4 (LOG_WARNING)
    Warning,

    /// Notice: PRIORITY == 5 (LOG_NOTICE)
    Notice,

    /// Debug: PRIORITY >= 7 (LOG_DEBUG)
    Debug,

    /// Normal/Info: PRIORITY == 6 (LOG_INFO)
    #[default]
    Normal,
}

impl Severity {
    /// Maps a journal `PRIORITY` string to a severity.
    ///
    /// Missing or unparseable values count as LOG_INFO (6). The C mirror
    /// (`str2i()`, same file) parses garbage as 0
    /// and reports Critical instead.
    pub fn from_priority(priority: Option<&str>) -> Self {
        let priority_num = priority.and_then(|s| s.parse::<i32>().ok()).unwrap_or(6); // Default to LOG_INFO if missing or invalid

        if priority_num <= 3 {
            Severity::Critical
        } else if priority_num <= 4 {
            Severity::Warning
        } else if priority_num <= 5 {
            Severity::Notice
        } else if priority_num >= 7 {
            Severity::Debug
        } else {
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
        // serde(rename_all = "lowercase") drives these strings
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
