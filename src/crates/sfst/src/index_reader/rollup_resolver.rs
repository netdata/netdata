//! The per-file rollup root-ref resolver — the string seam behind the
//! sfsq trace-level gate. The gate tests a rollup row's root refs
//! ([`TraceRollup::root_service_refs`](crate::TraceRollup::root_service_refs)
//! and [`root_name_refs`](crate::TraceRollup::root_name_refs),
//! file-interner [`KvId`](crate::KvId)s into the root fields'
//! dictionaries) against a compiled matcher; this resolver maps each
//! ref to its value string, decoding each root-field chunk at most once
//! per file — state a cross-crate caller cannot reach through the
//! private tier internals.
//!
//! Resolution is a CLOSED partition, not a case list. Exactly three
//! outcomes exist for a root ref:
//!
//! - [`ROLLUP_NO_REF`](crate::ROLLUP_NO_REF) is proven absence — the
//!   writer's sentinel, handled by callers; it never reaches this
//!   resolver.
//! - A ref inside the target field's own KvId range whose entry's key
//!   literally carries the `"{field_name}="` prefix is a proven
//!   [`Value`](RollupRefOutcome::Value).
//! - Anything else is [`Corrupt`](RollupRefOutcome::Corrupt): a ref in
//!   another field's range (chunk validation checks only `< kv_total`,
//!   so corruption can point across fields), an in-range ref past the
//!   decoded entries (a gap), an entry whose key names another field,
//!   or an undecodable chunk. The file has proven it cannot be
//!   trusted; the caller escalates per the corrupt-file principle
//!   (skip the file as a failed source AND surface the skip), never
//!   treating `Corrupt` as absence or a non-match.
//!
//! Values render through the same lossy-UTF-8 render the canonical row
//! path uses, and the prefix strip is value-identical to
//! [`kv_value`](super::kv_value)'s first-`=` split on every proven key
//! (field names carry no `=` — a flattener invariant — and
//! value-embedded `=` bytes survive both), so the gate and the
//! post-assembly truth cannot diverge on rendering. Low/mid
//! dictionaries decode as one table (their decode is a walk anyway);
//! high-card chunks stay random-access with a per-offset memo — the
//! gate tests only the few refs recorded roots carry, never the
//! dictionary.

use std::collections::HashMap;

use crate::index_reader::{IndexReader, field_table_tiered};
use crate::schema::{FieldTier, HighField};
use crate::trace_rollup::ROLLUP_NO_REF;

/// Outcome of resolving one root ref in one field — the two
/// non-absence arms of the closed partition in the module docs.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RollupRefOutcome<'s> {
    /// The ref's value half, proven to live in the target field.
    Value(&'s str),
    /// The ref cannot be mapped to a proven value in the target field —
    /// corruption evidence for the whole file.
    Corrupt,
}

/// One target field's decoded state, built on first use.
enum FieldState {
    /// Low/mid tier: `values[ref - start]` is the value half, or `None`
    /// for an entry without the `key=value` shape (corrupt).
    Table {
        start: u32,
        cardinality: u32,
        values: Vec<Option<String>>,
    },
    /// High tier: the decoded chunk plus a per-offset memo; a memoized
    /// `None` is a proven-corrupt offset (gap or shapeless key), not an
    /// unprobed one.
    High {
        start: u32,
        cardinality: u32,
        chunk: HighField,
        memo: HashMap<u32, Option<String>>,
    },
    /// The field is absent from this file's table, or its chunk failed
    /// to decode: no ref into it can ever be proven.
    Unresolvable,
}

/// The stateful per-file resolver. Open one per file per query; ask it
/// about many refs. Owns at most the two root-field dictionaries
/// (`name`, the resource `service.name` spelling).
pub struct RollupRootResolver<'r, 'a> {
    reader: &'r IndexReader<'a>,
    fields: HashMap<String, FieldState>,
}

impl<'r, 'a> RollupRootResolver<'r, 'a> {
    /// An empty resolver; a field's dictionary decodes on first use.
    pub fn new(reader: &'r IndexReader<'a>) -> Self {
        Self {
            reader,
            fields: HashMap::new(),
        }
    }

