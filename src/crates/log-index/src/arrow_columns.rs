//! Pre-downcast Arrow column accessors for OTAP attribute RecordBatches,
//! and the [`AttrsMap`] builder that turns them into interned `key=value` IDs.
//!
//! # Pre-computed hash optimization
//!
//! Building the inverted index requires converting each attribute row into a
//! `"key=value"` string, hashing it, and interning it. This is expensive:
//! strings average ~80 bytes, and the same pairs repeat across thousands of
//! log records. The producer (`otel-ingestor`) pre-computes `xxhash64("key=value")`
//! for every attribute and stores the hashes as a synthetic `_nd_kv_hash`
//! `BytesValue` — the last attribute in each log record's group.
//!
//! [`AttrsMap::build`] exploits this in a single pass:
//!
//! 1. Attributes for each `parent_id` are **contiguous** in the Arrow batch
//!    (guaranteed by OTAP's encoder).
//! 2. The last row of each group is checked for `_nd_kv_hash`.
//! 3. If present, its bytes contain N little-endian `u64` hashes, one per
//!    preceding attribute row, in order.
//! 4. For each attribute, try a **hash-only lookup** in the string interner.
//!    On hit (the `key=value` was already interned from a previous record),
//!    skip string formatting entirely. On miss (first encounter), format the
//!    string and intern it with the pre-computed hash so future lookups hit.
//!
//! The string interner uses an identity hasher (see [`crate::kv_interner`])
//! so the pre-computed `u64` is used directly as the HashMap bucket key —
//! no re-hashing.
//!
//! See `otel-ingestor/src/arrow_bridge.rs` for the producer side, including
//! the hash contract (value formatting rules that must match
//! [`AttrsColumns::append_value`]).

use arrow::array::*;
use arrow::datatypes::*;
use arrow::record_batch::RecordBatch;
use hashbrown::HashMap;

use crate::KeyValueId;
use crate::kv_interner::KeyValueInterner;

/// Name of the synthetic attribute holding pre-computed key=value hashes.
const KV_HASH_ATTR: &str = "_nd_kv_hash";

