# Internal Documentation Project

## TL;DR
Create comprehensive internal documentation in `docs/specs/` with flat structure. Each document covers: TL;DR, Features, User Configuration, Key Operations/Flows, telemetry, logging, events, tests, troubleshooting. Documents based on actual code review, validated through iterative AI review cycles (Codex/Gemini) for technical accuracy.

## Scope Analysis

**Total Documents Estimated: 55-65 files**

### Priority 1: Core Infrastructure (Critical)
1. **Architecture & Foundation** (5 docs)
   - architecture.md - Overall system architecture
   - session-lifecycle.md - Session creation, execution, cleanup
   - context-management.md - Token budgets, context guards
   - retry-strategy.md - Retries, fallbacks, error recovery
   - call-path.md - callPath, agentPath, turn, subturn tracking

2. **Models/Providers** (10 docs)
   - models-overview.md - Provider architecture
   - models-base.md - Base provider implementation
   - models-anthropic.md - Anthropic provider
   - models-openai.md - OpenAI provider
   - models-google.md - Google Gemini provider
   - models-ollama.md - Ollama provider
   - models-openrouter.md - OpenRouter provider
   - models-test.md - Test LLM provider
   - llm-request.md - Request construction, streaming
   - llm-response.md - Response parsing, token counting

3. **Tools** (12 docs)
   - tools-overview.md - Tool system architecture
   - tools-schema.md - Schema validation, conversion
   - tools-execution.md - Tool call lifecycle
   - tools-queue.md - Concurrency queue management
   - tools-mcp.md - MCP protocol integration
   - tools-rest.md - REST/OpenAPI tools
   - tools-openapi.md - OpenAPI schema import
   - tools-agent.md - Agent-as-tool (subagents)
   - tools-internal-overview.md - Internal tools system
   - tools-progress-report.md - progress_report tool
   - tools-final-report.md - final_report tool
   - tools-batch.md - Batch tool operations

### Priority 2: Headends & Server (User-Facing)
4. **Headends** (8 docs)
   - headends-overview.md - Headend architecture
   - headends-cli.md - CLI interface
   - headends-rest.md - REST API endpoint
   - headends-openai.md - OpenAI API compatibility
   - headends-anthropic.md - Anthropic API compatibility
   - headends-mcp.md - MCP server mode
   - headends-slack.md - Slack bot integration
   - headends-concurrency.md - Concurrency limiting

### Priority 3: Observability & Infrastructure
5. **Logging** (4 docs)
   - logging-overview.md - Structured logging system
   - logging-formats.md - Output formats (logfmt, JSON, rich)
   - logging-sinks.md - Output destinations (journald, console, file)
   - logging-messages.md - Message IDs and categories

6. **Telemetry** (3 docs)
   - telemetry-overview.md - OpenTelemetry integration
   - telemetry-metrics.md - Metrics collection
   - telemetry-traces.md - Distributed tracing

7. **Session Management** (5 docs)
   - session-manager.md - Session lifecycle management
   - session-tree.md - Session hierarchy
   - session-persistence.md - State persistence
   - session-failure.md - Failure modes and recovery
   - session-accounting.md - Token/cost accounting

### Priority 4: Configuration & Library API
8. **Configuration** (4 docs)
   - config-loading.md - Configuration resolution
   - config-frontmatter.md - Agent file parsing
   - config-options.md - CLI options and defaults
   - config-registry.md - Agent/subagent registry

9. **Library API** (3 docs)
   - library-api-overview.md - AIAgent, AIAgentSession exports
   - library-api-events.md - Event system
   - library-api-integration.md - Embedding ai-agent

### Priority 5: Advanced Features
10. **Snapshots** (2 docs)
    - snapshots-overview.md - Snapshot system
    - snapshots-format.md - Snapshot file format

11. **OpTree** (2 docs)
    - optree-overview.md - Operation tree tracking
    - optree-visualization.md - Tree rendering

12. **Meta** (3 docs)
    - README.md - Specs directory overview
    - CLAUDE.md - AI assistant instructions for specs
    - index.md - Document index with cross-references

