TL;DR
- Phase 1 deterministic harness still emits shutdown-related warnings (duplicate persistence dirs, lingering MCP children, env-file permission errors) each time we run `npm run test:phase1`.
- These warnings come from the new shutdown controller refactor and occasionally trip CI; we need a cleanup pass dedicated to the harness so the suite is quiet and reliable again.

Analysis
- `npm run test:phase1` now succeeds, but every run logs `persistSessionSnapshot` EEXIST errors because the shutdown flow leaves `sessions-blocker` directories behind; the harness retries and prints warnings.
- The same run logs repeated `failed to read env file ... EACCES` and `mcp_restart_failed` messages whenever the shutdown controller restarts MCP fixtures—evidence that our env-layer temp dirs and restart paths are not being torn down before the next scenario.
- After the shutdown rework, the harness frequently reports `lingering handles` (a list of ChildProcess PIDs) which means the shared shutdown controller is not fully draining headends between scenarios.
- These issues do not block local development but they make the harness noisy and brittle, and they hide real regressions because every run now produces dozens of warnings.

Decisions
- ✅ Track the shutdown-related harness cleanup work separately so we can stabilize the deterministic suite without mixing it into unrelated feature branches.
- ✅ Treat the repeated warnings as bugs: the harness must exit cleanly with zero lingering handles and zero persistence/env errors.
- 🔄 Need confirmation from Costa on whether we should gate CI on a clean run (no warnings) once this work lands, or if we merely suppress the noisy logs.

Plan
1. Reproduce the warnings with `npm run test:phase1` under DEBUG logging and capture which scenarios leak handles or temp dirs.
2. Audit the shutdown controller + persistence helpers to ensure each scenario removes its session dirs and env layers (fix the EEXIST/EACCES bursts).
3. Fix shared MCP lifecycle teardown so `lingering handles` disappears (ensure every fixture child is tracked and awaited before moving to the next scenario).
4. Update the harness assertions so the suite fails when warnings recur, then rerun `npm run test:phase1` in CI to confirm a completely clean run.

Implied decisions
- Cleaning up the harness will likely touch the shutdown controller implementation and persistence helpers; coordinate with the recent shutdown refactor owners to avoid regressions.
- We might need to tweak the fixture scripts (e.g., MCP stdio server) to support deterministic cleanup; that work is part of the same effort.

Testing requirements
- `npm run lint`
- `npm run test:phase1`
- Any targeted scenario reproductions (documented in the PR) to prove the lingering-handle/persistence warnings are gone.

Documentation updates required
- Update `docs/TESTING.md` (and related troubleshooting notes) once the harness cleanup is complete so contributors know the suite should run warning-free and what to do if it does not.
