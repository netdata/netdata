# Multi-Agent Code Review System

Comprehensive code review using specialized AI agents: discovery, security, tests, architecture, quality, complexity, and production readiness.

## Architecture

```
User Request (natural language)
         ↓
    Orchestrator
         ↓
    Discovery Agent (maps code, 100 turns)
         ↓
    Parallel Specialists (via agent__batch)
    ├── Security
    ├── Tests
    ├── Architecture
    ├── Code Quality
    ├── Complexity
    └── Production Readiness
         ↓
    Synthesis & Iteration
    (detect derived insights, deduplicate)
         ↓
    Human-Readable Report
```

## Features

- **Natural language input** - "Review src/auth.ts focusing on security"
- **Intelligent discovery** - maps code once, all specialists reuse (saves tokens)
- **Parallel analysis** - all 6 specialists run simultaneously via `agent__batch`
- **Derived insights** - cross-references findings (Security + Architecture = SQL injection)
- **Deduplication** - merges duplicate issues from multiple specialists
- **Human-readable output** - markdown report, not JSON
- **Iteration** - orchestrator re-queries specialists for clarifications

## Usage

### Basic Usage

```bash
cd code-review

# Review a single file
ai-agent --agent code-review.ai "Review src/auth/login.ts"

# Review a directory
ai-agent --agent code-review.ai "Review all files in src/services/"

# Review a GitHub PR
ai-agent --agent code-review.ai "Review https://github.com/user/repo/pull/123"
```

### Advanced Options

```bash
# Review specific categories only
ai-agent --agent code-review.ai "Review src/api/ focusing on security and production issues"

# Set severity threshold
ai-agent --agent code-review.ai "Review src/payment.ts and show only critical blockers"

# Provide project context
ai-agent --agent code-review.ai "Review src/ - this is a TypeScript backend API. Follow functional programming style. Focus on payment processing and authentication."
```

## Specialists

### 1. Discovery Agent

**Role**: Map code structure and changes (NOT analysis)

**Output**: Compact fact sheet with:
- Languages/frameworks used
- Changed files with line ranges
- Function locations and call flow
- Key pointers (user input, DB calls, auth)
- Change summary

**Turns**: 100 (traces function calls across files)

**Temperature**: 0.3 (factual, precise)

---

### 2. Security Specialist

**Focus**:
- Injection (SQL, command, path, XSS)
- Authentication/authorization flaws
- Data exposure, cryptographic failures
- Input validation, security misconfigurations

**Good Review**: Traces user input to dangerous operations with specific file:line evidence

**Bad Review**: Generic warnings without evidence, flags non-security issues

**Example Finding**:
```
[REQUIRED] SQL Injection in login query
Location: src/auth/validator.ts:132
Evidence: User input from req.body.username concatenated into SQL query
Recommendation: Use parameterized query - db.query('SELECT * FROM users WHERE username = ?', [username])
```

---

### 3. Tests Specialist

**Focus**:
- Test cheats (mocking code under test)
- False positives (tests always pass)
- Missing coverage (edge cases, error paths)
- Fragile/flaky tests, test clarity

**Good Review**: Validates tests actually test behavior, checks independence and coverage

**Bad Review**: Only counts coverage %, ignores test quality

---

### 4. Architecture Specialist

**Focus**:
- Layer violations (UI calling DB)
- God objects (too many responsibilities)
- Tight coupling, circular dependencies
- Leaky abstractions

**Good Review**: Identifies structural issues, assesses testability, suggests refactoring

**Bad Review**: Nitpicks naming, enforces dogma without context

---

### 5. Code Quality Specialist

**Focus**:
- Duplicate code (DRY violations)
- Magic numbers/strings
- Long methods (>30 lines), long parameter lists (>4)
- Poor naming, dead code

**Good Review**: Points to specific improvements, considers readability

**Bad Review**: Personal preferences, bikeshedding on trivial style

---

### 6. Complexity Specialist

**Focus**:
- High cyclomatic complexity (21+ requires refactor)
- Deep nesting (>3 levels)
- Complex conditionals, nested loops
- Over-engineering

**Good Review**: Measures objectively, provides thresholds, suggests refactoring

**Bad Review**: Subjective "feels complex" without metrics

**Thresholds**:
- 1-10: Simple, OK
- 11-20: Review (nit)
- 21-50: Refactor (optional)
- 51+: High risk (required)

---

### 7. Production Readiness Specialist

**Focus**:
- Missing error handling, no timeouts
- Resource leaks (connections, files)
- N+1 queries, unbounded operations
- Missing logging/monitoring
- Hardcoded config, no health checks

**Good Review**: Checks observability, validates resilience, assesses scalability

**Bad Review**: Only functional testing, ignores operational concerns

---

## Severity Levels

- **required** - Must fix before merging (security vulnerabilities, production blockers)
- **optional** - Should fix (important improvements, maintainability)
- **nit** - Nice-to-have (minor improvements, style)
- **fyi** - Informational (no action needed)

## Configuration

**Location**: `code-review/.ai-agent.json`

**MCP Servers**:
- `filesystem-cwd` - read local files
- `github` - fetch PR details (read-only, write tools denied)

**Model**: Anthropic Claude Sonnet 4.5 for all agents

**Tool Turns**:
- Discovery: 100 (thorough exploration)
- Specialists: 20 each
- Orchestrator: 15

## Testing

Run the agents against any file in your repository that you want reviewed:

```bash
cd code-review
ai-agent --agent code-review.ai "Review path/to/your/file.ts"
```

Pick a target that actually contains real issues (security, production, architecture, quality, tests, complexity) so each specialist has something meaningful to report.

## How It Works

1. **User provides natural language request** - orchestrator parses target, scope, severity
2. **Discovery runs** - maps files, flow, pointers (once, reused by all)
3. **Specialists run in parallel** - via `agent__batch` tool
4. **Orchestrator synthesizes** - deduplicates, detects derived insights, iterates if needed
5. **Human-readable report** - markdown with issues, recommendations, metadata

## Derived Insights Example

- Security finds: "User input in variable `userQuery`"
- Architecture finds: "`userQuery` passed directly to database"
- **Orchestrator derives**: Potential SQL injection → re-queries security specialist for confirmation

## Output Example

```markdown
# Code Review Report

## Summary
- Total Issues: 5
- Blockers (REQUIRED): 2
- Important (OPTIONAL): 2
- Nice-to-Have (NIT): 1

**Overall Recommendation**: REJECT (critical security vulnerability)

---

## Critical Issues (REQUIRED)

### [src/auth/login.ts:45] SQL Injection Vulnerability

**Severity**: REQUIRED (must fix before merging)
**Categories**: security, architecture

User input from `loginAttempt` variable is concatenated directly into SQL query...

**Recommendation**: Use parameterized queries...

---

## Review Metadata

- Reviewed: 2025-11-14T15:30:00Z
- Specialists Run: tests, architecture, code-quality, complexity, security, production
- Iterations: 1

**Derived Insights**:
- Security: User input + Architecture: Direct DB call = SQL Injection (CONFIRMED)
```

## Extending

Add new specialist by:
1. Create `code-review-NEWNAME.ai` with same structure
2. Add to orchestrator's `agents:` list in frontmatter
3. Update orchestrator prompt to include in batch call

## Troubleshooting

**"Discovery failed"** - Check target path/URL is valid, GitHub token configured

**"Specialist X failed"** - Check `maxTurns` sufficient, review specialist output for errors

**"No issues found"** - Verify specialists received discovery output, check severity threshold

**"Timeout"** - Reduce scope (review fewer files) or increase `maxTurns`
