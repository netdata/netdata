# Rust cross-crate doc references

Scope: doc comments (`///`, `//!`) and code comments in the workspace's
Rust crates that reference items of *other* crates. Established during
the otel/sfst refactoring round (2026-06-11) after plain-text
identifier pointers went stale within hours of a crate split.

## The rule

- **Prefer a verified intra-doc link** when the referencing crate has a
  normal dependency on the referenced crate: `` [`other_crate::Item`] ``.
  rustdoc resolves it on every `cargo doc` run, so a rename or removal
  breaks the build's doc check instead of silently misleading readers.
- **Name only public identifiers.** A private item of another crate
  MUST NOT be named in docs or comments — it is plain text, no tool
  validates it, and it rots invisibly (the failure that motivated this
  rule: `wal_otap::decode_frame` mentions surviving its privatization,
  and "`RowIndex` in the `sfst` crate" surviving the move to
  `sfst-indexer`). Describe the contract in natural language instead:
  "wal-otap's shared frame decode", "the attribute-column value
  formatting inside `wal-otap`".
- **Public name, no dependency** (the crate cannot link — reverse
  dependency, or dev-dependency-only from `src/`): naming the public
  identifier as plain text is acceptable; include the owning crate
  (e.g. "`KeyValueInterner::lookup_hash` in the `sfst-indexer` crate")
  so the pointer is followable by hand. Verify the path at review time
  — nothing else will.
- **Same-crate private items**: module docs may name them as plain
  text (`` `hash_value_display` below ``), never as intra-doc links
  from public docs — `rustdoc::private_intra_doc_links` warns, and the
  workspace treats doc warnings as findings.

## Validation

- `cargo doc --no-deps -p <crate>` MUST be warning-free; this is what
  makes the linked form load-bearing.
- When refactoring moves or privatizes an item, grep the workspace for
  the item's name in doc/comment text (not just `use` sites) — the
  plain-text mentions are the ones the compiler will not find.
