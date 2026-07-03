//! Detection of the former (experimental) otel.yaml schema, so that an
//! upgrading operator gets a migration guide instead of a bare
//! "unknown field" error.
//!
//! The former plugin configured logs with journal-file knobs
//! (`size_of_journal_file`, `number_of_journal_files`, ...). Values do not
//! carry over: the storage engine changed, file sizes and counts mean
//! different things, and the defaults changed too — so nothing is migrated
//! automatically. The operator re-decides each value; this module hands them
//! the key mapping. `logs.journal_dir` is the one former key that is still
//! valid (read-only pointer to the old journals; see
//! `resolve_legacy_journal_dir`).

use std::path::Path;

/// The former `logs:` keys that no longer exist, with their guidance.
/// `journal_dir` is absent: it is still a valid key.
const FORMER_LOGS_KEYS: [(&str, &str); 7] = [
    (
        "size_of_journal_file",
        "logs.rotation.default.max_file_size",
    ),
    (
        "entries_of_journal_file",
        "logs.rotation.default.max_log_entries",
    ),
    (
        "duration_of_journal_file",
        "logs.rotation.default.max_file_duration",
    ),
    (
        "number_of_journal_files",
        "logs.retention.default.max_files",
    ),
    (
        "size_of_journal_files",
        "logs.retention.default.max_total_size",
    ),
    (
        "duration_of_journal_files",
        "logs.retention.default.max_age",
    ),
    ("store_otlp_json", "removed (no replacement)"),
];

/// Wrap a user-file parse failure, adding a migration guide when the file is
/// recognizably the former schema. Always carries the "parsing <path>" context
/// the plain error path carries.
pub(super) fn enrich_parse_error(
    path: &Path,
    contents: &str,
    err: serde_yaml::Error,
) -> anyhow::Error {
    let base = anyhow::Error::new(err);
    match former_schema_guidance(contents) {
        Some(guidance) => base
            .context(guidance)
            .context(format!("parsing {}", path.display())),
        None => base.context(format!("parsing {}", path.display())),
    }
}

/// When `contents` is well-formed YAML whose `logs:` section carries former
/// schema keys, return the operator-facing migration guide. Returns `None` for
/// files that are not recognizably the former schema (including files that do
/// not parse as YAML at all — those keep the plain syntax error).
fn former_schema_guidance(contents: &str) -> Option<String> {
    let value: serde_yaml::Value = serde_yaml::from_str(contents).ok()?;
    let logs = value.get("logs")?.as_mapping()?;

    let found: Vec<&str> = FORMER_LOGS_KEYS
        .iter()
        .map(|(old, _)| *old)
        .filter(|old| logs.contains_key(serde_yaml::Value::from(*old)))
        .collect();
    if found.is_empty() {
        return None;
    }

    let mut guide = format!(
        "this file uses the former (experimental) otel.yaml schema: found logs.{}.\n\
         The logs storage engine and its configuration changed, and the defaults \
         changed too — old values do not carry over, so re-decide each one:\n",
        found.join(", logs.")
    );
    for (old, new) in FORMER_LOGS_KEYS {
        guide.push_str(&format!("  {old:<26}-> {new}\n"));
    }
    guide.push_str(
        "  journal_dir               -> still valid under logs: (read-only; keeps old logs queryable)\n\
         See the stock otel.yaml for the current schema and defaults. If you never \
         customized this file, delete it or re-copy it from stock. Old logs remain \
         queryable either way",
    );
    Some(guide)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The former plugin's stock `logs:` section, verbatim shape.
    const FORMER_FILE: &str = r#"
endpoint:
  path: "127.0.0.1:4317"
logs:
  journal_dir: /var/log/netdata/otel/v1
  size_of_journal_file: "100MB"
  entries_of_journal_file: 50000
  duration_of_journal_file: "2 hours"
  number_of_journal_files: 10
  size_of_journal_files: "1GB"
  duration_of_journal_files: "7 days"
  store_otlp_json: false
"#;

    #[test]
    fn former_stock_file_is_detected_with_full_guide() {
        let guide = former_schema_guidance(FORMER_FILE).expect("former schema detected");
        assert!(guide.contains("former (experimental) otel.yaml schema"));
        // Names every former key found in the file.
        assert!(guide.contains("found logs.size_of_journal_file"));
        assert!(guide.contains("logs.store_otlp_json"));
        // Maps every former key.
        assert!(guide.contains("size_of_journal_file      -> logs.rotation.default.max_file_size"));
        assert!(guide.contains("number_of_journal_files   -> logs.retention.default.max_files"));
        assert!(guide.contains("store_otlp_json           -> removed (no replacement)"));
        // Values must be re-decided, journal_dir stays valid, old logs stay queryable.
        assert!(guide.contains("re-decide each one"));
        assert!(guide.contains("journal_dir               -> still valid"));
        assert!(guide.contains("delete it or re-copy it from stock"));
    }

    #[test]
    fn partial_former_file_names_only_present_keys() {
        let yaml = "logs:\n  number_of_journal_files: 20\n";
        let guide = former_schema_guidance(yaml).expect("former key detected");
        assert!(guide.contains("found logs.number_of_journal_files."));
        assert!(!guide.contains("found logs.size_of_journal_file"));
    }

    #[test]
    fn current_schema_is_not_flagged() {
        let yaml = "logs:\n  rotation:\n    default:\n      max_file_size: \"25MB\"\n";
        assert!(former_schema_guidance(yaml).is_none());
    }

    #[test]
    fn journal_dir_alone_is_not_former_schema() {
        // journal_dir is a valid current key; its presence must not trigger the guide.
        let yaml = "logs:\n  journal_dir: /var/log/netdata/otel/v1\n";
        assert!(former_schema_guidance(yaml).is_none());
    }

    #[test]
    fn unparseable_yaml_is_not_flagged() {
        assert!(former_schema_guidance("logs: [unclosed").is_none());
    }
}
