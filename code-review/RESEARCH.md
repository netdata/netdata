# Code Review Best Practices Research

Research findings for each specialist agent to inform prompt design.

---

## 1. Security Code Review

### Sources
- OWASP Code Review Guide
- GitHub Code Security Documentation
- Industry best practices (SANS, CWE/SANS Top 25)

### Good Security Review
- **Traces data flow** - follows user input from entry point to dangerous operations
- **Specific evidence** - identifies exact file:line where vulnerability exists
- **Exploitability assessment** - explains HOW an attacker could exploit the issue
- **Context-aware** - understands the application's threat model
- **Prioritizes by risk** - focuses on actual exploitable vulnerabilities, not theoretical issues
- **Concrete remediation** - provides specific fix recommendations (use prepared statements, not "improve security")
- **Checks boundaries** - validates input sanitization at trust boundaries
- **Framework-aware** - understands security features of the language/framework in use

### Bad Security Review
- **Generic warnings** - "this could be vulnerable" without evidence
- **False positives** - flags secure code due to pattern matching
- **Misses obvious issues** - overlooks direct SQL injection or XSS
- **No severity** - treats all findings equally
- **Out of scope** - flags code quality issues as "security"
- **Checklist mentality** - mechanically checks boxes without understanding code flow
- **Missing context** - doesn't understand what the code actually does

### Key Focus Areas (OWASP Top 10 + CWE Top 25)
- Injection (SQL, command, LDAP, XPath, etc.)
- Broken authentication and session management
- Sensitive data exposure
- XML external entities (XXE)
- Broken access control
- Security misconfiguration
- Cross-site scripting (XSS)
- Insecure deserialization
- Using components with known vulnerabilities
- Insufficient logging and monitoring
- Server-side request forgery (SSRF)
- Path traversal
- Buffer overflows
- Integer overflows
- Race conditions
- Cryptographic failures

### Principles
- Defense in depth - multiple layers of security
- Least privilege - minimal permissions needed
- Fail securely - errors don't expose sensitive info
- Don't trust user input - validate and sanitize everything
- Secure by default - safe configurations out of the box

---

## 2. Architecture Code Review

### Sources
- Clean Architecture (Robert C. Martin)
- Domain-Driven Design principles
- Microservices patterns
- Industry consensus on layered architecture

### Good Architecture Review
- **Identifies coupling** - finds tight dependencies between components
- **Checks separation of concerns** - ensures each module has single responsibility
- **Evaluates abstraction levels** - verifies proper layering (no business logic in UI)
- **Assesses testability** - can components be tested in isolation?
- **Reviews dependencies** - are they pointing in the right direction?
- **Considers change impact** - how many files need changing for a new feature?
- **Checks consistency** - follows established patterns in the codebase

### Bad Architecture Review
- **Nitpicks naming** - focuses on variable names instead of structure
- **Enforces dogma** - demands specific patterns without justification
- **Ignores context** - applies enterprise patterns to simple scripts
- **Misses violations** - overlooks business logic in controllers
- **No actionable feedback** - "improve architecture" without specifics

### Key Focus Areas
- **Layer violations** - UI calling database directly, skipping service layer
- **God objects** - classes with too many responsibilities
- **Tight coupling** - changes in A require changes in B, C, D
- **Circular dependencies** - A depends on B depends on A
- **Hidden dependencies** - global state, singletons, service locators
- **Leaky abstractions** - implementation details exposed in interfaces
- **Anemic domain models** - data structures with no behavior
- **Transaction script** - procedural code in object-oriented context

### Principles
- Single Responsibility Principle (SRP)
- Open/Closed Principle (OCP)
- Dependency Inversion Principle (DIP)
- Interface Segregation Principle (ISP)
- Don't Repeat Yourself (DRY)
- Separation of Concerns (SoC)
- Loose coupling, high cohesion

---

## 3. Test Code Review

### Sources
- Test-Driven Development (Kent Beck)
- Growing Object-Oriented Software, Guided by Tests
- xUnit Test Patterns
- Industry testing best practices

### Good Test Review
- **Identifies test smells** - finds tests that don't actually test
- **Checks independence** - tests don't depend on each other or execution order
- **Validates assertions** - ensures tests actually verify behavior
- **Reviews test data** - are test cases representative and comprehensive?
- **Assesses maintainability** - are tests readable and easy to update?
- **Checks coverage** - are edge cases and error paths tested?
- **Evaluates speed** - are slow tests actually integration tests in disguise?

### Bad Test Review
- **Counts lines** - focuses on coverage percentage without checking test quality
- **Ignores test logic** - doesn't verify tests are actually testing the right thing
- **Misses false positives** - overlooks tests that always pass
- **No feedback on clarity** - doesn't comment on test readability

### Key Focus Areas
- **Test cheats** - mocking the code under test instead of dependencies
- **Fragile tests** - break with unrelated changes
- **Slow tests** - integration tests masquerading as unit tests
- **Mystery guest** - test setup hides important context
- **Assertion roulette** - multiple assertions without clear failure messages
- **Test code duplication** - copy-paste test setup
- **Magic numbers** - hardcoded values without explanation
- **Missing edge cases** - only happy path tested
- **No negative tests** - doesn't verify error handling
- **Flaky tests** - pass/fail inconsistently

### Principles
- Fast - tests run in milliseconds
- Independent - no shared state between tests
- Repeatable - same result every time
- Self-validating - clear pass/fail, no manual inspection
- Timely - written before or with production code
- AAA pattern - Arrange, Act, Assert
- One concept per test
- Test behavior, not implementation

---

## 4. Code Quality Review

