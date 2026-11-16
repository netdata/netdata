//! Unit tests for the logs step-2 page pieces that are testable at the
//! value level (the parent `page` module): the `PageShard` merge/fold
//! (reduce), `finalize_page` (root), and the `beyond_boundary`
//! early-termination check. Fixtures are plain `Cursor` values — one
//! `file_seq`/`part` throughout, so timestamp order alone decides cursor
//! order; no WAL/SFST bytes are read.
//!
//! Pins:
//!
//! - `PageShard::merge` pools candidates, re-orders them
//!   closest-to-anchor first, truncates to the nearest `bound`, and ORs
//!   `has_opposite`; `finalize_page` takes the nearest `limit` as the
//!   page (newest-first in both directions) and derives the has-more
//!   flags: one candidate beyond the page -> more rows in `direction`,
//!   `has_opposite` -> more on the other side.
//! - `merge_into` matches `merge`: folding shards one at a time
//!   (pairwise and across three) equals the all-at-once merge, in both
//!   directions, bounded or not — what lets `paginate` fold sources
//!   sequentially instead of pooling them.
//! - `beyond_boundary` skips a candidate file only when its entire
//!   second-granular range lies strictly beyond the page boundary
//!   (backward: strictly older; forward: strictly newer) — the
//!   second→nanosecond gap can never skip a file that could contribute.
//!
//! Not pinned here: `PageShard::evaluate` (needs a real SFST reader),
//! the anchor split in `from_cursors` (value-testable, untested),
//! `materialize`, and `paginate` itself (WAL-before-SFST seeding, early
//! termination over real files). Whole queries over real files run in
//! `tests/ng_wal_equivalence.rs`, which does not page from an anchor.
use super::*;
// Explicit (not via the `use super::*` glob) so the tests don't depend on
// `page.rs` happening to import these.
use crate::logs::cursor::{NS_PER_S, Part};

/// Fixed-identity cursor (`file_seq` 1, `Part::Indexed(0)`, `position =
/// ts as u32`): within these fixtures only the timestamp distinguishes
/// cursors.
fn cursor_at(ts: i64) -> Cursor {
    Cursor {
        timestamp_ns: ts,
        file_seq: 1,
        part: Part::Indexed(0),
        position: ts as u32,
    }
}

/// Timestamp projection the assertions compare against expected orders.
fn timestamps(cursors: &[Cursor]) -> Vec<i64> {
    cursors.iter().map(|c| c.timestamp_ns).collect()
}

#[test]
fn page_merge_backward_keeps_nearest_and_finalize_flags_more() {
    // Backward path pinned end-to-end: `merge` keeps the candidates
    // nearest the anchor (newest first), truncated to `bound`, and ORs
    // `has_opposite`; `finalize_page` takes the nearest `limit` as the
    // page, reading the extra candidate as `has_older` and `has_opposite`
    // as `has_newer`. `bound = limit + 1` mirrors the `page_bound`
    // `paginate` itself passes.
    let a = PageShard {
        cursors: vec![cursor_at(50), cursor_at(20)],
        has_opposite: false,
    };
    let b = PageShard {
        cursors: vec![cursor_at(40), cursor_at(30), cursor_at(10)],
        has_opposite: true,
    };

    let merged = PageShard::merge(vec![a, b], Direction::Backward, Some(3));
    assert_eq!(timestamps(&merged.cursors), vec![50, 40, 30]);
    assert!(merged.has_opposite);

    let selected = finalize_page(merged, Direction::Backward, 2);
    // Page is newest-first; backward is already in that order.
    assert_eq!(timestamps(&selected.cursors), vec![50, 40]);
    assert!(
        selected.has_older,
        "a 3rd candidate (30) lies beyond the page"
    );
    assert!(
        selected.has_newer,
        "has_opposite -> rows newer than the anchor"
    );
}

#[test]
fn page_merge_into_matches_merge_pairwise() {
    // Pins the equivalence `merge_into`'s contract claims: folding one
    // shard into another must equal the all-at-once merge of the same
    // two (same per-step re-order + bound), in both directions and
    // bounded or not.
    let shards = || {
        (
            PageShard {
                cursors: vec![cursor_at(50), cursor_at(20)],
                has_opposite: false,
            },
            PageShard {
                cursors: vec![cursor_at(40), cursor_at(30), cursor_at(10)],
                has_opposite: true,
            },
        )
    };

    for direction in [Direction::Backward, Direction::Forward] {
        for bound in [Some(3), None] {
            let (mut folded, b) = shards();
            folded.merge_into(b, direction, bound);

            let (a2, b2) = shards();
            let merged = PageShard::merge(vec![a2, b2], direction, bound);

            let ctx = format!("{direction:?} bound={bound:?}");
            assert_eq!(
                timestamps(&folded.cursors),
                timestamps(&merged.cursors),
                "{ctx}"
            );
            assert_eq!(folded.has_opposite, merged.has_opposite, "{ctx}");
        }
    }
}

