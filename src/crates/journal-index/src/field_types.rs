//! Typed field-name and field=value strings for the journal index.
//!
//! Two newtypes give the index a typed vocabulary:
//! - [`FieldName`] is a bare field name ("PRIORITY"). It keys the
//!   `FileIndex::file_fields`/`indexed_fields` sets (`src/file_index.rs`),
//!   lists the fields an index or histogram covers
//!   (`journal-engine/src/facets.rs`), and names the source-timestamp field
//!   that orders entries at index time (`collect_source_field_info` in
//!   `src/file_indexer.rs`) and timestamps them at query time
//!   (`get_timestamp_field` in `src/file_index.rs`).
//! - [`FieldValuePair`] is a "field=value" string with the '=' position
//!   cached. It is the key type of `FileIndex::bitmaps` (`src/file_index.rs`),
//!   so its derived `Eq`/`Hash` are load-bearing: two pairs are equal iff
//!   their "field=value" strings are equal (struct doc below).
//!
//! Values come verbatim from the journal: the indexer lossily-decodes each
//! field data object's raw "field=value" payload and parses it
//! (`build_entries_index` in `src/file_indexer.rs`); nothing here
//! normalizes, trims or case-folds.
//! [`parse_timestamp`] re-reads one field's data-object payload as a u64
//! (the `_SOURCE_REALTIME_TIMESTAMP` path). Other consumers: journal-engine
//! aggregates per-pair counts under these keys
//! (`journal-engine/src/histogram.rs`) and carries query-result fields as
//! pairs (`journal-engine/src/logs/query.rs`); otel-legacy-logs parses
//! request selections into filters (`otel-legacy-logs/src/handler.rs`);
//! journal-function counts facet values per pair
//! (`journal-function/src/netdata/facets.rs`).
//!
//! Both types derive `Serialize`/`Deserialize` - pairs and names travel
//! inside the serialized [`crate::FileIndex`] - and, under the
//! crate's `allocative` feature, `allocative::Allocative`.

use serde::{Deserialize, Serialize};
use std::fmt;

/// A journal field name (e.g. "PRIORITY", "SYSLOG_IDENTIFIER"), with no
/// value attached.
///
/// Wrapper over `String` (tuple field 0). `new` validates user-provided
/// strings; `new_unchecked` wraps names the code already trusts. Derived
/// `Eq`/`Hash`/`Ord` are the inner string's (lexicographic order, so
/// "_HOSTNAME" sorts after uppercase-letter names).
#[derive(Debug, Clone, Eq, PartialEq, Hash, Serialize, Deserialize, Ord, PartialOrd)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FieldName(String);

impl FieldName {
    /// Wrap a string as a FieldName without validating it.
    ///
    /// For strings the code already trusts: field-table names during
    /// indexing (`src/file_indexer.rs`) and hardcoded names such as the
    /// default facet list (`journal-engine/src/facets.rs`). An embedded
    /// '=' is not rejected here; [`FieldValuePair::new_unchecked`] splits
    /// at the name's length, so such a name keeps the '=' in its field
    /// portion (and the pair stops equaling [`FieldValuePair::parse`] of
    /// the same string).
    pub fn new_unchecked(name: impl Into<String>) -> Self {
        Self(name.into())
    }

    /// Wrap a string as a FieldName, rejecting invalid names.
    ///
    /// Returns None for an empty name or one containing '=' (the name must
    /// stay splittable from its value). Used wherever a name's content is
    /// not already trusted, e.g. configured facet names
    /// (`journal-engine/src/facets.rs`).
    pub fn new(name: impl Into<String>) -> Option<Self> {
        let name = name.into();
        if name.is_empty() || name.contains('=') {
            None
        } else {
            Some(Self(name))
        }
    }

    /// Get the field name as a string slice.
    pub fn as_str(&self) -> &str {
        &self.0
    }

    /// Get the field name as a byte slice.
    pub fn as_bytes(&self) -> &[u8] {
        self.0.as_bytes()
    }

    /// Convert into the inner String.
    pub fn into_inner(self) -> String {
        self.0
    }