    /// Resolve `kv_ref` as a value of `field_name`. `ROLLUP_NO_REF` is
    /// the caller's absence arm and must not be passed here.
    pub fn resolve(&mut self, field_name: &str, kv_ref: u32) -> RollupRefOutcome<'_> {
        debug_assert_ne!(kv_ref, ROLLUP_NO_REF, "NO_REF is absence, not a ref");
        if !self.fields.contains_key(field_name) {
            let state = decode_field(self.reader, field_name);
            self.fields.insert(field_name.to_string(), state);
        }
        let state = self
            .fields
            .get_mut(field_name)
            .expect("inserted just above");
        match state {
            FieldState::Unresolvable => RollupRefOutcome::Corrupt,
            FieldState::Table {
                start,
                cardinality,
                values,
            } => match in_field_offset(kv_ref, *start, *cardinality) {
                None => RollupRefOutcome::Corrupt, // another field's range
                Some(off) => match values.get(off as usize) {
                    Some(Some(v)) => RollupRefOutcome::Value(v),
                    _ => RollupRefOutcome::Corrupt, // gap or shapeless entry
                },
            },
            FieldState::High {
                start,
                cardinality,
                chunk,
                memo,
            } => match in_field_offset(kv_ref, *start, *cardinality) {
                None => RollupRefOutcome::Corrupt,
                Some(off) => {
                    let value = memo.entry(off).or_insert_with(|| {
                        ((off as usize) < chunk.len())
                            .then(|| String::from_utf8_lossy(chunk.key(off as usize)).into_owned())
                            .and_then(|key| value_half(&key, field_name))
                    });
                    match value {
                        Some(v) => RollupRefOutcome::Value(v),
                        None => RollupRefOutcome::Corrupt, // gap or shapeless entry
                    }
                }
            },
        }
    }
}

/// The ref's offset within `[start, start + cardinality)`, or `None`
/// when it falls in another field's range.
fn in_field_offset(kv_ref: u32, start: u32, cardinality: u32) -> Option<u32> {
    kv_ref.checked_sub(start).filter(|&off| off < cardinality)
}

/// The value half of `key`, PROVEN to belong to `field_name`: the key
/// must literally start with `"{field_name}="` — an entry inside the
/// field's metadata range whose key names another field is a decodable
/// corruption the range check alone cannot catch, and yields `None`
/// (→ [`RollupRefOutcome::Corrupt`]), never an unproven value.
fn value_half(key: &str, field_name: &str) -> Option<String> {
    let rest = key.strip_prefix(field_name)?;
    let value = rest.strip_prefix('=')?;
    Some(value.to_string())
}

/// Locate `field_name`'s KvId range and decode its dictionary once.
/// Mirrors `resolve_kv_strings`' per-field walk: KvIds are assigned in
/// field-table order, so a running cardinality sum yields each field's
/// `[start, start + cardinality)` range.
fn decode_field(reader: &IndexReader<'_>, field_name: &str) -> FieldState {
    let mut start = 0u32;
    for (field, ti) in field_table_tiered(reader.field_table()) {
        if field.name != field_name {
            start += field.cardinality;
            continue;
        }
        let keys: Vec<String> = match field.tier {
            FieldTier::Low => reader
                .primary
                .prefix_pairs(format!("{field_name}=").as_bytes())
                .into_iter()
                .map(|(key, _)| String::from_utf8_lossy(&key).into_owned())
                .collect(),
            FieldTier::Mid => match reader.sfst.mid_field(ti) {
                Ok(fst) => {
                    let mut keys = Vec::new();
                    fst.for_each(|key, _| {
                        keys.push(String::from_utf8_lossy(key).into_owned());
                    });
                    keys
                }
                Err(e) => {
                    tracing::warn!(
                        "sfst rollup resolver: field {field_name} chunk failed to decode: {e}"
                    );
                    return FieldState::Unresolvable;
                }
            },
            FieldTier::High => match reader.sfst.high_field(ti) {
                Ok(chunk) => {
                    return FieldState::High {
                        start,
                        cardinality: field.cardinality,
                        chunk,
                        memo: HashMap::new(),
                    };
                }
                Err(e) => {
                    tracing::warn!(
                        "sfst rollup resolver: field {field_name} chunk failed to decode: {e}"
                    );
                    return FieldState::Unresolvable;
                }
            },
        };
        let values = keys
            .into_iter()
            .map(|key| value_half(&key, field_name))
            .collect();
        return FieldState::Table {
            start,
            cardinality: field.cardinality,
            values,
        };
    }
    FieldState::Unresolvable // field absent: no ref into it is provable
}