#[test]
fn page_merge_into_multi_fold_matches_merge() {
    // `paginate` folds every source's shard through `merge_into`, bounded
    // at each step; that must equal the all-at-once merge of all shards.
    // The per-step bound can't change the outcome: a candidate truncated
    // at one fold is already beaten by `bound` nearer candidates the
    // final merge keeps ahead of it.
    let shards = || {
        [
            PageShard {
                cursors: vec![cursor_at(50), cursor_at(15)],
                has_opposite: false,
            },
            PageShard {
                cursors: vec![cursor_at(40), cursor_at(30)],
                has_opposite: true,
            },
            PageShard {
                cursors: vec![cursor_at(45), cursor_at(20), cursor_at(10)],
                has_opposite: false,
            },
        ]
    };

    for direction in [Direction::Backward, Direction::Forward] {
        let bound = Some(3);
        let mut folded = PageShard::default();
        for shard in shards() {
            folded.merge_into(shard, direction, bound);
        }
        let merged = PageShard::merge(Vec::from(shards()), direction, bound);

        assert_eq!(
            timestamps(&folded.cursors),
            timestamps(&merged.cursors),
            "{direction:?}"
        );
        assert_eq!(folded.has_opposite, merged.has_opposite, "{direction:?}");
    }
}

#[test]
fn page_merge_forward_orders_oldest_first_and_outputs_newest_first() {
    // Forward mirror of the backward test: closest-to-anchor is the
    // smallest (oldest) cursor, so `merge`'s output is oldest-first and
    // `finalize_page` reverses the page to newest-first, swapping the
    // flag sides (extra candidate -> `has_newer`, `has_opposite` ->
    // `has_older`).
    let a = PageShard {
        cursors: vec![cursor_at(50), cursor_at(20)],
        has_opposite: true,
    };
    let b = PageShard {
        cursors: vec![cursor_at(10), cursor_at(30), cursor_at(40)],
        has_opposite: false,
    };

    let merged = PageShard::merge(vec![a, b], Direction::Forward, Some(3));
    assert_eq!(timestamps(&merged.cursors), vec![10, 20, 30]);
    assert!(merged.has_opposite);

    let selected = finalize_page(merged, Direction::Forward, 2);
    // Nearest 2 are [10, 20] (oldest-first), reversed to newest-first.
    assert_eq!(timestamps(&selected.cursors), vec![20, 10]);
    assert!(
        selected.has_newer,
        "a 3rd candidate (30) lies beyond the page"
    );
    assert!(
        selected.has_older,
        "has_opposite -> rows older than the anchor"
    );
}

#[test]
fn beyond_boundary_backward_skips_strictly_older_files() {
    // Boundary at t = 100s (only its `timestamp_ns` is consulted).
    // Backward looks for cursors *newer* than the boundary, so a
    // second-granular file range is skippable only if its whole range is
    // older — even the newest possible cursor fails to beat it.
    let boundary = cursor_at(100 * NS_PER_S);
    // Ends at 99s → newest possible cursor < 100s → can't beat → skip.
    assert!(beyond_boundary(Direction::Backward, boundary, 0, 99));
    // Ends at 100s → a row inside second 100 can still sit at/past the
    // boundary → keep.
    assert!(!beyond_boundary(Direction::Backward, boundary, 0, 100));
    // Ends at 101s → clearly overlaps → keep.
    assert!(!beyond_boundary(Direction::Backward, boundary, 0, 101));
}

#[test]
fn beyond_boundary_forward_skips_strictly_newer_files() {
    // Boundary at t = 100s. Forward looks for cursors *older* than the
    // boundary, so a second-granular file range is skippable only if its
    // whole range is newer — even the oldest possible cursor must be
    // strictly past it.
    let boundary = cursor_at(100 * NS_PER_S);
    // Starts at 101s → oldest possible cursor > 100s → can't beat → skip.
    assert!(beyond_boundary(Direction::Forward, boundary, 101, u32::MAX));
    // Starts at 100s → its oldest row could sit exactly at the boundary
    // (not strictly newer) → keep.
    assert!(!beyond_boundary(
        Direction::Forward,
        boundary,
        100,
        u32::MAX
    ));
    // Starts at 99s → clearly overlaps → keep.
    assert!(!beyond_boundary(Direction::Forward, boundary, 99, u32::MAX));
}
