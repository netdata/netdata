# Rust Crates Area Instructions

The repo-root `AGENTS.md` applies in full; where the two conflict, this more specific file wins.

## Writing User-Facing Files

- When you write or edit user-facing files, write about 80% of the way to ASD-STE100. User-facing files are:
  - under this directory: `README.md`, `metadata.yaml`, comments in shipped config files;
  - the related documentation pages under `docs/`, such as `docs/opentelemetry/` and `docs/npm/network-flows/`.
- This does not apply to code, code comments, developer specs (for example `FORMAT.md`), or your chat replies.
- Do not edit generated `integrations/*.md` files; edit the crate's `metadata.yaml` instead.

## Dependency Advisories

- `.github/workflows/rust-advisories.yml` runs `cargo deny check advisories` on master daily and posts to Slack on
  failure. Run the same check locally from this directory: `cargo deny --locked check advisories`.
- `deny.toml` holds vetted exceptions as `ignore` entries, each with a reason. Add an entry only for an advisory
  that was vetted; do not disable a whole advisory category.
- `cargo update -p <crate>` can also re-point unrelated `Cargo.lock` dependency entries to other already-locked
  versions (crates with multi-version ranges such as `windows-sys`). Review the whole lockfile diff and revert
  those entries; a crate with a single locked version is written by bare name.
