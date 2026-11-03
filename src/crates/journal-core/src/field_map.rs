//! Field-name compatibility between original log field names and systemd
//! journal field names.
//! `journal-log-writer` stores each field under its original name when
//! [`is_systemd_compatible`] accepts it, otherwise under a remapped name from
//! `rdp::encode_full`, and records the `remapped-name=original-name` pairs in
//! journal entries tagged with [`REMAPPING_MARKER`]. [`FieldMap`] is the
//! bidirectional map both sides of that protocol share: the writer fills it as
//! it remaps entries, and `JournalReader::load_remappings` (in
//! `src/crates/journal-core/src/file/reader.rs`) rebuilds it from those
//! entries so queries can use the original names again.
use crate::collections::{HashMap, HashSet};

/// The journal item that tags a remapping bookkeeping entry.
///
/// `journal-log-writer` writes it into every entry that records
/// `remapped-name=original-name` pairs. Consumers detect those entries by this
/// exact payload: `JournalReader::load_remappings` rebuilds [`FieldMap`] from
/// them, journal-index excludes them from the index, and journal-engine skips
/// them in query results. The field-name part, `ND_REMAPPING`, passes
/// [`is_systemd_compatible`].
pub const REMAPPING_MARKER: &[u8] = b"ND_REMAPPING=1";

/// Returns `true` when `field_name` can be written to a journal file
/// unchanged: 1..=64 bytes, starting with an uppercase letter, all bytes
/// `A-Z`, `0-9` or `_`.
///
/// `journal-log-writer` runs this on every field name and remaps the failures
/// with `rdp::encode_full` (see [`FieldMap`]).
pub fn is_systemd_compatible(field_name: &[u8]) -> bool {
    if field_name.is_empty() || field_name.len() > 64 {
        return false;
    }

    if !field_name[0].is_ascii_uppercase() {
        return false;
    }

    field_name
        .iter()
        .all(|&b| b.is_ascii_uppercase() || b.is_ascii_digit() || b == b'_')
}

/// Returns the field name of a `KEY=VALUE` journal item: the bytes before the
/// first `=`, borrowed from `item`.
///
/// `None` when the item contains no `=`. The name may be empty (`=value`);
/// values may themselves contain `=`, hence "first".
pub fn extract_field_name(item: &[u8]) -> Option<&[u8]> {
    item.iter().position(|&b| b == b'=').map(|pos| &item[..pos])
}

/// Bidirectional map between original field names and their remapped
/// systemd-compatible names.
///
/// `journal-log-writer` fills it as it renames fields; the remapped=original
/// pairs are also written to the journal's remapping bookkeeping entries, from
/// which `JournalReader::load_remappings` reconstructs the map to translate
/// queries back to original names. A map covers one journal file: the writer
/// clears it on rotation, the reader rebuilds it per file.
///
/// Keys are compared as exact bytes; only the first mapping per original name
/// is kept (see [`FieldMap::add_otel_mapping`]).
#[derive(Debug, Default)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FieldMap {
    /// Original field name → remapped journal name from `rdp::encode_full`.
    otel_to_systemd: HashMap<Vec<u8>, String>,
    /// Remapped journal name → original field name (reverse lookup).
    systemd_to_otel: HashMap<String, Vec<u8>>,
}

impl FieldMap {
    /// Creates an empty map.
    pub fn new() -> Self {
        Self {
            otel_to_systemd: HashMap::default(),
            systemd_to_otel: HashMap::default(),
        }
    }

    /// Registers a mapping in both directions.
    ///
    /// Returns `true` when the original name was new; a repeated original name
    /// changes nothing and returns `false`, so the first mapping per name
    /// wins.
    pub fn add_otel_mapping(&mut self, otel_name: Vec<u8>, systemd_name: String) -> bool {
        if self.otel_to_systemd.contains_key(&otel_name) {
            return false;
        }

        self.systemd_to_otel
            .insert(systemd_name.clone(), otel_name.clone());
        self.otel_to_systemd.insert(otel_name, systemd_name);
        true
    }

    /// Returns the remapped journal name for `otel_name`, or `None` if
    /// unmapped.
    pub fn get_systemd_name(&self, otel_name: &[u8]) -> Option<&str> {
        self.otel_to_systemd.get(otel_name).map(|s| s.as_str())
    }

    /// Returns the original field name for a remapped journal name, or `None`
    /// if unmapped.
    pub fn get_otel_name(&self, systemd_name: &str) -> Option<&[u8]> {
        self.systemd_to_otel.get(systemd_name).map(|v| v.as_slice())
    }

