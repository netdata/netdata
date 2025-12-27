//! Type-safe wrappers for field names and field=value pairs.
//!
//! This module provides newtypes that distinguish between:
//! - Field names (e.g., "PRIORITY")
//! - Field=value pairs (e.g., "PRIORITY=error")
//!
//! These types are used throughout the journal indexing system to ensure
//! type safety and prevent mixing different concepts.

use serde::{Deserialize, Serialize};
use std::fmt;

/// A field name (e.g., "PRIORITY", "SYSLOG_IDENTIFIER").
///
/// This represents just the field name without any associated value.
#[derive(Debug, Clone, Eq, PartialEq, Hash, Serialize, Deserialize, Ord, PartialOrd)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FieldName(String);

impl FieldName {
    /// Create a new FieldName without validation.
    ///
    /// Use this when you know the string is a valid field name
    /// (e.g., from trusted sources like hardcoded constants).
    pub fn new_unchecked(name: impl Into<String>) -> Self {
        Self(name.into())
    }

    /// Create a FieldName with validation.
    ///
    /// Returns None if the name contains '=' or is empty.
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

/// A field=value pair (e.g., "PRIORITY=error", "SYSLOG_IDENTIFIER=systemd").
///
/// Invariant: Always in the format "field=value". The value portion may contain '=' characters.
/// The split is always at the first '=' character.
#[derive(Debug, Clone, Eq, PartialEq, Hash, Serialize, Deserialize, Ord, PartialOrd)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FieldValuePair {
    // Store the formatted string for efficient HashMap lookups
    key: String,
    // Cache the split position for fast field/value extraction
    split_pos: usize,
}

impl FieldValuePair {
    /// Create a new FieldValuePair from field and value components.
    ///
    /// This is unchecked - assumes field doesn't contain '='.
    pub fn new_unchecked(field: FieldName, value: String) -> Self {
        let split_pos = field.as_str().len();
        let key = format!("{}={}", field.as_str(), value);
        Self { key, split_pos }
    }

    /// Parse a "field=value" string into a FieldValuePair.
    ///
    /// Returns None if the string doesn't contain '=' or if the field name is empty.
    /// The value portion may contain '=' characters - parsing splits on the first '=' only.
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

    /// Extract the value portion from a field=value byte slice.
    ///
    /// Returns the value bytes if the payload is in the format "field_name=value",
    /// where `field_name` matches the provided bytes. This is a zero-copy operation.
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
        // Check that payload starts with field_name
        if !payload.starts_with(field_name) {
            return None;
        }

        let offset = field_name.len();

        // Check that there's an '=' after the field name
        if payload.len() <= offset || payload[offset] != b'=' {
            return None;
        }

        // Return the value portion after '='
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

// Conversion helpers for backward compatibility
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

/// Parse a u64 timestamp from a field data object.
pub fn parse_timestamp(
    field_name: &[u8],
    data_object: &journal_core::file::DataObject<&[u8]>,
) -> crate::Result<u64> {
    let payload = data_object.payload_bytes();

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
