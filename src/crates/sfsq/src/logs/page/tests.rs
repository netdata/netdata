use super::*;
// Explicit (not via the `use super::*` glob) so the tests don't depend on
// `page.rs` happening to import these.
use crate::logs::cursor::{NS_PER_S, Part};

fn cursor_at(ts: i64) -> Cursor {
    Cursor {
        timestamp_ns: ts,
        file_seq: 1,
        part: Part::Indexed(0),
        position: ts as u32,
    }
}

fn timestamps(cursors: &[Cursor]) -> Vec<i64> {
    cursors.iter().map(|c| c.timestamp_ns).collect()
}

#[test]
fn page_merge_backward_keeps_nearest_and_finalize_flags_more() {
    // Backward: closest-to-anchor is the largest (newest) cursor. With
    // limit 2 the bound is 3; merge keeps the nearest 3, finalize takes 2.
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
    // The incremental fold must match the all-at-once merge of the same
    // two shards (same per-step order + bound), in both directions and
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
    // `paginate` folds N sources sequentially; folding three shards
    // left-to-right with `merge_into` must match the all-at-once merge of
    // all three (associative up to the bound).
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
    // Forward: closest-to-anchor is the smallest (oldest) cursor; the page
    // is reversed to newest-first for output, and the flags swap sides.
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
    // Boundary at t = 100s. Backward looks for cursors *newer* than the
    // boundary, so a file is skippable only if its whole range is older.
    let boundary = cursor_at(100 * NS_PER_S);
    // Ends at 99s → newest possible cursor < 100s → can't beat → skip.
    assert!(beyond_boundary(Direction::Backward, boundary, 0, 99));
    // Ends at 100s → could tie within the boundary second → keep.
    assert!(!beyond_boundary(Direction::Backward, boundary, 0, 100));
    // Ends at 101s → clearly overlaps → keep.
    assert!(!beyond_boundary(Direction::Backward, boundary, 0, 101));
}

#[test]
fn beyond_boundary_forward_skips_strictly_newer_files() {
    // Boundary at t = 100s. Forward looks for cursors *older* than the
    // boundary, so a file is skippable only if its whole range is newer.
    let boundary = cursor_at(100 * NS_PER_S);
    // Starts at 101s → oldest possible cursor > 100s → can't beat → skip.
    assert!(beyond_boundary(Direction::Forward, boundary, 101, u32::MAX));
    // Starts at 100s → could tie within the boundary second → keep.
    assert!(!beyond_boundary(
        Direction::Forward,
        boundary,
        100,
        u32::MAX
    ));
    // Starts at 99s → clearly overlaps → keep.
    assert!(!beyond_boundary(Direction::Forward, boundary, 99, u32::MAX));
}