    /// Returns `true` when a mapping exists for this original field name.
    pub fn contains_otel_name(&self, otel_name: &[u8]) -> bool {
        self.otel_to_systemd.contains_key(otel_name)
    }

    /// Returns `true` when the map has no mappings.
    pub fn is_empty(&self) -> bool {
        self.otel_to_systemd.is_empty()
    }

    /// Returns the number of mappings in the map.
    pub fn len(&self) -> usize {
        self.otel_to_systemd.len()
    }

    /// Discards all mappings in both directions.
    pub fn clear(&mut self) {
        self.otel_to_systemd.clear();
        self.systemd_to_otel.clear();
    }

    /// Consumes the map and returns the set of original field names.
    ///
    /// Keys are reinterpreted as UTF-8 without validation
    /// (`String::from_utf8_unchecked`); sound only while original names are
    /// valid UTF-8.
    pub fn fields(self) -> HashSet<String> {
        self.otel_to_systemd
            .into_keys()
            .map(|x| unsafe { String::from_utf8_unchecked(x) })
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_systemd_compatible() {
        // Valid field names
        assert!(is_systemd_compatible(b"MESSAGE"));
        assert!(is_systemd_compatible(b"PRIORITY"));
        assert!(is_systemd_compatible(b"USER_ID"));
        assert!(is_systemd_compatible(b"A"));
        assert!(is_systemd_compatible(b"A1"));
        assert!(is_systemd_compatible(b"A_B_C"));
        assert!(is_systemd_compatible(b"Z9_"));
        assert!(is_systemd_compatible(b"ND_REMAPPING")); // Field-name part of `REMAPPING_MARKER`

        // Invalid field names - lowercase
        assert!(!is_systemd_compatible(b"message"));
        assert!(!is_systemd_compatible(b"Message"));
        assert!(!is_systemd_compatible(b"mESSAGE"));

        // Invalid field names - special chars
        assert!(!is_systemd_compatible(b"my.field"));
        assert!(!is_systemd_compatible(b"my-field"));
        assert!(!is_systemd_compatible(b"my:field"));
        assert!(!is_systemd_compatible(b"my field"));

        // Invalid field names - doesn't start with uppercase
        assert!(!is_systemd_compatible(b"1MESSAGE"));
        assert!(!is_systemd_compatible(b"_MESSAGE"));

        // Invalid field names - empty or too long
        assert!(!is_systemd_compatible(b""));
        assert!(!is_systemd_compatible(&[b'A'; 65]));
    }

    #[test]
    fn test_extract_field_name() {
        assert_eq!(
            extract_field_name(b"MESSAGE=hello"),
            Some(b"MESSAGE".as_ref())
        );
        assert_eq!(
            extract_field_name(b"PRIORITY=5"),
            Some(b"PRIORITY".as_ref())
        );
        // First `=` wins; empty key and missing `=` are both valid outcomes.
        assert_eq!(extract_field_name(b"A="), Some(b"A".as_ref()));
        assert_eq!(extract_field_name(b"=value"), Some(b"".as_ref()));
        assert_eq!(extract_field_name(b"NO_EQUALS"), None);
        assert_eq!(extract_field_name(b""), None);
    }

    #[test]
    fn test_remapping_registry() {
        let mut registry = FieldMap::new();

        assert!(registry.is_empty());
        assert_eq!(registry.len(), 0);

        // Names come from the real encoder journal-log-writer uses.
        let otel_name = b"my.field.name".to_vec();
        let systemd_name = rdp::encode_full(&otel_name);
        assert!(registry.add_otel_mapping(otel_name.clone(), systemd_name.clone()));

        assert!(!registry.is_empty());
        assert_eq!(registry.len(), 1);

        assert_eq!(
            registry.get_systemd_name(&otel_name),
            Some(systemd_name.as_str())
        );
        assert_eq!(
            registry.get_otel_name(&systemd_name),
            Some(otel_name.as_slice())
        );

        // Duplicate original name: rejected, the mapping stays unchanged.
        assert!(!registry.add_otel_mapping(otel_name.clone(), systemd_name.clone()));
        assert_eq!(registry.len(), 1);

        let otel_name2 = b"trace-id".to_vec();
        let systemd_name2 = rdp::encode_full(&otel_name2);
        assert!(registry.add_otel_mapping(otel_name2.clone(), systemd_name2.clone()));

        assert_eq!(registry.len(), 2);

        assert!(registry.contains_otel_name(&otel_name));
        assert!(registry.contains_otel_name(&otel_name2));

        assert_eq!(
            registry.get_systemd_name(&otel_name2),
            Some(systemd_name2.as_str())
        );
        assert_eq!(
            registry.get_otel_name(&systemd_name2),
            Some(otel_name2.as_slice())
        );
    }
}
