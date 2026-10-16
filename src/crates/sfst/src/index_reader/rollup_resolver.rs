//! The per-file rollup ref resolver — the string seam behind the sfsq
//! trace-level gate and the search rows path (which also resolves the
//! `TRSS` per-service refs). The gate tests a rollup row's root refs
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
//! post-assembly truth cannot diverge on rendering. Each ref resolves
//! through the file's [`KvResolver`] (a position lookup in the field's
//! dictionary) and is memoized per offset — the gate tests only the few
//! refs recorded roots carry, never the whole dictionary.

use std::collections::HashMap;
use std::collections::hash_map::Entry;

use crate::index_reader::{IndexReader, KvResolver};
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

/// One target field's state, located on first use.
enum FieldState {
    /// The field's KvId range and a per-offset memo of resolved value
    /// halves; a memoized `None` is a proven-corrupt offset (gap or
    /// shapeless key), not an unprobed one.
    Range {
        start: u32,
        cardinality: u32,
        memo: HashMap<u32, Option<String>>,
    },
    /// The field is absent from this file's table, or its chunk failed
    /// to decode: no ref into it can ever be proven.
    Unresolvable,
}

/// The stateful per-file resolver. Open one per file per query; ask it
/// about many refs, always with the same file's reader (it takes the
/// reader per call, so a caller can hold the resolver while it lends the
/// reader elsewhere). Holds at most the two root fields' dictionaries
/// (`name`, the resource `service.name` spelling) and their memos.
#[derive(Default)]
pub struct RollupRefResolver {
    kv: KvResolver,
    fields: HashMap<String, FieldState>,
}

impl RollupRefResolver {
    /// An empty resolver; a field's dictionary decodes on first use.
    pub fn new() -> Self {
        Self::default()
    }

    /// Resolve `kv_ref` as a value of `field_name` in `reader`'s file.
    /// `ROLLUP_NO_REF` is the caller's absence arm and must not be passed
    /// here.
    pub fn resolve(
        &mut self,
        reader: &IndexReader<'_>,
        field_name: &str,
        kv_ref: u32,
    ) -> RollupRefOutcome<'_> {
        debug_assert_ne!(kv_ref, ROLLUP_NO_REF, "NO_REF is absence, not a ref");
        if !self.fields.contains_key(field_name) {
            let state = locate_field(reader, field_name);
            self.fields.insert(field_name.to_string(), state);
        }
        // Probe and memoize first; the answer borrows the memo afterwards.
        let probe = match self
            .fields
            .get_mut(field_name)
            .expect("inserted just above")
        {
            FieldState::Unresolvable => return RollupRefOutcome::Corrupt,
            FieldState::Range {
                start,
                cardinality,
                memo,
            } => {
                // A ref in another field's range: chunk validation checks
                // only `< kv_total`, so corruption can point across fields.
                let Some(off) = in_field_offset(kv_ref, *start, *cardinality) else {
                    return RollupRefOutcome::Corrupt;
                };
                match memo.entry(off) {
                    Entry::Occupied(_) => Ok(off),
                    Entry::Vacant(slot) => {
                        let mut key = Vec::new();
                        self.kv.key_into(reader, kv_ref, &mut key).map(|found| {
                            // `false` is a gap: no key at the ref's position.
                            slot.insert(
                                found
                                    .then(|| value_half(&String::from_utf8_lossy(&key), field_name))
                                    .flatten(),
                            );
                            off
                        })
                    }
                }
            }
        };
        let off = match probe {
            Ok(off) => off,
            Err(e) => {
                tracing::warn!(
                    "sfst rollup resolver: field {field_name} chunk failed to decode: {e}"
                );
                self.fields
                    .insert(field_name.to_string(), FieldState::Unresolvable);
                return RollupRefOutcome::Corrupt;
            }
        };
        match self.fields.get(field_name) {
            Some(FieldState::Range { memo, .. }) => match memo.get(&off) {
                Some(Some(v)) => RollupRefOutcome::Value(v),
                _ => RollupRefOutcome::Corrupt, // gap or shapeless entry
            },
            _ => unreachable!("a probed field stays a range"),
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

/// Locate `field_name`'s KvId range: KvIds are assigned in field-table
/// order, so a running cardinality sum yields each field's
/// `[start, start + cardinality)` (the KvId resolver locates ids the same
/// way).
fn locate_field(reader: &IndexReader<'_>, field_name: &str) -> FieldState {
    let mut start = 0u32;
    for field in reader.field_table().iter() {
        if field.name == field_name {
            return FieldState::Range {
                start,
                cardinality: field.cardinality,
                memo: HashMap::new(),
            };
        }
        start = start.saturating_add(field.cardinality);
    }
    FieldState::Unresolvable // field absent: no ref into it is provable
}