    /// Combine this field name with a value to create a FieldValuePair.
    pub fn with_value(&self, value: impl AsRef<str>) -> FieldValuePair {
        FieldValuePair::new_unchecked(self.clone(), value.as_ref().to_string())
    }
}

impl fmt::Display for FieldName {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl AsRef<str> for FieldName {
    fn as_ref(&self) -> &str {
        &self.0
    }
}

/// A "field=value" string (e.g. "PRIORITY=error"), the unit the index is
/// keyed on.
///
/// Stored as the full `key` string plus `split_pos`, the offset of the '='
/// between field and value, so field()/value() are plain slices. The value
/// part may itself contain '='; the split is the first '=' (parse) or the
/// field-name length (the unchecked constructors).
///
/// `Eq`/`Hash`/`Ord` derive over `(key, split_pos)`, which compares equal
/// exactly when the "field=value" strings are equal, as long as field
/// names are '='-free ([`FieldName::new`] enforces this). That equivalence
/// is what lets this type stand in for the plain string as the key of
/// `FileIndex::bitmaps` (`src/file_index.rs`), journal-engine's per-bucket
/// `fv_counts` (`journal-engine/src/histogram.rs`) and journal-function's
/// facet counts (`journal-function/src/netdata/facets.rs`). Ordering is
/// the string's lexicographic order ("PRIORITY=debug" < "PRIORITY=error").
#[derive(Debug, Clone, Eq, PartialEq, Hash, Serialize, Deserialize, Ord, PartialOrd)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FieldValuePair {
    key: String,
    split_pos: usize,
}

impl FieldValuePair {
    /// Build a pair from an already-separated field name and value.
    ///
    /// Unchecked: split_pos is taken to be the field name's length, so
    /// `field` must not contain '='; otherwise the cached position is not
    /// the first '=' of the key and the pair stops comparing equal to
    /// `parse` of the same string. The indexer re-keys a parsed pair under
    /// the requested (remapped) field name this way
    /// (`build_entries_index` in `src/file_indexer.rs`).
    pub fn new_unchecked(field: FieldName, value: String) -> Self {
        let split_pos = field.as_str().len();
        let key = format!("{}={}", field.as_str(), value);
        Self { key, split_pos }
    }

    /// Parse a "field=value" string, splitting at the first '='.
    ///
    /// Returns None when there is no '=' or the field name would be empty
    /// ("=value"); everything after the first '=' stays in the value,
    /// further '=' included. The indexer parses journal payloads with this
    /// (`build_entries_index` in `src/file_indexer.rs`); the engine re-parses
    /// serialized bitmap keys with it (`journal-engine/src/histogram.rs`).
    pub fn parse(s: impl AsRef<str>) -> Option<Self> {
        let s = s.as_ref();
        let split_pos = s.find('=')?;

        if split_pos == 0 {
            // Empty field name
            return None;
        }

        Some(Self {
            key: s.to_string(),
            split_pos,
        })
    }

    /// Get the field name portion.
    pub fn field(&self) -> &str {
        &self.key[..self.split_pos]
    }

    /// Get the value portion.
    pub fn value(&self) -> &str {
        &self.key[self.split_pos + 1..]
    }

    /// Get the full "field=value" string.
    pub fn as_str(&self) -> &str {
        &self.key
    }

    /// Get the full "field=value" as a byte slice.
    pub fn as_bytes(&self) -> &[u8] {
        self.key.as_bytes()
    }

    /// Convert into the inner String.
    pub fn into_inner(self) -> String {
        self.key
    }

    /// Extract the field name as a FieldName.
    pub fn extract_field(&self) -> FieldName {
        FieldName::new_unchecked(self.field())
    }

    /// Decompose into (field_name, value).
    pub fn decompose(self) -> (FieldName, String) {
        let field = FieldName::new_unchecked(self.field());
        let value = self.value().to_string();
        (field, value)
    }