**Total: ~61 documents**

## Process Per Document

1. **Research Phase** (Code Review)
   - Read relevant source files
   - Trace key code paths
   - Identify all configuration options
   - Map telemetry/logging hooks
   - Find existing tests

2. **Writing Phase**
   - Write document following standard template
   - Include code references (file:line)
   - Document all edge cases and conditions

3. **Review Phase** (Iterative)
   - Call Codex CLI for technical accuracy review
   - Call Gemini CLI for technical accuracy review
   - Validate their claims against actual code
   - Fix issues found
   - Repeat until no major issues

4. **Finalization**
   - Cross-reference with related docs
   - Update index.md
   - Mark as complete

## Decisions Required

1. **Document Template Structure**
   ```markdown
   # [Component Name]

   ## TL;DR
   [Brief description]

   ## Features
   [Bulleted list]

   ## User Configuration
   [Settings that affect this component]

   ## Key Operations/Flows
   [Detailed flows with conditions/checks/retries]

   ## Telemetry
   [Metrics, traces emitted]

   ## Logging
   [Log messages produced]

   ## Events
   [Events emitted/consumed]

   ## Tests
   [Test coverage and locations]

   ## Troubleshooting
   [Common issues and solutions]
   ```

2. **Cross-References**
   - Use relative markdown links: `[Session Manager](session-manager.md)`
   - Reference source code: `src/ai-agent.ts:123`

3. **Code Examples**
   - Include minimal, focused examples
   - Show actual configuration snippets from codebase

## Implied Decisions

1. Each document is standalone but cross-referenced
2. Technical accuracy verified via code review, not assumptions
3. No duplication of information - link to source document
4. Update AI-AGENT-GUIDE.md if specs reveal undocumented behavior
5. All new documents go in `docs/specs/` (flat structure)

## Testing Requirements

- Each document should list relevant phase1/phase2 tests
- Missing test coverage should be noted
- No new tests written as part of this exercise (documentation only)

## Documentation Updates Required

- Create docs/specs/CLAUDE.md with instructions for maintaining these specs
- Create docs/specs/README.md as entry point
- Create docs/specs/index.md with complete document list
- Possibly update root CLAUDE.md to reference docs/specs/

## Effort Estimate

Per document:
- Research: 10-30 minutes (depends on complexity)
- Writing: 15-30 minutes
- AI Review Cycles: 15-30 minutes (2-3 iterations)
- **Total per doc: 40-90 minutes**

Total project:
- 61 documents × 60 min average = **~61 hours**
- With overhead: **70-80 hours of work**

## Recommended Execution Plan

### Phase 1: Foundation (8 docs)
1. docs/specs/CLAUDE.md (meta)
2. docs/specs/README.md (meta)
3. architecture.md
4. session-lifecycle.md
5. models-overview.md
6. tools-overview.md
7. headends-overview.md
8. logging-overview.md

### Phase 2: Models Deep Dive (10 docs)
Complete all models-*.md and llm-*.md documents

### Phase 3: Tools Deep Dive (12 docs)
Complete all tools-*.md documents

### Phase 4: Headends (8 docs)
Complete all headends-*.md documents

### Phase 5: Remaining (23 docs)
Logging, telemetry, session management, config, library API, snapshots, opTree

### Phase 6: Cross-References & Index
- Create index.md
- Verify all cross-references
- Final consistency check

## Next Steps

**Pending User Approval:**
1. Confirm document template structure is acceptable
2. Confirm priority order is correct
3. Approve to start with Phase 1: Foundation
4. Clarify any scope adjustments (add/remove documents)

## Status

- [ ] Plan approved by user
- [ ] Phase 1: Foundation (0/8)
- [ ] Phase 2: Models (0/10)
- [ ] Phase 3: Tools (0/12)
- [ ] Phase 4: Headends (0/8)
- [ ] Phase 5: Remaining (0/23)
- [ ] Phase 6: Cross-References (0/3)