### Sources
- Clean Code (Robert C. Martin)
- Refactoring (Martin Fowler)
- Code Complete (Steve McConnell)
- Industry maintainability standards

### Good Quality Review
- **Points to specific improvements** - identifies concrete refactoring opportunities
- **Considers readability** - is code understandable to team members?
- **Identifies duplication** - finds copy-paste code
- **Checks naming** - are variables/functions clearly named?
- **Reviews comments** - are they explaining "why" or just "what"?
- **Assesses complexity** - flags overly complicated logic
- **Suggests simplifications** - provides clearer alternatives

### Bad Quality Review
- **Stylistic preferences** - enforces personal style over team conventions
- **Bikeshedding** - focuses on trivial issues (spacing, braces) over substance
- **No prioritization** - treats all quality issues equally
- **Vague feedback** - "this is messy" without specifics

### Key Focus Areas
- **Long methods** - functions over 20-30 lines (language dependent)
- **Long parameter lists** - more than 3-4 parameters
- **Duplicate code** - same logic in multiple places
- **Magic numbers** - unexplained constants
- **Dead code** - unreachable or unused code
- **Commented-out code** - should be removed (use version control)
- **Poor naming** - variables named `x`, `temp`, `data`
- **Inconsistent naming** - camelCase mixed with snake_case
- **Missing error handling** - happy path only
- **Primitive obsession** - using primitives instead of domain objects
- **Feature envy** - method uses another class's data more than its own

### Principles
- DRY (Don't Repeat Yourself)
- KISS (Keep It Simple, Stupid)
- YAGNI (You Aren't Gonna Need It)
- Boy Scout Rule - leave code cleaner than you found it
- Principle of Least Surprise
- Self-documenting code

---

## 5. Complexity Review

### Sources
- Cyclomatic Complexity (McCabe)
- Cognitive Complexity (SonarSource)
- Code metrics research
- Industry complexity thresholds

### Good Complexity Review
- **Measures objectively** - counts branches, loops, nesting
- **Provides thresholds** - complexity 1-10 OK, 11-20 review, 21+ refactor
- **Explains impact** - high complexity → hard to test, more bugs
- **Suggests refactoring** - extract method, simplify conditionals, use polymorphism
- **Considers context** - complex algorithms may be inherently complex

### Bad Complexity Review
- **Subjective judgment** - "feels complex" without metrics
- **No actionable advice** - "simplify this" without how
- **Ignores business logic** - demands simplification of inherently complex domains

### Key Focus Areas
- **High cyclomatic complexity** - many branches/paths
  - 1-10: Simple, low risk
  - 11-20: Moderate, medium risk
  - 21-50: Complex, high risk
  - 51+: Untestable, very high risk
- **Deep nesting** - more than 3 levels of indentation
- **Long conditionals** - complex boolean expressions
- **Nested loops** - O(n²) or worse performance
- **Callback hell** - deeply nested callbacks/promises
- **Switch statement sprawl** - huge switch with many cases

### Principles
- Single Responsibility - do one thing well
- Extract Method - break complex methods into smaller pieces
- Guard Clauses - fail fast, reduce nesting
- Replace Conditional with Polymorphism
- Decompose Conditional - extract boolean expressions into named functions

---

## 6. Production Readiness Review

### Sources
- Site Reliability Engineering (Google)
- Release It! (Michael Nygard)
- Production-Ready Microservices (Susan Fowler)
- Cloud-native patterns

### Good Production Review
- **Checks observability** - logging, metrics, tracing
- **Validates error handling** - timeouts, retries, circuit breakers
- **Reviews resource management** - connection pools, memory leaks, file handles
- **Assesses scalability** - N+1 queries, unbounded operations
- **Verifies resilience** - graceful degradation, fallbacks
- **Checks configuration** - externalized config, no hardcoded values
- **Reviews deployment** - health checks, rolling updates, rollback capability

### Bad Production Review
- **Only functional testing** - ignores operational concerns
- **No performance consideration** - doesn't think about scale
- **Missing monitoring** - no way to know if it breaks in production

### Key Focus Areas
- **Missing error handling** - no try-catch, no error checking
- **No timeouts** - external calls can hang forever
- **Resource leaks** - connections/files not closed
- **N+1 queries** - loading records in a loop
- **Unbounded operations** - no pagination, loads entire table
- **No logging** - can't debug production issues
- **No metrics** - can't measure performance
- **Hardcoded config** - database URLs, API keys in code
- **No health checks** - can't tell if service is healthy
- **No graceful shutdown** - abrupt termination loses data
- **Missing retries** - transient failures cause permanent errors
- **No circuit breakers** - cascading failures
- **Synchronous blocking** - holds threads waiting for I/O

### Principles
- Design for failure - everything fails eventually
- Timeouts on all external calls
- Retries with exponential backoff
- Circuit breakers for failing dependencies
- Bulkheads - isolate failures
- Monitoring and alerting - know when things break
- Graceful degradation - partial functionality > total failure
- Externalized configuration
- Health checks and readiness probes
- Structured logging for observability

---

## Summary for Prompt Design

Each specialist agent should:

1. **Be concise** - remind, don't teach
2. **Focus on principles** - not exhaustive checklists
3. **Provide 3-5 examples** - good vs bad patterns
4. **Set clear expectations** - what makes a thorough review
5. **Define boundaries** - what's in/out of scope
6. **Trust the LLM** - it knows these concepts, just needs focus

**Prompt structure**:
- Role (1 line)
- Focus areas (5-10 bullets)
- Good review = (3-5 points)
- Bad review = (3-5 points)
- Checklist (5-10 items as reminders)

**Total length**: ~20-30 lines per specialist
