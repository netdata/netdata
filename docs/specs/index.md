# AI-Agent Specifications Index

> **Verification status**: All documents in this index were reviewed and updated for accuracy on **2025-11-16**.

## Meta
- [CLAUDE.md](CLAUDE.md) - Instructions for AI assistants using specs
- [README.md](README.md) - Directory overview and purpose

## Core Architecture
- [architecture.md](architecture.md) - Core system architecture, components, data flow
- [session-lifecycle.md](session-lifecycle.md) - Session creation, execution, cleanup

## Session Management
- [retry-strategy.md](retry-strategy.md) - Retry and backoff logic
- [context-management.md](context-management.md) - Token budget tracking and limits
- [call-path.md](call-path.md) - Trace context and hierarchical paths
- [snapshots.md](snapshots.md) - Session snapshot persistence
- [accounting.md](accounting.md) - Token usage and cost tracking
- [optree.md](optree.md) - Hierarchical operation tree structure

## LLM Providers

### Overview
- [models-overview.md](models-overview.md) - LLM provider architecture

### Concrete Providers
- [providers-anthropic.md](providers-anthropic.md) - Anthropic Claude provider
- [providers-openai.md](providers-openai.md) - OpenAI provider
- [providers-google.md](providers-google.md) - Google Generative AI provider
- [providers-ollama.md](providers-ollama.md) - Ollama local inference
- [providers-openrouter.md](providers-openrouter.md) - OpenRouter aggregator
- [providers-test.md](providers-test.md) - Test harness provider

## Tool System

### Overview
- [tools-overview.md](tools-overview.md) - Tool system architecture
- [tools-xml-transport.md](tools-xml-transport.md) - XML final-report transport, XML-NEXT, slot validation

### Internal Tools
- [tools-final-report.md](tools-final-report.md) - Final report delivery
- [tools-progress-report.md](tools-progress-report.md) - Progress updates
- [tools-batch.md](tools-batch.md) - Batch tool execution

### Tool Providers
- [tools-mcp.md](tools-mcp.md) - MCP tool provider
- [tools-rest.md](tools-rest.md) - REST API tool provider
- [tools-agent.md](tools-agent.md) - Sub-agent tool provider

## Headends

### Overview
- [headends-overview.md](headends-overview.md) - Network service endpoints

### Manager
- [headend-manager.md](headend-manager.md) - Headend lifecycle orchestration

### Concrete Headends
- [headend-slack.md](headend-slack.md) - Slack Socket Mode integration
- [headend-mcp.md](headend-mcp.md) - MCP server headend
- [headend-openai.md](headend-openai.md) - OpenAI-compatible chat API
- [headend-anthropic.md](headend-anthropic.md) - Anthropic-compatible messages API
- [headend-rest.md](headend-rest.md) - REST API headend

## Observability
- [logging-overview.md](logging-overview.md) - Structured logging system
- [telemetry-overview.md](telemetry-overview.md) - Metrics, traces, and log export

## Configuration
- [configuration-loading.md](configuration-loading.md) - Config resolution and merging
- [frontmatter.md](frontmatter.md) - Agent frontmatter schema
- [pricing.md](pricing.md) - Token pricing tables

## Library API
- [library-api.md](library-api.md) - Public API for programmatic use

## Document Categories

### Completed (38 documents)
Core infrastructure, session management, internal tools, all LLM providers, all headends, observability, configuration, library API

### Planned
None - all core specifications completed

## Usage

### Verify Implementation
```
Query: "Is implementation aligned with spec X?"
Expected: Systematic review of:
- Matching behaviors
- Deviating behaviors
- Missing behaviors
- Undocumented behaviors
```

### Find Test Gaps
```
Query: "What tests are missing for feature Y?"
Expected: Cross-reference spec with test coverage
```

### Understand Behavior
```
Query: "How does Z work?"
Expected: Follow spec for exact behavior contract
```

## Maintenance

### When Updating Code
1. Identify affected specs
2. Update spec BEFORE implementation
3. Ensure spec matches implementation
4. Update test coverage section

### When Adding Features
1. Create spec document first
2. Define all behaviors
3. Document configuration
4. Add to this index

### When Reviewing
1. Use specs as source of truth
2. Report deviations
3. Track undocumented behaviors
4. Suggest spec updates
