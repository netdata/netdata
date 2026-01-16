# Contributing

Guidelines for contributing to AI Agent.

---

## Overview

This section covers:
- Testing approach and commands
- Code style and linting requirements
- Documentation standards

---

## Contributing Topics

| Document | Description |
|----------|-------------|
| [Testing Guide](Contributing-Testing) | Test phases and harness |
| [Code Style](Contributing-Code-Style) | Style and linting rules |
| [Documentation Standards](Contributing-Documentation) | Doc maintenance rules |

---

## Quick Start

### Development Setup

```bash
# Clone repository
git clone https://github.com/netdata/ai-agent.git
cd ai-agent

# Install dependencies
npm install

# Build
npm run build

# Lint
npm run lint
```

### Before Submitting

1. **Build passes**: `npm run build`
2. **Lint passes**: `npm run lint` (zero warnings/errors)
3. **Tests pass**: `npm test`
4. **Docs updated**: If changing behavior

---

## Key Requirements

### Code Quality

- TypeScript strict mode
- Zero lint warnings
- Prefer precise types over `any`
- Use type guards for `unknown` values

### Testing

- Phase 1: Unit tests (vitest)
- Phase 2: Deterministic harness
- Phase 3: Live provider tests

### Documentation

- Every behavior change updates specs
- Specs stay synchronized with code
- AI-AGENT-GUIDE.md is source of truth for AI assistants

---

## Principles

### 1. Fail-Fast with Boom

- Silent failures are not justified
- All error conditions MUST be logged
- Only user config errors stop execution

### 2. Model-Facing Error Quality

Error messages to the model must be:
- Extremely detailed and descriptive
- Provide direct instructions to overcome

### 3. Thin Orchestration Loops

- Keep main loops lean
- Move complexity to specialized modules
- Separation of concerns is paramount

### 4. Semantics over Optimization

- Pooling/caching must not change behavior
- Isolation must be preserved
- Performance work respects contracts

---

## See Also

- [Technical-Specs](Technical-Specs) - Implementation specs
- [CLAUDE.md](../CLAUDE.md) - Development instructions

