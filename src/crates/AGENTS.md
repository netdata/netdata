# Rust Workspace Contributor Guide

This file is for contributors and AI assistants working under `src/crates/`.
Goal: write code that matches the existing Rust style in this workspace and passes owner-team review with minimal friction.

## Scope

- Applies to all crates under `src/crates/`.
- Local crate rules (if any) take precedence over this file.

## Baseline Conventions

- Use workspace settings from `src/crates/Cargo.toml`:
- `edition = "2024"`, `rust-version = "1.85"`.
- Prefer `workspace = true` dependencies in crate `Cargo.toml` files.
- Respect workspace lint settings (`[workspace.lints.clippy]`).

## Coding Style Rules

- Follow existing naming and module layout in the target crate before adding new code.
- Keep modules small and focused; avoid large mixed-responsibility files.
- Prefer typed errors in libraries (`thiserror` + crate-local `Result` alias) and avoid stringly errors.
- Use `tracing` for runtime logging; avoid ad-hoc `println!` logging in library code.
- Use comments to explain why, not what.
- Avoid panics in library paths; return typed errors instead.
- Keep public APIs minimal and explicit.

## Async and Runtime Patterns

- Match the runtime model already used by the target crate (`tokio`, sync primitives, background tasks).
- For async plugin binaries, follow existing startup/error-handling patterns (setup, tracing init, run loop, graceful failure paths).

## Safety and Performance

- For mmap/journal file code paths, preserve safety checks and signal-handling patterns already present in workspace crates.
- Avoid unnecessary allocations in hot paths.
- Keep field/record transformation deterministic.

## Tests

- Add unit tests close to the modified module when possible.
- Add integration tests in `tests/` when behavior spans modules/crates.
- For bug fixes, add a regression test when practical.

## Required Validation Before PR

Run from `src/crates/`:

```bash
cargo fmt --all
cargo check --workspace
cargo test --workspace
```

When touching a specific crate, also run targeted checks:

```bash
cargo test -p <crate_name>
```

Use stricter lint runs when the crate already enforces them in CI.

## Review Checklist (Mandatory)

- Code style matches neighboring files in the same crate.
- Error handling, logging, and tests follow crate-local patterns.
- No new conventions introduced without documented reason.
- Formatting and tests executed for the touched scope.
