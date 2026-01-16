# Contributing

Guidelines for contributing to AI Agent development.

---

## Table of Contents

- [Overview](#overview) - What this section covers and how to get started
- [Contributing Topics](#contributing-topics) - Links to detailed guides
- [Development Setup](#development-setup) - Clone, install, build
- [Before Submitting](#before-submitting) - Pre-commit checklist
- [Key Requirements](#key-requirements) - Code quality and testing standards
- [Core Principles](#core-principles) - Design philosophy
- [See Also](#see-also) - Related documentation

---

## Overview

This section helps you contribute code, tests, and documentation to AI Agent. Whether you are fixing a bug, adding a feature, or improving documentation, these guides ensure your contribution meets project standards.

**What you will learn**:
- How to set up your development environment
- Code style and linting requirements
- Testing approach and commands
- Documentation standards

---

## Contributing Topics

| Document | Description |
|----------|-------------|
| [Testing Guide](Contributing-Testing) | Test phases, harness usage, and coverage requirements |
| [Code Style](Contributing-Code-Style) | TypeScript guidelines, naming conventions, ESLint rules |
| [Documentation Standards](Contributing-Documentation) | How to maintain specs and wiki pages |

---

## Development Setup

### Step 1: Clone the Repository

```bash
git clone https://github.com/netdata/ai-agent.git
cd ai-agent
```

### Step 2: Install Dependencies

```bash
npm install
```

### Step 3: Build the Project

```bash
npm run build
```

**Expected output**: TypeScript compiles to `dist/` directory with no errors.

### Step 4: Run Linter

```bash
npm run lint
```

**Expected output**: Zero warnings, zero errors.

### Step 5: Run Tests

```bash
npm test
```

**Expected output**: All Phase 1 and Phase 2 tests pass.

---

## Before Submitting

Complete this checklist before creating a pull request:

| Step | Command | Requirement |
|------|---------|-------------|
| 1 | `npm run build` | No compilation errors |
| 2 | `npm run lint` | Zero warnings, zero errors |
| 3 | `npm test` | All tests pass |
| 4 | Manual check | Documentation updated if behavior changed |

### Commit Guidelines

- Write clear, descriptive commit messages
- Reference related issues when applicable
- Keep commits focused on a single change

### Pull Request Guidelines

- Describe what the PR changes and why
- Include test coverage for new features
- Update documentation for user-facing changes

---

## Key Requirements

### Code Quality

| Requirement | Description |
|-------------|-------------|
| TypeScript strict mode | All code must pass strict type checking |
| Zero lint warnings | No warnings or errors from ESLint |
| Precise types | Use `Record<string, unknown>` over `any` |
| Type guards | Narrow `unknown` values with runtime checks |

See [Code Style](Contributing-Code-Style) for detailed guidelines.

### Testing

| Phase | Purpose | Command |
|-------|---------|---------|
| Phase 1 | Unit tests (vitest) | `npm run test:phase1` |
| Phase 2 | Deterministic harness scenarios | `npm run test:phase2` |
| Phase 3 | Live provider integration | `npm run test:phase3` |

See [Testing Guide](Contributing-Testing) for detailed instructions.

### Documentation

| Rule | Description |
|------|-------------|
| Synchronized updates | Every behavior change updates specs at the same commit |
| Spec-first development | Write or update specs before implementation |
| AI-AGENT-GUIDE.md | Source of truth for AI assistants creating agents |

See [Documentation Standards](Contributing-Documentation) for detailed guidelines.

---

## Core Principles

These principles guide all development decisions:

### 1. Fail-Fast with Boom

- Silent failures are not justified
- All error conditions MUST be logged
- Only user configuration errors stop execution
- All other errors: log and recover

### 2. Model-Facing Error Quality

Error messages sent to the LLM must be:
- Extremely detailed and descriptive
- Include specific instructions to overcome the issue
- Never generic like "something went wrong"

**Why**: Models cannot fix issues they do not understand. Detailed error messages keep sessions running.

### 3. Thin Orchestration Loops

- Keep main session loops lean
- Move complexity to specialized modules
- Separation of concerns is paramount
- Every change should simplify, not complicate

### 4. Semantics over Optimization

- Pooling and caching must not change observable behavior
- Session isolation must be preserved
- Performance work respects existing contracts

---

## See Also

- [Technical-Specs](Technical-Specs) - Architecture and implementation specifications
- [Getting-Started](Getting-Started) - Installation and first steps
- [CLAUDE.md](https://github.com/netdata/ai-agent/blob/master/CLAUDE.md) - Full development instructions for AI assistants
