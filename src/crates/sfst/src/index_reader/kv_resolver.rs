//! KvId → `key=value` resolution by position lookup — the string seam of
//! the trace session, logs rows ([`IndexReader::materialize_rows`]) and
//! the rollup ref resolver.
//!
//! KvIds are assigned per field in field-table order (tier by tier, fields
//! name-sorted, values in dictionary order), so a running cardinality sum
//! locates an id's field and its offset within the field. Every dictionary
//! FST maps a key to its sorted position (`PrefixMap::build`), so the key
//! at a position is a reverse lookup in O(key length) instead of a walk:
//!
//! - low tier: the primary FST holds every low-card field. A field's keys
//!   are contiguous there (they share the `field=` prefix), but the fields
//!   are NOT in field-table order — one name dot-extending another sorts
//!   `a.b=` before `a=` ('.' < '=') — so each field's base position is
//!   looked up, never summed;
//! - mid tier: the field's own FST, decoded without its position bitmaps;
//! - high tier: the field's sorted columnar chunk, by index.
//!
//! One resolver per file, owned by its caller: each chunk decodes on first
//! use and stays until the resolver is dropped, so a caller with a bounded
//! loaded layer (the trace session) frees it with that layer. An id no
//! dictionary covers (past the file's ids, or a corrupt cardinality) is
//! unresolved, never an error; a chunk that fails to decode is an error
//! and is not cached.

use std::collections::HashMap;

use crate::index_reader::{IndexReader, field_table_tiered};
use crate::schema::{FieldTier, HighField};

/// One field's dictionary, decoded on first use.
enum Dict {
    Unloaded,
    /// Low tier: the primary-FST position of the field's first key; `None`
    /// when the file holds no `field=` key (corrupt).
    Low(Option<u64>),
    Mid(fst::Map<Vec<u8>>),
    High(HighField),
}

/// A field of the file's table: tier, tier-relative chunk index, and its
/// dictionary.
struct Field {
    tier: FieldTier,
    chunk: u16,
    dict: Dict,
}

/// See the module docs. Always used with the same file's reader.
#[derive(Default)]
pub(crate) struct KvResolver {
    /// Parallel to the file's field table; empty until first use.
    fields: Vec<Field>,
    /// The first KvId of each field, then the file's id total.
    starts: Vec<u32>,
    /// Chunk decodes (mid and high), for tests that pin the cost.
    #[cfg(test)]
    pub(crate) loads: usize,
    /// Chunk decodes that failed, for the same tests.
    #[cfg(test)]
    pub(crate) failed_loads: usize,
}

impl KvResolver {
    pub(crate) fn new() -> Self {
        Self::default()
    }

    /// Whether anything is decoded (a trace session's loaded layer).
    pub(crate) fn is_loaded(&self) -> bool {
        !self.starts.is_empty()
    }

    /// Write `id`'s `key=value` bytes into `key` (cleared first).
    /// `Ok(false)`, with `key` empty, when no dictionary covers `id`.
    pub(crate) fn key_into(
        &mut self,
        reader: &IndexReader<'_>,
        id: u32,
        key: &mut Vec<u8>,
    ) -> Result<bool, crate::Error> {
        key.clear();
        let found = self.lookup(reader, id, key)?;
        if !found {
            // The low-tier guard rejects a key it has already written.
            key.clear();
        }
        Ok(found)
    }

    fn lookup(
        &mut self,
        reader: &IndexReader<'_>,
        id: u32,
        key: &mut Vec<u8>,
    ) -> Result<bool, crate::Error> {
        if self.starts.is_empty() {
            self.init(reader);
        }
        let total = *self.starts.last().expect("init pushes the total");
        if id >= total {
            return Ok(false);
        }
        // The last field starting at or before `id`: a field of zero
        // cardinality shares its start with the next and is skipped.
        let i = self.starts.partition_point(|&start| start <= id) - 1;
        let offset = id - self.starts[i];
        let field = &mut self.fields[i];
        match field.tier {
            FieldTier::Low => {
                let name = reader.field_table()[i].name.as_bytes();
                let base = match &field.dict {
                    Dict::Low(base) => *base,
                    _ => {
                        let mut prefix = Vec::with_capacity(name.len() + 1);
                        prefix.extend_from_slice(name);
                        prefix.push(b'=');
                        let base = reader.primary.first_index_with_prefix(&prefix);
                        field.dict = Dict::Low(base);
                        base
                    }
                };
                // A corrupt cardinality may point past the field's keys,
                // into the next field's: only a `field=` key is this id's.
                Ok(
                    base.is_some_and(|base| reader.primary.key_at(base + u64::from(offset), key))
                        && key.len() > name.len()
                        && key.starts_with(name)
                        && key[name.len()] == b'=',
                )
            }
            FieldTier::Mid => {
                if matches!(field.dict, Dict::Unloaded) {
                    let decoded = reader.sfst.mid_field_fst(field.chunk);
                    #[cfg(test)]
                    {
                        if decoded.is_ok() {
                            self.loads += 1;
                        } else {
                            self.failed_loads += 1;
                        }
                    }
                    field.dict = Dict::Mid(decoded?);
                }
                let Dict::Mid(map) = &field.dict else {
                    unreachable!("loaded above");
                };
                Ok(crate::prefix_map::key_at(map, u64::from(offset), key))
            }
            FieldTier::High => {
                if matches!(field.dict, Dict::Unloaded) {
                    let decoded = reader.sfst.high_field(field.chunk);
                    #[cfg(test)]
                    {
                        if decoded.is_ok() {
                            self.loads += 1;
                        } else {
                            self.failed_loads += 1;
                        }
                    }
                    field.dict = Dict::High(decoded?);
                }
                let Dict::High(chunk) = &field.dict else {
                    unreachable!("loaded above");
                };
                let offset = offset as usize;
                if offset < chunk.len() {
                    key.extend_from_slice(chunk.key(offset));
                    Ok(true)
                } else {
                    Ok(false)
                }
            }
        }
    }

    /// `ids` resolved to strings (lossy UTF-8); unresolved ids are left out.
    pub(crate) fn strings(
        &mut self,
        reader: &IndexReader<'_>,
        ids: &[u32],
    ) -> Result<HashMap<u32, String>, crate::Error> {
        let mut out = HashMap::with_capacity(ids.len());
        let mut key = Vec::new();
        for &id in ids {
            if self.key_into(reader, id, &mut key)? {
                out.insert(id, String::from_utf8_lossy(&key).into_owned());
            }
        }
        Ok(out)
    }

    fn init(&mut self, reader: &IndexReader<'_>) {
        let mut start = 0u32;
        for (entry, chunk) in field_table_tiered(reader.field_table()) {
            self.starts.push(start);
            self.fields.push(Field {
                tier: entry.tier,
                chunk,
                dict: Dict::Unloaded,
            });
            start = start.saturating_add(entry.cardinality);
        }
        self.starts.push(start);
    }
}

#[cfg(test)]
mod tests;
