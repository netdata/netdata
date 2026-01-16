# Testing Guide

Test phases, harness, and coverage.

---

## Test Phase Overview

| Phase | Type | Command | Description |
|-------|------|---------|-------------|
| **Phase 1** | Parallel | `npm run test:phase1` | Unit tests (vitest, isolated) |
| **Phase 2** | Parallel + Sequential | `npm run test:phase2` | Deterministic harness scenarios |
| **Phase 3** | Real LLM | `npm run test:phase3` | Live provider integration tests |

---

## Phase 1: Unit Tests

Unit tests live under `src/tests/unit/*.spec.ts` and run in parallel via Vitest.

```bash
npm run test:phase1
```

**Characteristics**:
- Isolated with no shared state
- Fast execution (~300ms for ~200 tests)
- Covers: JSON repair, tool_output handling, XML tools, Slack formatting, etc.

---

## Phase 2: Deterministic Harness

Scripted scenarios against the core agent loop without production providers.

```bash
# Run all scenarios
npm run test:phase2

# Run only parallel tests
npm run test:phase2:parallel

# Run only sequential tests
npm run test:phase2:sequential
```

### Key Files

| File | Purpose |
|------|---------|
| `src/llm-providers/test-llm.ts` | Scripted LLM provider |
| `src/tests/mcp/test-stdio-server.ts` | Deterministic MCP server |
| `src/tests/fixtures/test-llm-scenarios.ts` | Scenario definitions |
| `src/tests/phase2-harness.ts` | Harness executor |

### Harness Controls

Phase 2 tests manipulate five inputs:

1. **User prompt** - Task, history, configuration
2. **Test MCP tool** - Mocked tool responses
3. **Test LLM provider** - Scripted assistant outputs
4. **Final response** - Inspect final report
5. **Accounting** - Observe LLM/tool entries

### Contract Testing

Tests assert the **observable contract**, not internal implementation:

- Configuration settings respected
- Business rules enforced
- Final report structure correct
- Accounting reflects behavior

**Do NOT** assert on:
- Internal log strings
- Log identifiers
- Log severities

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `PHASE1_ONLY_SCENARIO` | Filter to specific scenarios |
| `PHASE2_MODE` | all/parallel/sequential |
| `PHASE2_PARALLEL_CONCURRENCY` | Concurrency limit |
| `PHASE2_FORCE_SEQUENTIAL` | Force sequential execution |

---

## Phase 3: Live Provider Tests

Real LLM integration tests with actual providers.

```bash
# Run all tiers
npm run test:phase3

# Run only Tier 1
npm run test:phase3:tier1

# Run specific scenario
node dist/tests/phase3-runner.js --model=nova/glm-4.7 --scenario=tool-output-auto
```

### Model Tiers

| Tier | Models |
|------|--------|
| 1 | minimax-m2.1, glm-4.5-air, glm-4.6, glm-4.7 |

### Test Agents

Located in `src/tests/phase3/test-agents/`:
- Base: `test-master.ai`, `test-agent1.ai`
- Tool output: `tool-output-agent.ai`
- Orchestration: `orchestration-*.ai`

### Safeguards

| Variable | Purpose |
|----------|---------|
| `PHASE3_STOP_ON_FAILURE` | Stop on first failure (default: 1) |
| `PHASE3_TRACE_LLM` | Enable LLM tracing |
| `PHASE3_TRACE_MCP` | Enable MCP tracing |
| `PHASE3_VERBOSE` | Enable verbose output |
| `PHASE3_DUMP_LLM` | Dump failing LLM payloads |

---

## Coverage

```bash
# V8 coverage
NODE_V8_COVERAGE=coverage npm run test:phase2

# c8 summarized
npx c8 npm run test:phase2
```

---

## Test MCP Server

The test MCP server exposes tools for testing:

| Tool | Purpose |
|------|---------|
| `test` | Basic test responses |
| `test-summary` | Summary responses |

**Behaviors**:
- Success responses
- Explicit error responses
- Large payload storage
- Simulated timeouts

---

## Running All Tests

```bash
# Phase 1 + Phase 2
npm test

# All phases including live tests
npm run test:all
```

---

## Before Submitting Changes

1. **Build**: `npm run build`
2. **Lint**: `npm run lint`
3. **Phase 1 + 2**: `npm test`
4. **Full suite for core changes**: `npm run test:phase2` without filters

---

## See Also

- [Technical-Specs](Technical-Specs) - Implementation specs
- [docs/specs/](specs/) - Detailed contributor specifications

