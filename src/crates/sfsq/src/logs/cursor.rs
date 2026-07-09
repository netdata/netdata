//! Opaque pagination cursor for the log-row table.
//!
//! Encodes the global total order over log rows — `(timestamp_ns,
//! file_seq, part, position)` — as a compact colon-delimited string.
//! It rides in the response's hidden cursor column and the consumer
//! echoes it back verbatim as the `anchor` request param. The `:`
//! separators keep it non-numeric, which the consuming UI relies on: it
//! NaN-guards a numeric parse of the pagination column, so a purely
//! numeric anchor would instead coerce to a wrong value.

/// Nanoseconds per second. The cursor orders by `timestamp_ns` (nanoseconds);
/// code holding second-granular values (e.g. `beyond_boundary` comparing SFST
/// summary bounds) multiplies by it to convert seconds → nanoseconds.
pub(super) const NS_PER_S: i64 = 1_000_000_000;

/// The sub-source discriminator within one `file_seq` — the cursor's
/// third sort key.
///
/// A `file_seq` is either a single sealed SFST or one active WAL (with ≥0
/// in-memory chunks plus a row-scanned tail). [`Part::Indexed`] covers the
/// indexed sources — a sealed SFST (index `0`) or an in-memory chunk (its
/// 0-based index), both evaluated through the SFST engine; [`Part::Tail`]
/// is the active WAL's tail, evaluated by a row scan.
///
/// Sealed-vs-chunk is deliberately *not* modelled: a sealed file and an
/// active WAL's chunk 0 both encode to the wire integer `0`, and the two
/// never coexist under one `file_seq`, so a decoded cursor cannot — and
/// need not — tell them apart. The only runtime distinction is
/// tail-vs-indexed, which routes a cursor to the WAL row scanner vs an
/// SFST reader.
///
/// Variant order is load-bearing: `Indexed` is declared before `Tail`, so
/// the derived `Ord` gives `Indexed(0) < … < Indexed(n) < Tail`, which
/// reproduces the wire order `0 < … < u32::MAX` exactly (the tail sorts
/// after every chunk of the same `file_seq`). Keep `Indexed` first.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum Part {
    /// An indexed source: a sealed SFST (index `0`) or an in-memory WAL
    /// chunk (its 0-based index).
    ///
    /// Invariant: the index is `< u32::MAX` — that value is reserved as
    /// the [`Tail`](Part::Tail) wire sentinel, so `Indexed(u32::MAX)`
    /// would encode to the same wire integer as `Tail` and decode back as
    /// `Tail`. Real indices are bounded by the chunk count (a WAL never
    /// approaches 2^32 chunks); `to_wire`'s `debug_assert!` catches a
    /// violation in tests.
    Indexed(u32),
    /// An active WAL's row-scanned tail. Sorts after every `Indexed(_)` of
    /// the same `file_seq`.
    Tail,
}

impl Part {
    /// Wire sentinel for [`Part::Tail`]; `Indexed` owns the rest of the
    /// `u32` space (`0..u32::MAX`).
    const TAIL_WIRE: u32 = u32::MAX;

    /// The `u32` this part encodes as on the wire (see [`Cursor::encode`]).
    fn to_wire(self) -> u32 {
        match self {
            Part::Indexed(n) => {
                debug_assert!(
                    n != Self::TAIL_WIRE,
                    "chunk index collides with the tail sentinel"
                );
                n
            }
            Part::Tail => Self::TAIL_WIRE,
        }
    }

    /// Inverse of [`to_wire`](Self::to_wire): the sentinel decodes to the
    /// tail, every other value to an indexed source.
    fn from_wire(n: u32) -> Part {
        if n == Self::TAIL_WIRE {
            Part::Tail
        } else {
            Part::Indexed(n)
        }
    }
}

/// A decoded pagination cursor.
///
/// Ordering is lexicographic over `(timestamp_ns, file_seq, part,
/// position)` — the total order the multi-file merge and the exclusive
/// anchor comparison rely on. `file_seq` is the SFST/WAL file's monotonic
/// `seq`, unique within one process instance's local files. Across process
/// instances or machines (a post-restore archive) two files can share a
/// `seq`, so a `file_seq` tie is theoretically possible when their
/// `timestamp_ns` is also equal; that collision is accepted here (the cursor
/// is an opaque wire string the consumer echoes back — widening it to a full
/// identity+seq key is a format break, tracked as a P9 follow-up). `part`
/// distinguishes the sub-sources of one active WAL that share a `seq` (see
/// [`Part`]); it only breaks ties at equal `(timestamp_ns, file_seq)`, exactly
/// as `position` breaks ties within one chunk/file.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub struct Cursor {
    pub timestamp_ns: i64,
    pub file_seq: u64,
    pub part: Part,
    /// Tie-breaker within one `(timestamp_ns, file_seq, part)`: the
    /// chronological row index within an SFST, or the insertion index
    /// within a tail scan. Both are plain `u32` offsets into the source's
    /// rows; the reader that materializes the cursor already knows which
    /// meaning applies, so the two never need to be told apart here.
    pub position: u32,
}

impl Cursor {
    /// Encode as `"{timestamp_ns}:{file_seq}:{part}:{position}"`, where
    /// `part` is its wire discriminator — `0..` for an indexed source,
    /// `u32::MAX` for the tail.
    pub fn encode(&self) -> String {
        format!(
            "{}:{}:{}:{}",
            self.timestamp_ns,
            self.file_seq,
            self.part.to_wire(),
            self.position
        )
    }

    /// Decode the string form. Returns `None` for any malformed input
    /// (wrong field count, non-integer field, trailing garbage) so the
    /// handler can treat a bad anchor as "no anchor" rather than error.
    /// A legacy 3-field cursor (pre-`part`) is therefore treated as no
    /// anchor — a one-time reset to the page edge across the upgrade.
    pub fn decode(s: &str) -> Option<Cursor> {
        let mut parts = s.split(':');
        let timestamp_ns: i64 = parts.next()?.parse().ok()?;
        let file_seq: u64 = parts.next()?.parse().ok()?;
        let part = Part::from_wire(parts.next()?.parse().ok()?);
        let position: u32 = parts.next()?.parse().ok()?;
        if parts.next().is_some() {
            return None;
        }
        Some(Cursor {
            timestamp_ns,
            file_seq,
            part,
            position,
        })
    }

    /// The synthetic cursor that sorts after every real cursor at
    /// `timestamp_ns` — the comparison-only boundary `Anchor::Timestamp`
    /// resolves to, so a backward page shows the newest rows up to that
    /// instant. It is the total-order maximum at that timestamp: `file_seq`
    /// and `position` maxed, and [`Part::Tail`] (which sorts after every
    /// indexed source). The `Part::Tail` here means "the maximum", not that
    /// the anchor points at a tail row — this cursor is only ever compared,
    /// never materialized.
    pub(super) fn synthetic_max(timestamp_ns: i64) -> Cursor {
        Cursor {
            timestamp_ns,
            file_seq: u64::MAX,
            part: Part::Tail,
            position: u32::MAX,
        }
    }
}

#[cfg(test)]
mod tests;
