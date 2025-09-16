# Logging and opTree Linking Policy

Status: Enforced
Scope: Entire application (CLI/Library/Server)

## Canonical Source of Truth

- The session operation tree (opTree) is the canonical source for all logs and accounting.
- Every log must be attached to an opTree operation (op) with a stable hierarchical path label.
- Console or stdout/stderr are fallbacks only when opTree attachment fails.

## Hierarchical Path Labels

- Each log line includes `[txn:<originTxnId>] [path:<label>]`.
- Parent → child sessions form numeric paths (e.g., `1.2.1.1`).
- Sub-agent logs inherit the parent session op path and append their own child path.

## Where Logs Attach

- LLM: attach to the active LLM op for the current turn.
- Tool: attach to the current tool op.
- Sub-agent: attach to the child session ops; when forwarded to parent, prefix the child path with the parent session op path.
- System (init/fin): preflight or session-level events that don’t belong to LLM/tool attach to dedicated ‘system’ ops in turn 0:
  - `init`: session initialization checks (e.g., billing ledger preflight)
  - `fin`: session finalization

## Error Handling

- No silent failures: all try/catch blocks must append an ERR (or WRN) log to the correct op before any console output.
- If opTree append fails, emit a fallback console line with `[FALLBACK]` and the error details.
- Severe conditions (e.g., billing ledger unwritable) are logged at preflight and again at finalization if persisting fails.

## Console Output Policy

- Console (stdout/stderr) may be used to enhance TTY UX (THK streaming, VRB) but only after the log is attached to opTree.
- For headend/server logs unrelated to a run, console is acceptable; if later associated to a session, prefer opTree logs.

## Accounting

- Accounting entries are appended to opTree ops.
- Session-level totals are recomputed after every mutation and verified at end-of-session; mismatch logs an error and recomputed totals win.
- Ledger file writes occur at end-of-session; failures are logged as WRN and do not block completion.

## Slack/UI Consumption

- Slack progress and totals derive from opTree only; overlays from flat event streams are not used.
- Completed sub-agents disappear as soon as their child session ends.

## Compliance Checklist

- [ ] Every log path links to an op in opTree (LLM/tool/session/system)
- [ ] All catches append ERR/WRN to opTree before any console output
- [ ] Console-only logs are used as fallbacks and clearly marked
- [ ] Sub-agent logs show hierarchical `[path:parent.child...]`
- [ ] Preflight ‘init’ and finalization ‘fin’ system ops exist in turn 0
- [ ] End-of-session accounting totals verification and WRN on mismatch
- [ ] Slack and ASCII tree reflect opTree structure and totals

---

This policy is mandatory. Changes to logging must preserve these invariants.
