# TL;DR
- Investigate duplicate PID logging (`ai-agent[2293031]`, `ai-agent[2293032]`) when starting `neda.service`.
- Understand why one PID stops quickly and why certain log lines only appear for the other PID.

# Analysis
- `systemctl status neda` shows main Node process `2293007` plus two helper processes `2293031` and `2293032` named `systemd-cat-native`; these are journald shims spawned because `StandardOutput=journal` is set in `/etc/systemd/system/neda.service`.
- The helper PIDs inherit the `ai-agent` identifier, so journal entries are attributed to them instead of the main process; stdout/stderr are handled by separate shims, explaining two distinct PIDs.
- Logs attributed to PID `2293031` stop once stdout is flushed and the shim exits or idles, while PID `2293032` continues to emit entries for subsequent writes (likely stderr or a reopened stream).
- Service definition (`/etc/systemd/system/neda.service`) launches `/opt/neda/bin/neda`, which `exec`s `/opt/ai-agent/ai-agent`, which in turn `exec`s `node /opt/ai-agent/dist/cli.js`, so no extra fork from the application itself.

# Decisions
- Costa approved consolidating journald logging into a single shared `systemd-cat-native` helper for the entire process.
- On helper exit we must attempt an immediate restart; if the restart fails, the system must fall back to logfmt on stderr automatically (no manual intervention).

# Plan
- [x] Inspect unit file to confirm service type, restart policy, and logging directives.
- [x] Check active processes via `systemctl status neda` to identify actual main PID and helper processes.
- [x] Refactor `src/logging/journald-sink.ts` into a singleton shared across the process, with auto-restart and fallback logic.
- [x] Update `src/logging/structured-logger.ts` (and any dependants) to consume the shared sink and enable automatic logfmt fallback when journald disables itself.
- [x] Document behaviour change (journald singleton + fallback) once implementation confirmed.
- [x] Handle `child.stdin` `'error'` events (e.g., EPIPE) gracefully so shutdown logs still flow and no uncaught exception is raised.
- [ ] Verify fallback message sequence appears in journald during shutdown (requires manual test after code fix).
- [ ] Investigate SIGTERM hang: HTTP headends keep `Connection: keep-alive` open, causing `server.close()` to wait indefinitely.
- [ ] Implement forced connection shutdown (e.g., track sockets or use `server.closeAllConnections` when available) for headends to ensure fast stop.
- [ ] Make CLI signal handler await shutdown / exit once `stopManager()` resolves to avoid fall-through.

# Implied decisions
- None taken yet; awaiting explicit instructions before modifying service or application logging configuration.

# Testing requirements
- After code changes, run `npm run lint` and `npm run build`.
- After deployment, restart `neda.service` and check `journalctl -u neda.service` to confirm log entries now originate from a single helper PID (or fall back cleanly).

# Documentation updates
- Update `docs/LOGS.md` (or new operations note) explaining the shared helper, auto-restart, and stderr fallback semantics.
