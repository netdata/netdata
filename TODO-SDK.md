# TODO – AI SDK Alignment Checklist

## P0: Verify Current Alignment
- [ ] Compare `ConversationMessage`, tool schemas, and streaming callbacks against the latest Vercel AI SDK types to confirm our structures match exactly.
- [ ] Re-run the deterministic harness using the latest SDK release and record any diffs in reasoning/signature handling.
- [ ] Document discrepancies (if any) and open follow-up tickets for fixes.

## Maintenance
- [ ] Schedule a recurring review (per release or monthly) to repeat the comparison whenever the AI SDK publishes a new version.
