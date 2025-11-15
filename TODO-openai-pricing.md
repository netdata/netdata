# TODO – Update OpenAI Pricing Table (neda/.ai-agent.json)

## TL;DR
- Refresh the `pricing.openai` section in `neda/.ai-agent.json` to reflect Costa’s latest reference tables (Batch/Flex/Standard/Priority tiers, plus image/audio/video/fine-tuning/tool costs from November 2025 OpenAI pricing sheet).

## Analysis
- Current file (`neda/.ai-agent.json`) stores `pricing` as `provider -> model -> { unit, currency, prompt, completion, cacheRead?, cacheWrite? }`, covering a single set of rates (roughly the legacy “Standard” tier).
- Costa provided a full Markdown-like table covering **four** processing tiers (Batch, Flex, Standard, Priority) plus dedicated sections for image/audio/video, fine-tuning, built-in tools, AgentKit, transcription/tts, image-per-resolution, embeddings, moderation, legacy models, etc.
- The agent’s cost calculator (`src/ai-agent.ts`, `src/llm-client.ts`) currently assumes a flat mapping (no tier dimension) and looks up `pricing[provider][modelName]`. There is no notion of tier selection in runtime config.
- Without extra metadata, we cannot encode multiple tier prices per model without changing the schema or denormalizing names (e.g., `gpt-5.1@priority`). We need Costa’s direction before modifying the schema.

## Decisions (Need Costa’s confirmation)
1. **Schema shape** – Should we extend `pricing.openai` to include tier objects (e.g., `{ "standard": { "gpt-5.1": {...} }, "priority": { ... } }`) and update runtime cost lookups accordingly? Or should we continue storing a single “standard” set while embedding the full Markdown tables in a doc string for reference?
2. **Runtime tier selection** – If we store multiple tiers, how should the agent pick which tier to bill against? (e.g., new CLI flag/config `openaiProcessingTier`, or assume default `standard` with optional overrides?). Need explicit guidance.
3. **Non-token pricing (image/video/fine-tuning)** – The provided data includes per-image/per-second/per-minute pricing. Should these be stored in `pricing.openai` even though the current cost code only handles token units? If yes, do we add new fields (e.g., `unit: "per_image"`) or stash them under a separate documentation block?
4. **Legacy entries** – Should legacy model rates (e.g., `gpt-4-0613`, `davinci-002`) remain even if not part of the new sheet? Confirm whether to keep them with updated numbers or prune.

## Plan
1. **Confirm schema + tier strategy** with Costa (see Decisions).
2. **Implement schema updates** (if required):
   - Adjust `Configuration['pricing']` typing if we nest by tier.
   - Update `llm-client` cost lookup logic to honor tier selection.
   - Provide config/CLI surfaces to choose tiers (if needed).
3. **Populate `neda/.ai-agent.json`** with the new pricing data, covering all models/tables provided.
4. **Cross-validate**: run a script or lint to ensure no duplicate keys/missing values; add comments (if allowed) describing data source/version.
5. **Docs**: note the new pricing/tier behavior (e.g., README or internal doc) so future updates know how to maintain it.

## Implied Decisions
- Updated pricing must remain machine-readable for the accounting logic (not just prose).
- If schema changes, corresponding tests (Phase 1 pricing coverage scenarios) must be updated to avoid regressions.

## Testing Requirements
- `npm run lint` and `npm run build` after schema/logic updates.
- If runtime cost calculation changes, extend Phase 1 scenarios (pricing coverage) to assert the correct tier is chosen.

## Documentation Updates Required
- README or `docs/AI-AGENT-GUIDE.md` (pricing/accounting section) to explain tier handling.
- Inline comment or README entry in `neda/.ai-agent.json` describing data provenance/date.
