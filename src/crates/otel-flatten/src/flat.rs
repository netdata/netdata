use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub enum Value {
    Str(String),
    I64(i64),
    U64(u64),
    F64(f64),
    Bool(bool),
    Bytes(Vec<u8>),
    Null,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Entry {
    /// Resolved timestamp in nanoseconds.
    /// Fallback chain: time_unix_nano → observed_time_unix_nano → ingestion_time_ns.
    pub timestamp_ns: u64,
    /// Key-value pairs. The key is an index into `Frame::keys_offsets`.
    pub kv_pairs: Vec<(u32, Value)>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Frame {
    /// Concatenated key strings.
    pub keys_blob: Vec<u8>,
    /// Byte offsets into `keys_blob`. Length is `key_count + 1` (sentinel).
    /// Key `i` is `keys_blob[keys_offsets[i]..keys_offsets[i+1]]`.
    pub keys_offsets: Vec<u32>,
    pub entries: Vec<Entry>,
}

impl Frame {
    /// Resolve a key index to its string.
    pub fn key(&self, idx: u32) -> &str {
        let start = self.keys_offsets[idx as usize] as usize;
        let end = self.keys_offsets[idx as usize + 1] as usize;
        // SAFETY: keys_blob is built from valid UTF-8 strings in flatten_resource_logs.
        unsafe { std::str::from_utf8_unchecked(&self.keys_blob[start..end]) }
    }
}
