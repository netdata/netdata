# Rust Crates Area Instructions

The repo-root `AGENTS.md` applies in full; where the two conflict, this more specific file wins.

## Writing User-Facing Files

- When you write or edit user-facing files, write about 80% of the way to ASD-STE100. User-facing files are:
  - under this directory: `README.md`, `metadata.yaml`, comments in shipped config files;
  - the related documentation pages under `docs/`, such as `docs/opentelemetry/` and `docs/npm/network-flows/`.
- This does not apply to code, code comments, developer specs (for example `FORMAT.md`), or your chat replies.
- Do not edit generated `integrations/*.md` files; edit the crate's `metadata.yaml` instead.

## Compact Instructions

When the session works from a status repo (`.local/status` in the worktree), the summary MUST keep:

- the status repo path, the live item and the next action;
- work done but not yet recorded in the status repo;
- every background shell, monitor or agent still running, with its ID and purpose;
- open questions for the user;
- the user's instructions not yet recorded in the status repo's `OPERATIONS.md`, verbatim.
