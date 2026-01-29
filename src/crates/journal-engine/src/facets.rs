//! Field facets configuration for indexing
//!
//! Facets determine which fields should be indexed when processing journal files.

use journal_index::FieldName;
use std::hash::Hash;
use std::sync::Arc;

/// Configuration specifying which fields should be indexed.
///
/// Facets are used as part of the cache key for file indexes, since different
/// field selections produce different indexes.
///
/// # Serialization
///
/// Facets serializes as a sequence of field name strings. The precomputed hash
/// is NOT serialized - it is recomputed during deserialization to maintain the
/// invariant that `precomputed_hash == hash(fields)`.
#[derive(Debug, Clone)]
pub struct Facets {
    fields: Arc<Vec<FieldName>>,
    precomputed_hash: u64,
}

impl Hash for Facets {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        state.write_u64(self.precomputed_hash);
    }
}

impl PartialEq for Facets {
    fn eq(&self, other: &Self) -> bool {
        if self.precomputed_hash != other.precomputed_hash {
            return false;
        }

        Arc::ptr_eq(&self.fields, &other.fields) || self.fields == other.fields
    }
}

impl Eq for Facets {}

impl Facets {
    fn default_facets() -> Vec<FieldName> {
        let v: Vec<&str> = vec![
            "_HOSTNAME",
            "PRIORITY",
            "SYSLOG_FACILITY",
            "ERRNO",
            "SYSLOG_IDENTIFIER",
            // "UNIT",
            "USER_UNIT",
            "MESSAGE_ID",
            "_BOOT_ID",
            "_SYSTEMD_OWNER_UID",
            "_UID",
            "OBJECT_SYSTEMD_OWNER_UID",
            "OBJECT_UID",
            "_GID",
            "OBJECT_GID",
            "_CAP_EFFECTIVE",
            "_AUDIT_LOGINUID",
            "OBJECT_AUDIT_LOGINUID",
            "CODE_FUNC",
            "ND_LOG_SOURCE",
            "CODE_FILE",
            "ND_ALERT_NAME",
            "ND_ALERT_CLASS",
            "_SELINUX_CONTEXT",
            "_MACHINE_ID",
            "ND_ALERT_TYPE",
            "_SYSTEMD_SLICE",
            "_EXE",
            // "_SYSTEMD_UNIT",
            "_NAMESPACE",
            "_TRANSPORT",
            "_RUNTIME_SCOPE",
            "_STREAM_ID",
            "ND_NIDL_CONTEXT",
            "ND_ALERT_STATUS",
            // "_SYSTEMD_CGROUP",
            "ND_NIDL_NODE",
            "ND_ALERT_COMPONENT",
            "_COMM",
            "_SYSTEMD_USER_UNIT",
            "_SYSTEMD_USER_SLICE",
            // "_SYSTEMD_SESSION",
            "__logs_sources",
            "log.severity_number",
        ];

        v.into_iter().map(FieldName::new_unchecked).collect()
    }

    pub fn new(facets: &[String]) -> Self {
        let mut facets = if facets.is_empty() {
            Self::default_facets()
        } else {
            // Parse and validate each facet string into FieldName
            facets
                .iter()
                .filter_map(|s| FieldName::new(s.clone()))
                .collect()
        };

        // Sort in order to get the same hash for the same set of fields
        facets.sort();

        use std::hash::Hasher;
        let mut hasher = std::hash::DefaultHasher::new();
        // Hash the string representation for consistency
        for field in &facets {
            field.as_str().hash(&mut hasher);
        }
        let precomputed_hash = hasher.finish();

        Self {
            fields: Arc::new(facets),
            precomputed_hash,
        }
    }

    /// Returns an iterator over the facet field names
    pub fn iter(&self) -> impl Iterator<Item = &FieldName> {
        self.fields.iter()
    }

    /// Returns the facet fields as a slice
    pub fn as_slice(&self) -> &[FieldName] {
        &self.fields
    }

    /// Returns the number of facet fields
    pub fn len(&self) -> usize {
        self.fields.len()
    }

    /// Returns true if there are no facet fields
    #[allow(dead_code)]
    pub fn is_empty(&self) -> bool {
        self.fields.is_empty()
    }

    pub fn precomputed_hash(&self) -> u64 {
        self.precomputed_hash
    }
}

impl serde::Serialize for Facets {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        // Serialize as a sequence of field name strings.
        // The precomputed_hash is NOT serialized - it will be recomputed on deserialization.
        use serde::ser::SerializeSeq;
        let mut seq = serializer.serialize_seq(Some(self.fields.len()))?;
        for field in self.fields.iter() {
            seq.serialize_element(field.as_str())?;
        }
        seq.end()
    }
}

impl<'de> serde::Deserialize<'de> for Facets {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        // Deserialize as a sequence of strings, then reconstruct via Facets::new()
        // which will recompute the hash, maintaining the invariant.
        let fields: Vec<String> = Vec::deserialize(deserializer)?;
        Ok(Facets::new(&fields))
    }
}
