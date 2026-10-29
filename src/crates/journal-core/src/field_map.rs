//! Field-name compatibility between original log field names and systemd
//! journal field names.
//! The plugin's former journal writer stored each field under its original
//! name when journald accepted it (1..=64 bytes of `A-Z`, `0-9`, `_`, starting
//! with `A-Z`), otherwise under a remapped name, and recorded the
//! `remapped-name=original-name` pairs in journal entries tagged with
//! [`REMAPPING_MARKER`]. `JournalReader::load_remappings` (in
//! `src/crates/journal-core/src/file/reader.rs`) rebuilds a [`FieldMap`] from
//! those entries so queries can use the original names again.
use crate::collections::{HashMap, HashSet};

/// The journal item that tags a remapping bookkeeping entry.
///
/// The plugin's former journal writer wrote it into every entry that records
/// `remapped-name=original-name` pairs. Consumers detect those entries by this
/// exact payload: `JournalReader::load_remappings` rebuilds [`FieldMap`] from
/// them, journal-index excludes them from the index, and journal-engine skips
/// them in query results.
pub const REMAPPING_MARKER: &[u8] = b"ND_REMAPPING=1";

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
/// `JournalReader::load_remappings` builds it from the journal's remapping
/// bookkeeping entries to translate queries back to original names. A map
/// covers one journal file.
///
/// Keys are compared as exact bytes; only the first mapping per original name
/// is kept (see [`FieldMap::add_otel_mapping`]).
#[derive(Debug, Default)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FieldMap {
    /// Original field name → remapped journal name.
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

    /// Returns `true` when the map has no mappings.
    pub fn is_empty(&self) -> bool {
        self.otel_to_systemd.is_empty()
    }

    /// Returns the number of mappings in the map.
    pub fn len(&self) -> usize {
        self.otel_to_systemd.len()
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

        // Both remapped shapes the former writer produced: an encoded name
        // and the `ND_` + 32 uppercase hex MD5 fallback.
        let otel_name = b"log.body.hostname".to_vec();
        let systemd_name = "NDAAE_LB_HOSTNAME".to_string();
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

        let otel_name2 = b"trace id".to_vec();
        let systemd_name2 = "ND_0123456789ABCDEF0123456789ABCDEF".to_string();
        assert!(registry.add_otel_mapping(otel_name2.clone(), systemd_name2.clone()));

        assert_eq!(registry.len(), 2);

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