    /// Extract the value bytes from a "field=value" payload, zero-copy.
    ///
    /// Returns the subslice after '=' when `payload` starts with exactly
    /// `field_name` followed by '=' (a name run-on like "PRIORITYX=6" is
    /// rejected), and None otherwise. Backs [`parse_timestamp`] on journal
    /// data-object payloads.
    ///
    /// # Examples
    ///
    /// ```
    /// # use journal_index::FieldValuePair;
    /// let payload = b"PRIORITY=6";
    /// let field_name = b"PRIORITY";
    /// let value = FieldValuePair::strip_field_prefix(field_name, payload);
    /// assert_eq!(value, Some(&b"6"[..]));
    ///
    /// // Wrong field name
    /// assert_eq!(FieldValuePair::strip_field_prefix(b"MESSAGE", payload), None);
    ///
    /// // Missing '='
    /// assert_eq!(FieldValuePair::strip_field_prefix(b"PRIORITY", b"PRIORITY6"), None);
    /// ```
    pub fn strip_field_prefix<'a>(field_name: &[u8], payload: &'a [u8]) -> Option<&'a [u8]> {
        if !payload.starts_with(field_name) {
            return None;
        }

        let offset = field_name.len();

        if payload.len() <= offset || payload[offset] != b'=' {
            return None;
        }

        Some(&payload[offset + 1..])
    }
}

impl fmt::Display for FieldValuePair {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.key)
    }
}

impl AsRef<str> for FieldValuePair {
    fn as_ref(&self) -> &str {
        &self.key
    }
}

// String conversions for string-expecting call sites: the by-value forms
// hand over the inner String, the by-reference forms copy through
// `to_string()`.
impl From<FieldValuePair> for String {
    fn from(pair: FieldValuePair) -> String {
        pair.into_inner()
    }
}

impl From<&FieldValuePair> for String {
    fn from(pair: &FieldValuePair) -> String {
        pair.to_string()
    }
}

impl From<FieldName> for String {
    fn from(name: FieldName) -> String {
        name.into_inner()
    }
}

impl From<&FieldName> for String {
    fn from(name: &FieldName) -> String {
        name.to_string()
    }
}

