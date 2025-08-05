use crate::collections::{HashMap, HashSet};

pub const REMAPPING_MARKER: &[u8] = b"ND_REMAPPING=1";

/// Validates if a field name is compatible with systemd journal requirements.
pub fn is_systemd_compatible(field_name: &[u8]) -> bool {
    if field_name.is_empty() || field_name.len() > 64 {
        return false;
    }

    // First byte must be uppercase A-Z
    if !field_name[0].is_ascii_uppercase() {
        return false;
    }

    // All bytes must be uppercase A-Z, digit 0-9, or underscore
    field_name
        .iter()
        .all(|&b| b.is_ascii_uppercase() || b.is_ascii_digit() || b == b'_')
}

/// Extracts the field name from a `KEY=VALUE` pair.
pub fn extract_field_name(item: &[u8]) -> Option<&[u8]> {
    item.iter().position(|&b| b == b'=').map(|pos| &item[..pos])
}

/// Bidirectional mapping registry between original field names and their systemd-compatible versions.
#[derive(Debug, Default)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FieldMap {
    /// Maps original field name → remapped name (ND_<md5>)
    otel_to_systemd: HashMap<Vec<u8>, String>,
    /// Maps remapped name (ND_<md5>) → original field name
    systemd_to_otel: HashMap<String, Vec<u8>>,
}

impl FieldMap {
    /// Creates a new empty remapping registry.
    pub fn new() -> Self {
        Self {
            otel_to_systemd: HashMap::default(),
            systemd_to_otel: HashMap::default(),
        }
    }

    /// Adds a new mapping to the registry.
    ///
    /// Returns `true` if this is a new mapping, `false` if it already existed.
    pub fn add_otel_mapping(&mut self, otel_name: Vec<u8>, systemd_name: String) -> bool {
        if self.otel_to_systemd.contains_key(&otel_name) {
            return false;
        }

        self.systemd_to_otel
            .insert(systemd_name.clone(), otel_name.clone());
        self.otel_to_systemd.insert(otel_name, systemd_name);
        true
    }

    /// Gets the systemd-compatible name for an original field name.
    ///
    /// Returns `None` if no mapping exists for this field name.
    pub fn get_systemd_name(&self, otel_name: &[u8]) -> Option<&str> {
        self.otel_to_systemd.get(otel_name).map(|s| s.as_str())
    }

    /// Gets the original field name for a systemd-compatible name.
    ///
    /// Returns `None` if no mapping exists for this systemd name.
    pub fn get_otel_name(&self, systemd_name: &str) -> Option<&[u8]> {
        self.systemd_to_otel.get(systemd_name).map(|v| v.as_slice())
    }

    /// Returns `true` if the registry contains a mapping for this original field name.
    pub fn contains_otel_name(&self, otel_name: &[u8]) -> bool {
        self.otel_to_systemd.contains_key(otel_name)
    }

    /// Returns `true` if the registry is empty.
    pub fn is_empty(&self) -> bool {
        self.otel_to_systemd.is_empty()
    }

    /// Returns the number of mappings in the registry.
    pub fn len(&self) -> usize {
        self.otel_to_systemd.len()
    }

    pub fn clear(&mut self) {
        self.otel_to_systemd.clear();
        self.systemd_to_otel.clear();
    }

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
        assert!(is_systemd_compatible(b"ND_REMAPPING")); // Our marker field

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

        // Add first mapping
        let otel_name = b"my.field.name".to_vec();
        let systemd_name = rdp::encode_full(&otel_name);
        assert!(registry.add_otel_mapping(otel_name.clone(), systemd_name.clone()));

        assert!(!registry.is_empty());
        assert_eq!(registry.len(), 1);

        // Lookup works both ways
        assert_eq!(
            registry.get_systemd_name(&otel_name),
            Some(systemd_name.as_str())
        );
        assert_eq!(
            registry.get_otel_name(&systemd_name),
            Some(otel_name.as_slice())
        );

        // Adding same mapping again returns false
        assert!(!registry.add_otel_mapping(otel_name.clone(), systemd_name.clone()));
        assert_eq!(registry.len(), 1);

        // Add different mapping
        let otel_name2 = b"trace-id".to_vec();
        let systemd_name2 = rdp::encode_full(&otel_name2);
        assert!(registry.add_otel_mapping(otel_name2.clone(), systemd_name2.clone()));

        assert_eq!(registry.len(), 2);

        // Both mappings exist
        assert!(registry.contains_otel_name(&otel_name));
        assert!(registry.contains_otel_name(&otel_name2));

        // Lookups still work
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
