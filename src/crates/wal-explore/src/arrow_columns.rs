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
//!
//! # Why pre-downcast?
//!
//! Arrow stores all columns as `dyn Array`, requiring a runtime downcast to
//! access typed values. The OTAP encoder (pdata) uses dictionary encoding for
//! string and integer columns, which means a column might be stored as any of:
//!
//! - `DictionaryArray<UInt8Type>`  — up to 256 distinct values
//! - `DictionaryArray<UInt16Type>` — up to 65,536 distinct values
//! - Plain array (e.g., `StringArray`, `Int64Array`) — fallback when the
//!   dictionary overflows
//!
//! pdata starts with `Dict<UInt8, T>` and automatically upgrades to
//! `Dict<UInt16, T>`, then to a plain array if the dictionary grows too large.
//! This means we don't know the concrete column type until we inspect each
//! RecordBatch at runtime.
//!
//! Rather than downcasting on every row access (which adds branching overhead
//! in tight loops), [`DictUtf8`] and [`DictI64`] downcast once when the batch
//! is loaded and store a reference to the concrete typed array. Subsequent
//! row reads are a simple enum match with no further downcasting.
//!
//! # OTAP attribute schema
//!
//! All three attribute batch types (ResourceAttrs, ScopeAttrs, LogAttrs) share
//! the same schema:
//!
//! | Column      | Type                       | Description                         |
//! |-------------|----------------------------|-------------------------------------|
//! | `parent_id` | `UInt16`                   | Links to the parent entity's ID     |
//! | `key`       | `Dict<UInt8/16, Utf8>`     | Attribute key (e.g., "service.name")|
//! | `type`      | `UInt8`                    | Value type discriminator (see below)|
//! | `str`       | `Dict<UInt8/16, Utf8>`     | String value (when type=1)          |
//! | `int`       | `Dict<UInt8/16, Int64>`    | Integer value (when type=2)         |
//! | `double`    | `Float64`                  | Double value (when type=3)          |
//! | `bool`      | `Boolean`                  | Boolean value (when type=4)         |
//! | `bytes`     | `Binary`                   | Byte array value (when type=7)      |
//!
//! The `type` column uses the OTAP `AttributeValueType` enum:
//!
//! | Value | Meaning          |
//! |-------|------------------|
//! | 0     | Empty            |
//! | 1     | String (`str`)   |
//! | 2     | Integer (`int`)  |
//! | 3     | Double (`double`)|
//! | 4     | Boolean (`bool`) |
//! | 5     | Map (CBOR)       |
//! | 6     | Slice (CBOR)     |
//! | 7     | Bytes (`bytes`)  |

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
///
/// # Dictionary encoding in Arrow
///
/// A `DictionaryArray<K>` stores values as a pair of arrays:
/// - **keys**: an array of integer indices (type `K`, e.g., `UInt8` or `UInt16`)
/// - **values**: a dense array of unique values (e.g., `StringArray`)
///
/// To read row `i`: look up `keys[i]` to get the index, then read `values[index]`.
/// This is efficient when many rows share the same value (common for attributes
/// like `service.name`).
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
    ///
    /// For dictionary-encoded columns, this resolves the key to its
    /// corresponding value in the dictionary's values array.
    pub fn value(&self, row: usize) -> Option<&str> {
        match self {
            Self::DictU8(d) => {
                if d.is_null(row) {
                    return None;
                }
                // d.keys() gives us the UInt8 index array; d.values() is the StringArray.
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
///
/// pdata dictionary-encodes integer attribute values the same way as strings.
pub enum DictI64<'a> {
    DictU8(&'a DictionaryArray<UInt8Type>),
    DictU16(&'a DictionaryArray<UInt16Type>),
    Plain(&'a Int64Array),
}

impl<'a> DictI64<'a> {
    /// Downcast a `dyn Array` column into the appropriate variant.
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

    /// Read the i64 value at `row`, or `None` if null.
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
///
/// pdata dictionary-encodes binary attribute values the same way as strings.
pub enum DictBinary<'a> {
    DictU8(&'a DictionaryArray<UInt8Type>),
    DictU16(&'a DictionaryArray<UInt16Type>),
    Plain(&'a BinaryArray),
}

impl<'a> DictBinary<'a> {
    /// Downcast a `dyn Array` column into the appropriate variant.
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

    /// Read the binary value at `row`, or `None` if null.
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
///
/// # Example: reading attributes from a batch
///
/// ```ignore
/// let cols = AttrsColumns::try_from(&record_batch).unwrap();
/// for row in 0..cols.num_rows {
///     let parent_id = cols.parent_id.value(row);
///     let key = cols.key.value(row);
///     // ...
/// }
/// ```
pub struct AttrsColumns<'a> {
    /// Number of rows in the batch.
    pub num_rows: usize,
    /// The `parent_id` column — links each attribute row to its parent entity
    /// (a log, resource, or scope row in the Logs batch).
    pub parent_id: &'a UInt16Array,
    /// The `key` column — the attribute name (e.g., "service.name").
    pub key: DictUtf8<'a>,
    /// The `type` column — discriminates which value column to read.
    pub type_col: Option<&'a UInt8Array>,
    /// The `str` value column (type=1).
    pub str_col: Option<DictUtf8<'a>>,
    /// The `int` value column (type=2).
    pub int_col: Option<DictI64<'a>>,
    /// The `double` value column (type=3).
    pub double_col: Option<&'a Float64Array>,
    /// The `bool` value column (type=4).
    pub bool_col: Option<&'a BooleanArray>,
    /// The `bytes` value column (type=7).
    pub bytes_col: Option<DictBinary<'a>>,
}

impl<'a> AttrsColumns<'a> {
    /// Downcast all columns from an attribute RecordBatch.
    ///
    /// Returns `None` if the batch lacks required columns (`parent_id`, `key`).
    /// Optional value columns (`str`, `int`, `double`, `bool`) are set to
    /// `None` if missing.
    pub fn try_from(rb: &'a RecordBatch) -> Option<Self> {
        // parent_id and key are required — without them we can't build an index.
        let parent_id = rb
            .column_by_name("parent_id")?
            .as_any()
            .downcast_ref::<UInt16Array>()?;
        let key_col = rb.column_by_name("key")?;
        let key = DictUtf8::try_from(key_col.as_ref())?;

        // Value columns are optional — a batch might only contain string
        // attributes, for example, in which case `int`, `double`, `bool`
        // columns will be all-null or absent.
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
    ///
    /// - String  → raw string (no quotes)
    /// - Int     → decimal
    /// - Double  → decimal
    /// - Bool    → `true` / `false`
    /// - Bytes   → lowercase hex
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
///
/// Built from an attribute RecordBatch by iterating all rows, formatting each
/// attribute as `"key=value"`, interning the string, and grouping the
/// resulting IDs by `parent_id`.
///
/// When the batch contains pre-computed `_nd_kv_hash` entries (from the
/// producer), the build exploits the fact that attributes for each
/// `parent_id` are contiguous and `_nd_kv_hash` is always the last row
/// in each group. This allows a single-pass, group-by-group approach:
///
/// 1. Scan the `parent_id` column to find contiguous groups.
/// 2. For each group, check if the last row is `_nd_kv_hash`.
/// 3. If so, extract the pre-computed hashes and use hash-only lookups
///    for the preceding rows (skipping string formatting on cache hits).
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
    ///
    /// The fast path avoids: (1) reading the key/value columns, (2) formatting
    /// the `"key=value"` string, (3) computing xxhash64. It only does a single
    /// HashMap lookup by u64.
    pub fn build(rb: Option<&RecordBatch>, interner: &mut KeyValueInterner) -> Self {
        let Some(rb) = rb else {
            return AttrsMap::default();
        };

        let Some(cols) = AttrsColumns::try_from(rb) else {
            return AttrsMap::default();
        };

        let mut map: HashMap<u16, Vec<KeyValueId>> = HashMap::new();
        let mut buf = String::new();

        // Process each contiguous parent_id group.
        let mut group_start = 0;
        while group_start < cols.num_rows {
            // Skip null parent_id rows.
            if cols.parent_id.is_null(group_start) {
                group_start += 1;
                continue;
            }

            let pid = cols.parent_id.value(group_start);

            // Find the end of this parent_id group.
            let mut group_end = group_start + 1;
            while group_end < cols.num_rows
                && !cols.parent_id.is_null(group_end)
                && cols.parent_id.value(group_end) == pid
            {
                group_end += 1;
            }

            // Check if the last row in the group is _nd_kv_hash.
            let last = group_end - 1;
            let hash_bytes = if cols.key.value(last) == Some(KV_HASH_ATTR) {
                cols.bytes_col.as_ref().and_then(|c| c.value(last))
            } else {
                None
            };

            // Determine the range of attribute rows (excluding _nd_kv_hash).
            let attr_end = if hash_bytes.is_some() {
                last
            } else {
                group_end
            };

            for row in group_start..attr_end {
                // Each attribute row has a corresponding pre-computed hash.
                let hash = hash_bytes.and_then(|hb| {
                    let idx = row - group_start;
                    let off = idx * 8;

                    hb.get(off..off + 8)
                        .map(|b| u64::from_le_bytes(b.try_into().unwrap()))
                });

                // Fast path: hash-only lookup (no string formatting needed).
                let id = hash
                    .and_then(|h| interner.lookup_hash(h))
                    .unwrap_or_else(|| {
                        // Slow path: format "key=value" and intern.
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
    ///
    /// Returns an empty slice if no attributes exist for this parent.
    pub fn get(&self, parent_id: u16) -> &[KeyValueId] {
        self.map
            .get(&parent_id)
            .map(|v| v.as_slice())
            .unwrap_or(&[])
    }

    /// Iterate over all (parent_id, attribute_ids) pairs.
    pub fn iter(&self) -> impl Iterator<Item = (&u16, &Vec<KeyValueId>)> {
        self.map.iter()
    }
}