/// Parse a u64 timestamp out of a field's data object.
///
/// Takes the object's raw payload (the stored "field=value" bytes), strips
/// the "field_name=" prefix with [`FieldValuePair::strip_field_prefix`],
/// requires the value to be UTF-8 and parses it as u64; each step maps to
/// its `IndexError` variant (`InvalidFieldPrefix`, `NonUtf8Payload`,
/// `NonIntegerPayload`). The payload is used as-is - no decompression - so
/// a compressed data object normally fails to parse and its entries fall
/// back to another timestamp source (`get_entry_timestamp` in
/// `src/file_index.rs`).
///
/// Used for the source timestamp field (typically
/// `_SOURCE_REALTIME_TIMESTAMP`, microseconds since the epoch): the
/// indexer orders entries by it (`collect_source_field_info` in
/// `src/file_indexer.rs`) and per-entry
/// timestamp lookups reuse it (`get_timestamp_field` in
/// `src/file_index.rs`).
pub fn parse_timestamp(
    field_name: &[u8],
    data_object: &journal_core::file::DataObject<&[u8]>,
) -> crate::Result<u64> {
    let payload = data_object.raw_payload();

    let value_bytes = FieldValuePair::strip_field_prefix(field_name, payload)
        .ok_or_else(|| crate::IndexError::InvalidFieldPrefix)?;

    let timestamp_str =
        std::str::from_utf8(value_bytes).map_err(|_| crate::IndexError::NonUtf8Payload)?;

    timestamp_str
        .parse::<u64>()
        .map_err(|_| crate::IndexError::NonIntegerPayload)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_field_name_creation() {
        assert!(FieldName::new("PRIORITY").is_some());
        assert!(FieldName::new("SYSLOG_IDENTIFIER").is_some());
        assert!(FieldName::new("").is_none());
        assert!(FieldName::new("PRIORITY=error").is_none());
    }

    #[test]
    fn test_field_name_as_bytes() {
        let field = FieldName::new("PRIORITY").unwrap();
        assert_eq!(field.as_bytes(), b"PRIORITY");
        assert_eq!(field.as_str(), "PRIORITY");
    }

    #[test]
    fn test_field_value_pair_parsing() {
        let pair = FieldValuePair::parse("PRIORITY=error").unwrap();
        assert_eq!(pair.field(), "PRIORITY");
        assert_eq!(pair.value(), "error");
        assert_eq!(pair.as_str(), "PRIORITY=error");
        assert_eq!(pair.as_bytes(), b"PRIORITY=error");

        // Values can contain '=' characters
        let pair = FieldValuePair::parse("MESSAGE=IN=eth0 OUT= MAC=aa:bb:cc").unwrap();
        assert_eq!(pair.field(), "MESSAGE");
        assert_eq!(pair.value(), "IN=eth0 OUT= MAC=aa:bb:cc");

        assert!(FieldValuePair::parse("PRIORITY").is_none());
        assert!(FieldValuePair::parse("=error").is_none());
    }

    #[test]
    fn test_field_with_value() {
        let field = FieldName::new("PRIORITY").unwrap();
        let pair = field.with_value("error");

        assert_eq!(pair.field(), "PRIORITY");
        assert_eq!(pair.value(), "error");
        assert_eq!(pair.as_str(), "PRIORITY=error");
    }

    #[test]
    fn test_field_name_ordering() {
        let mut fields = vec![
            FieldName::new("PRIORITY").unwrap(),
            FieldName::new("_HOSTNAME").unwrap(),
            FieldName::new("SYSLOG_IDENTIFIER").unwrap(),
            FieldName::new("ERRNO").unwrap(),
        ];

        fields.sort();

        assert_eq!(fields[0].as_str(), "ERRNO");
        assert_eq!(fields[1].as_str(), "PRIORITY");
        assert_eq!(fields[2].as_str(), "SYSLOG_IDENTIFIER");
        assert_eq!(fields[3].as_str(), "_HOSTNAME");
    }

    #[test]
    fn test_field_value_pair_ordering() {
        let mut pairs = vec![
            FieldValuePair::parse("PRIORITY=error").unwrap(),
            FieldValuePair::parse("PRIORITY=debug").unwrap(),
            FieldValuePair::parse("_HOSTNAME=server2").unwrap(),
            FieldValuePair::parse("_HOSTNAME=server1").unwrap(),
        ];

        pairs.sort();

        assert_eq!(pairs[0].as_str(), "PRIORITY=debug");
        assert_eq!(pairs[1].as_str(), "PRIORITY=error");
        assert_eq!(pairs[2].as_str(), "_HOSTNAME=server1");
        assert_eq!(pairs[3].as_str(), "_HOSTNAME=server2");
    }

    #[test]
    fn test_strip_field_prefix() {
        // Valid field=value
        let payload = b"PRIORITY=6";
        let value = FieldValuePair::strip_field_prefix(b"PRIORITY", payload);
        assert_eq!(value, Some(&b"6"[..]));

        // Value with special characters
        let payload = b"MESSAGE=error: connection=failed";
        let value = FieldValuePair::strip_field_prefix(b"MESSAGE", payload);
        assert_eq!(value, Some(&b"error: connection=failed"[..]));

        // Empty value
        let payload = b"FIELD=";
        let value = FieldValuePair::strip_field_prefix(b"FIELD", payload);
        assert_eq!(value, Some(&b""[..]));

        // Wrong field name
        let payload = b"PRIORITY=6";
        let value = FieldValuePair::strip_field_prefix(b"MESSAGE", payload);
        assert_eq!(value, None);

        // Missing '='
        let payload = b"PRIORITY6";
        let value = FieldValuePair::strip_field_prefix(b"PRIORITY", payload);
        assert_eq!(value, None);

        // Field name matches but continues (no '=')
        let payload = b"PRIORITYX=6";
        let value = FieldValuePair::strip_field_prefix(b"PRIORITY", payload);
        assert_eq!(value, None);

        // Empty payload
        let value = FieldValuePair::strip_field_prefix(b"PRIORITY", b"");
        assert_eq!(value, None);

        // Payload shorter than field name
        let value = FieldValuePair::strip_field_prefix(b"PRIORITY", b"PRI");
        assert_eq!(value, None);
    }
}
