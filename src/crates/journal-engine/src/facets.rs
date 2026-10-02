//! Which journal fields get per-value bitmaps in a file index.
//!
//! A facet is a field whose distinct values get a `Bitmap` at index time
//! (`journal-index/src/file_indexer.rs` `FileIndexer`; each FileIndex stores
//! one `HashMap<FieldValuePair, Bitmap>`,
//! `journal-index/src/file_index.rs` `FileIndex`). Fields outside the set are not
//! indexed and surface as `unindexed_fields` in
//! [`crate::histogram::BucketResponse`]s.
//!
//! [`Facets`] holds that set canonicalized — validated, sorted, deduped —
//! so the value is order-insensitive and any two instances over the same
//! fields compare equal. It also precomputes the hash of the canonical
//! names at construction: the `Hash` and `PartialEq` impls below consume
//! that value, so hashing this type inside a cache-key context costs a
//! single u64 write. The hash is not serialized; deserialization rebuilds
//! it through [`Facets::new`], restoring the
//! `precomputed_hash == hash(fields)` invariant by construction.
//!
//! [`Facets`] is the facet component of the file-index cache key
//! ([`crate::cache::FileIndexKey`]) and reaches indexing as a slice via
//! [`Facets::as_slice`].
//! In-tree builders: the legacy-logs handler from the request's facet
//! list (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`), the histogram engine
//! ([`crate::histogram::HistogramEngine::compute_from_indexes`]), and the example (`examples/index.rs` `main`).
//! Re-exported by journal-engine (crate root) and journal-function
//! (`journal-function/src/lib.rs`).

use journal_index::FieldName;
use std::hash::Hash;
use std::sync::Arc;

/// The canonicalized set of fields a file index extracts per-value
/// bitmaps for.
///
/// [`Self::new`] is the only constructor and canonicalizes its input —
/// invalid names dropped, sorted, deduped — so input order cannot leak
/// into the value, and two instances over the same fields always compare
/// equal. That is what lets the same field set hit the same file-index
/// cache entry regardless of input order ([`crate::cache::FileIndexKey`]).
///
/// # Serialization
///
/// The wire format is a plain sequence of field-name strings.
/// `precomputed_hash` is not part of it: deserialization rebuilds the
/// value through [`Self::new`], restoring the
/// `precomputed_hash == hash(fields)` invariant by construction.
#[derive(Debug, Clone)]
pub struct Facets {
    /// The field names in canonical order: sorted (`FieldName` derives
    /// Ord, `journal-index/src/field_types.rs` `FieldName`), deduped, caller
    /// names validated (`journal-index/src/field_types.rs` `FieldName::new`). Shared
    /// behind an `Arc` so the per-file cache keys cloned from one
    /// `Facets` copy a pointer, not the list ([`crate::cache::FileIndexKey::new`]).
    fields: Arc<Vec<FieldName>>,
    /// Cached hash of the canonical names, taken at construction;
    /// consumed by the `Hash` and `PartialEq` impls below.
    precomputed_hash: u64,
}

impl Hash for Facets {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        // The precomputed value is the hash of the canonical names
        // (taken by `Facets::new`), so it stands in for re-hashing them
        // — one u64 write instead of N.
        state.write_u64(self.precomputed_hash);
    }
}

impl PartialEq for Facets {
    fn eq(&self, other: &Self) -> bool {
        // Fast reject on the precomputed hash: equal sets always hash
        // equal, so a mismatch proves inequality. On a match,
        // `Arc::ptr_eq` skips the Vec compare for clones sharing the
        // allocation, and the full value compare settles hash collisions
        // — equality never trusts the hash alone.
        if self.precomputed_hash != other.precomputed_hash {
            return false;
        }

        Arc::ptr_eq(&self.fields, &other.fields) || self.fields == other.fields
    }
}

impl Eq for Facets {}

impl Facets {
    /// Default facet set used when a caller passes no names.
    ///
    /// Trusted constants, so they skip validation via
    /// `FieldName::new_unchecked`
    /// (`journal-index/src/field_types.rs` `FieldName::new_unchecked`). Commented-out entries
    /// are candidates kept disabled in the default set; the list is
    /// unsorted here — `Facets::new` canonicalizes it.
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

    /// Builds the canonical facet set from field-name strings.
    ///
    /// An empty list selects the default set. Names are validated
    /// (`journal-index/src/field_types.rs` `FieldName::new`) and invalid ones — empty,
    /// or containing `=` — are dropped silently, so an all-invalid list
    /// yields an empty set. Names are matched verbatim (no case folding),
    /// sorted and deduped before hashing, so input order is irrelevant.
    pub fn new(facets: &[String]) -> Self {
        let mut facets = if facets.is_empty() {
            Self::default_facets()
        } else {
            // Invalid names (empty, or containing '=') drop silently.
            facets
                .iter()
                .filter_map(|s| FieldName::new(s.clone()))
                .collect()
        };

        // Canonical form: sorted, then deduped — input order cannot leak
        // into the value.
        facets.sort();
        facets.dedup();

        use std::hash::Hasher;
        let mut hasher = std::hash::DefaultHasher::new();
        // Hash the canonical name strings in order: same fields in, same
        // hash out. std's fixed-key DefaultHasher makes this
        // deterministic across instances.
        for field in &facets {
            field.as_str().hash(&mut hasher);
        }
        let precomputed_hash = hasher.finish();

        Self {
            fields: Arc::new(facets),
            precomputed_hash,
        }
    }

    /// Iterates the facet fields in canonical (sorted) order.
    pub fn iter(&self) -> impl Iterator<Item = &FieldName> {
        self.fields.iter()
    }

    /// The facet fields as a slice, in canonical order — what
    /// `FileIndexer::index` receives in `batch_compute_file_indexes`.
    pub fn as_slice(&self) -> &[FieldName] {
        &self.fields
    }

    /// Number of facet fields.
    pub fn len(&self) -> usize {
        self.fields.len()
    }

    /// True only when every input name was filtered out; `Facets::new`
    /// maps an empty input to the default set instead.
    #[allow(dead_code)]
    pub fn is_empty(&self) -> bool {
        self.fields.is_empty()
    }

    /// The precomputed `hash(fields)` value.
    pub fn precomputed_hash(&self) -> u64 {
        self.precomputed_hash
    }
}

impl serde::Serialize for Facets {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
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
        // Rebuilds via `Facets::new`, so the hash is re-derived on load
        // and the empty-input rule applies again: a serialized empty
        // list comes back as the default set, not an empty one.
        let fields: Vec<String> = Vec::deserialize(deserializer)?;
        Ok(Facets::new(&fields))
    }
}
