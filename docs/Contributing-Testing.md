# Testing Guide

Test phases, harness usage, and coverage requirements for AI Agent.

---

## Table of Contents

- [Test Phase Overview](#test-phase-overview) - Summary of all test phases
- [Phase 1: Unit Tests](#phase-1-unit-tests) - Fast, isolated unit tests with Vitest
- [Phase 2: Deterministic Harness](#phase-2-deterministic-harness) - Scripted scenarios without live LLMs
- [Phase 3: Live Provider Tests](#phase-3-live-provider-tests) - Real LLM integration tests
- [Coverage Reports](#coverage-reports) - How to measure code coverage
- [Test MCP Server](#test-mcp-server) - Tools available for testing
- [Running All Tests](#running-all-tests) - Full test suite commands
- [Before Submitting Changes](#before-submitting-changes) - Pre-commit testing checklist
- [See Also](#see-also) - Related documentation

---

## Test Phase Overview

AI Agent uses three test phases, each with a different purpose:

| Phase       | Type                                | Command               | Speed         | Uses Real LLM |
| ----------- | ----------------------------------- | --------------------- | ------------- | ------------- |
| **Phase 1** | Unit tests                          | `npm run test:phase1` | Fast (~300ms) | No            |
| **Phase 2** | Deterministic + live provider tests | `npm run test:phase2` | Medium        | Yes\*         |
| **Phase 3** | Live integration                    | `npm run test:phase3` | Slow          | Yes           |

**Phase 2 modes**:

- Deterministic harness: `src/tests/phase2-harness-scenarios/phase2-runner.ts` uses test-llm provider (no real LLMs)
- Live provider tests: `src/tests/phase2-runner.ts` uses real models (vllm, ollama, anthropic, openrouter, etc.)

**When to run each phase**:

- **Phase 1**: Run after every code change
- **Phase 2**: Run before committing (includes deterministic harness + live provider tests)
- **Phase 3**: Run when changing LLM interaction logic

---

## Phase 1: Unit Tests

Unit tests run in parallel via Vitest. They test isolated functions without external dependencies.

### Running Phase 1

```bash
npm run test:phase1
```

**Expected output**: All unit tests pass.

### Test Location

Unit tests live in: `src/tests/unit/*.spec.ts`

### What Phase 1 Covers

| Area                  | Examples                 |
| --------------------- | ------------------------ |
| JSON repair           | Malformed JSON handling  |
| Tool output parsing   | `tool_output` extraction |
| XML tool handling     | Tool call parsing        |
| Slack formatting      | Message formatting       |
| Configuration parsing | Frontmatter validation   |

### Writing Unit Tests

```typescript
import { describe, it, expect } from "vitest";
import { repairJson } from "../utils/json-repair.js";

describe("repairJson", () => {
  it("fixes trailing commas", () => {
    const input = '{"a": 1,}';
    const result = repairJson(input);
    expect(result).toEqual({ a: 1 });
  });
});
```

**Guidelines**:

- One describe block per function or module
- Test edge cases and error conditions
- Keep tests isolated with no shared state

---

## Phase 2: Deterministic + Live Provider Tests

Phase 2 has two test modes:

1. **Deterministic harness**: Scripted scenarios against the core agent loop using the test-llm provider (no real LLMs). Tests orchestration logic with predictable inputs and outputs.

2. **Live provider tests**: Real LLM provider testing using models from `phase2-models.ts` (anthropic, ollama, openrouter, etc.).

### Running Phase 2

```bash
# Run all scenarios (both deterministic harness + live provider tests)
npm run test:phase2
```

### Key Files

| File                                                  | Purpose                                                                                    |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `src/llm-providers/test-llm.ts`                       | Scripted LLM provider for deterministic harness                                            |
| `src/tests/mcp/test-stdio-server.ts`                  | Deterministic MCP server for tool testing                                                  |
| `src/tests/fixtures/test-llm-scenarios.ts`            | Scenario definitions for deterministic harness                                             |
| `src/tests/phase2-harness.ts`                         | Deterministic harness entry point (delegates to phase2-harness-scenarios/phase2-runner.ts) |
| `src/tests/phase2-runner.ts`                          | Live provider test runner (real LLMs)                                                      |
| `src/tests/phase2-models.ts`                          | Model configurations for live provider tests                                               |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | Phase 2 deterministic test harness implementation                                          |

### Deterministic Harness Controls

The deterministic harness (`phase2-harness-scenarios/phase2-runner.ts`) manipulates five inputs:

| Input             | What You Control                         |
| ----------------- | ---------------------------------------- |
| User prompt       | Task description, history, configuration |
| Test MCP tool     | Mocked tool responses                    |
| Test LLM provider | Scripted assistant outputs               |
| Final response    | What to inspect in final report          |
| Accounting        | Observable LLM/tool entries              |

### Contract Testing

Tests assert the **observable contract**, not internal implementation.

**DO assert on**:

- Configuration settings are respected
- Business rules are enforced
- Final report structure is correct
- Accounting reflects actual behavior

**DO NOT assert on**:

- Internal log strings
- Log identifiers
- Log severities

### Environment Variables

| Variable                      | Purpose                      | Example/Default                 |
| ----------------------------- | ---------------------------- | ------------------------------- |
| `PHASE1_ONLY_SCENARIO`        | Filter to specific scenarios | `PHASE1_ONLY_SCENARIO=retry`    |
| `PHASE2_MODE`                 | Test mode                    | `all`, `parallel`, `sequential` |
| `PHASE2_PARALLEL_CONCURRENCY` | Concurrency limit            | `4`                             |
| `PHASE2_FORCE_SEQUENTIAL`     | Force sequential execution   | `1`                             |
| `PHASE2_STOP_ON_FAILURE`      | Stop on first failure        | `0`                             |
| `PHASE2_TRACE_LLM`            | Enable LLM tracing           | `0`                             |
| `PHASE2_TRACE_MCP`            | Enable MCP tracing           | `0`                             |
| `PHASE2_TRACE_SDK`            | Enable SDK tracing           | `0`                             |
| `PHASE2_VERBOSE`              | Enable verbose output        | `0`                             |
| `PHASE2_CONFIG`               | Path to custom config file   | `neda/.ai-agent.json`           |

### Writing Phase 2 Tests

```typescript
// In src/tests/fixtures/test-llm-scenarios.ts
export const scenarios: ScenarioDefinition[] = [
  {
    id: "run-test-1",
    description: "LLM + MCP success path.",
    systemPromptMustInclude: ["Phase 1 deterministic harness"],
    turns: [
      {
        turn: 1,
        response: {
          kind: "tool-call",
          toolCalls: [
            {
              toolName: "test__test",
              arguments: { text: "phase-1-tool-success" },
            },
          ],
        },
      },
      {
        turn: 2,
        response: {
          kind: "final-report",
          reportContent:
            "# Phase 1 Result\n\nTool execution succeeded and data was recorded.",
          reportFormat: "markdown",
          status: "success",
        },
      },
    ],
  },
];
```

---

## Phase 3: Live Provider Tests

Phase 3 runs tests with real LLM providers. These tests verify actual provider integration.

### Running Phase 3

```bash
# Run all tiers
npm run test:phase3

# Run only Tier 1
npm run test:phase3:tier1

# Run specific scenario with specific model
node dist/tests/phase3-runner.js --model=nova/glm-4.7 --scenario=tool-output-auto
```

### Model Tiers

| Tier | Models                                                          | Purpose                     |
| ---- | --------------------------------------------------------------- | --------------------------- |
| 1    | nova/minimax-m2.1, nova/glm-4.5-air, nova/glm-4.6, nova/glm-4.7 | Primary integration testing |

### Test Agents

Test agents are located in `src/tests/phase3/test-agents/`:

| Agent                  | Purpose                         |
| ---------------------- | ------------------------------- |
| `test-master.ai`       | Base test agent                 |
| `test-agent1.ai`       | Secondary test agent            |
| `tool-output-agent.ai` | Tool output handling tests      |
| `orchestration-*.ai`   | Multi-agent orchestration tests |

### Safeguards

| Variable                 | Purpose                    | Default               |
| ------------------------ | -------------------------- | --------------------- |
| `PHASE3_STOP_ON_FAILURE` | Stop on first failure      | `0`                   |
| `PHASE3_TRACE_LLM`       | Enable LLM tracing         | `0`                   |
| `PHASE3_TRACE_MCP`       | Enable MCP tracing         | `0`                   |
| `PHASE3_TRACE_SDK`       | Enable SDK tracing         | `0`                   |
| `PHASE3_VERBOSE`         | Enable verbose output      | `0`                   |
| `PHASE3_DUMP_LLM`        | Dump failing LLM payloads  | `0`                   |
| `PHASE3_CONFIG`          | Path to custom config file | `neda/.ai-agent.json` |

### Example: Running with Tracing

```bash
PHASE3_TRACE_LLM=1 PHASE3_VERBOSE=1 npm run test:phase3:tier1
```

---

## Coverage Reports

### V8 Coverage

```bash
NODE_V8_COVERAGE=coverage npm run test:phase2
```

Output goes to `coverage/` directory.

### c8 Summary

```bash
npx c8 npm run test:phase2
```

Provides a summarized coverage report in the terminal.

### Coverage Guidelines

| Area                     | Target                 |
| ------------------------ | ---------------------- |
| Core orchestration       | High coverage required |
| Provider implementations | Medium coverage        |
| Utilities                | High coverage required |
| CLI parsing              | Medium coverage        |

---

## Test MCP Server

The test MCP server (`src/tests/mcp/test-stdio-server.ts`) provides deterministic tool responses for Phase 2 testing.

### Available Tools

| Tool           | Purpose              |
| -------------- | -------------------- |
| `test`         | Basic test responses |
| `test-summary` | Summary responses    |

### Supported Behaviors

| Behavior                 | Description                       |
| ------------------------ | --------------------------------- |
| Success responses        | Returns configured success data   |
| Explicit error responses | Returns configured error messages |
| Large payload storage    | Tests context window handling     |
| Simulated timeouts       | Tests timeout handling            |

---

## Running All Tests

### Quick Test (Phase 1 Unit Tests)

```bash
npm test
```

### Full Test Suite (All Phases)

```bash
npm run test:all
```

### Individual Phases

```bash
npm run test:phase1    # Unit tests only
npm run test:phase2    # Harness tests only
npm run test:phase3    # Live provider tests only
```

---

## Before Submitting Changes

### Minimum Requirements

| Step | Command            | Passes When                                 |
| ---- | ------------------ | ------------------------------------------- |
| 1    | `npm run build`    | No compilation errors                       |
| 2    | `npm run lint`     | Zero warnings, zero errors                  |
| 3    | `npm run test:all` | Phase 1, Phase 2, and Phase 3 (Tier 1) pass |

### For Core Changes

If you modify core orchestration logic (session runner, LLM loop, tool execution):

```bash
# Run full Phase 2 without filters
npm run test:phase2

# Run Phase 3 with relevant models
npm run test:phase3:tier1
```

### For New Features

1. Add Phase 1 unit tests for isolated logic
2. Add Phase 2 tests (deterministic harness or live provider tests)
3. Run full test suite before PR

---

## See Also

- [Contributing](Contributing) - Contribution overview and setup
- [Code Style](Contributing-Code-Style) - Code style requirements
- [Technical-Specs](Technical-Specs) - Implementation specifications
- [docs/specs/](https://github.com/netdata/ai-agent/tree/master/docs/specs) - Detailed contributor specifications