/// A pre-downcast UTF-8 string column.
///
/// Wraps the three possible encodings that pdata may produce for a Utf8 column.
/// Created once via [`DictUtf8::try_from`], then used for fast per-row access.
pub enum DictUtf8<'a> {
    /// Dictionary with `UInt8` keys — up to 256 distinct values.
    DictU8(&'a DictionaryArray<UInt8Type>),
    /// Dictionary with `UInt16` keys — up to 65,536 distinct values.
    DictU16(&'a DictionaryArray<UInt16Type>),
    /// Plain `StringArray` — no dictionary encoding.
    Plain(&'a StringArray),
}

impl<'a> DictUtf8<'a> {
    /// Downcast a `dyn Array` column into the appropriate variant.
    ///
    /// Returns `None` if the column is not a recognized string type
    /// (shouldn't happen for well-formed OTAP data).
    pub fn try_from(col: &'a dyn Array) -> Option<Self> {
        if let Some(d) = col.as_any().downcast_ref::<DictionaryArray<UInt8Type>>() {
            Some(Self::DictU8(d))
        } else if let Some(d) = col.as_any().downcast_ref::<DictionaryArray<UInt16Type>>() {
            Some(Self::DictU16(d))
        } else if let Some(a) = col.as_any().downcast_ref::<StringArray>() {
            Some(Self::Plain(a))
        } else {
            None
        }
    }

    /// Read the string value at `row`, or `None` if the value is null.
    pub fn value(&self, row: usize) -> Option<&str> {
        match self {
            Self::DictU8(d) => {
                if d.is_null(row) {
                    return None;
                }
                let vals = d.values().as_any().downcast_ref::<StringArray>()?;
                Some(vals.value(d.keys().value(row) as usize))
            }
            Self::DictU16(d) => {
                if d.is_null(row) {
                    return None;
                }
                let vals = d.values().as_any().downcast_ref::<StringArray>()?;
                Some(vals.value(d.keys().value(row) as usize))
            }
            Self::Plain(a) => {
                if a.is_null(row) {
                    return None;
                }
                Some(a.value(row))
            }
        }
    }
}

/// A pre-downcast Int64 column, following the same pattern as [`DictUtf8`].
pub enum DictI64<'a> {
    DictU8(&'a DictionaryArray<UInt8Type>),
    DictU16(&'a DictionaryArray<UInt16Type>),
    Plain(&'a Int64Array),
}

impl<'a> DictI64<'a> {
    pub fn try_from(col: &'a dyn Array) -> Option<Self> {
        if let Some(d) = col.as_any().downcast_ref::<DictionaryArray<UInt8Type>>() {
            Some(Self::DictU8(d))
        } else if let Some(d) = col.as_any().downcast_ref::<DictionaryArray<UInt16Type>>() {
            Some(Self::DictU16(d))
        } else if let Some(a) = col.as_any().downcast_ref::<Int64Array>() {
            Some(Self::Plain(a))
        } else {
            None
        }
    }

    pub fn value(&self, row: usize) -> Option<i64> {
        match self {
            Self::DictU8(d) => {
                if d.is_null(row) {
                    return None;
                }
                let vals = d.values().as_any().downcast_ref::<Int64Array>()?;
                Some(vals.value(d.keys().value(row) as usize))
            }
            Self::DictU16(d) => {
                if d.is_null(row) {
                    return None;
                }
                let vals = d.values().as_any().downcast_ref::<Int64Array>()?;
                Some(vals.value(d.keys().value(row) as usize))
            }
            Self::Plain(a) => {
                if a.is_null(row) {
                    return None;
                }
                Some(a.value(row))
            }
        }
    }
}

/// A pre-downcast Binary column, following the same pattern as [`DictUtf8`].
pub enum DictBinary<'a> {
    DictU8(&'a DictionaryArray<UInt8Type>),
    DictU16(&'a DictionaryArray<UInt16Type>),
    Plain(&'a BinaryArray),
}

impl<'a> DictBinary<'a> {
    pub fn try_from(col: &'a dyn Array) -> Option<Self> {
        if let Some(d) = col.as_any().downcast_ref::<DictionaryArray<UInt8Type>>() {
            Some(Self::DictU8(d))
        } else if let Some(d) = col.as_any().downcast_ref::<DictionaryArray<UInt16Type>>() {
            Some(Self::DictU16(d))
        } else if let Some(a) = col.as_any().downcast_ref::<BinaryArray>() {
            Some(Self::Plain(a))
        } else {
            None
        }
    }

    pub fn value(&self, row: usize) -> Option<&[u8]> {
        match self {
            Self::DictU8(d) => {
                if d.is_null(row) {
                    return None;
                }
                let vals = d.values().as_any().downcast_ref::<BinaryArray>()?;
                Some(vals.value(d.keys().value(row) as usize))
            }
            Self::DictU16(d) => {
                if d.is_null(row) {
                    return None;
                }
                let vals = d.values().as_any().downcast_ref::<BinaryArray>()?;
                Some(vals.value(d.keys().value(row) as usize))
            }
            Self::Plain(a) => {
                if a.is_null(row) {
                    return None;
                }
                Some(a.value(row))
            }
        }
    }
}

/// Pre-downcast columns for an OTAP attribute RecordBatch.
///
/// All three attribute batch types (ResourceAttrs, ScopeAttrs, LogAttrs) share
/// the same column schema. This struct downcasts all columns once at construction
/// time, avoiding repeated downcasts during row iteration.
pub struct AttrsColumns<'a> {
    pub num_rows: usize,
    pub parent_id: &'a UInt16Array,
    pub key: DictUtf8<'a>,
    pub type_col: Option<&'a UInt8Array>,
    pub str_col: Option<DictUtf8<'a>>,
    pub int_col: Option<DictI64<'a>>,
    pub double_col: Option<&'a Float64Array>,
    pub bool_col: Option<&'a BooleanArray>,
    pub bytes_col: Option<DictBinary<'a>>,
}

impl<'a> AttrsColumns<'a> {
    pub fn try_from(rb: &'a RecordBatch) -> Option<Self> {
        let parent_id = rb
            .column_by_name("parent_id")?
            .as_any()
            .downcast_ref::<UInt16Array>()?;
        let key_col = rb.column_by_name("key")?;
        let key = DictUtf8::try_from(key_col.as_ref())?;

        let type_col = rb
            .column_by_name("type")
            .and_then(|c| c.as_any().downcast_ref::<UInt8Array>());
        let str_col = rb
            .column_by_name("str")
            .and_then(|c| DictUtf8::try_from(c.as_ref()));
        let int_col = rb
            .column_by_name("int")
            .and_then(|c| DictI64::try_from(c.as_ref()));
        let double_col = rb
            .column_by_name("double")
            .and_then(|c| c.as_any().downcast_ref::<Float64Array>());
        let bool_col = rb
            .column_by_name("bool")
            .and_then(|c| c.as_any().downcast_ref::<BooleanArray>());
        let bytes_col = rb
            .column_by_name("bytes")
            .and_then(|c| DictBinary::try_from(c.as_ref()));

        Some(Self {
            num_rows: rb.num_rows(),
            parent_id,
            key,
            type_col,
            str_col,
            int_col,
            double_col,
            bool_col,
            bytes_col,
        })
    }

    /// Format the attribute value at `row` and append it to `buf`.
    ///
    /// The formatting matches the producer-side hash computation in
    /// `hash_value_display` so that `xxhash64("key=<formatted_value>")`
    /// produces the same hash as the pre-computed `_nd_kv_hash` entry.
    pub fn append_value(&self, row: usize, buf: &mut String) {
        let type_val = self
            .type_col
            .and_then(|c| {
                if c.is_null(row) {
                    None
                } else {
                    Some(c.value(row))
                }
            })
            .unwrap_or(0);

        match type_val {
            1 => {
                if let Some(s) = self.str_col.as_ref().and_then(|c| c.value(row)) {
                    buf.push_str(s);
                }
            }
            2 => {
                if let Some(v) = self.int_col.as_ref().and_then(|c| c.value(row)) {
                    buf.push_str(&v.to_string());
                }
            }
            3 => {
                if let Some(c) = self.double_col {
                    if !c.is_null(row) {
                        buf.push_str(&c.value(row).to_string());
                    }
                }
            }
            4 => {
                if let Some(c) = self.bool_col {
                    if !c.is_null(row) {
                        buf.push_str(if c.value(row) { "true" } else { "false" });
                    }
                }
            }
            7 => {
                if let Some(bytes) = self.bytes_col.as_ref().and_then(|c| c.value(row)) {
                    const HEX: &[u8; 16] = b"0123456789abcdef";
                    for &byte in bytes {
                        buf.push(HEX[(byte >> 4) as usize] as char);
                        buf.push(HEX[(byte & 0xf) as usize] as char);
                    }
                }
            }
            _ => {}
        }
    }
}

/// Maps `parent_id` → list of interned `key=value` pair IDs for a single
/// frame's attribute batch.
#[derive(Default)]
pub struct AttrsMap {
    map: HashMap<u16, Vec<KeyValueId>>,
}

impl AttrsMap {
    /// Build an `AttrsMap` from an attribute RecordBatch.
    ///
    /// For each contiguous `parent_id` group, checks whether the last row
    /// is `_nd_kv_hash`. If so, uses the pre-computed hashes for hash-only
    /// interner lookups (fast path). On cache miss (first time seeing this
    /// `key=value`), falls back to formatting the string and interning it
    /// with the pre-computed hash so subsequent lookups hit.
    pub fn build(rb: Option<&RecordBatch>, interner: &mut KeyValueInterner) -> Self {
        let Some(rb) = rb else {
            return AttrsMap::default();
        };

        let Some(cols) = AttrsColumns::try_from(rb) else {
            return AttrsMap::default();
        };

        let mut map: HashMap<u16, Vec<KeyValueId>> = HashMap::new();
        let mut buf = String::new();

        let mut group_start = 0;
        while group_start < cols.num_rows {
            if cols.parent_id.is_null(group_start) {
                group_start += 1;
                continue;
            }

            let pid = cols.parent_id.value(group_start);

            let mut group_end = group_start + 1;
            while group_end < cols.num_rows
                && !cols.parent_id.is_null(group_end)
                && cols.parent_id.value(group_end) == pid
            {
                group_end += 1;
            }

            let last = group_end - 1;
            let hash_bytes = if cols.key.value(last) == Some(KV_HASH_ATTR) {
                cols.bytes_col.as_ref().and_then(|c| c.value(last))
            } else {
                None
            };

            let attr_end = if hash_bytes.is_some() {
                last
            } else {
                group_end
            };

            for row in group_start..attr_end {
                let hash = hash_bytes.and_then(|hb| {
                    let idx = row - group_start;
                    let off = idx * 8;

                    hb.get(off..off + 8)
                        .map(|b| u64::from_le_bytes(b.try_into().unwrap()))
                });

                let id = hash
                    .and_then(|h| interner.lookup_hash(h))
                    .unwrap_or_else(|| {
                        buf.clear();

                        if let Some(k) = cols.key.value(row) {
                            buf.push_str(k);
                        }
                        buf.push('=');
                        cols.append_value(row, &mut buf);

                        match hash {
                            Some(h) => interner.intern_with_hash(h, &buf),
                            None => interner.intern(&buf),
                        }
                    });

                map.entry(pid).or_default().push(id);
            }

            group_start = group_end;
        }

        Self { map }
    }

    /// Get the interned attribute IDs for a given `parent_id`.
    pub fn get(&self, parent_id: u16) -> &[KeyValueId] {
        self.map
            .get(&parent_id)
            .map(|v| v.as_slice())
            .unwrap_or(&[])
    }
}
